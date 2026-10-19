package report

import (
	"context"
	"database/sql"
	"sort"
)

// CategoryTotal — расходы на покупки по категории позиции.
type CategoryTotal struct {
	CategoryID int64  `json:"category_id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Total      int64  `json:"total"`
}

// StoreTotal — расходы по магазину; StoreID == nil — покупки без магазина.
type StoreTotal struct {
	StoreID *int64 `json:"store_id"`
	Name    string `json:"name"`
	Total   int64  `json:"total"`
}

// MonthTotal — расходы за месяц ГГГГ-ММ.
type MonthTotal struct {
	Month string `json:"month"`
	Total int64  `json:"total"`
}

// Spending — расходы на покупки за период (ФТ-10.2.2). Деньги — копейки.
type Spending struct {
	Total          int64           `json:"total"`
	PurchasesCount int64           `json:"purchases_count"`
	ByCategory     []CategoryTotal `json:"by_category"`
	ByStore        []StoreTotal    `json:"by_store"`
	ByMonth        []MonthTotal    `json:"by_month"`
}

// NoStoreName — подпись покупок без магазина.
const NoStoreName = "Без магазина"

const spendFrom = ` FROM purchase_lines l
	JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
	WHERE pu.user_id = ? AND pu.date BETWEEN ? AND ? `

// Spending считает расходы на покупки: packages × price_per_package строк
// покупок, чья дата попала в период. Категория — у позиции строки.
func (s *Service) Spending(ctx context.Context, userID int64, p Period) (Spending, error) {
	args := []any{userID, p.From.Format(dateLayout), p.To.Format(dateLayout)}
	out := Spending{ByCategory: []CategoryTotal{}, ByStore: []StoreTotal{}, ByMonth: []MonthTotal{}}

	err := s.query(ctx, "итог", `SELECT COALESCE(SUM(l.packages * l.price_per_package), 0), COUNT(DISTINCT pu.id)`+spendFrom,
		args, func(r *sql.Rows) error { return r.Scan(&out.Total, &out.PurchasesCount) })
	if err != nil {
		return out, err
	}
	err = s.query(ctx, "категории", `SELECT c.id, c.name, c.color, SUM(l.packages * l.price_per_package)
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		JOIN products pr ON pr.user_id = l.user_id AND pr.id = l.product_id
		JOIN categories c ON c.user_id = pr.user_id AND c.id = pr.category_id
		WHERE pu.user_id = ? AND pu.date BETWEEN ? AND ?
		GROUP BY c.id ORDER BY 4 DESC, c.name COLLATE NOCASE, c.id`, args, func(r *sql.Rows) error {
		var t CategoryTotal
		if err := r.Scan(&t.CategoryID, &t.Name, &t.Color, &t.Total); err != nil {
			return err
		}
		out.ByCategory = append(out.ByCategory, t)
		return nil
	})
	if err != nil {
		return out, err
	}
	err = s.query(ctx, "магазины", `SELECT st.id, st.name, SUM(l.packages * l.price_per_package)
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		LEFT JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE pu.user_id = ? AND pu.date BETWEEN ? AND ?
		GROUP BY st.id ORDER BY 3 DESC, st.name COLLATE NOCASE, st.id`, args, func(r *sql.Rows) error {
		var t StoreTotal
		var id sql.NullInt64
		var name sql.NullString
		if err := r.Scan(&id, &name, &t.Total); err != nil {
			return err
		}
		if id.Valid {
			v := id.Int64
			t.StoreID, t.Name = &v, name.String
		} else {
			t.Name = NoStoreName
		}
		out.ByStore = append(out.ByStore, t)
		return nil
	})
	if err != nil {
		return out, err
	}
	byMonth := map[string]int64{}
	err = s.query(ctx, "месяцы", `SELECT substr(pu.date, 1, 7), SUM(l.packages * l.price_per_package)`+spendFrom+`GROUP BY 1`,
		args, func(r *sql.Rows) error {
			var m string
			var v int64
			if err := r.Scan(&m, &v); err != nil {
				return err
			}
			byMonth[m] = v
			return nil
		})
	if err != nil {
		return out, err
	}
	for _, m := range p.months() {
		out.ByMonth = append(out.ByMonth, MonthTotal{Month: m, Total: byMonth[m]})
	}
	sort.SliceStable(out.ByMonth, func(i, j int) bool { return out.ByMonth[i].Month < out.ByMonth[j].Month })
	return out, nil
}
