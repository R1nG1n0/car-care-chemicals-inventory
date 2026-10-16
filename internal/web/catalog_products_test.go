package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

type catDetail struct {
	Product   catalog.ProductSummary `json:"product"`
	Units     []catalog.Unit         `json:"units"`
	Purchases []catalog.PurchaseRow  `json:"purchases"`
	Movements []catalog.Movement     `json:"movements"`
}

func catNames(t *testing.T, c *client, query string) []string {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/products"+query, nil)
	expect(t, rec, http.StatusOK, "список позиций "+query)
	out := []string{}
	for _, p := range decode[catProducts](t, rec).Products {
		out = append(out, p.Name)
	}
	return out
}

func catEqual(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s: %v, ожидалось %v", what, got, want)
	}
}

func TestCatProductValidationHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	valid := func() map[string]any {
		return map[string]any{"category_id": cat, "name": "Gsf", "brand": "Koch", "unit": "ml",
			"package_size": 1000, "barcode": "4601", "min_stock": 500, "note": "1:10"}
	}
	cases := []struct {
		field string
		value any
	}{
		{"name", ""}, {"name", strings.Repeat("ш", 81)}, {"brand", strings.Repeat("б", 61)},
		{"unit", "l"}, {"package_size", 0}, {"package_size", 100001}, {"barcode", "12a"},
		{"barcode", strings.Repeat("1", 33)}, {"min_stock", -1}, {"min_stock", 1000001}, {"min_stock", "много"},
		{"note", strings.Repeat("з", 501)}, {"category_id", nil},
	}
	for _, tc := range cases {
		body := valid()
		if tc.value == nil {
			delete(body, tc.field)
		} else {
			body[tc.field] = tc.value
		}
		catExpectField(t, c, http.MethodPost, "/api/products", body, http.StatusBadRequest, tc.field)
	}

	p := catProduct(t, c, valid())
	if p.Name != "Gsf" || p.MinStock == nil || *p.MinStock != 500 || !p.Low || p.LocationIDs == nil || p.LastConsumeQty != nil {
		t.Fatalf("новая позиция: %+v", p)
	}
	body := valid()
	body["min_stock"] = nil
	if got := catProduct(t, c, body); got.MinStock != nil || got.Low {
		t.Fatalf("min_stock null: %+v", got)
	}

	// Архивная категория — 400, несуществующая — 404.
	expect(t, c.do(http.MethodPatch, fmt.Sprintf("/api/categories/%d", cat), map[string]any{"archived": true}), 200, "архив категории")
	catExpectField(t, c, http.MethodPost, "/api/products", valid(), http.StatusBadRequest, "category_id")
	body = valid()
	body["category_id"] = 99999
	expect(t, c.do(http.MethodPost, "/api/products", body), http.StatusNotFound, "несуществующая категория")
	expect(t, c.do(http.MethodGet, "/api/products?filter=bad", nil), http.StatusBadRequest, "неверный фильтр")
	expect(t, c.do(http.MethodGet, "/api/products?category_id=x", nil), http.StatusBadRequest, "неверная категория")

	// PATCH: позиция в архивной категории правится без смены категории.
	target := fmt.Sprintf("/api/products/%d", p.ID)
	rec := c.do(http.MethodPatch, target, map[string]any{"min_stock": nil, "name": "Gsf 2"})
	expect(t, rec, http.StatusOK, "правка позиции")
	if got := decode[catalog.ProductSummary](t, rec); got.MinStock != nil || got.Name != "Gsf 2" || got.Brand != "Koch" {
		t.Fatalf("правка: %+v", got)
	}
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"package_size": -5}, 400, "package_size")
	rec = c.do(http.MethodPatch, target, map[string]any{"unit": "pcs"})
	expect(t, rec, http.StatusOK, "смена единицы без экземпляров")
	e.catUnit(p.ID, nil, 10, 10, "finished", nil)
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"unit": "ml"}, http.StatusConflict, "unit")
	expect(t, c.do(http.MethodPatch, "/api/products/99999", map[string]any{}), http.StatusNotFound, "нет позиции")
}

func TestCatProductListHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	loc := decode[catalog.Location](t, c.do(http.MethodPost, "/api/locations", map[string]any{"name": "Коробка"}))
	mk := func(name, brand, unit string, size int, min any) catalog.ProductSummary {
		return catProduct(t, c, map[string]any{"category_id": cat, "name": name, "brand": brand, "unit": unit,
			"package_size": size, "min_stock": min})
	}
	shampoo := mk("Шампунь Активный", "Grass", "ml", 1000, 500)
	foam := mk("пена", "Кох Хеми", "ml", 1000, nil)
	cloth := mk("Микрофибра", "", "pcs", 10, 2)
	e.catExec(`UPDATE products SET barcode = '4601234' WHERE id = ?`, cloth.ID)
	e.catUnit(shampoo.ID, loc.ID, 1000, 300, "open", 1_792_000_000)
	e.catUnit(foam.ID, nil, 1000, 1000, "sealed", nil)

	catEqual(t, "по имени без регистра", catNames(t, c, ""), "Микрофибра", "пена", "Шампунь Активный")
	catEqual(t, "кириллица", catNames(t, c, "?q="+url.QueryEscape("ШАМПУНЬ")), "Шампунь Активный")
	catEqual(t, "бренд", catNames(t, c, "?q="+url.QueryEscape("хеми")), "пена")
	catEqual(t, "штрихкод", catNames(t, c, "?q=4601"), "Микрофибра")
	catEqual(t, "low", catNames(t, c, "?filter=low"), "Микрофибра", "Шампунь Активный")
	catEqual(t, "out", catNames(t, c, "?filter=out"), "Микрофибра")
	catEqual(t, "in", catNames(t, c, "?filter=in"), "пена", "Шампунь Активный")
	catEqual(t, "место", catNames(t, c, fmt.Sprintf("?location_id=%d", loc.ID)), "Шампунь Активный")
	catEqual(t, "категория", catNames(t, c, fmt.Sprintf("?category_id=%d", cat)), "Микрофибра", "пена", "Шампунь Активный")
	expect(t, c.do(http.MethodPatch, fmt.Sprintf("/api/products/%d", foam.ID), map[string]any{"archived": true}), 200, "архив")
	catEqual(t, "архивные", catNames(t, c, "?archived=1"), "пена")
	catEqual(t, "действующие", catNames(t, c, ""), "Микрофибра", "Шампунь Активный")

	rec := c.do(http.MethodGet, "/api/brands", nil)
	expect(t, rec, http.StatusOK, "бренды")
	if got := decode[struct{ Brands []string }](t, rec).Brands; strings.Join(got, "|") != "Grass|Кох Хеми" {
		t.Fatalf("бренды: %v", got)
	}
}

func TestCatProductDetailHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	loc := decode[catalog.Location](t, c.do(http.MethodPost, "/api/locations", map[string]any{"name": "Полка"}))
	shop := decode[catalog.Store](t, c.do(http.MethodPost, "/api/stores", map[string]any{"name": "Автохим"}))
	p := catProduct(t, c, map[string]any{"category_id": cat, "name": "Gsf", "unit": "ml", "package_size": 1000, "min_stock": 1500})

	// Две покупки по экземпляру; один вскрыт, расход двумя движениями одного нажатия.
	e.catPurchase(p.ID, shop.ID, "2026-10-01", 119000, 1_792_000_000)
	e.catPurchase(p.ID, nil, "2026-10-05", 125000, 1_792_100_000)
	var units []int64
	rows, _ := e.db.Query(`SELECT id FROM units WHERE product_id = ? ORDER BY id`, p.ID)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		units = append(units, id)
	}
	rows.Close()
	e.catExec(`UPDATE units SET status = 'open', remaining_qty = 320, opened_at = 1792100000, location_id = ? WHERE id = ?`, loc.ID, units[1])
	first := e.catExec(`INSERT INTO movements (user_id, unit_id, type, qty_delta, created_at)
		SELECT user_id, id, 'consume', -40, 1792200000 FROM units WHERE id = ?`, units[0])
	e.catExec(`UPDATE movements SET batch_id = id WHERE id = ?`, first)
	e.catExec(`INSERT INTO movements (user_id, unit_id, type, qty_delta, batch_id, created_at)
		SELECT user_id, id, 'consume', -640, ?, 1792200000 FROM units WHERE id = ?`, first, units[1])

	rec := c.do(http.MethodGet, fmt.Sprintf("/api/products/%d", p.ID), nil)
	expect(t, rec, http.StatusOK, "карточка")
	d := decode[catDetail](t, rec)
	st := d.Product.Stock
	if st.Total != 1320 || *st.OpenQty != 320 || *st.OpenCapacity != 1000 || st.SealedCount != 1 || st.UnitsCount != 2 {
		t.Fatalf("сводка: %+v", st)
	}
	if !d.Product.Low || *d.Product.LastConsumeQty != 680 || len(d.Product.LocationIDs) != 1 || d.Product.LocationIDs[0] != loc.ID {
		t.Fatalf("сводка: %+v", d.Product)
	}
	if len(d.Units) != 2 || d.Units[0].ID != units[1] || d.Units[0].Status != "open" || *d.Units[1].UnitPrice != 119000 {
		t.Fatalf("экземпляры: %+v", d.Units)
	}
	if len(d.Purchases) != 2 || d.Purchases[0].Date != "2026-10-05" || d.Purchases[0].Store != nil ||
		*d.Purchases[1].Store != "Автохим" || *d.Purchases[1].PricePer100 != 11900 {
		t.Fatalf("покупки: %+v", d.Purchases)
	}
	if len(d.Movements) != 2 || d.Movements[0].Cost == nil {
		t.Fatalf("движения: %+v", d.Movements)
	}
	// Для шт — price_per_unit, а price_per_100 в ответе нет.
	pcs := catProduct(t, c, map[string]any{"category_id": cat, "name": "Губка", "unit": "pcs", "package_size": 3})
	e.catPurchase(pcs.ID, nil, "2026-10-01", 10000, 1_792_000_000)
	body := c.do(http.MethodGet, fmt.Sprintf("/api/products/%d", pcs.ID), nil).Body.String()
	if !strings.Contains(body, `"price_per_unit":3333`) || strings.Contains(body, "price_per_100") {
		t.Fatalf("покупки в шт: %s", body)
	}
	expect(t, c.do(http.MethodGet, "/api/products/0", nil), http.StatusNotFound, "id 0")
}
