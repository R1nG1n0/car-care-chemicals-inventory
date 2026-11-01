package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Пределы заметки (ФТ-18.1).
const (
	NoteTitleMax = 120
	NoteBodyMax  = 10000
	// MaxNoteLinks — сколько позиций и сколько шаблонов можно привязать к заметке.
	MaxNoteLinks = 50
)

const (
	msgNoteNotFound   = "Заметка не найдена"
	msgRecipeNotFound = "Шаблон не найден"
)

// Note — заметка со связями.
type Note struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	ProductIDs []int64 `json:"product_ids"`
	RecipeIDs  []int64 `json:"recipe_ids"`
	CreatedAt  int64   `json:"created_at"`
	UpdatedAt  int64   `json:"updated_at"`
}

// NoteInput — создание заметки.
type NoteInput struct {
	Title      string
	Body       string
	ProductIDs []int64
	RecipeIDs  []int64
}

// NotePatch — изменение заметки; nil — поле не меняется, списки заменяются целиком.
type NotePatch struct {
	Title      *string
	Body       *string
	ProductIDs *[]int64
	RecipeIDs  *[]int64
}

// NoteFilter — параметры списка заметок.
type NoteFilter struct {
	Query     string // по заголовку и тексту, без учёта регистра
	ProductID int64  // 0 — любая
	RecipeID  int64  // 0 — любой
}

// ValidateNoteText проверяет и нормализует заголовок и текст заметки
// (также для импорта файла). Переносы строк внутри текста сохраняются.
func ValidateNoteText(title, body string) (string, string, error) {
	title, err := cleanName("title", title, NoteTitleMax, "Заголовок")
	if err != nil {
		return "", "", err
	}
	body, err = optText("body", body, NoteBodyMax, "Текст")
	if err != nil {
		return "", "", err
	}
	return title, body, nil
}

// dedupLinks убирает повторы id (порядок ввода сохраняется) и проверяет предел.
func dedupLinks(field string, ids []int64) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > MaxNoteLinks {
		return nil, invalid(field, fmt.Sprintf("Связей — не больше %d", MaxNoteLinks))
	}
	return out, nil
}

// checkOwned: каждая запись table принадлежит пользователю (иначе 404 с msg).
func (s *Service) checkOwned(ctx context.Context, table string, userID int64, ids []int64, msg string) error {
	for _, id := range ids {
		var one int
		err := s.q.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE user_id = ? AND id = ?`, userID, id).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound(msg)
		}
		if err != nil {
			return fmt.Errorf("связи заметки: %w", err)
		}
	}
	return nil
}

// setNoteLinks заменяет связи заметки с таблицей link (колонка col) списком ids.
func (s *Service) setNoteLinks(ctx context.Context, link, col string, userID, noteID int64, ids []int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM `+link+` WHERE user_id = ? AND note_id = ?`, userID, noteID); err != nil {
		return fmt.Errorf("связи заметки: %w", err)
	}
	for _, id := range ids {
		if _, err := s.q.ExecContext(ctx, `INSERT INTO `+link+` (user_id, note_id, `+col+`) VALUES (?, ?, ?)`,
			userID, noteID, id); err != nil {
			return fmt.Errorf("связи заметки: %w", err)
		}
	}
	return nil
}

// prepareLinks проверяет оба списка связей: без повторов, в пределах, свои.
func (s *Service) prepareLinks(ctx context.Context, userID int64, products, recipes []int64) ([]int64, []int64, error) {
	p, err := dedupLinks("product_ids", products)
	if err != nil {
		return nil, nil, err
	}
	r, err := dedupLinks("recipe_ids", recipes)
	if err != nil {
		return nil, nil, err
	}
	if err := s.checkOwned(ctx, "products", userID, p, msgProductNotFound); err != nil {
		return nil, nil, err
	}
	if err := s.checkOwned(ctx, "recipes", userID, r, msgRecipeNotFound); err != nil {
		return nil, nil, err
	}
	return p, r, nil
}

// CreateNote добавляет заметку; at — время (секунды Unix).
func (s *Service) CreateNote(ctx context.Context, userID int64, in NoteInput, at int64) (Note, error) {
	title, body, err := ValidateNoteText(in.Title, in.Body)
	if err != nil {
		return Note{}, err
	}
	var out Note
	err = s.inTxSvc(ctx, func(t *Service) error {
		products, recipes, err := t.prepareLinks(ctx, userID, in.ProductIDs, in.RecipeIDs)
		if err != nil {
			return err
		}
		res, err := t.q.ExecContext(ctx, `INSERT INTO notes (user_id, title, body, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`, userID, title, body, at, at)
		if err != nil {
			return fmt.Errorf("создание заметки: %w", err)
		}
		id, _ := res.LastInsertId()
		if err := t.setNoteLinks(ctx, "note_products", "product_id", userID, id, products); err != nil {
			return err
		}
		if err := t.setNoteLinks(ctx, "note_recipes", "recipe_id", userID, id, recipes); err != nil {
			return err
		}
		out, err = t.Note(ctx, userID, id)
		return err
	})
	return out, err
}

// UpdateNote меняет заметку и обновляет updated_at.
func (s *Service) UpdateNote(ctx context.Context, userID, id int64, p NotePatch, at int64) (Note, error) {
	var out Note
	err := s.inTxSvc(ctx, func(t *Service) error {
		cur, err := t.Note(ctx, userID, id)
		if err != nil {
			return err
		}
		if p.Title != nil {
			cur.Title = *p.Title
		}
		if p.Body != nil {
			cur.Body = *p.Body
		}
		if cur.Title, cur.Body, err = ValidateNoteText(cur.Title, cur.Body); err != nil {
			return err
		}
		products, recipes := cur.ProductIDs, cur.RecipeIDs
		if p.ProductIDs != nil {
			products = *p.ProductIDs
		}
		if p.RecipeIDs != nil {
			recipes = *p.RecipeIDs
		}
		if p.ProductIDs != nil || p.RecipeIDs != nil {
			if products, recipes, err = t.prepareLinks(ctx, userID, products, recipes); err != nil {
				return err
			}
		}
		if _, err := t.q.ExecContext(ctx, `UPDATE notes SET title = ?, body = ?, updated_at = ?
			WHERE user_id = ? AND id = ?`, cur.Title, cur.Body, at, userID, id); err != nil {
			return fmt.Errorf("изменение заметки: %w", err)
		}
		if p.ProductIDs != nil {
			if err := t.setNoteLinks(ctx, "note_products", "product_id", userID, id, products); err != nil {
				return err
			}
		}
		if p.RecipeIDs != nil {
			if err := t.setNoteLinks(ctx, "note_recipes", "recipe_id", userID, id, recipes); err != nil {
				return err
			}
		}
		out, err = t.Note(ctx, userID, id)
		return err
	})
	return out, err
}

// DeleteNote удаляет заметку со связями; чужая или несуществующая — 404.
func (s *Service) DeleteNote(ctx context.Context, userID, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM notes WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("удаление заметки: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound(msgNoteNotFound)
	}
	return nil
}

// Note — одна заметка; чужая или несуществующая — ErrNotFound.
func (s *Service) Note(ctx context.Context, userID, id int64) (Note, error) {
	list, err := s.queryNotes(ctx, `WHERE n.user_id = ? AND n.id = ?`, []any{userID, id})
	if err != nil {
		return Note{}, err
	}
	if len(list) == 0 {
		return Note{}, notFound(msgNoteNotFound)
	}
	return list[0], nil
}

// Notes — заметки по updated_at убыв. Поиск — в Go: LOWER в SQLite знает
// только латиницу.
func (s *Service) Notes(ctx context.Context, userID int64, f NoteFilter) ([]Note, error) {
	where := `WHERE n.user_id = ?`
	args := []any{userID}
	if f.ProductID != 0 {
		where += ` AND EXISTS (SELECT 1 FROM note_products x WHERE x.user_id = n.user_id AND x.note_id = n.id AND x.product_id = ?)`
		args = append(args, f.ProductID)
	}
	if f.RecipeID != 0 {
		where += ` AND EXISTS (SELECT 1 FROM note_recipes x WHERE x.user_id = n.user_id AND x.note_id = n.id AND x.recipe_id = ?)`
		args = append(args, f.RecipeID)
	}
	all, err := s.queryNotes(ctx, where, args)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q == "" {
		return all, nil
	}
	out := []Note{}
	for _, n := range all {
		if strings.Contains(strings.ToLower(n.Title), q) || strings.Contains(strings.ToLower(n.Body), q) {
			out = append(out, n)
		}
	}
	return out, nil
}

// queryNotes читает заметки по условию where и добавляет связи.
func (s *Service) queryNotes(ctx context.Context, where string, args []any) ([]Note, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT n.id, n.title, n.body, n.created_at, n.updated_at FROM notes n `+
		where+` ORDER BY n.updated_at DESC, n.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("заметки: %w", err)
	}
	out := []Note{}
	byID := map[int64]int{}
	for rows.Next() {
		n := Note{ProductIDs: []int64{}, RecipeIDs: []int64{}}
		if err := rows.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("заметки: %w", err)
		}
		byID[n.ID] = len(out)
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("заметки: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	userID := args[0].(int64)
	for _, l := range []struct{ table, col string }{{"note_products", "product_id"}, {"note_recipes", "recipe_id"}} {
		lr, err := s.q.QueryContext(ctx, `SELECT note_id, `+l.col+` FROM `+l.table+` WHERE user_id = ? ORDER BY `+l.col, userID)
		if err != nil {
			return nil, fmt.Errorf("связи заметок: %w", err)
		}
		for lr.Next() {
			var noteID, target int64
			if err := lr.Scan(&noteID, &target); err != nil {
				lr.Close()
				return nil, fmt.Errorf("связи заметок: %w", err)
			}
			i, ok := byID[noteID]
			if !ok {
				continue
			}
			if l.table == "note_products" {
				out[i].ProductIDs = append(out[i].ProductIDs, target)
			} else {
				out[i].RecipeIDs = append(out[i].RecipeIDs, target)
			}
		}
		lr.Close()
		if err := lr.Err(); err != nil {
			return nil, fmt.Errorf("связи заметок: %w", err)
		}
	}
	return out, nil
}
