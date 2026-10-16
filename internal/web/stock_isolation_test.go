package web

import (
	"net/http"
	"strings"
	"testing"
)

// ТБ-5: второй пользователь получает 404 на все адреса склада с чужими id —
// в адресе и в теле запроса — и не видит чужих записей в списках.
func TestStockIsolation(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	b := stkSignIn(e, "friend")

	catA := stkCategory(a, "Тест")
	prodA := stkProduct(a, catA, "Gsf", "ml", 500)
	locA := stkLocation(a, "Коробка")
	storeA := stkCreate(a, "/api/stores", map[string]any{"name": "Автохим", "url": ""})
	rec := a.do(http.MethodPost, "/api/purchases", map[string]any{"date": "2026-10-02", "store_id": storeA,
		"lines": []any{map[string]any{"product_id": prodA, "packages": 2, "price_per_package": 1000}}})
	expect(t, rec, http.StatusCreated, "покупка A")
	purchaseA := decode[stkPurchaseBody](t, rec).Purchase
	unitA := purchaseA.Lines[0].UnitIDs[0]
	rec = a.do(http.MethodPost, "/api/consume", map[string]any{"product_id": prodA, "qty": 10})
	expect(t, rec, http.StatusCreated, "расход A")
	moveA := decode[stkConsumeBody](t, rec).Movements[0].ID
	recipeA := stkCreate(a, "/api/recipes", map[string]any{"name": "Мойка",
		"items": []any{map[string]any{"product_id": prodA, "qty": 5}}})

	catB := stkCategory(b, "Тест")
	prodB := stkProduct(b, catB, "Своя", "ml", 100)
	locB := stkLocation(b, "Полка")
	unitB := stkBuy(b, prodB, 1, 100, nil)[0]
	recipeB := stkCreate(b, "/api/recipes", map[string]any{"name": "Своя",
		"items": []any{map[string]any{"product_id": prodB, "qty": 1}}})

	item := func(product int64) []any { return []any{map[string]any{"product_id": product, "qty": 1}} }
	line := func(product int64, extra map[string]any) map[string]any {
		l := map[string]any{"product_id": product, "packages": 1, "price_per_package": 1}
		for k, v := range extra {
			l[k] = v
		}
		return l
	}
	cases := []struct {
		name, method, target string
		body                 any
	}{
		{"покупка", http.MethodGet, stkPath("/api/purchases/", purchaseA.ID, ""), nil},
		{"позиция в строке покупки", http.MethodPost, "/api/purchases",
			map[string]any{"date": "2026-10-02", "lines": []any{line(prodA, nil)}}},
		{"место в строке покупки", http.MethodPost, "/api/purchases",
			map[string]any{"date": "2026-10-02", "lines": []any{line(prodB, map[string]any{"location_id": locA})}}},
		{"магазин покупки", http.MethodPost, "/api/purchases",
			map[string]any{"date": "2026-10-02", "store_id": storeA, "lines": []any{line(prodB, nil)}}},
		{"ввод остатка: позиция", http.MethodPost, "/api/units", map[string]any{"product_id": prodA, "remaining_qty": 1}},
		{"ввод остатка: место", http.MethodPost, "/api/units",
			map[string]any{"product_id": prodB, "remaining_qty": 1, "location_id": locA}},
		{"перемещение чужого", http.MethodPost, stkPath("/api/units/", unitA, "/move"), map[string]any{"location_id": locB}},
		{"перемещение в чужое место", http.MethodPost, stkPath("/api/units/", unitB, "/move"), map[string]any{"location_id": locA}},
		{"корректировка", http.MethodPost, stkPath("/api/units/", unitA, "/adjust"), map[string]any{"remaining_qty": 1}},
		{"списание", http.MethodPost, stkPath("/api/units/", unitA, "/writeoff"), map[string]any{"reason": "Испорчено"}},
		{"расход чужой позиции", http.MethodPost, "/api/consume", map[string]any{"product_id": prodA, "qty": 1}},
		{"расход из чужого экземпляра", http.MethodPost, "/api/consume",
			map[string]any{"product_id": prodB, "qty": 1, "unit_id": unitA}},
		{"отмена чужого движения", http.MethodPost, "/api/movements/revert", map[string]any{"ids": []int64{moveA}}},
		{"журнал чужой позиции", http.MethodGet, stkPath("/api/movements?product_id=", prodA, ""), nil},
		{"шаблон", http.MethodGet, stkPath("/api/recipes/", recipeA, ""), nil},
		{"замена шаблона", http.MethodPut, stkPath("/api/recipes/", recipeA, ""), map[string]any{"name": "X", "items": item(prodB)}},
		{"удаление шаблона", http.MethodDelete, stkPath("/api/recipes/", recipeA, ""), nil},
		{"применение шаблона", http.MethodPost, stkPath("/api/recipes/", recipeA, "/apply"), map[string]any{"items": item(prodB)}},
		{"позиция в шаблоне", http.MethodPost, "/api/recipes", map[string]any{"name": "X", "items": item(prodA)}},
		{"позиция при замене своего", http.MethodPut, stkPath("/api/recipes/", recipeB, ""), map[string]any{"name": "X", "items": item(prodA)}},
		{"позиция при применении своего", http.MethodPost, stkPath("/api/recipes/", recipeB, "/apply"), map[string]any{"items": item(prodA)}},
	}
	for _, c := range cases {
		rec := b.do(c.method, c.target, c.body)
		expect(t, rec, http.StatusNotFound, c.name)
		if msg := decode[errBody](t, rec).Error; !strings.Contains(msg, "не найден") {
			t.Fatalf("%s: текст %q", c.name, msg)
		}
	}
	expect(t, b.do(http.MethodPost, "/api/recipes/reorder", map[string]any{"ids": []int64{recipeA}}),
		http.StatusBadRequest, "порядок с чужим шаблоном")

	// В списках — только свои записи.
	for target, want := range map[string]string{
		"/api/purchases": `"purchases":[{`, "/api/movements": `"movements":[{`, "/api/recipes": `"recipes":[{`,
	} {
		rec := b.do(http.MethodGet, target, nil)
		expect(t, rec, http.StatusOK, target)
		body := rec.Body.String()
		if !strings.Contains(body, want) || strings.Contains(body, "Gsf") || strings.Contains(body, "Мойка") ||
			strings.Contains(body, "Автохим") {
			t.Fatalf("%s: %s", target, body)
		}
	}
	rec = b.do(http.MethodGet, "/api/purchases", nil)
	if n := strings.Count(rec.Body.String(), `"id":`); n != 1 {
		t.Fatalf("покупок у второго: %d", n)
	}

	// Данные первого не изменились.
	rec = a.do(http.MethodGet, stkPath("/api/movements?product_id=", prodA, ""), nil)
	if n := strings.Count(rec.Body.String(), `"type":`); n != 3 {
		t.Fatalf("движений у первого: %d; %s", n, rec.Body.String())
	}
	expect(t, a.do(http.MethodGet, stkPath("/api/recipes/", recipeA, ""), nil), http.StatusOK, "шаблон первого")

	// Без входа — 401.
	anon := e.client()
	for _, target := range []string{"/api/purchases", "/api/movements", "/api/recipes"} {
		expect(t, anon.do(http.MethodGet, target, nil), http.StatusUnauthorized, target+" без входа")
	}
}
