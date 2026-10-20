package stock

import (
	"context"
	"database/sql"
	"fmt"
)

// consumeOrder — порядок выбора экземпляров для расхода (ФТ-5.1): сначала
// вскрытые (самый ранний opened_at, без opened_at — в конце, затем id), затем запечатанные — самый
// старый по дате покупки (у введённых без покупки — по дню создания),
// затем по id.
const consumeOrder = `
	SELECT u.id, u.product_id, p.unit, u.location_id, u.initial_qty, u.remaining_qty, u.status,
	       u.opened_at, l.price_per_package
	FROM units u
	JOIN products p ON p.user_id = u.user_id AND p.id = u.product_id
	LEFT JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id
	LEFT JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
	WHERE u.user_id = ? AND u.product_id = ? AND u.status IN ('sealed', 'open') AND u.remaining_qty > 0
	ORDER BY CASE u.status WHEN 'open' THEN 0 ELSE 1 END,
	         CASE u.status WHEN 'open' THEN COALESCE(u.opened_at, 9223372036854775807) END,
	         CASE u.status WHEN 'sealed' THEN COALESCE(pu.date, date(u.created_at, 'unixepoch')) END,
	         u.id`

// take — сколько взять из экземпляра.
type take struct {
	unit unitState
	n    int64
}

// plan — расклад расхода по экземплярам.
type plan struct {
	takes     []take
	available int64  // всего на складе
	unit      string // ml | pcs
	cost      *int64 // себестоимость взятого; nil — цен нет
}

// planConsume раскладывает qty по экземплярам позиции, ничего не меняя.
// startUnit — экземпляр, с которого начать (ФТ-5.2). Если на складе меньше
// qty, берётся всё, что есть.
func planConsume(ctx context.Context, q querier, userID int64, p productInfo, qty int64, startUnit *int64) (plan, error) {
	pl := plan{unit: p.Unit}
	rows, err := q.QueryContext(ctx, consumeOrder, userID, p.ID)
	if err != nil {
		return pl, fmt.Errorf("выбор экземпляров: %w", err)
	}
	defer rows.Close()
	var units []unitState
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return pl, err
		}
		units = append(units, u)
		pl.available += u.Remaining
	}
	if err := rows.Err(); err != nil {
		return pl, err
	}
	rows.Close()

	if startUnit != nil {
		u, err := loadUnit(ctx, q, userID, *startUnit)
		if err != nil {
			return pl, err
		}
		if u.ProductID != p.ID {
			return pl, errUnitNotFound
		}
		if !u.active() {
			return pl, errUnitInactive
		}
		ordered := []unitState{u}
		for _, x := range units {
			if x.ID != u.ID {
				ordered = append(ordered, x)
			}
		}
		units = ordered
	}

	need := qty
	for _, u := range units {
		if need == 0 {
			break
		}
		n := min(u.Remaining, need)
		need -= n
		pl.takes = append(pl.takes, take{unit: u, n: n})
		pl.cost = addCost(pl.cost, UnitCost(n, u.Price, u.Initial))
	}
	return pl, nil
}

// ConsumeResult — итог расхода по одной позиции.
type ConsumeResult struct {
	ProductID       int64
	Requested       int64
	Consumed        int64
	MovementIDs     []int64
	OpenedUnitIDs   []int64
	FinishedUnitIDs []int64
	Cost            *int64
}

// consumeTx списывает qty позиции внутри транзакции по правилам ФТ-5.1.
// partial — нехватка не ошибка (шаблоны, ФТ-8.6): списывается сколько есть.
// batch — группа движений запроса (см. insertMovement).
func consumeTx(ctx context.Context, tx *sql.Tx, userID int64, p productInfo, qty int64, startUnit *int64,
	runID sql.NullInt64, batch *int64, now int64, partial bool) (ConsumeResult, error) {
	res := ConsumeResult{ProductID: p.ID, Requested: qty, MovementIDs: []int64{},
		OpenedUnitIDs: []int64{}, FinishedUnitIDs: []int64{}}
	pl, err := planConsume(ctx, tx, userID, p, qty, startUnit)
	if err != nil {
		return res, err
	}
	if pl.available < qty && !partial {
		return res, &ShortageError{Available: pl.available, Unit: p.Unit}
	}
	for _, t := range pl.takes {
		u := t.unit
		left := u.Remaining - t.n
		status := "open"
		if left == 0 {
			status = "finished"
		}
		// Условие на прежний остаток — защита от гонки: если экземпляр
		// изменили между чтением и записью, строка не обновится.
		r, err := tx.ExecContext(ctx, `
			UPDATE units SET remaining_qty = ?, status = ?,
			       opened_at = COALESCE(opened_at, ?),
			       finished_at = CASE WHEN ? = 'finished' THEN ? ELSE finished_at END
			WHERE user_id = ? AND id = ? AND remaining_qty = ? AND status IN ('sealed', 'open')`,
			left, status, now, status, now, userID, u.ID, u.Remaining)
		if err != nil {
			return res, fmt.Errorf("расход экземпляра: %w", err)
		}
		if n, err := r.RowsAffected(); err != nil || n != 1 {
			return res, errUnitChanged
		}
		id, err := insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: u.ID, typ: "consume", delta: -t.n, runID: runID, batch: batch, at: now,
		})
		if err != nil {
			return res, err
		}
		res.MovementIDs = append(res.MovementIDs, id)
		res.Consumed += t.n
		if u.Status == "sealed" {
			res.OpenedUnitIDs = append(res.OpenedUnitIDs, u.ID)
		}
		if left == 0 {
			res.FinishedUnitIDs = append(res.FinishedUnitIDs, u.ID)
		}
	}
	res.Cost = pl.cost
	return res, nil
}

// ConsumeInput — тело POST /api/consume.
type ConsumeInput struct {
	ProductID int64  `json:"product_id"`
	Qty       int64  `json:"qty"`
	UnitID    *int64 `json:"unit_id"`
	Op
}

// Consume списывает qty позиции одной транзакцией (ФТ-5.1, ФТ-5.2). Всего
// меньше qty — ShortageError, ничего не меняется.
func (s *Service) Consume(ctx context.Context, userID int64, in ConsumeInput) (ConsumeResult, error) {
	res, _, _, err := s.ConsumeOp(ctx, userID, in, nil)
	return res, err
}

// ConsumeOp — Consume с номером операции и временем с телефона (ФТ-12.4).
// При in.ClientOpID render собирает ответ в той же транзакции, он
// сохраняется; повтор с тем же номером ничего не списывает и возвращает
// сохранённый body (replay = true, res пуст).
func (s *Service) ConsumeOp(ctx context.Context, userID int64, in ConsumeInput,
	render Render[ConsumeResult]) (res ConsumeResult, body []byte, replay bool, err error) {
	if in.Qty < 1 || in.Qty > maxQty {
		return res, nil, false, &ValidationError{Field: "qty", Message: "Количество — от 1 до 100 000"}
	}
	return runOp(ctx, s, userID, in.Op, opKindConsume, func(tx *sql.Tx, at int64) (ConsumeResult, error) {
		p, err := product(ctx, tx, userID, in.ProductID)
		if err != nil {
			return ConsumeResult{}, err
		}
		var batch int64
		return consumeTx(ctx, tx, userID, p, in.Qty, in.UnitID, sql.NullInt64{}, &batch, at, false)
	}, render)
}
