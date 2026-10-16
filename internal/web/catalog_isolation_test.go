package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// ТБ-5: второй пользователь не видит и не меняет данные первого ни через
// один адрес каталога.
func TestCatIsolationHTTP(t *testing.T) {
	e := catNewEnv(t)
	first := e.catLogin("ringino")
	second := e.catLogin("second")

	cat := catFirstCategory(t, first)
	loc := decode[catalog.Location](t, first.do(http.MethodPost, "/api/locations", map[string]any{"name": "Коробка"}))
	shop := decode[catalog.Store](t, first.do(http.MethodPost, "/api/stores", map[string]any{"name": "Автохим"}))
	p := catProduct(t, first, map[string]any{"category_id": cat, "name": "Секретный шампунь", "brand": "Тайна",
		"unit": "ml", "package_size": 1000, "min_stock": 100})
	e.catUnit(p.ID, loc.ID, 1000, 50, "open", 1)
	item := decode[catalog.ShoppingItem](t, first.do(http.MethodPost, "/api/shopping", map[string]any{"product_id": p.ID}))

	// Чужие id — 404, как несуществующие.
	notFound := []struct {
		method, target string
		body           any
	}{
		{http.MethodPatch, fmt.Sprintf("/api/categories/%d", cat), map[string]any{"name": "Взлом"}},
		{http.MethodPatch, fmt.Sprintf("/api/locations/%d", loc.ID), map[string]any{"name": "Взлом"}},
		{http.MethodPatch, fmt.Sprintf("/api/stores/%d", shop.ID), map[string]any{"name": "Взлом"}},
		{http.MethodGet, fmt.Sprintf("/api/products/%d", p.ID), nil},
		{http.MethodPatch, fmt.Sprintf("/api/products/%d", p.ID), map[string]any{"name": "Взлом"}},
		{http.MethodPatch, fmt.Sprintf("/api/shopping/%d", item.ID), map[string]any{"done": true}},
		{http.MethodDelete, fmt.Sprintf("/api/shopping/%d", item.ID), nil},
		// Ссылки на чужое в теле.
		{http.MethodPost, "/api/products", map[string]any{"category_id": cat, "name": "X", "unit": "ml", "package_size": 1}},
		{http.MethodPost, "/api/shopping", map[string]any{"product_id": p.ID}},
	}
	for _, tc := range notFound {
		expect(t, second.do(tc.method, tc.target, tc.body), http.StatusNotFound, "чужое: "+tc.method+" "+tc.target)
	}

	// Перенос своей позиции в чужую категорию — тоже 404.
	own := catProduct(t, second, map[string]any{"category_id": catFirstCategory(t, second), "name": "Свой",
		"unit": "ml", "package_size": 1})
	expect(t, second.do(http.MethodPatch, fmt.Sprintf("/api/products/%d", own.ID), map[string]any{"category_id": cat}),
		http.StatusNotFound, "чужая категория в PATCH")

	// Порядок с чужими id — 400 (не все свои действующие).
	theirs := decode[catCategories](t, first.do(http.MethodGet, "/api/categories", nil)).Categories
	ids := []int64{}
	for _, c := range theirs {
		ids = append(ids, c.ID)
	}
	expect(t, second.do(http.MethodPost, "/api/categories/reorder", map[string]any{"ids": ids}), http.StatusBadRequest, "чужой порядок")
	expect(t, second.do(http.MethodPost, "/api/locations/reorder", map[string]any{"ids": []int64{loc.ID}}), http.StatusBadRequest, "чужие места")

	// В списках второго нет ничего от первого.
	lists := []string{"/api/categories?archived=1", "/api/locations?archived=1", "/api/stores?archived=1",
		"/api/products", "/api/products?archived=1", "/api/products?q=" + "секрет", "/api/brands", "/api/shopping",
		fmt.Sprintf("/api/products?category_id=%d", cat), fmt.Sprintf("/api/products?location_id=%d", loc.ID)}
	for _, target := range lists {
		rec := second.do(http.MethodGet, target, nil)
		expect(t, rec, http.StatusOK, target)
		body := rec.Body.String()
		for _, leak := range []string{"Секретный", "Тайна", "Коробка", "Автохим", fmt.Sprintf(`"id":%d,`, p.ID)} {
			if strings.Contains(body, leak) {
				t.Fatalf("%s: второй видит %q: %s", target, leak, body)
			}
		}
	}
	if got := decode[catProducts](t, second.do(http.MethodGet, fmt.Sprintf("/api/products?category_id=%d", cat), nil)); len(got.Products) != 0 {
		t.Fatalf("фильтр по чужой категории: %+v", got)
	}

	// Данные первого не пострадали.
	d := decode[catDetail](t, first.do(http.MethodGet, fmt.Sprintf("/api/products/%d", p.ID), nil))
	if d.Product.Name != "Секретный шампунь" || d.Product.Stock.Total != 50 {
		t.Fatalf("данные первого изменились: %+v", d.Product)
	}
	list := decode[catalog.ShoppingList](t, first.do(http.MethodGet, "/api/shopping", nil))
	if len(list.Manual) != 1 || list.Manual[0].DoneAt != nil || len(list.Low) != 1 {
		t.Fatalf("список первого: %+v", list)
	}

	// Без сессии — 401.
	anon := e.client()
	for _, target := range []string{"/api/categories", "/api/locations", "/api/stores", "/api/products", "/api/brands", "/api/shopping"} {
		expect(t, anon.do(http.MethodGet, target, nil), http.StatusUnauthorized, "без входа "+target)
	}
}
