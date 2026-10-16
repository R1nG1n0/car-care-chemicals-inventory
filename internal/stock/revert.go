package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Revert отменяет движения (ФТ-5.6): все или ничего. Разрешены только свои
// consume, writeoff и adjust; остаток экземпляра возвращается, движения
// удаляются из журнала. Возвращает id затронутых позиций.
//
// Статус экземпляра после отмены:
//   - остаток 0 → finished (например, отменён ввод имеющегося остатка);
//   - остаток равен объёму и по экземпляру не осталось движений расхода,
//     списания и корректировки → снова sealed, opened_at = NULL. Движение
//     ввода имеющегося остатка («Начальный остаток» — самое первое движение
//     экземпляра без покупки) при этом не считается: оно не вскрывает;
//   - иначе → open (в том числе finished и written_off с ненулевым остатком).
func (s *Service) Revert(ctx context.Context, userID int64, ids []int64) ([]int64, error) {
	ids = uniqueIDs(ids)
	if len(ids) < 1 || len(ids) > maxRevertIDs {
		return nil, &ValidationError{Field: "ids", Message: "Укажите от 1 до 100 движений"}
	}
	now := s.now().Unix()
	var products []int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		deltas, err := revertDeltas(ctx, tx, userID, ids)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM movements WHERE user_id = ? AND id IN (`+placeholders(len(ids))+`)`,
			append([]any{userID}, int64Args(ids)...)...); err != nil {
			return fmt.Errorf("удаление движений: %w", err)
		}
		unitIDs := make([]int64, 0, len(deltas))
		for id := range deltas {
			unitIDs = append(unitIDs, id)
		}
		sort.Slice(unitIDs, func(i, j int) bool { return unitIDs[i] < unitIDs[j] })
		seen := map[int64]bool{}
		for _, unitID := range unitIDs {
			pid, err := revertUnit(ctx, tx, userID, unitID, deltas[unitID], now)
			if err != nil {
				return err
			}
			if !seen[pid] {
				seen[pid] = true
				products = append(products, pid)
			}
		}
		return nil
	})
	return products, err
}

// revertDeltas проверяет движения и возвращает сумму qty_delta по экземплярам.
func revertDeltas(ctx context.Context, tx *sql.Tx, userID int64, ids []int64) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, unit_id, type, qty_delta FROM movements WHERE user_id = ? AND id IN (`+placeholders(len(ids))+`)`,
		append([]any{userID}, int64Args(ids)...)...)
	if err != nil {
		return nil, fmt.Errorf("чтение движений: %w", err)
	}
	defer rows.Close()
	deltas := map[int64]int64{}
	found := 0
	var badType bool
	for rows.Next() {
		var id, unitID, delta int64
		var typ string
		if err := rows.Scan(&id, &unitID, &typ, &delta); err != nil {
			return nil, err
		}
		found++
		if typ != "consume" && typ != "writeoff" && typ != "adjust" {
			badType = true
		}
		deltas[unitID] += delta
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if found != len(ids) {
		return nil, errMovementNotFound
	}
	if badType {
		return nil, &ConflictError{Message: "Отменить можно только расход, списание и корректировку"}
	}
	return deltas, nil
}

// revertUnit возвращает экземпляру остаток после удаления его движений с
// суммой delta и выставляет статус. Возвращает id позиции.
func revertUnit(ctx context.Context, tx *sql.Tx, userID, unitID, delta, now int64) (int64, error) {
	var productID, initial, remaining int64
	var fromPurchase bool
	err := tx.QueryRowContext(ctx, `
		SELECT product_id, initial_qty, remaining_qty, purchase_line_id IS NOT NULL
		FROM units WHERE user_id = ? AND id = ?`, userID, unitID).
		Scan(&productID, &initial, &remaining, &fromPurchase)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errUnitNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("чтение экземпляра: %w", err)
	}
	left := remaining - delta
	if left < 0 || left > initial {
		return 0, &ConflictError{Message: "Нельзя отменить: остаток экземпляра вышел бы за пределы объёма. Отмените сначала более поздние движения"}
	}

	// Остались ли движения, которые вскрывают экземпляр.
	q := `SELECT COUNT(*) FROM movements WHERE user_id = ? AND unit_id = ?
	      AND type IN ('consume', 'writeoff', 'adjust')`
	if !fromPurchase {
		q += ` AND id <> (SELECT MIN(id) FROM movements WHERE user_id = ? AND unit_id = ?)`
	}
	args := []any{userID, unitID}
	if !fromPurchase {
		args = append(args, userID, unitID)
	}
	var touched int64
	if err := tx.QueryRowContext(ctx, q, args...).Scan(&touched); err != nil {
		return 0, fmt.Errorf("подсчёт движений экземпляра: %w", err)
	}

	switch {
	case left == 0:
		_, err = tx.ExecContext(ctx, `
			UPDATE units SET remaining_qty = 0, status = 'finished', finished_at = COALESCE(finished_at, ?)
			WHERE user_id = ? AND id = ?`, now, userID, unitID)
	case touched == 0 && left == initial:
		_, err = tx.ExecContext(ctx, `
			UPDATE units SET remaining_qty = ?, status = 'sealed', opened_at = NULL, finished_at = NULL
			WHERE user_id = ? AND id = ?`, left, userID, unitID)
	default:
		_, err = tx.ExecContext(ctx, `
			UPDATE units SET remaining_qty = ?, status = 'open', opened_at = COALESCE(opened_at, ?),
			                 finished_at = NULL
			WHERE user_id = ? AND id = ?`, left, now, userID, unitID)
	}
	if err != nil {
		return 0, fmt.Errorf("восстановление экземпляра: %w", err)
	}
	return productID, nil
}

// MovementIDs — журнал движений, новые сверху: id для catalog.MovementsByIDs.
// productID 0 — все позиции; before 0 — с начала.
func (s *Service) MovementIDs(ctx context.Context, userID, productID, before int64, limit int) ([]int64, error) {
	limit, err := validLimit(limit)
	if err != nil {
		return nil, err
	}
	if productID != 0 {
		if _, err := product(ctx, s.db, userID, productID); err != nil {
			return nil, err
		}
	}
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id FROM movements m
		JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
		WHERE m.user_id = ? AND m.id < ? AND (? = 0 OR u.product_id = ?)
		ORDER BY m.id DESC LIMIT ?`, userID, before, productID, productID, limit)
	if err != nil {
		return nil, fmt.Errorf("журнал движений: %w", err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func uniqueIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func int64Args(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
