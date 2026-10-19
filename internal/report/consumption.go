package report

import (
	"context"
	"database/sql"
)

// ProductUse — расход (или списание) по позиции. Cost == nil — ни у одного
// движения нет цены.
type ProductUse struct {
	ProductID int64  `json:"product_id"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Qty       int64  `json:"qty"`
	Cost      *int64 `json:"cost"`
}

// MonthUse — расход за месяц ГГГГ-ММ.
type MonthUse struct {
	Month  string `json:"month"`
	Cost   int64  `json:"cost"`
	QtyML  int64  `json:"qty_ml"`
	QtyPcs int64  `json:"qty_pcs"`
}

// Writeoffs — списания (причина «испорчено», «не найден» и т. п.) за период.
type Writeoffs struct {
	ByProduct []ProductUse `json:"by_product"`
	Cost      int64        `json:"cost"`
}

// Consumption — расход за период (ФТ-10.2.3): движения consume, в том числе
// по шаблонам. Себестоимость каждого движения считается как Movement.cost.
type Consumption struct {
	TotalCost int64        `json:"total_cost"`
	ByProduct []ProductUse `json:"by_product"`
	ByMonth   []MonthUse   `json:"by_month"`
	Writeoffs Writeoffs    `json:"writeoffs"`
}

// moveCost — стоимость движения в копейках: |qty_delta| × цена упаковки ÷
// initial_qty, к ближайшему; NULL — у экземпляра нет цены. Совпадает с
// catalog.Movement.cost (деление целых, оба числа неотрицательны).
const moveCost = `(CASE WHEN l.price_per_package IS NULL THEN NULL
	ELSE (-m.qty_delta * l.price_per_package + u.initial_qty / 2) / u.initial_qty END)`

const moveFrom = ` FROM movements m
	JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
	JOIN products pr ON pr.user_id = u.user_id AND pr.id = u.product_id
	LEFT JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id
	WHERE m.user_id = ? AND m.type = ? AND m.created_at >= ? AND m.created_at < ? `

// Consumption считает расход и списания пользователя за период.
func (s *Service) Consumption(ctx context.Context, userID int64, p Period) (Consumption, error) {
	from, to := p.bounds()
	out := Consumption{ByProduct: []ProductUse{}, ByMonth: []MonthUse{}, Writeoffs: Writeoffs{ByProduct: []ProductUse{}}}

	var err error
	if out.ByProduct, out.TotalCost, err = s.byProduct(ctx, userID, "consume", from, to); err != nil {
		return out, err
	}
	if out.Writeoffs.ByProduct, out.Writeoffs.Cost, err = s.byProduct(ctx, userID, "writeoff", from, to); err != nil {
		return out, err
	}
	byMonth := map[string]MonthUse{}
	off := int(p.TZ) * 60
	err = s.query(ctx, "расход по месяцам", `SELECT strftime('%Y-%m', m.created_at + ?, 'unixepoch'), pr.unit,
		SUM(-m.qty_delta), COALESCE(SUM(`+moveCost+`), 0)`+moveFrom+`GROUP BY 1, 2`,
		[]any{off, userID, "consume", from, to}, func(r *sql.Rows) error {
			var month, unit string
			var qty, cost int64
			if err := r.Scan(&month, &unit, &qty, &cost); err != nil {
				return err
			}
			u := byMonth[month]
			u.Month = month
			u.Cost += cost
			if unit == "ml" {
				u.QtyML += qty
			} else {
				u.QtyPcs += qty
			}
			byMonth[month] = u
			return nil
		})
	if err != nil {
		return out, err
	}
	for _, m := range p.months() {
		u := byMonth[m]
		u.Month = m
		out.ByMonth = append(out.ByMonth, u)
	}
	return out, nil
}

// byProduct — движения типа typ по позициям: по убыванию стоимости, без цены — в конце.
func (s *Service) byProduct(ctx context.Context, userID int64, typ string, from, to int64) ([]ProductUse, int64, error) {
	list := []ProductUse{}
	var total int64
	err := s.query(ctx, "по позициям", `SELECT pr.id, pr.name, pr.unit, SUM(-m.qty_delta), SUM(`+moveCost+`), COUNT(`+moveCost+`)`+
		moveFrom+`GROUP BY pr.id ORDER BY COALESCE(SUM(`+moveCost+`), -1) DESC, SUM(-m.qty_delta) DESC, pr.name COLLATE NOCASE, pr.id`,
		[]any{userID, typ, from, to}, func(r *sql.Rows) error {
			var u ProductUse
			var cost sql.NullInt64
			var priced int64
			if err := r.Scan(&u.ProductID, &u.Name, &u.Unit, &u.Qty, &cost, &priced); err != nil {
				return err
			}
			if priced > 0 {
				c := cost.Int64
				u.Cost = &c
				total += c
			}
			list = append(list, u)
			return nil
		})
	return list, total, err
}
