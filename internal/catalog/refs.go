package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Category — категория позиций.
type Category struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Color         string `json:"color"`
	Sort          int64  `json:"sort"`
	Archived      bool   `json:"archived"`
	ProductsCount int64  `json:"products_count"`
}

// Location — место хранения.
type Location struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Sort       int64  `json:"sort"`
	Archived   bool   `json:"archived"`
	UnitsCount int64  `json:"units_count"`
}

// Store — магазин.
type Store struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Archived bool   `json:"archived"`
}

// Colors — восемь цветов категорий дизайн-системы (схема, CHECK).
var Colors = []string{"blue", "berry", "olive", "ochre", "petrol", "plum", "slate", "umber"}

// RefPatch — изменение записи справочника; nil — поле не меняется.
type RefPatch struct {
	Name     *string
	Color    *string // только категории
	URL      *string // только магазины
	Archived *bool
}

const refNameMax = 40

// cleanName обрезает пробелы и проверяет длину 1…max символов.
func cleanName(field, name string, max int, what string) (string, error) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || n < 1 || n > max {
		return "", invalid(field, fmt.Sprintf("%s — от 1 до %d символов", what, max))
	}
	return name, nil
}

// optText проверяет необязательный текст 0…max символов.
func optText(field, s string, max int, what string) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", invalid(field, fmt.Sprintf("%s — не больше %d символов", what, max))
	}
	return s, nil
}

func validColor(c string) error {
	for _, v := range Colors {
		if c == v {
			return nil
		}
	}
	return invalid("color", "Неизвестный цвет")
}

func validStoreURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(raw) > 500 {
		return "", invalid("url", "Ссылка — пусто или адрес http(s)://…, до 500 символов")
	}
	return raw, nil
}

func nameTaken(err error) error {
	if isUniqueViolation(err) {
		return conflict("name", "Такое название уже есть")
	}
	return err
}

// ---------- Категории ----------

const categorySelect = `SELECT c.id, c.name, c.color, c.sort, c.archived,
	(SELECT COUNT(*) FROM products p WHERE p.user_id = c.user_id AND p.category_id = c.id AND p.archived = 0)
	FROM categories c `

func scanCategory(sc interface{ Scan(...any) error }) (Category, error) {
	var c Category
	err := sc.Scan(&c.ID, &c.Name, &c.Color, &c.Sort, &c.Archived, &c.ProductsCount)
	return c, err
}

// Categories — категории по sort, затем по имени; withArchived — вместе с архивными.
func (s *Service) Categories(ctx context.Context, userID int64, withArchived bool) ([]Category, error) {
	rows, err := s.q.QueryContext(ctx, categorySelect+
		`WHERE c.user_id = ? AND (? OR c.archived = 0) ORDER BY c.sort, c.name, c.id`, userID, withArchived)
	if err != nil {
		return nil, fmt.Errorf("категории: %w", err)
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("категории: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) category(ctx context.Context, userID, id int64) (Category, error) {
	c, err := scanCategory(s.q.QueryRowContext(ctx, categorySelect+`WHERE c.user_id = ? AND c.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, notFound(msgCategoryNotFound)
	}
	return c, err
}

// CreateCategory добавляет категорию в конец списка.
func (s *Service) CreateCategory(ctx context.Context, userID int64, name, color string) (Category, error) {
	name, err := cleanName("name", name, refNameMax, "Название")
	if err != nil {
		return Category{}, err
	}
	if err := validColor(color); err != nil {
		return Category{}, err
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO categories (user_id, name, color, sort)
		VALUES (?, ?, ?, (SELECT COALESCE(MAX(sort), -1) + 1 FROM categories WHERE user_id = ?))`,
		userID, name, color, userID)
	if err != nil {
		return Category{}, nameTaken(err)
	}
	id, _ := res.LastInsertId()
	return s.category(ctx, userID, id)
}

// UpdateCategory меняет имя, цвет и архив. Возвращённая из архива
// категория встаёт в конец списка.
func (s *Service) UpdateCategory(ctx context.Context, userID, id int64, p RefPatch) (Category, error) {
	cur, err := s.category(ctx, userID, id)
	if err != nil {
		return Category{}, err
	}
	if p.Name != nil {
		if cur.Name, err = cleanName("name", *p.Name, refNameMax, "Название"); err != nil {
			return Category{}, err
		}
	}
	if p.Color != nil {
		if err := validColor(*p.Color); err != nil {
			return Category{}, err
		}
		cur.Color = *p.Color
	}
	sortExpr := "sort"
	if p.Archived != nil {
		if cur.Archived && !*p.Archived {
			sortExpr = "(SELECT COALESCE(MAX(sort), -1) + 1 FROM categories WHERE user_id = ? AND archived = 0)"
		}
		cur.Archived = *p.Archived
	}
	args := []any{cur.Name, cur.Color, cur.Archived}
	if sortExpr != "sort" {
		args = append(args, userID)
	}
	args = append(args, userID, id)
	if _, err := s.q.ExecContext(ctx, `UPDATE categories SET name = ?, color = ?, archived = ?, sort = `+
		sortExpr+` WHERE user_id = ? AND id = ?`, args...); err != nil {
		return Category{}, nameTaken(err)
	}
	return s.category(ctx, userID, id)
}

// reorder задаёт порядок действующих записей таблицы: ids — ровно все они.
func (s *Service) reorder(ctx context.Context, table string, userID int64, ids []int64) error {
	return s.inTx(ctx, func(q querier) error {
		rows, err := q.QueryContext(ctx, `SELECT id FROM `+table+` WHERE user_id = ? AND archived = 0`, userID)
		if err != nil {
			return fmt.Errorf("порядок: %w", err)
		}
		active := map[int64]bool{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return fmt.Errorf("порядок: %w", err)
			}
			active[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("порядок: %w", err)
		}
		bad := invalid("ids", "Список устарел: обновите страницу")
		if len(ids) != len(active) {
			return bad
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			if !active[id] || seen[id] {
				return bad
			}
			seen[id] = true
		}
		for i, id := range ids {
			if _, err := q.ExecContext(ctx, `UPDATE `+table+` SET sort = ? WHERE user_id = ? AND id = ?`,
				i, userID, id); err != nil {
				return fmt.Errorf("порядок: %w", err)
			}
		}
		return nil
	})
}

// ReorderCategories — новый порядок действующих категорий.
func (s *Service) ReorderCategories(ctx context.Context, userID int64, ids []int64) error {
	return s.reorder(ctx, "categories", userID, ids)
}

// ---------- Места ----------

const locationSelect = `SELECT l.id, l.name, l.sort, l.archived,
	(SELECT COUNT(*) FROM units u WHERE u.user_id = l.user_id AND u.location_id = l.id AND u.status IN ('sealed', 'open'))
	FROM locations l `

func scanLocation(sc interface{ Scan(...any) error }) (Location, error) {
	var l Location
	err := sc.Scan(&l.ID, &l.Name, &l.Sort, &l.Archived, &l.UnitsCount)
	return l, err
}

// Locations — места по sort, затем по имени; withArchived — вместе с архивными.
func (s *Service) Locations(ctx context.Context, userID int64, withArchived bool) ([]Location, error) {
	rows, err := s.q.QueryContext(ctx, locationSelect+
		`WHERE l.user_id = ? AND (? OR l.archived = 0) ORDER BY l.sort, l.name, l.id`, userID, withArchived)
	if err != nil {
		return nil, fmt.Errorf("места: %w", err)
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("места: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Service) location(ctx context.Context, userID, id int64) (Location, error) {
	l, err := scanLocation(s.q.QueryRowContext(ctx, locationSelect+`WHERE l.user_id = ? AND l.id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Location{}, notFound(msgLocationNotFound)
	}
	return l, err
}

// CreateLocation добавляет место в конец списка.
func (s *Service) CreateLocation(ctx context.Context, userID int64, name string) (Location, error) {
	name, err := cleanName("name", name, refNameMax, "Название")
	if err != nil {
		return Location{}, err
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO locations (user_id, name, sort)
		VALUES (?, ?, (SELECT COALESCE(MAX(sort), -1) + 1 FROM locations WHERE user_id = ?))`,
		userID, name, userID)
	if err != nil {
		return Location{}, nameTaken(err)
	}
	id, _ := res.LastInsertId()
	return s.location(ctx, userID, id)
}

// UpdateLocation меняет имя и архив. Место с экземплярами sealed/open
// архивировать нельзя (ФТ-2.2).
func (s *Service) UpdateLocation(ctx context.Context, userID, id int64, p RefPatch) (Location, error) {
	cur, err := s.location(ctx, userID, id)
	if err != nil {
		return Location{}, err
	}
	if p.Name != nil {
		if cur.Name, err = cleanName("name", *p.Name, refNameMax, "Название"); err != nil {
			return Location{}, err
		}
	}
	sortExpr := "sort"
	if p.Archived != nil {
		if *p.Archived && !cur.Archived && cur.UnitsCount > 0 {
			return Location{}, conflict("", fmt.Sprintf("В месте лежат экземпляры: %d. Сначала переложите их", cur.UnitsCount))
		}
		if cur.Archived && !*p.Archived {
			sortExpr = "(SELECT COALESCE(MAX(sort), -1) + 1 FROM locations WHERE user_id = ? AND archived = 0)"
		}
		cur.Archived = *p.Archived
	}
	args := []any{cur.Name, cur.Archived}
	if sortExpr != "sort" {
		args = append(args, userID)
	}
	args = append(args, userID, id)
	if _, err := s.q.ExecContext(ctx, `UPDATE locations SET name = ?, archived = ?, sort = `+
		sortExpr+` WHERE user_id = ? AND id = ?`, args...); err != nil {
		return Location{}, nameTaken(err)
	}
	return s.location(ctx, userID, id)
}

// ReorderLocations — новый порядок действующих мест.
func (s *Service) ReorderLocations(ctx context.Context, userID int64, ids []int64) error {
	return s.reorder(ctx, "locations", userID, ids)
}

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
	res, err := s.q.ExecContext(ctx, `INSERT INTO stores (user_id, name, url) VALUES (?, ?, ?)`, userID, name, u)
	if err != nil {
		return Store{}, nameTaken(err)
	}
	id, _ := res.LastInsertId()
	return s.store(ctx, userID, id)
}

// UpdateStore меняет имя, ссылку и архив.
func (s *Service) UpdateStore(ctx context.Context, userID, id int64, p RefPatch) (Store, error) {
	cur, err := s.store(ctx, userID, id)
	if err != nil {
		return Store{}, err
	}
	if p.Name != nil {
		if cur.Name, err = cleanName("name", *p.Name, refNameMax, "Название"); err != nil {
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
