package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ---------- Магазины ----------

const storeSelect = `SELECT id, name, url, archived FROM stores `

func scanStore(sc interface{ Scan(...any) error }) (Store, error) {
	var st Store
	err := sc.Scan(&st.ID, &st.Name, &st.URL, &st.Archived)
	return st, err
}

// Stores — магазины по имени; withArchived — вместе с архивными.
func (s *Service) Stores(ctx context.Context, userID int64, withArchived bool) ([]Store, error) {
	rows, err := s.q.QueryContext(ctx, storeSelect+
		`WHERE user_id = ? AND (? OR archived = 0) ORDER BY name, id`, userID, withArchived)
	if err != nil {
		return nil, fmt.Errorf("магазины: %w", err)
	}
	defer rows.Close()
	out := []Store{}
	for rows.Next() {
		st, err := scanStore(rows)
		if err != nil {
			return nil, fmt.Errorf("магазины: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Service) store(ctx context.Context, userID, id int64) (Store, error) {
	st, err := scanStore(s.q.QueryRowContext(ctx, storeSelect+`WHERE user_id = ? AND id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Store{}, notFound(msgStoreNotFound)
	}
	return st, err
}

// CreateStore добавляет магазин.
func (s *Service) CreateStore(ctx context.Context, userID int64, name, rawURL string) (Store, error) {
	name, err := cleanName("name", name, refNameMax, "Название")
	if err != nil {
		return Store{}, err
	}
	u, err := validStoreURL(rawURL)
	if err != nil {
		return Store{}, err
	}
	var out Store
	err = s.inTxSvc(ctx, func(t *Service) error {
		if err := nameFree(ctx, t.q, "stores", userID, 0, name); err != nil {
			return err
		}
		res, err := t.q.ExecContext(ctx, `INSERT INTO stores (user_id, name, url) VALUES (?, ?, ?)`, userID, name, u)
		if err != nil {
			return nameTaken(err)
		}
		id, _ := res.LastInsertId()
		out, err = t.store(ctx, userID, id)
		return err
	})
	return out, err
}

// UpdateStore меняет имя, ссылку и архив.
func (s *Service) UpdateStore(ctx context.Context, userID, id int64, p RefPatch) (Store, error) {
	var out Store
	err := s.inTxSvc(ctx, func(t *Service) (err error) {
		out, err = t.updateStore(ctx, userID, id, p)
		return err
	})
	return out, err
}

func (s *Service) updateStore(ctx context.Context, userID, id int64, p RefPatch) (Store, error) {
	cur, err := s.store(ctx, userID, id)
	if err != nil {
		return Store{}, err
	}
	if p.Name != nil {
		if cur.Name, err = cleanName("name", *p.Name, refNameMax, "Название"); err != nil {
			return Store{}, err
		}
		if err := nameFree(ctx, s.q, "stores", userID, id, cur.Name); err != nil {
			return Store{}, err
		}
	}
	if p.URL != nil {
		if cur.URL, err = validStoreURL(*p.URL); err != nil {
			return Store{}, err
		}
	}
	if p.Archived != nil {
		cur.Archived = *p.Archived
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE stores SET name = ?, url = ?, archived = ? WHERE user_id = ? AND id = ?`,
		cur.Name, cur.URL, cur.Archived, userID, id); err != nil {
		return Store{}, nameTaken(err)
	}
	return cur, nil
}
