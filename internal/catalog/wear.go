package catalog

import (
	"context"
	"fmt"
)

// Пределы ресурса тряпки в стирках (ФТ-19.1).
const (
	WearLimitMin = 1
	WearLimitMax = 1000
)

// RagCounts — тряпки позиции в работе (ФТ-19.5): всего, «скоро менять»
// (от 90 % ресурса, но ещё не 100 %) и «пора менять» (от 100 %).
type RagCounts struct {
	Active int64 `json:"active"`
	Soon   int64 `json:"soon"`
	Due    int64 `json:"due"`
}

// validateWearLimit: ресурс — только у позиции в шт, 1…1000.
func validateWearLimit(unit string, limit *int64) error {
	if limit == nil {
		return nil
	}
	if unit != "pcs" {
		return invalid("wear_limit", "Ресурс стирок бывает только у позиций в шт")
	}
	if *limit < WearLimitMin || *limit > WearLimitMax {
		return invalid("wear_limit", "Ресурс — от 1 до 1000 стирок или пусто")
	}
	return nil
}

// ValidateWearLimit — проверка ресурса для импорта файла.
func ValidateWearLimit(unit string, limit *int64) error { return validateWearLimit(unit, limit) }

// fillRags считает тряпки в работе у позиций с ресурсом; у остальных Rags = nil.
// Порог тот же, что у stock.RagLevel: soon — washes×100 ≥ limit×90, due — washes ≥ limit.
func (s *Service) fillRags(ctx context.Context, userID int64, in string, args []any, byID map[int64]*ProductSummary) error {
	rows, err := s.q.QueryContext(ctx, `
		SELECT r.product_id, COUNT(*),
		       COALESCE(SUM(r.washes < p.wear_limit AND r.washes * 100 >= p.wear_limit * 90), 0),
		       COALESCE(SUM(r.washes >= p.wear_limit), 0)
		FROM rags r JOIN products p ON p.user_id = r.user_id AND p.id = r.product_id
		WHERE r.user_id = ? AND r.status = 'active' AND p.wear_limit IS NOT NULL
		  AND r.product_id IN (`+in+`)
		GROUP BY r.product_id`, args...)
	if err != nil {
		return fmt.Errorf("тряпки позиций: %w", err)
	}
	defer rows.Close()
	counts := map[int64]RagCounts{}
	for rows.Next() {
		var productID int64
		var c RagCounts
		if err := rows.Scan(&productID, &c.Active, &c.Soon, &c.Due); err != nil {
			return fmt.Errorf("тряпки позиций: %w", err)
		}
		counts[productID] = c
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("тряпки позиций: %w", err)
	}
	for id, p := range byID {
		if p.WearLimit != nil {
			c := counts[id]
			p.Rags = &c
		}
	}
	return nil
}
