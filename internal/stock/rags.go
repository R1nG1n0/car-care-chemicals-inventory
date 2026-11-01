package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// Тряпки и губки с ресурсом (ФТ-19). Позиция в шт с wear_limit: «В работу»
// списывает 1 шт обычным расходом и заводит тряпку, стирки считаются у каждой
// отдельно. Списание тряпки склад не трогает — шт уже списана.

const (
	opKindRag   = "rag"
	ragLabelMax = 20
	maxRagIDs   = 100

	ragActive  = "active"
	ragRetired = "retired"
)

// Уровни износа тряпки (Rag.Level).
const (
	LevelOK   = "ok"
	LevelSoon = "soon"
	LevelDue  = "due"
)

// Причины списания тряпки.
var ragReasons = map[string]bool{"worn": true, "lost": true, "damaged": true}

// Rag — тряпка в работе или списанная.
type Rag struct {
	ID           int64   `json:"id"`
	ProductID    int64   `json:"product_id"`
	Label        string  `json:"label"`
	Number       int64   `json:"number"`
	Washes       int64   `json:"washes"`
	WearLimit    *int64  `json:"wear_limit"`
	Level        string  `json:"level"`
	Status       string  `json:"status"`
	RetireReason *string `json:"retire_reason"`
	CreatedAt    int64   `json:"created_at"`
	RetiredAt    *int64  `json:"retired_at"`
	LastWashedAt *int64  `json:"last_washed_at"`
}

// RagLevel — износ по числу стирок и ресурсу: от 90 % — «soon», от 100 % —
// «due». Без ресурса (limit == nil) тряпка всегда «ok». Тот же порог в SQL
// у catalog (сводка позиции).
func RagLevel(washes int64, limit *int64) string {
	switch {
	case limit == nil:
		return LevelOK
	case washes >= *limit:
		return LevelDue
	case washes*100 >= *limit*90:
		return LevelSoon
	}
	return LevelOK
}

const ragSelect = `SELECT r.id, r.product_id, r.label, r.number, r.washes, p.wear_limit, r.status,
	r.retire_reason, r.created_at, r.retired_at, r.last_washed_at
	FROM rags r JOIN products p ON p.user_id = r.user_id AND p.id = r.product_id `

func scanRag(sc interface{ Scan(...any) error }) (Rag, error) {
	var r Rag
	var limit, retiredAt, washedAt sql.NullInt64
	var reason sql.NullString
	err := sc.Scan(&r.ID, &r.ProductID, &r.Label, &r.Number, &r.Washes, &limit, &r.Status,
		&reason, &r.CreatedAt, &retiredAt, &washedAt)
	if limit.Valid {
		r.WearLimit = &limit.Int64
	}
	if reason.Valid {
		r.RetireReason = &reason.String
	}
	if retiredAt.Valid {
		r.RetiredAt = &retiredAt.Int64
	}
	if washedAt.Valid {
		r.LastWashedAt = &washedAt.Int64
	}
	r.Level = RagLevel(r.Washes, r.WearLimit)
	return r, err
}

func ragByID(ctx context.Context, q querier, userID, id int64) (Rag, error) {
	r, err := scanRag(q.QueryRowContext(ctx, ragSelect+`WHERE r.user_id = ? AND r.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, errRagNotFound
	}
	if err != nil {
		return r, fmt.Errorf("тряпка: %w", err)
	}
	return r, nil
}

var errRagNotFound = &NotFoundError{Message: "Тряпка не найдена"}

// RagStartInput — тело POST /api/products/{id}/rags.
type RagStartInput struct {
	Label  string `json:"label"`
	UnitID *int64 `json:"unit_id"`
	Op
}

// RagStartResult — итог «В работу»: расход и новая тряпка.
type RagStartResult struct {
	Consume ConsumeResult
	Rag     Rag
}

// StartRag — «В работу» (ФТ-19.2): в одной транзакции списывает 1 шт позиции
// (как обычный расход, с идемпотентностью client_op_id) и заводит тряпку с
// номером max+1 по позиции. Нет ресурса — ConflictError, нет в запасе —
// ShortageError.
func (s *Service) StartRag(ctx context.Context, userID, productID int64, in RagStartInput,
	render Render[RagStartResult]) (res RagStartResult, body []byte, replay bool, err error) {
	label := ""
	if in.Label != "" {
		if label, err = cleanText(in.Label, 1, ragLabelMax, "label", "Метка — от 1 до 20 символов"); err != nil {
			return
		}
	}
	return runOp(ctx, s, userID, in.Op, opKindRag, func(tx *sql.Tx, at int64) (RagStartResult, error) {
		var out RagStartResult
		p, err := product(ctx, tx, userID, productID)
		if err != nil {
			return out, err
		}
		var limit sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT wear_limit FROM products WHERE user_id = ? AND id = ?`,
			userID, productID).Scan(&limit); err != nil {
			return out, fmt.Errorf("ресурс позиции: %w", err)
		}
		if !limit.Valid {
			return out, &ConflictError{Field: "wear_limit", Message: "Сначала задайте ресурс стирок"}
		}
		var batch int64
		if out.Consume, err = consumeTx(ctx, tx, userID, p, 1, in.UnitID, sql.NullInt64{}, &batch, at, false, nil); err != nil {
			return out, err
		}
		var number int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0) + 1 FROM rags
			WHERE user_id = ? AND product_id = ?`, userID, productID).Scan(&number); err != nil {
			return out, fmt.Errorf("номер тряпки: %w", err)
		}
		name := label
		if name == "" {
			name = "№ " + strconv.FormatInt(number, 10)
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO rags (user_id, product_id, label, number, consume_movement_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, userID, productID, name, number, out.Consume.MovementIDs[0], at)
		if err != nil {
			return out, fmt.Errorf("создание тряпки: %w", err)
		}
		id, _ := r.LastInsertId()
		out.Rag, err = ragByID(ctx, tx, userID, id)
		return out, err
	}, render)
}

// Rags — тряпки позиции по номеру: действующие, а с all — и списанные.
func (s *Service) Rags(ctx context.Context, userID, productID int64, all bool) ([]Rag, error) {
	if _, err := product(ctx, s.db, userID, productID); err != nil {
		return nil, err
	}
	return listRags(ctx, s.db, userID, productID, all)
}

func listRags(ctx context.Context, q querier, userID, productID int64, all bool) ([]Rag, error) {
	rows, err := q.QueryContext(ctx, ragSelect+`WHERE r.user_id = ? AND r.product_id = ? AND (? OR r.status = 'active')
		ORDER BY r.number, r.id`, userID, productID, all)
	if err != nil {
		return nil, fmt.Errorf("тряпки позиции: %w", err)
	}
	defer rows.Close()
	out := []Rag{}
	for rows.Next() {
		r, err := scanRag(rows)
		if err != nil {
			return nil, fmt.Errorf("тряпки позиции: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RenameRag меняет метку тряпки (1…20 символов).
func (s *Service) RenameRag(ctx context.Context, userID, id int64, label string) (Rag, error) {
	label, err := cleanText(label, 1, ragLabelMax, "label", "Метка — от 1 до 20 символов")
	if err != nil {
		return Rag{}, err
	}
	var out Rag
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := ragByID(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rags SET label = ? WHERE user_id = ? AND id = ?`, label, userID, id); err != nil {
			return fmt.Errorf("метка тряпки: %w", err)
		}
		out, err = ragByID(ctx, tx, userID, id)
		return err
	})
	return out, err
}

// Wash добавляет по одной стирке каждой действующей тряпке из ids (ФТ-19.3).
// Любая списанная, чужая или несуществующая — 404, ничего не меняется.
func (s *Service) Wash(ctx context.Context, userID int64, ids []int64) ([]Rag, error) {
	return s.shiftWashes(ctx, userID, ids, 1)
}

// Unwash снимает по одной стирке (не ниже 0) — «Отменить» после Wash.
func (s *Service) Unwash(ctx context.Context, userID int64, ids []int64) ([]Rag, error) {
	return s.shiftWashes(ctx, userID, ids, -1)
}

func (s *Service) shiftWashes(ctx context.Context, userID int64, ids []int64, delta int64) ([]Rag, error) {
	if len(ids) == 0 || len(ids) > maxRagIDs {
		return nil, &ValidationError{Field: "ids", Message: "Укажите от 1 до 100 тряпок"}
	}
	uniq := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	at := s.now().Unix()
	out := make([]Rag, 0, len(uniq))
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		out = out[:0]
		for _, id := range uniq {
			r, err := ragByID(ctx, tx, userID, id)
			if err != nil {
				return err
			}
			if r.Status != ragActive {
				return errRagNotFound
			}
			if _, err := tx.ExecContext(ctx, `UPDATE rags SET washes = MAX(washes + ?, 0),
				last_washed_at = CASE WHEN ? > 0 THEN ? WHEN washes + ? <= 0 THEN NULL ELSE last_washed_at END
				WHERE user_id = ? AND id = ?`, delta, delta, at, delta, userID, id); err != nil {
				return fmt.Errorf("стирка тряпки: %w", err)
			}
			if r, err = ragByID(ctx, tx, userID, id); err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}

// RetireRag списывает тряпку с причиной worn, lost или damaged (ФТ-19.4).
// Склад не трогает. Уже списанная — 404.
func (s *Service) RetireRag(ctx context.Context, userID, id int64, reason string) (Rag, error) {
	if !ragReasons[reason] {
		return Rag{}, &ValidationError{Field: "reason", Message: "Причина — worn, lost или damaged"}
	}
	at := s.now().Unix()
	var out Rag
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		r, err := ragByID(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		if r.Status != ragActive {
			return errRagNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rags SET status = 'retired', retire_reason = ?, retired_at = ?
			WHERE user_id = ? AND id = ?`, reason, at, userID, id); err != nil {
			return fmt.Errorf("списание тряпки: %w", err)
		}
		out, err = ragByID(ctx, tx, userID, id)
		return err
	})
	return out, err
}

// UnstartRag отменяет «В работу»: только действующая тряпка без стирок, чей
// расход ещё в журнале. Расход откатывается как Revert (шт возвращается
// экземпляру), тряпка удаляется, номер освобождается. Иначе ConflictError;
// чужая или несуществующая — 404. Возвращает id позиции.
func (s *Service) UnstartRag(ctx context.Context, userID, id int64) (int64, error) {
	now := s.now().Unix()
	var productID int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		r, err := ragByID(ctx, tx, userID, id)
		if err != nil {
			return err
		}
		var movement sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT consume_movement_id FROM rags WHERE user_id = ? AND id = ?`,
			userID, id).Scan(&movement); err != nil {
			return fmt.Errorf("расход тряпки: %w", err)
		}
		var exists bool
		if movement.Valid {
			// id движений переиспользуются: это должен быть именно расход 1 шт
			// экземпляра той же позиции.
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM movements m
				JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
				WHERE m.user_id = ? AND m.id = ? AND m.type = 'consume' AND m.qty_delta = -1 AND u.product_id = ?)`,
				userID, movement.Int64, r.ProductID).Scan(&exists); err != nil {
				return fmt.Errorf("расход тряпки: %w", err)
			}
		}
		if r.Status != ragActive || r.Washes != 0 || !exists {
			return &ConflictError{Message: "Тряпку уже стирали или списали — отменить нельзя"}
		}
		if _, err := revertTx(ctx, tx, userID, []int64{movement.Int64}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rags WHERE user_id = ? AND id = ?`, userID, id); err != nil {
			return fmt.Errorf("удаление тряпки: %w", err)
		}
		productID = r.ProductID
		return nil
	})
	return productID, err
}
