package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// accSnapshot — тела всех GET-адресов этапа 2 у пользователя.
func accSnapshot(t *testing.T, c *client, targets []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, target := range targets {
		rec := c.do(http.MethodGet, target, nil)
		expect(t, rec, http.StatusOK, "снимок "+target)
		out[target] = rec.Body.String()
	}
	return out
}

// ТБ-5: второй пользователь проходит все адреса раздела «Этап 2» с чужими id
// в адресе и в теле — везде 404 (reorder — 400), данные владельца не меняются,
// в списках второго — только своё.
func TestAccIsolationAllStage2(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	b := stkSignIn(e, "friend")

	// Владелец: по объекту каждого вида, имена уникальны для поиска утечек.
	catA := stkCategory(a, "Тайная категория")
	locA := stkLocation(a, "Тайник")
	storeA := stkCreate(a, "/api/stores", map[string]any{"name": "Тайный магазин", "url": ""})
	prodA := stkCreate(a, "/api/products", map[string]any{"category_id": catA, "name": "Секретный шампунь",
		"brand": "Тайнобренд", "unit": "ml", "package_size": 500, "min_stock": 2000})
	line := accLine(prodA, 2, 119000)
	line["location_id"] = locA
	purchaseA := accBuy(t, a, map[string]any{"date": "2026-10-02", "store_id": storeA, "note": "Тайная заметка",
		"lines": []any{line}}).Purchase
	unitA := purchaseA.Lines[0].UnitIDs[0]
	moveA := accConsume(t, a, prodA, 10).Movements[0].ID
	itemA := stkCreate(a, "/api/shopping", map[string]any{"product_id": prodA})
	recipeA := stkCreate(a, "/api/recipes", map[string]any{"name": "Тайный рецепт",
		"items": []any{map[string]any{"product_id": prodA, "qty": 5}}})

	// Второй: свои объекты для ссылок «своё + чужое».
	catB := stkCategory(b, "Своя категория")
	locB := stkLocation(b, "Своя полка")
	prodB := stkProduct(b, catB, "Свой шампунь", "ml", 100)
	unitB := accBuy(t, b, map[string]any{"date": "2026-10-03", "lines": []any{accLine(prodB, 1, 100)}}).Purchase.Lines[0].UnitIDs[0]
	recipeB := stkCreate(b, "/api/recipes", map[string]any{"name": "Свой рецепт",
		"items": []any{map[string]any{"product_id": prodB, "qty": 1}}})

	ownerGets := []string{
		"/api/categories?archived=1", "/api/locations?archived=1", "/api/stores?archived=1",
		"/api/products", "/api/products?archived=1", stkPath("/api/products/", prodA, ""), "/api/brands",
		"/api/shopping", "/api/purchases", stkPath("/api/purchases/", purchaseA.ID, ""),
		"/api/movements", stkPath("/api/movements?product_id=", prodA, ""),
		"/api/recipes", stkPath("/api/recipes/", recipeA, ""),
		"/api/reports/spending?from=2026-01-01&to=2026-12-31", "/api/reports/consumption?from=2026-01-01&to=2026-12-31",
		"/api/export.json", "/api/export/products.csv", "/api/export/purchases.csv", "/api/export/movements.csv",
	}
	before := accSnapshot(t, a, ownerGets)

	item := func(product int64) []any { return []any{map[string]any{"product_id": product, "qty": 1}} }
	buy := func(product int64, extra map[string]any) map[string]any {
		body := map[string]any{"date": "2026-10-04", "lines": []any{accLine(product, 1, 1)}}
		for k, v := range extra {
			if k == "location_id" {
				body["lines"].([]any)[0].(map[string]any)[k] = v
			} else {
				body[k] = v
			}
		}
		return body
	}
	cases := []struct {
		name, method, target string
		body                 any
		code                 int
	}{
		// Справочники.
		{"переименование категории", http.MethodPatch, stkPath("/api/categories/", catA, ""), map[string]any{"name": "Взлом"}, 404},
		{"архив категории", http.MethodPatch, stkPath("/api/categories/", catA, ""), map[string]any{"archived": true}, 404},
		{"порядок: чужая категория", http.MethodPost, "/api/categories/reorder", map[string]any{"ids": []int64{catA}}, 400},
		{"переименование места", http.MethodPatch, stkPath("/api/locations/", locA, ""), map[string]any{"name": "Взлом"}, 404},
		{"архив места", http.MethodPatch, stkPath("/api/locations/", locA, ""), map[string]any{"archived": true}, 404},
		{"экземпляры чужого места", http.MethodGet, stkPath("/api/locations/", locA, "/units"), nil, 404},
		{"отметка проверки чужого места", http.MethodPost, stkPath("/api/locations/", locA, "/checked"), nil, 404},
		{"порядок: чужое место", http.MethodPost, "/api/locations/reorder", map[string]any{"ids": []int64{locA}}, 400},
		{"порядок: своё и чужое место", http.MethodPost, "/api/locations/reorder", map[string]any{"ids": []int64{locB, locA}}, 400},
		{"переименование магазина", http.MethodPatch, stkPath("/api/stores/", storeA, ""), map[string]any{"name": "Взлом"}, 404},
		{"архив магазина", http.MethodPatch, stkPath("/api/stores/", storeA, ""), map[string]any{"archived": true}, 404},
		// Позиции.
		{"карточка позиции", http.MethodGet, stkPath("/api/products/", prodA, ""), nil, 404},
		{"правка позиции", http.MethodPatch, stkPath("/api/products/", prodA, ""), map[string]any{"name": "Взлом"}, 404},
		{"архив позиции", http.MethodPatch, stkPath("/api/products/", prodA, ""), map[string]any{"archived": true}, 404},
		{"новая позиция в чужой категории", http.MethodPost, "/api/products",
			map[string]any{"category_id": catA, "name": "X", "unit": "ml", "package_size": 1}, 404},
		{"перенос своей позиции в чужую категорию", http.MethodPatch, stkPath("/api/products/", prodB, ""),
			map[string]any{"category_id": catA}, 404},
		// Список покупок.
		{"пункт с чужой позицией", http.MethodPost, "/api/shopping", map[string]any{"product_id": prodA}, 404},
		{"отметка чужого пункта", http.MethodPatch, stkPath("/api/shopping/", itemA, ""), map[string]any{"done": true}, 404},
		{"снятие отметки чужого пункта", http.MethodPatch, stkPath("/api/shopping/", itemA, ""), map[string]any{"done": false}, 404},
		{"удаление чужого пункта", http.MethodDelete, stkPath("/api/shopping/", itemA, ""), nil, 404},
		// Покупки и экземпляры.
		{"покупка чужой позиции", http.MethodPost, "/api/purchases", buy(prodA, nil), 404},
		{"покупка в чужое место", http.MethodPost, "/api/purchases", buy(prodB, map[string]any{"location_id": locA}), 404},
		{"покупка в чужом магазине", http.MethodPost, "/api/purchases", buy(prodB, map[string]any{"store_id": storeA}), 404},
		{"чужая покупка", http.MethodGet, stkPath("/api/purchases/", purchaseA.ID, ""), nil, 404},
		{"ввод остатка чужой позиции", http.MethodPost, "/api/units", map[string]any{"product_id": prodA, "remaining_qty": 1}, 404},
		{"ввод остатка в чужое место", http.MethodPost, "/api/units",
			map[string]any{"product_id": prodB, "remaining_qty": 1, "location_id": locA}, 404},
		{"перемещение чужого экземпляра", http.MethodPost, stkPath("/api/units/", unitA, "/move"), map[string]any{"location_id": locB}, 404},
		{"перемещение своего в чужое место", http.MethodPost, stkPath("/api/units/", unitB, "/move"), map[string]any{"location_id": locA}, 404},
		{"корректировка чужого", http.MethodPost, stkPath("/api/units/", unitA, "/adjust"), map[string]any{"remaining_qty": 1}, 404},
		{"списание чужого", http.MethodPost, stkPath("/api/units/", unitA, "/writeoff"), map[string]any{"reason": "Испорчено"}, 404},
		// Расход и журнал.
		{"расход чужой позиции", http.MethodPost, "/api/consume", map[string]any{"product_id": prodA, "qty": 1}, 404},
		{"расход из чужого экземпляра", http.MethodPost, "/api/consume",
			map[string]any{"product_id": prodB, "qty": 1, "unit_id": unitA}, 404},
		{"отмена чужого движения", http.MethodPost, "/api/movements/revert", map[string]any{"ids": []int64{moveA}}, 404},
		{"журнал чужой позиции", http.MethodGet, stkPath("/api/movements?product_id=", prodA, ""), nil, 404},
		// Шаблоны.
		{"чужой шаблон", http.MethodGet, stkPath("/api/recipes/", recipeA, ""), nil, 404},
		{"замена чужого шаблона", http.MethodPut, stkPath("/api/recipes/", recipeA, ""), map[string]any{"name": "Взлом", "items": item(prodB)}, 404},
		{"удаление чужого шаблона", http.MethodDelete, stkPath("/api/recipes/", recipeA, ""), nil, 404},
		{"применение чужого шаблона", http.MethodPost, stkPath("/api/recipes/", recipeA, "/apply"), map[string]any{"items": item(prodB)}, 404},
		{"шаблон с чужой позицией", http.MethodPost, "/api/recipes", map[string]any{"name": "Взлом", "items": item(prodA)}, 404},
		{"замена своего на чужую позицию", http.MethodPut, stkPath("/api/recipes/", recipeB, ""), map[string]any{"name": "Взлом", "items": item(prodA)}, 404},
		{"применение своего к чужой позиции", http.MethodPost, stkPath("/api/recipes/", recipeB, "/apply"), map[string]any{"items": item(prodA)}, 404},
		{"порядок: чужой шаблон", http.MethodPost, "/api/recipes/reorder", map[string]any{"ids": []int64{recipeA}}, 400},
		{"порядок: свой и чужой шаблон", http.MethodPost, "/api/recipes/reorder", map[string]any{"ids": []int64{recipeB, recipeA}}, 400},
		// Импорт: ссылки в файле только на id внутри файла, чужие id из базы не годятся.
		{"импорт со ссылкой на чужую категорию", http.MethodPost, "/api/import", map[string]any{
			"format": "garage-export", "version": 1,
			"products": []any{map[string]any{"id": 1, "category_id": catA, "name": "X", "unit": "ml", "package_size": 1}}}, 400},
		{"импорт: покупка в чужой магазин", http.MethodPost, "/api/import", map[string]any{
			"format": "garage-export", "version": 1, "purchases": []any{map[string]any{"id": 1, "store_id": storeA,
				"date": "2026-10-04", "lines": []any{map[string]any{"id": 1, "product_id": prodA, "packages": 1}}}}}, 400},
		{"импорт в непустой склад", http.MethodPost, "/api/import", map[string]any{"format": "garage-export", "version": 1}, 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := b.do(tc.method, tc.target, tc.body)
			expect(t, rec, tc.code, tc.method+" "+tc.target)
			if msg := decode[errBody](t, rec).Error; tc.code == 404 && !strings.Contains(msg, "не найден") {
				t.Fatalf("текст ошибки %q, ожидалось «… не найдено»", msg)
			}
		})
	}
	// Порядок категорий: все свои действующие плюс чужая — тоже 400.
	own := decode[catCategories](t, b.do(http.MethodGet, "/api/categories", nil)).Categories
	ids := []int64{catA}
	for _, c := range own {
		ids = append(ids, c.ID)
	}
	expect(t, b.do(http.MethodPost, "/api/categories/reorder", map[string]any{"ids": ids}), http.StatusBadRequest,
		"порядок: все свои категории и чужая")

	// Данные владельца не изменились.
	after := accSnapshot(t, a, ownerGets)
	for _, target := range ownerGets {
		if before[target] != after[target] {
			t.Fatalf("у владельца изменился %s:\nдо    %s\nпосле %s", target, before[target], after[target])
		}
	}

	// Списки второго — только своё.
	leaks := []string{"Тайная категория", "Тайник", "Тайный магазин", "Секретный", "Тайнобренд", "Тайная заметка", "Тайный рецепт"}
	lists := []string{
		"/api/categories?archived=1", "/api/locations?archived=1", "/api/stores?archived=1",
		"/api/products", "/api/products?archived=1", "/api/products?q=секрет", "/api/products?filter=low",
		stkPath("/api/products?category_id=", catA, ""), stkPath("/api/products?location_id=", locA, ""),
		"/api/brands", "/api/shopping", "/api/purchases", "/api/movements", "/api/recipes",
		"/api/reports/spending?from=2026-01-01&to=2026-12-31", "/api/reports/consumption?from=2026-01-01&to=2026-12-31",
		"/api/export.json", "/api/export/products.csv", "/api/export/purchases.csv", "/api/export/movements.csv",
	}
	for _, target := range lists {
		rec := b.do(http.MethodGet, target, nil)
		expect(t, rec, http.StatusOK, "список второго "+target)
		for _, leak := range leaks {
			if strings.Contains(rec.Body.String(), leak) {
				t.Fatalf("%s: второй видит %q: %s", target, leak, rec.Body.String())
			}
		}
	}
	for _, q := range []string{"?q=секрет", stkPath("?category_id=", catA, ""), stkPath("?location_id=", locA, ""), "?filter=low"} {
		if got := accListed(t, b, q); len(got) != 0 {
			t.Fatalf("/api/products%s у второго не пуст: %+v", q, got)
		}
	}
	if got := accListed(t, b, ""); len(got) != 1 || got[0].ID != prodB {
		t.Fatalf("позиции второго: %+v", got)
	}
	if got := decode[catalog.ShoppingList](t, b.do(http.MethodGet, "/api/shopping", nil)); len(got.Low) != 0 || len(got.Manual) != 0 {
		t.Fatalf("список покупок второго: %+v", got)
	}
	if got := decode[struct {
		Brands []string `json:"brands"`
	}](t, b.do(http.MethodGet, "/api/brands", nil)).Brands; len(got) != 0 {
		t.Fatalf("бренды второго: %v", got)
	}
	purchases := decode[struct {
		Purchases []struct {
			ID int64 `json:"id"`
		} `json:"purchases"`
	}](t, b.do(http.MethodGet, "/api/purchases", nil)).Purchases
	if len(purchases) != 1 || purchases[0].ID == purchaseA.ID {
		t.Fatalf("покупки второго: %+v", purchases)
	}
	for _, m := range decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, b.do(http.MethodGet, "/api/movements", nil)).Movements {
		if m.ProductID != prodB {
			t.Fatalf("в журнале второго чужое движение: %+v", m)
		}
	}
	recipes := decode[struct {
		Recipes []struct {
			ID int64 `json:"id"`
		} `json:"recipes"`
	}](t, b.do(http.MethodGet, "/api/recipes", nil)).Recipes
	if len(recipes) != 1 || recipes[0].ID != recipeB {
		t.Fatalf("шаблоны второго: %+v", recipes)
	}
	for _, r := range recipes {
		if r.ID == recipeA {
			t.Fatalf("второй видит чужой шаблон: %+v", recipes)
		}
	}

	// Имена уникальны у пользователя, а не глобально: второй может завести такие же.
	for target, body := range map[string]map[string]any{
		"/api/categories": {"name": "Тайная категория", "color": "blue"},
		"/api/locations":  {"name": "Тайник"},
		"/api/stores":     {"name": "Тайный магазин", "url": ""},
		"/api/recipes":    {"name": "Тайный рецепт", "items": item(prodB)},
	} {
		expect(t, b.do(http.MethodPost, target, body), http.StatusCreated, "то же имя у второго: "+target)
	}

	// Без входа — 401 на списки этапа 2.
	anon := e.client()
	for _, target := range ownerGets {
		expect(t, anon.do(http.MethodGet, target, nil), http.StatusUnauthorized, "без входа "+target)
	}
}
