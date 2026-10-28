package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Purpose — назначение средства (ФТ-15.1): ЛКП, стекло, кожа и т. п.
type Purpose struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Sort          int64  `json:"sort"`
	Archived      bool   `json:"archived"`
	ProductsCount int64  `json:"products_count"`
}

const msgPurposeNotFound = "Назначение не найдено"

// MaxProductPurposes — сколько назначений можно указать у одной позиции.
const MaxProductPurposes = 50

const purposeSelect = `SELECT c.id, c.name, c.sort, c.archived,
	(SELECT COUNT(*) FROM product_purposes pp JOIN products p ON p.user_id = pp.user_id AND p.id = pp.product_id
	  WHERE pp.user_id = c.user_id AND pp.purpose_id = c.id AND p.archived = 0)
	FROM purposes c `

func scanPurpose(sc interface{ Scan(...any) error }) (Purpose, error) {
	var c Purpose
	err := sc.Scan(&c.ID, &c.Name, &c.Sort, &c.Archived, &c.ProductsCount)
	return c, err
}

// Purposes — назначения по sort, затем по имени; withArchived — вместе с архивными.
func (s *Service) Purposes(ctx context.Context, userID int64, withArchived bool) ([]Purpose, error) {
	rows, err := s.q.QueryContext(ctx, purposeSelect+
		`WHERE c.user_id = ? AND (? OR c.archived = 0) ORDER BY c.sort, c.name, c.id`, userID, withArchived)
	if err != nil {
		return nil, fmt.Errorf("назначения: %w", err)
	}
	defer rows.Close()
	out := []Purpose{}
	for rows.Next() {
		c, err := scanPurpose(rows)
		if err != nil {
			return nil, fmt.Errorf("назначения: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) purpose(ctx context.Context, userID, id int64) (Purpose, error) {
	c, err := scanPurpose(s.q.QueryRowContext(ctx, purposeSelect+`WHERE c.user_id = ? AND c.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Purpose{}, notFound(msgPurposeNotFound)
	}
	return c, err
}

// CreatePurpose добавляет назначение в конец списка.
func (s *Service) CreatePurpose(ctx context.Context, userID int64, name string) (Purpose, error) {
	name, err := cleanName("name", name, refNameMax, "Название")
	if err != nil {
		return Purpose{}, err
	}
	var out Purpose
	err = s.inTxSvc(ctx, func(t *Service) error {
		if err := nameFree(ctx, t.q, "purposes", userID, 0, name); err != nil {
			return err
		}
		res, err := t.q.ExecContext(ctx, `INSERT INTO purposes (user_id, name, sort)
			VALUES (?, ?, (SELECT COALESCE(MAX(sort), -1) + 1 FROM purposes WHERE user_id = ?))`,
			userID, name, userID)
		if err != nil {
			return nameTaken(err)
		}
		id, _ := res.LastInsertId()
		out, err = t.purpose(ctx, userID, id)
		return err
	})
	return out, err
}

// UpdatePurpose меняет имя и архив. Возвращённое из архива назначение встаёт
// в конец списка. Архивное остаётся у позиций (ФТ-15.1.3).
func (s *Service) UpdatePurpose(ctx context.Context, userID, id int64, p RefPatch) (Purpose, error) {
	var out Purpose
	err := s.inTxSvc(ctx, func(t *Service) (err error) {
		out, err = t.updatePurpose(ctx, userID, id, p)
		return err
	})
	return out, err
}

func (s *Service) updatePurpose(ctx context.Context, userID, id int64, p RefPatch) (Purpose, error) {
	cur, err := s.purpose(ctx, userID, id)
	if err != nil {
		return Purpose{}, err
	}
	if p.Name != nil {
		if cur.Name, err = cleanName("name", *p.Name, refNameMax, "Название"); err != nil {
			return Purpose{}, err
		}
		if err := nameFree(ctx, s.q, "purposes", userID, id, cur.Name); err != nil {
			return Purpose{}, err
		}
	}
	sortExpr := "sort"
	if p.Archived != nil {
		if cur.Archived && !*p.Archived {
			sortExpr = "(SELECT COALESCE(MAX(sort), -1) + 1 FROM purposes WHERE user_id = ? AND archived = 0)"
		}
		cur.Archived = *p.Archived
	}
	args := []any{cur.Name, cur.Archived}
	if sortExpr != "sort" {
		args = append(args, userID)
	}
	args = append(args, userID, id)
	if _, err := s.q.ExecContext(ctx, `UPDATE purposes SET name = ?, archived = ?, sort = `+
		sortExpr+` WHERE user_id = ? AND id = ?`, args...); err != nil {
		return Purpose{}, nameTaken(err)
	}
	return s.purpose(ctx, userID, id)
}

// ReorderPurposes — новый порядок действующих назначений.
func (s *Service) ReorderPurposes(ctx context.Context, userID int64, ids []int64) error {
	return s.reorder(ctx, "purposes", userID, ids)
}

// ---------- Назначения позиции ----------

// checkPurposeIDs проверяет список назначений позиции: свои (иначе 404),
// без повторов, не больше MaxProductPurposes; архивное допустимо только
// если оно уже есть у позиции (productID; 0 — позиция новая). Возвращает
// список без повторов в порядке ввода.
func (s *Service) checkPurposeIDs(ctx context.Context, userID, productID int64, ids []int64) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > MaxProductPurposes {
		return nil, invalid("purpose_ids", fmt.Sprintf("Назначений у позиции — не больше %d", MaxProductPurposes))
	}
	for _, id := range out {
		var archived, linked bool
		err := s.q.QueryRowContext(ctx, `SELECT c.archived,
			EXISTS (SELECT 1 FROM product_purposes pp WHERE pp.user_id = c.user_id AND pp.purpose_id = c.id AND pp.product_id = ?)
			FROM purposes c WHERE c.user_id = ? AND c.id = ?`, productID, userID, id).Scan(&archived, &linked)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound(msgPurposeNotFound)
		}
		if err != nil {
			return nil, fmt.Errorf("назначение позиции: %w", err)
		}
		if archived && !linked {
			return nil, invalid("purpose_ids", "Назначение в архиве")
		}
	}
	return out, nil
}

// setProductPurposes заменяет назначения позиции (ids уже проверены).
func (s *Service) setProductPurposes(ctx context.Context, userID, productID int64, ids []int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM product_purposes WHERE user_id = ? AND product_id = ?`,
		userID, productID); err != nil {
		return fmt.Errorf("назначения позиции: %w", err)
	}
	for _, id := range ids {
		if _, err := s.q.ExecContext(ctx, `INSERT INTO product_purposes (user_id, product_id, purpose_id)
			VALUES (?, ?, ?)`, userID, productID, id); err != nil {
			return fmt.Errorf("назначения позиции: %w", err)
		}
	}
	return nil
}

// fillPurposes добавляет к сводкам purpose_ids (по возрастанию id).
func (s *Service) fillPurposes(ctx context.Context, userID int64, in string, args []any, byID map[int64]*ProductSummary) error {
	rows, err := s.q.QueryContext(ctx, `SELECT product_id, purpose_id FROM product_purposes
		WHERE user_id = ? AND product_id IN (`+in+`) ORDER BY purpose_id`, args...)
	if err != nil {
		return fmt.Errorf("назначения позиций: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var productID, purposeID int64
		if err := rows.Scan(&productID, &purposeID); err != nil {
			return fmt.Errorf("назначения позиций: %w", err)
		}
		if p := byID[productID]; p != nil {
			p.PurposeIDs = append(p.PurposeIDs, purposeID)
		}
	}
	return rows.Err()
}

// purposeNames — имена всех назначений пользователя (в нижнем регистре) по id.
func (s *Service) purposeNames(ctx context.Context, userID int64) (map[int64]string, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name FROM purposes WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("назначения: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("назначения: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}
