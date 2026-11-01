package transfer

import (
	"context"
	"database/sql"
	"fmt"
)

func exportNotes(ctx context.Context, q querier, uid int64, f *File) error {
	byID := map[int64]int{}
	err := each(ctx, q, "заметки", `SELECT id, title, body, created_at, updated_at FROM notes
		WHERE user_id = ? ORDER BY id`, []any{uid}, func(r *sql.Rows) error {
		n := Note{ProductIDs: []int64{}, RecipeIDs: []int64{}}
		if err := r.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return err
		}
		byID[n.ID] = len(f.Notes)
		f.Notes = append(f.Notes, n)
		return nil
	})
	if err != nil {
		return err
	}
	for _, l := range []struct{ table, col string }{{"note_products", "product_id"}, {"note_recipes", "recipe_id"}} {
		err := each(ctx, q, "связи заметок", `SELECT note_id, `+l.col+` FROM `+l.table+`
			WHERE user_id = ? ORDER BY note_id, `+l.col, []any{uid}, func(r *sql.Rows) error {
			var note, target int64
			if err := r.Scan(&note, &target); err != nil {
				return err
			}
			i, ok := byID[note]
			if !ok {
				return fmt.Errorf("связь: нет заметки %d", note)
			}
			if l.table == "note_products" {
				f.Notes[i].ProductIDs = append(f.Notes[i].ProductIDs, target)
			} else {
				f.Notes[i].RecipeIDs = append(f.Notes[i].RecipeIDs, target)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func exportRags(ctx context.Context, q querier, uid int64, f *File) error {
	return each(ctx, q, "тряпки", `SELECT id, product_id, label, number, washes, status, retire_reason,
		(SELECT m.id FROM movements m WHERE m.user_id = rags.user_id AND m.id = rags.consume_movement_id),
		created_at, retired_at, last_washed_at FROM rags WHERE user_id = ? ORDER BY id`,
		[]any{uid}, func(r *sql.Rows) error {
			var g Rag
			var reason sql.NullString
			var movement, retired, washed sql.NullInt64
			if err := r.Scan(&g.ID, &g.ProductID, &g.Label, &g.Number, &g.Washes, &g.Status, &reason,
				&movement, &g.CreatedAt, &retired, &washed); err != nil {
				return err
			}
			if reason.Valid {
				g.RetireReason = &reason.String
			}
			g.ConsumeMovementID, g.RetiredAt, g.LastWashedAt = ptr(movement), ptr(retired), ptr(washed)
			f.Rags = append(f.Rags, g)
			return nil
		})
}

// notesStep переносит заметки и их связи (позиции и шаблоны уже перенесены).
func (im *importer) notesStep(f *File) error {
	if len(f.Notes) == 0 {
		return nil
	}
	ins, err := im.prepare("заметка", `INSERT INTO notes (user_id, title, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	linkP, err := im.prepare("связь заметки с позицией", `INSERT INTO note_products (user_id, note_id, product_id)
		VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	linkR, err := im.prepare("связь заметки с шаблоном", `INSERT INTO note_recipes (user_id, note_id, recipe_id)
		VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, n := range f.Notes {
		id, err := im.run(ins, "заметка", im.uid, n.Title, n.Body, n.CreatedAt, n.UpdatedAt)
		if err != nil {
			return err
		}
		for _, p := range n.ProductIDs {
			if _, err := im.run(linkP, "связь заметки с позицией", im.uid, id, im.products[p]); err != nil {
				return err
			}
		}
		for _, r := range n.RecipeIDs {
			if _, err := im.run(linkR, "связь заметки с шаблоном", im.uid, id, im.recipes[r]); err != nil {
				return err
			}
		}
	}
	return nil
}

// ragsStep переносит тряпки (позиции и движения уже перенесены).
func (im *importer) ragsStep(f *File) error {
	if len(f.Rags) == 0 {
		return nil
	}
	ins, err := im.prepare("тряпка", `INSERT INTO rags (user_id, product_id, label, number, washes, status,
		retire_reason, consume_movement_id, created_at, retired_at, last_washed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, g := range f.Rags {
		if _, err := im.run(ins, "тряпка", im.uid, im.products[g.ProductID], g.Label, g.Number, g.Washes, g.Status,
			g.RetireReason, mapped(im.movements, g.ConsumeMovementID), g.CreatedAt, g.RetiredAt, g.LastWashedAt); err != nil {
			return err
		}
	}
	return nil
}
