package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// unitState — экземпляр, как его видит склад.
type unitState struct {
	ID         int64
	ProductID  int64
	Unit       string // ml | pcs — единица позиции
	LocationID sql.NullInt64
	Initial    int64
	Remaining  int64
	Status     string
	OpenedAt   sql.NullInt64
	Price      sql.NullInt64 // цена упаковки; NULL — без покупки
}

func (u unitState) active() bool { return u.Status == "sealed" || u.Status == "open" }

const unitSelect = `
	SELECT u.id, u.product_id, p.unit, u.location_id, u.initial_qty, u.remaining_qty, u.status,
	       u.opened_at, l.price_per_package
	FROM units u
	JOIN products p ON p.user_id = u.user_id AND p.id = u.product_id
	LEFT JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id`

func scanUnit(sc interface{ Scan(...any) error }) (unitState, error) {
	var u unitState
	err := sc.Scan(&u.ID, &u.ProductID, &u.Unit, &u.LocationID, &u.Initial, &u.Remaining, &u.Status,
		&u.OpenedAt, &u.Price)
	return u, err
}

// loadUnit читает свой экземпляр; чужой или несуществующий — 404.
func loadUnit(ctx context.Context, q querier, userID, id int64) (unitState, error) {
	u, err := scanUnit(q.QueryRowContext(ctx, unitSelect+` WHERE u.user_id = ? AND u.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return u, errUnitNotFound
	}
	if err != nil {
		return u, fmt.Errorf("чтение экземпляра: %w", err)
	}
	return u, nil
}

var errUnitInactive = &ConflictError{Message: "Экземпляр уже закончился или списан"}

var errUnitPlaceArchived = &ConflictError{Message: "Место экземпляра в архиве — сначала верните место из архива"}

// unitPlaceActive проверяет перед возвратом экземпляра в sealed/open, что
// его место не в архиве: в архивном месте не может лежать экземпляров
// (закончившийся мог остаться в месте, которое потом архивировали). Без
// места — можно.
func unitPlaceActive(ctx context.Context, q querier, userID, unitID int64) error {
	var archived bool
	err := q.QueryRowContext(ctx, `
		SELECT l.archived FROM units u
		JOIN locations l ON l.user_id = u.user_id AND l.id = u.location_id
		WHERE u.user_id = ? AND u.id = ?`, userID, unitID).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("чтение места экземпляра: %w", err)
	}
	if archived {
		return errUnitPlaceArchived
	}
	return nil
}

// errUnitChanged — экземпляр изменили между чтением и записью.
var errUnitChanged = &ConflictError{Message: "Остаток изменился, повторите"}

// updatedOne проверяет, что UPDATE с условием на прежнее состояние
// изменил ровно одну строку; иначе — errUnitChanged.
func updatedOne(r sql.Result, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n, err := r.RowsAffected(); err != nil || n != 1 {
		return errUnitChanged
	}
	return nil
}

// AddUnitInput — ввод имеющегося остатка (POST /api/units).
type AddUnitInput struct {
	ProductID    int64  `json:"product_id"`
	RemainingQty int64  `json:"remaining_qty"`
	InitialQty   *int64 `json:"initial_qty"`
	LocationID   *int64 `json:"location_id"`
}

// AddUnit заводит экземпляр без покупки — то, что уже лежит дома. Статус
// sealed, если остаток равен объёму, иначе open; движение adjust
// (+остаток, «Начальный остаток»). Место — как у покупки.
func (s *Service) AddUnit(ctx context.Context, userID int64, in AddUnitInput) (int64, error) {
	now := s.now().Unix()
	var unitID int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		p, err := product(ctx, tx, userID, in.ProductID)
		if err != nil {
			return err
		}
		initial := p.PackageSize
		if in.InitialQty != nil {
			initial = *in.InitialQty
		}
		if initial < 1 || initial > maxQty {
			return &ValidationError{Field: "initial_qty", Message: "Объём — от 1 до 100 000"}
		}
		if in.RemainingQty < 1 || in.RemainingQty > initial {
			return &ValidationError{Field: "remaining_qty", Message: fmt.Sprintf("Остаток — от 1 до %d", initial)}
		}
		loc, err := resolveLocation(ctx, tx, userID, p.ID, in.LocationID)
		if err != nil {
			return err
		}
		status, opened := "sealed", sql.NullInt64{}
		if in.RemainingQty < initial {
			status, opened = "open", sql.NullInt64{Int64: now, Valid: true}
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO units (user_id, product_id, location_id, initial_qty, remaining_qty, status,
			                   opened_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, p.ID, loc, initial, in.RemainingQty, status, opened, now)
		if err != nil {
			return fmt.Errorf("запись экземпляра: %w", err)
		}
		if unitID, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: unitID, typ: "adjust", delta: in.RemainingQty,
			to: loc, reason: initialReason, at: now,
		})
		return err
	})
	return unitID, err
}

// Move перекладывает экземпляр (sealed/open) в другое место: движение move
// (0, from/to). То же место — ничего не меняется.
func (s *Service) Move(ctx context.Context, userID, unitID, locationID int64) error {
	now := s.now().Unix()
	return s.inTx(ctx, func(tx *sql.Tx) error {
		u, err := loadUnit(ctx, tx, userID, unitID)
		if err != nil {
			return err
		}
		if err := activeLocation(ctx, tx, userID, locationID); err != nil {
			return err
		}
		if !u.active() {
			return errUnitInactive
		}
		if u.LocationID.Valid && u.LocationID.Int64 == locationID {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE units SET location_id = ? WHERE user_id = ? AND id = ?`,
			locationID, userID, unitID); err != nil {
			return fmt.Errorf("перемещение экземпляра: %w", err)
		}
		_, err = insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: unitID, typ: "move",
			from: u.LocationID, to: sql.NullInt64{Int64: locationID, Valid: true}, at: now,
		})
		return err
	})
}

// Adjust задаёт фактический остаток (ФТ-5.5): движение adjust на разницу.
// 0 → finished; больше 0 → open (у sealed, finished и written_off). Вернуть
// в open экземпляр, чьё место в архиве, нельзя — 409.
// Тот же остаток — ничего не меняется. Возвращает id движения; 0 — его нет.
func (s *Service) Adjust(ctx context.Context, userID, unitID, remaining int64, reason string) (int64, error) {
	reason, err := cleanText(reason, 0, maxReasonLen, "reason", "Причина — не длиннее 80 символов")
	if err != nil {
		return 0, err
	}
	now := s.now().Unix()
	var moved int64
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		u, err := loadUnit(ctx, tx, userID, unitID)
		if err != nil {
			return err
		}
		if remaining < 0 || remaining > u.Initial {
			return &ValidationError{Field: "remaining_qty", Message: fmt.Sprintf("Остаток — от 0 до %d", u.Initial)}
		}
		if remaining == u.Remaining {
			return nil
		}
		// Условие на прежние остаток и статус — защита от гонки, как в расходе.
		var r sql.Result
		if remaining == 0 {
			r, err = tx.ExecContext(ctx, `
				UPDATE units SET remaining_qty = 0, status = 'finished', finished_at = ?
				WHERE user_id = ? AND id = ? AND remaining_qty = ? AND status = ?`,
				now, userID, unitID, u.Remaining, u.Status)
		} else {
			if !u.active() {
				if err := unitPlaceActive(ctx, tx, userID, unitID); err != nil {
					return err
				}
			}
			r, err = tx.ExecContext(ctx, `
				UPDATE units SET remaining_qty = ?, status = 'open', opened_at = COALESCE(opened_at, ?),
				                 finished_at = NULL
				WHERE user_id = ? AND id = ? AND remaining_qty = ? AND status = ?`,
				remaining, now, userID, unitID, u.Remaining, u.Status)
		}
		if err := updatedOne(r, err, "корректировка экземпляра"); err != nil {
			return err
		}
		moved, err = insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: unitID, typ: "adjust", delta: remaining - u.Remaining,
			reason: reason, at: now,
		})
		return err
	})
	return moved, err
}

// Writeoff списывает часть или весь остаток с причиной (ФТ-5.3). qty nil —
// весь остаток. Дошедший до 0 — written_off, частично списанный
// запечатанный становится open. Возвращает id движения.
func (s *Service) Writeoff(ctx context.Context, userID, unitID int64, qty *int64, reason string) (int64, error) {
	reason, err := cleanText(reason, 1, maxReasonLen, "reason", "Укажите причину — до 80 символов")
	if err != nil {
		return 0, err
	}
	now := s.now().Unix()
	var moved int64
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		u, err := loadUnit(ctx, tx, userID, unitID)
		if err != nil {
			return err
		}
		if !u.active() {
			return errUnitInactive
		}
		n := u.Remaining
		if qty != nil {
			n = *qty
		}
		if n < 1 || n > maxQty {
			return &ValidationError{Field: "qty", Message: "Количество — от 1 до 100 000"}
		}
		if n > u.Remaining {
			return &ShortageError{Available: u.Remaining, Unit: u.Unit}
		}
		left := u.Remaining - n
		var r sql.Result
		if left == 0 {
			r, err = tx.ExecContext(ctx, `
				UPDATE units SET remaining_qty = 0, status = 'written_off', finished_at = ?
				WHERE user_id = ? AND id = ? AND remaining_qty = ? AND status IN ('sealed', 'open')`,
				now, userID, unitID, u.Remaining)
		} else {
			r, err = tx.ExecContext(ctx, `
				UPDATE units SET remaining_qty = ?, status = 'open', opened_at = COALESCE(opened_at, ?)
				WHERE user_id = ? AND id = ? AND remaining_qty = ? AND status IN ('sealed', 'open')`,
				left, now, userID, unitID, u.Remaining)
		}
		if err := updatedOne(r, err, "списание экземпляра"); err != nil {
			return err
		}
		moved, err = insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: unitID, typ: "writeoff", delta: -n, reason: reason, at: now,
		})
		return err
	})
	return moved, err
}
