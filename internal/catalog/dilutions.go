package catalog

import (
	"context"
	"fmt"
)

// Пределы пропорций позиции (ФТ-17.1).
const (
	MaxProductDilutions = 10
	dilutionNameMax     = 40
	dilutionRatioMax    = 1000
)

const msgDilutionNotFound = "Пропорция не найдена"

// Dilution — пропорция разбавления позиции: название и 1:Ratio.
type Dilution struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Ratio int64  `json:"ratio"`
	Sort  int64  `json:"sort"`
}

// DilutionInput — пропорция в запросе. ID 0 — новая; иначе — существующая
// пропорция этой же позиции (id сохраняется, меняются название и пропорция).
type DilutionInput struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Ratio int64  `json:"ratio"`
}

// validateDilutions проверяет и нормализует список пропорций позиции с единицей
// unit: у позиции в шт пропорций нет, не больше MaxProductDilutions.
func validateDilutions(unit string, list []DilutionInput) ([]DilutionInput, error) {
	if len(list) == 0 {
		return nil, nil
	}
	if unit != "ml" {
		return nil, invalid("dilutions", "Пропорции бывают только у позиций в мл")
	}
	if len(list) > MaxProductDilutions {
		return nil, invalid("dilutions", fmt.Sprintf("Пропорций у позиции — не больше %d", MaxProductDilutions))
	}
	out := make([]DilutionInput, 0, len(list))
	seen := map[int64]bool{}
	for _, d := range list {
		name, err := cleanName("dilutions", d.Name, dilutionNameMax, "Название пропорции")
		if err != nil {
			return nil, err
		}
		if d.Ratio < 1 || d.Ratio > dilutionRatioMax {
			return nil, invalid("dilutions", "Пропорция — 1:N, N от 1 до 1000")
		}
		if d.ID < 0 || (d.ID != 0 && seen[d.ID]) {
			return nil, invalid("dilutions", "Пропорция повторяется в списке")
		}
		seen[d.ID] = true
		out = append(out, DilutionInput{ID: d.ID, Name: name, Ratio: d.Ratio})
	}
	return out, nil
}

// setDilutions заменяет пропорции позиции списком list (уже проверенным) в
// его порядке. Пропорция с id обновляется (чужой или несуществующий id — 404),
// остальные прежние удаляются.
func (s *Service) setDilutions(ctx context.Context, userID, productID int64, list []DilutionInput) error {
	old, err := s.dilutionIDs(ctx, userID, productID)
	if err != nil {
		return err
	}
	keep := map[int64]bool{}
	for _, d := range list {
		if d.ID != 0 {
			if !old[d.ID] {
				return notFound(msgDilutionNotFound)
			}
			keep[d.ID] = true
		}
	}
	for id := range old {
		if keep[id] {
			continue
		}
		if _, err := s.q.ExecContext(ctx, `DELETE FROM product_dilutions WHERE user_id = ? AND id = ?`, userID, id); err != nil {
			return fmt.Errorf("пропорции позиции: %w", err)
		}
	}
	for i, d := range list {
		var err error
		if d.ID == 0 {
			_, err = s.q.ExecContext(ctx, `INSERT INTO product_dilutions (user_id, product_id, name, ratio, sort)
				VALUES (?, ?, ?, ?, ?)`, userID, productID, d.Name, d.Ratio, i)
		} else {
			_, err = s.q.ExecContext(ctx, `UPDATE product_dilutions SET name = ?, ratio = ?, sort = ?
				WHERE user_id = ? AND product_id = ? AND id = ?`, d.Name, d.Ratio, i, userID, productID, d.ID)
		}
		if err != nil {
			return fmt.Errorf("пропорции позиции: %w", err)
		}
	}
	return nil
}

// dilutionIDs — id пропорций позиции.
func (s *Service) dilutionIDs(ctx context.Context, userID, productID int64) (map[int64]bool, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id FROM product_dilutions WHERE user_id = ? AND product_id = ?`, userID, productID)
	if err != nil {
		return nil, fmt.Errorf("пропорции позиции: %w", err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("пропорции позиции: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// fillDilutions добавляет к сводкам пропорции в порядке sort, затем id.
func (s *Service) fillDilutions(ctx context.Context, userID int64, in string, args []any, byID map[int64]*ProductSummary) error {
	rows, err := s.q.QueryContext(ctx, `SELECT product_id, id, name, ratio, sort FROM product_dilutions
		WHERE user_id = ? AND product_id IN (`+in+`) ORDER BY sort, id`, args...)
	if err != nil {
		return fmt.Errorf("пропорции позиций: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var productID int64
		var d Dilution
		if err := rows.Scan(&productID, &d.ID, &d.Name, &d.Ratio, &d.Sort); err != nil {
			return fmt.Errorf("пропорции позиций: %w", err)
		}
		if p := byID[productID]; p != nil {
			p.Dilutions = append(p.Dilutions, d)
		}
	}
	return rows.Err()
}
