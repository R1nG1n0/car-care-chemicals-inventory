package catalog

import (
	"context"
	"fmt"
)

// InventoryProduct — данные позиции в строке инвентаризации.
type InventoryProduct struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Brand       string `json:"brand"`
	Unit        string `json:"unit"`
	PackageSize int64  `json:"package_size"`
}

// InventoryUnit — экземпляр места вместе с позицией (ФТ-10.1.1).
type InventoryUnit struct {
	Unit
	Product InventoryProduct `json:"product"`
}

// LocationUnits — экземпляры sealed/open действующих позиций в месте:
// вскрытые первыми, затем по названию позиции и id. Чужое место — ErrNotFound.
// Архивное место отдаёт пустой список.
func (s *Service) LocationUnits(ctx context.Context, userID, locationID int64) ([]InventoryUnit, error) {
	if _, err := s.location(ctx, userID, locationID); err != nil {
		return nil, err
	}
	rows, err := s.q.QueryContext(ctx, `SELECT `+unitColumns+`, p.name, p.brand, p.unit, p.package_size`+unitFrom+`
		JOIN products p ON p.user_id = u.user_id AND p.id = u.product_id
		JOIN locations lc ON lc.user_id = u.user_id AND lc.id = u.location_id
		WHERE u.user_id = ? AND u.location_id = ? AND lc.archived = 0 AND u.status IN ('sealed', 'open') AND p.archived = 0
		ORDER BY p.name COLLATE NOCASE, p.id, CASE u.status WHEN 'open' THEN 0 ELSE 1 END, u.id`,
		userID, locationID)
	if err != nil {
		return nil, fmt.Errorf("экземпляры места: %w", err)
	}
	defer rows.Close()
	out := []InventoryUnit{}
	for rows.Next() {
		var iu InventoryUnit
		var u Unit
		u, err = scanUnitExtra(rows, &iu.Product)
		if err != nil {
			return nil, fmt.Errorf("экземпляры места: %w", err)
		}
		iu.Unit = u
		iu.Product.ID = u.ProductID
		out = append(out, iu)
	}
	return out, rows.Err()
}

// MarkLocationChecked ставит месту отметку «проверено» = at. Архивное место
// проверять нельзя (409), чужое — ErrNotFound. Возвращает обновлённое место.
func (s *Service) MarkLocationChecked(ctx context.Context, userID, id, at int64) (Location, error) {
	var out Location
	err := s.inTxSvc(ctx, func(t *Service) error {
		cur, err := t.location(ctx, userID, id)
		if err != nil {
			return err
		}
		if cur.Archived {
			return conflict("", "Место в архиве: проверять нечего")
		}
		if _, err := t.q.ExecContext(ctx, `UPDATE locations SET checked_at = ? WHERE user_id = ? AND id = ?`,
			at, userID, id); err != nil {
			return fmt.Errorf("отметка проверки: %w", err)
		}
		out, err = t.location(ctx, userID, id)
		return err
	})
	return out, err
}
