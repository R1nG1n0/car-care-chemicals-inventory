package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Пределы числа аналогов (ФТ-15.5.2, ФТ-15.5.3).
const (
	AnalogsLimit         = 5 // в карточке позиции
	ShoppingAnalogsLimit = 3 // у позиции ниже минимума в списке покупок
)

// AnalogBrief — короткая строка аналога в списке покупок.
type AnalogBrief struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Brand string `json:"brand"`
	Unit  string `json:"unit"`
	Total int64  `json:"total"`
}

// Analogs — до limit позиций «есть дома для того же» (ФТ-15.5): действующие,
// в наличии (остаток больше нуля), с хотя бы одним общим назначением, не сама
// позиция и не «Не брать больше». Порядок: больше общих назначений, выше
// оценка (без оценки — после оценённых), больше остаток, имя.
func (s *Service) Analogs(ctx context.Context, userID, productID int64, limit int) ([]ProductSummary, error) {
	if _, err := s.Summary(ctx, userID, productID); err != nil {
		return nil, err
	}
	rows, err := s.q.QueryContext(ctx, `SELECT o.product_id, COUNT(*)
		FROM product_purposes me
		JOIN product_purposes o ON o.user_id = me.user_id AND o.purpose_id = me.purpose_id
		JOIN products p ON p.user_id = o.user_id AND p.id = o.product_id
		WHERE me.user_id = ? AND me.product_id = ? AND o.product_id <> me.product_id
		  AND p.archived = 0 AND p.do_not_buy = 0
		GROUP BY o.product_id`, userID, productID)
	if err != nil {
		return nil, fmt.Errorf("аналоги: %w", err)
	}
	common := map[int64]int{}
	var ids []int64
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			rows.Close()
			return nil, fmt.Errorf("аналоги: %w", err)
		}
		common[id] = n
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("аналоги: %w", err)
	}
	cands, err := s.Summaries(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	out := cands[:0]
	for _, c := range cands {
		if c.Stock.Total > 0 {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if common[a.ID] != common[b.ID] {
			return common[a.ID] > common[b.ID]
		}
		if (a.Rating == nil) != (b.Rating == nil) {
			return a.Rating != nil
		}
		if a.Rating != nil && *a.Rating != *b.Rating {
			return *a.Rating > *b.Rating
		}
		if a.Stock.Total != b.Stock.Total {
			return a.Stock.Total > b.Stock.Total
		}
		na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if na != nb {
			return na < nb
		}
		return a.ID < b.ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// AnalogBriefs — Analogs в коротком виде; список не nil.
func (s *Service) AnalogBriefs(ctx context.Context, userID, productID int64, limit int) ([]AnalogBrief, error) {
	list, err := s.Analogs(ctx, userID, productID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]AnalogBrief, 0, len(list))
	for _, a := range list {
		out = append(out, AnalogBrief{ID: a.ID, Name: a.Name, Brand: a.Brand, Unit: a.Unit, Total: a.Stock.Total})
	}
	return out, nil
}
