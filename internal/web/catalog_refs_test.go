package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

func TestCatCategoriesHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")

	rec := c.do(http.MethodGet, "/api/categories", nil)
	expect(t, rec, http.StatusOK, "список категорий")
	list := decode[catCategories](t, rec).Categories
	if len(list) != 11 || list[0].Name != "Шампуни" || list[1].Color != "berry" || list[0].ProductsCount != 0 {
		t.Fatalf("категории по умолчанию: %+v", list)
	}

	rec = c.do(http.MethodPost, "/api/categories", map[string]any{"name": "Глины", "color": "plum"})
	expect(t, rec, http.StatusCreated, "новая категория")
	glina := decode[catalog.Category](t, rec)
	if glina.Sort <= list[len(list)-1].Sort || glina.Archived {
		t.Fatalf("новая категория не в конце: %+v", glina)
	}
	catExpectField(t, c, http.MethodPost, "/api/categories", map[string]any{"name": "", "color": "plum"}, 400, "name")
	catExpectField(t, c, http.MethodPost, "/api/categories", map[string]any{"name": strings.Repeat("я", 41), "color": "plum"}, 400, "name")
	catExpectField(t, c, http.MethodPost, "/api/categories", map[string]any{"name": "Ок", "color": "red"}, 400, "color")
	catExpectField(t, c, http.MethodPost, "/api/categories", map[string]any{"name": "Шампуни", "color": "blue"}, 409, "name")
	expect(t, c.do(http.MethodPost, "/api/categories", "{"), http.StatusBadRequest, "битый JSON")

	// Переименование в существующее — 409; цвет и архив меняются.
	target := fmt.Sprintf("/api/categories/%d", glina.ID)
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"name": "Шампуни"}, 409, "name")
	rec = c.do(http.MethodPatch, target, map[string]any{"color": "olive", "archived": true})
	expect(t, rec, http.StatusOK, "правка категории")
	if got := decode[catalog.Category](t, rec); got.Color != "olive" || !got.Archived || got.Name != "Глины" {
		t.Fatalf("правка: %+v", got)
	}
	if n := len(decode[catCategories](t, c.do(http.MethodGet, "/api/categories", nil)).Categories); n != 11 {
		t.Fatalf("архивная в списке действующих: %d", n)
	}
	if n := len(decode[catCategories](t, c.do(http.MethodGet, "/api/categories?archived=1", nil)).Categories); n != 12 {
		t.Fatalf("archived=1 — все: %d", n)
	}
	expect(t, c.do(http.MethodPatch, "/api/categories/abc", map[string]any{}), http.StatusNotFound, "id не число")

	// Порядок: ровно все действующие, иначе 400.
	ids := []int64{}
	for i := len(list) - 1; i >= 0; i-- {
		ids = append(ids, list[i].ID)
	}
	catExpectField(t, c, http.MethodPost, "/api/categories/reorder", map[string]any{"ids": ids[1:]}, 400, "ids")
	catExpectField(t, c, http.MethodPost, "/api/categories/reorder", map[string]any{"ids": append(ids, glina.ID)}, 400, "ids")
	expect(t, c.do(http.MethodPost, "/api/categories/reorder", map[string]any{"ids": ids}), http.StatusNoContent, "порядок")
	if got := decode[catCategories](t, c.do(http.MethodGet, "/api/categories", nil)).Categories; got[0].Name != "Прочее" {
		t.Fatalf("порядок не применился: %+v", got[0])
	}

	// products_count — только неархивные позиции.
	catProduct(t, c, map[string]any{"category_id": list[0].ID, "name": "A", "unit": "ml", "package_size": 500})
	p := catProduct(t, c, map[string]any{"category_id": list[0].ID, "name": "B", "unit": "ml", "package_size": 500})
	expect(t, c.do(http.MethodPatch, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"archived": true}), 200, "архив позиции")
	for _, cat := range decode[catCategories](t, c.do(http.MethodGet, "/api/categories", nil)).Categories {
		if cat.ID == list[0].ID && cat.ProductsCount != 1 {
			t.Fatalf("products_count: %+v", cat)
		}
	}
}

func TestCatLocationsHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")

	if got := decode[catLocations](t, c.do(http.MethodGet, "/api/locations", nil)); len(got.Locations) != 0 {
		t.Fatalf("места нового пользователя: %+v", got)
	}
	var locs []catalog.Location
	for _, name := range []string{"Коробка 3", "Полка 2"} {
		rec := c.do(http.MethodPost, "/api/locations", map[string]any{"name": name})
		expect(t, rec, http.StatusCreated, "место "+name)
		locs = append(locs, decode[catalog.Location](t, rec))
	}
	catExpectField(t, c, http.MethodPost, "/api/locations", map[string]any{"name": "Коробка 3"}, 409, "name")
	catExpectField(t, c, http.MethodPost, "/api/locations", map[string]any{}, 400, "name")
	expect(t, c.do(http.MethodPost, "/api/locations/reorder", map[string]any{"ids": []int64{locs[1].ID, locs[0].ID}}),
		http.StatusNoContent, "порядок мест")
	if got := decode[catLocations](t, c.do(http.MethodGet, "/api/locations", nil)).Locations; got[0].ID != locs[1].ID {
		t.Fatalf("порядок мест: %+v", got)
	}

	p := catProduct(t, c, map[string]any{"category_id": catFirstCategory(t, c), "name": "Gsf", "unit": "ml", "package_size": 1000})
	for i := 0; i < 5; i++ {
		e.catUnit(p.ID, locs[0].ID, 1000, 1000, "sealed", nil)
	}
	e.catUnit(p.ID, locs[1].ID, 1000, 0, "finished", nil)

	target := fmt.Sprintf("/api/locations/%d", locs[0].ID)
	rec := c.do(http.MethodPatch, target, map[string]any{"archived": true})
	expect(t, rec, http.StatusConflict, "архив места с экземплярами")
	if got := decode[errBody](t, rec); got.Error != "В месте лежат экземпляры: 5. Сначала переложите их" {
		t.Fatalf("текст: %q", got.Error)
	}
	// Переименование места с экземплярами — можно; units_count считает sealed/open.
	rec = c.do(http.MethodPatch, target, map[string]any{"name": "Коробка 4"})
	expect(t, rec, http.StatusOK, "переименование")
	if got := decode[catalog.Location](t, rec); got.Name != "Коробка 4" || got.UnitsCount != 5 {
		t.Fatalf("переименование: %+v", got)
	}
	// Место только с законченными — архивируется и возвращается в конец.
	rec = c.do(http.MethodPatch, fmt.Sprintf("/api/locations/%d", locs[1].ID), map[string]any{"archived": true})
	expect(t, rec, http.StatusOK, "архив пустого места")
	if got := decode[catLocations](t, c.do(http.MethodGet, "/api/locations?archived=1", nil)); len(got.Locations) != 2 {
		t.Fatalf("archived=1: %+v", got)
	}
	expect(t, c.do(http.MethodPatch, fmt.Sprintf("/api/locations/%d", locs[1].ID), map[string]any{"archived": false}),
		http.StatusOK, "возврат из архива")
}

func TestCatStoresHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")

	for _, name := range []string{"Озон", "Автохим"} {
		expect(t, c.do(http.MethodPost, "/api/stores", map[string]any{"name": name, "url": ""}), http.StatusCreated, name)
	}
	rec := c.do(http.MethodPost, "/api/stores", map[string]any{"name": "Детейлинг", "url": "https://shop.example/x"})
	expect(t, rec, http.StatusCreated, "магазин со ссылкой")
	shop := decode[catalog.Store](t, rec)
	catExpectField(t, c, http.MethodPost, "/api/stores", map[string]any{"name": "X", "url": "javascript:alert(1)"}, 400, "url")
	catExpectField(t, c, http.MethodPost, "/api/stores", map[string]any{"name": "Озон"}, 409, "name")

	list := decode[catStores](t, c.do(http.MethodGet, "/api/stores", nil)).Stores
	if len(list) != 3 || list[0].Name != "Автохим" || list[2].Name != "Озон" {
		t.Fatalf("магазины по имени: %+v", list)
	}
	rec = c.do(http.MethodPatch, fmt.Sprintf("/api/stores/%d", shop.ID), map[string]any{"url": "", "archived": true})
	expect(t, rec, http.StatusOK, "правка магазина")
	if got := decode[catalog.Store](t, rec); got.URL != "" || !got.Archived {
		t.Fatalf("правка магазина: %+v", got)
	}
	if n := len(decode[catStores](t, c.do(http.MethodGet, "/api/stores", nil)).Stores); n != 2 {
		t.Fatalf("архивный магазин в списке: %d", n)
	}
	expect(t, c.do(http.MethodPost, "/api/stores/reorder", map[string]any{"ids": []int64{}}), http.StatusMethodNotAllowed, "reorder у магазинов нет")
}
