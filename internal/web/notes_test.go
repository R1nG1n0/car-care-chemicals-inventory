package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Заметки (ФТ-18): API целиком.

type noteList struct {
	Notes []catalog.Note `json:"notes"`
}

func noteIDs(list []catalog.Note) []int64 {
	out := make([]int64, len(list))
	for i, n := range list {
		out[i] = n.ID
	}
	return out
}

func TestNotesCRUDSearchAndLinks(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	prod1 := stkProduct(c, cat, "Шампунь", "ml", 500)
	prod2 := stkProduct(c, cat, "Салфетки", "pcs", 10)
	rec := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка", "items": []any{map[string]any{"product_id": prod1, "qty": 5}}})

	// Создание: тело с переносами сохраняется, связи без повторов.
	r := c.do(http.MethodPost, "/api/notes", map[string]any{"title": "  Пропорции зимой ", "body": "Строка 1\nСтрока 2\nhttps://example.com",
		"product_ids": []int64{prod1, prod1, prod2}, "recipe_ids": []int64{rec}})
	expect(t, r, http.StatusCreated, "создание")
	n1 := decode[catalog.Note](t, r)
	if n1.Title != "Пропорции зимой" || n1.Body != "Строка 1\nСтрока 2\nhttps://example.com" || len(n1.ProductIDs) != 2 ||
		len(n1.RecipeIDs) != 1 || n1.CreatedAt == 0 || n1.UpdatedAt != n1.CreatedAt {
		t.Fatalf("заметка: %+v", n1)
	}
	// Без body и связей: пустые массивы, не null.
	r = c.do(http.MethodPost, "/api/notes", map[string]any{"title": "ЧЁРНАЯ ПЯТНИЦА"})
	expect(t, r, http.StatusCreated, "минимум")
	if body := r.Body.String(); !strings.Contains(body, `"product_ids":[]`) || !strings.Contains(body, `"recipe_ids":[]`) {
		t.Fatalf("JSON: %s", body)
	}
	n2 := decode[catalog.Note](t, r)

	// Список: новые сверху (при равном времени — по id).
	list := decode[noteList](t, c.do(http.MethodGet, "/api/notes", nil)).Notes
	if got := noteIDs(list); len(got) != 2 || got[0] != n2.ID || got[1] != n1.ID {
		t.Fatalf("порядок: %v", got)
	}
	// Поиск без учёта регистра, в том числе кириллицей, по заголовку и тексту.
	for q, want := range map[string]int64{"пропорции": n1.ID, "ПРОПОРЦИИ": n1.ID, "чёрная": n2.ID, "строка 2": n1.ID, "EXAMPLE.com": n1.ID} {
		got := decode[noteList](t, c.do(http.MethodGet, "/api/notes?q="+strings.ReplaceAll(q, " ", "%20"), nil)).Notes
		if len(got) != 1 || got[0].ID != want {
			t.Fatalf("q=%q: %+v", q, got)
		}
	}
	if got := decode[noteList](t, c.do(http.MethodGet, "/api/notes?q=нету", nil)).Notes; len(got) != 0 {
		t.Fatalf("пустой поиск: %+v", got)
	}
	// Фильтры по позиции и шаблону.
	if got := decode[noteList](t, c.do(http.MethodGet, stkPath("/api/notes?product_id=", prod2, ""), nil)).Notes; len(got) != 1 || got[0].ID != n1.ID {
		t.Fatalf("product_id: %+v", got)
	}
	if got := decode[noteList](t, c.do(http.MethodGet, stkPath("/api/notes?recipe_id=", rec, ""), nil)).Notes; len(got) != 1 || got[0].ID != n1.ID {
		t.Fatalf("recipe_id: %+v", got)
	}
	expect(t, c.do(http.MethodGet, "/api/notes?product_id=abc", nil), http.StatusBadRequest, "плохой фильтр")

	// Карточка и правка: списки заменяются целиком, без поля — не меняются.
	got := decode[catalog.Note](t, c.do(http.MethodGet, stkPath("/api/notes/", n1.ID, ""), nil))
	if got.ID != n1.ID {
		t.Fatalf("карточка: %+v", got)
	}
	r = c.do(http.MethodPatch, stkPath("/api/notes/", n1.ID, ""), map[string]any{"body": "Новый текст", "product_ids": []int64{prod2}})
	expect(t, r, http.StatusOK, "правка")
	upd := decode[catalog.Note](t, r)
	if upd.Body != "Новый текст" || upd.Title != n1.Title || len(upd.ProductIDs) != 1 || upd.ProductIDs[0] != prod2 ||
		len(upd.RecipeIDs) != 1 || upd.UpdatedAt < n1.UpdatedAt {
		t.Fatalf("после правки: %+v", upd)
	}
	upd = decode[catalog.Note](t, c.do(http.MethodPatch, stkPath("/api/notes/", n1.ID, ""), map[string]any{"recipe_ids": []int64{}, "body": ""}))
	if len(upd.RecipeIDs) != 0 || upd.Body != "" || len(upd.ProductIDs) != 1 {
		t.Fatalf("очистка: %+v", upd)
	}

	// Каскад: удаление позиции в приложении — архив, поэтому проверяем шаблон; позицию — в тесте миграции/БД.
	expect(t, c.do(http.MethodDelete, stkPath("/api/notes/", n2.ID, ""), nil), http.StatusNoContent, "удаление")
	expect(t, c.do(http.MethodGet, stkPath("/api/notes/", n2.ID, ""), nil), http.StatusNotFound, "после удаления")
	expect(t, c.do(http.MethodDelete, stkPath("/api/notes/", n2.ID, ""), nil), http.StatusNotFound, "повторное удаление")
}

func TestNotesValidation(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	prod := stkProduct(c, cat, "Шампунь", "ml", 500)

	many := make([]int64, 51)
	for i := range many {
		many[i] = int64(i + 1000)
	}
	cases := []struct {
		name  string
		body  map[string]any
		code  int
		field string
	}{
		{"без заголовка", map[string]any{"title": "  "}, 400, "title"},
		{"заголовок 121", map[string]any{"title": strings.Repeat("я", 121)}, 400, "title"},
		{"текст 10001", map[string]any{"title": "а", "body": strings.Repeat("я", 10001)}, 400, "body"},
		{"51 позиция", map[string]any{"title": "а", "product_ids": many}, 400, "product_ids"},
		{"51 шаблон", map[string]any{"title": "а", "recipe_ids": many}, 400, "recipe_ids"},
		{"нет позиции", map[string]any{"title": "а", "product_ids": []int64{prod + 100}}, 404, ""},
		{"нет шаблона", map[string]any{"title": "а", "recipe_ids": []int64{12345}}, 404, ""},
	}
	for _, tc := range cases {
		rec := c.do(http.MethodPost, "/api/notes", tc.body)
		expect(t, rec, tc.code, tc.name)
		if f := decode[errBody](t, rec).Field; f != tc.field {
			t.Fatalf("%s: поле %q", tc.name, f)
		}
	}
	// Границы допустимы: 120 символов заголовка, 10000 текста.
	expect(t, c.do(http.MethodPost, "/api/notes", map[string]any{"title": strings.Repeat("я", 120), "body": strings.Repeat("я", 10000)}),
		http.StatusCreated, "границы")
	// Ничего не создалось от отказов.
	if got := decode[noteList](t, c.do(http.MethodGet, "/api/notes", nil)).Notes; len(got) != 1 {
		t.Fatalf("заметок %d", len(got))
	}
	// Правка: те же проверки; неудача ничего не меняет.
	id := decode[catalog.Note](t, c.do(http.MethodPost, "/api/notes", map[string]any{"title": "Ок", "product_ids": []int64{prod}})).ID
	rec := c.do(http.MethodPatch, stkPath("/api/notes/", id, ""), map[string]any{"title": "", "product_ids": []int64{}})
	expect(t, rec, http.StatusBadRequest, "пустой заголовок")
	expect(t, c.do(http.MethodPatch, stkPath("/api/notes/", id, ""), map[string]any{"product_ids": []int64{prod + 100}}), http.StatusNotFound, "чужая позиция")
	if n := decode[catalog.Note](t, c.do(http.MethodGet, stkPath("/api/notes/", id, ""), nil)); n.Title != "Ок" || len(n.ProductIDs) != 1 {
		t.Fatalf("изменилось после отказа: %+v", n)
	}
}

// Удаление шаблона и позиции снимает связи, заметка остаётся (ФТ-18.1).
func TestNotesCascadeOnDelete(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	prod := stkProduct(c, cat, "Шампунь", "ml", 500)
	rec := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка", "items": []any{map[string]any{"product_id": prod, "qty": 5}}})
	id := decode[catalog.Note](t, c.do(http.MethodPost, "/api/notes", map[string]any{"title": "Связанная",
		"product_ids": []int64{prod}, "recipe_ids": []int64{rec}})).ID

	expect(t, c.do(http.MethodDelete, stkPath("/api/recipes/", rec, ""), nil), http.StatusNoContent, "удаление шаблона")
	n := decode[catalog.Note](t, c.do(http.MethodGet, stkPath("/api/notes/", id, ""), nil))
	if len(n.RecipeIDs) != 0 || len(n.ProductIDs) != 1 {
		t.Fatalf("после удаления шаблона: %+v", n)
	}
	if _, err := e.db.Exec(`DELETE FROM products WHERE id = ?`, prod); err != nil {
		t.Fatal(err)
	}
	n = decode[catalog.Note](t, c.do(http.MethodGet, stkPath("/api/notes/", id, ""), nil))
	if len(n.ProductIDs) != 0 || n.Title != "Связанная" {
		t.Fatalf("после удаления позиции: %+v", n)
	}
}
