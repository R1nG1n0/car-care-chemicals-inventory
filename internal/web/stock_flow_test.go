package web

import (
	"net/http"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Сценарий П-4, П-5 через HTTP: покупка, расход с добором, нехватка, отмена.
func TestStockConsumeFlow(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	cat := stkCategory(c, "Тест")
	p := stkProduct(c, cat, "Koch Chemie Gsf", "ml", 500)
	box := stkLocation(c, "Коробка 3")
	units := stkBuy(c, p, 2, 119000, &box)
	if len(units) != 2 {
		t.Fatalf("экземпляры: %v", units)
	}

	rec := c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 600})
	expect(t, rec, http.StatusCreated, "расход 600")
	got := decode[stkConsumeBody](t, rec)
	if len(got.Movements) != 2 || got.Product.Stock.Total != 400 || len(got.FinishedUnitIDs) != 1 ||
		len(got.OpenedUnitIDs) != 2 || got.Product.LocationIDs[0] != box {
		t.Fatalf("ответ расхода: %+v", got)
	}
	// 500 мл по 119000 за 500 + 100 мл: 119000 + 23800.
	if got.Cost == nil || *got.Cost != 142800 {
		t.Fatalf("себестоимость: %v", got.Cost)
	}
	if got.Movements[0].Type != "consume" || got.Movements[0].QtyDelta != -500 ||
		got.Movements[1].BatchID != got.Movements[0].ID || got.Movements[0].BatchID != got.Movements[0].ID {
		t.Fatalf("движение: %+v", got.Movements[0])
	}

	rec = c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 401})
	expect(t, rec, http.StatusConflict, "нехватка")
	if sb := decode[stkShortageBody](t, rec); sb.Available != 400 || sb.Error != "Не хватает: есть 400 мл" {
		t.Fatalf("нехватка: %+v", sb)
	}
	expect(t, c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 0}), http.StatusBadRequest, "qty 0")

	// Журнал: новые сверху, по позиции.
	rec = c.do(http.MethodGet, stkPath("/api/movements?product_id=", p, "&limit=3"), nil)
	expect(t, rec, http.StatusOK, "журнал")
	log := decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, rec).Movements
	if len(log) != 3 || log[0].ID < log[1].ID || log[2].Type != "purchase" {
		t.Fatalf("журнал: %+v", log)
	}

	// Отмена — снова два запечатанных.
	rec = c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(got.Movements)})
	expect(t, rec, http.StatusOK, "отмена")
	prods := decode[struct {
		Products []catalog.ProductSummary `json:"products"`
	}](t, rec).Products
	if len(prods) != 1 || prods[0].Stock.Total != 1000 || prods[0].Stock.SealedCount != 2 {
		t.Fatalf("после отмены: %+v", prods)
	}
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(got.Movements)}),
		http.StatusNotFound, "повторная отмена")

	// Явный экземпляр, перемещение, корректировка, списание, ввод остатка.
	rec = c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 10, "unit_id": units[1]})
	expect(t, rec, http.StatusCreated, "расход из выбранного")
	if got := decode[stkConsumeBody](t, rec); got.Movements[0].UnitID != units[1] {
		t.Fatalf("выбранный экземпляр: %+v", got.Movements)
	}
	shelf := stkLocation(c, "Полка")
	rec = c.do(http.MethodPost, stkPath("/api/units/", units[0], "/move"), map[string]any{"location_id": shelf})
	expect(t, rec, http.StatusOK, "перемещение")
	if u := decode[catalog.Unit](t, rec); u.LocationID == nil || *u.LocationID != shelf {
		t.Fatalf("после перемещения: %+v", u)
	}
	rec = c.do(http.MethodPost, stkPath("/api/units/", units[1], "/adjust"), map[string]any{"remaining_qty": 0})
	expect(t, rec, http.StatusOK, "корректировка")
	if u := decode[stkUnitMovement](t, rec); u.Status != "finished" || u.Movement == nil ||
		u.Movement.Type != "adjust" || u.Movement.QtyDelta != -490 || u.Movement.BatchID != u.Movement.ID {
		t.Fatalf("после корректировки: %+v", u)
	}
	rec = c.do(http.MethodPost, stkPath("/api/units/", units[1], "/adjust"), map[string]any{"remaining_qty": 0})
	expect(t, rec, http.StatusOK, "корректировка без изменений")
	if u := decode[stkUnitMovement](t, rec); u.Movement != nil {
		t.Fatalf("тот же остаток — без движения: %+v", u.Movement)
	}
	expect(t, c.do(http.MethodPost, stkPath("/api/units/", units[0], "/writeoff"), map[string]any{}),
		http.StatusBadRequest, "списание без причины")
	rec = c.do(http.MethodPost, stkPath("/api/units/", units[0], "/writeoff"), map[string]any{"reason": "Испорчено"})
	expect(t, rec, http.StatusOK, "списание")
	if u := decode[stkUnitMovement](t, rec); u.Status != "written_off" || u.RemainingQty != 0 ||
		u.Movement == nil || u.Movement.Type != "writeoff" || u.Movement.QtyDelta != -500 || u.Movement.Reason != "Испорчено" {
		t.Fatalf("после списания: %+v", u)
	}
	rec = c.do(http.MethodPost, "/api/units", map[string]any{"product_id": p, "remaining_qty": 120})
	expect(t, rec, http.StatusCreated, "ввод остатка")
	if u := decode[catalog.Unit](t, rec); u.Status != "open" || u.InitialQty != 500 || u.UnitPrice != nil ||
		u.LocationID == nil || *u.LocationID != box {
		t.Fatalf("введённый экземпляр: %+v", u)
	}

	// Покупки.
	rec = c.do(http.MethodGet, "/api/purchases", nil)
	expect(t, rec, http.StatusOK, "список покупок")
	if list := decode[struct {
		Purchases []struct {
			Total int64 `json:"total"`
		} `json:"purchases"`
	}](t, rec).Purchases; len(list) != 1 || list[0].Total != 238000 {
		t.Fatalf("список покупок: %+v", list)
	}
	expect(t, c.do(http.MethodGet, "/api/purchases?limit=0x", nil), http.StatusBadRequest, "неверный limit")
}

// stkUnitMovement — ответ корректировки и списания: экземпляр и его движение.
type stkUnitMovement struct {
	catalog.Unit
	Movement *catalog.Movement `json:"movement"`
}

func TestStockRecipeFlow(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	cat := stkCategory(c, "Тест")
	shampoo := stkProduct(c, cat, "Шампунь", "ml", 1000)
	foam := stkProduct(c, cat, "Пена", "ml", 500)
	stkBuy(c, shampoo, 1, 100000, nil)
	stkBuy(c, foam, 1, 50000, nil)

	id := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка", "items": []any{
		map[string]any{"product_id": shampoo, "qty": 40}, map[string]any{"product_id": foam, "qty": 600}}})
	rec := c.do(http.MethodGet, stkPath("/api/recipes/", id, ""), nil)
	expect(t, rec, http.StatusOK, "шаблон")
	type recipe struct {
		Name  string `json:"name"`
		Cost  *int64 `json:"cost"`
		Items []struct {
			Product   catalog.ProductSummary `json:"product"`
			Available int64                  `json:"available"`
			Short     bool                   `json:"short"`
		} `json:"items"`
	}
	r := decode[struct {
		Recipe recipe `json:"recipe"`
	}](t, rec).Recipe
	if r.Name != "Мойка" || len(r.Items) != 2 || r.Items[0].Product.Name != "Шампунь" || !r.Items[1].Short ||
		r.Cost == nil || *r.Cost != 54000 {
		t.Fatalf("шаблон: %+v", r)
	}

	rec = c.do(http.MethodPost, stkPath("/api/recipes/", id, "/apply"), map[string]any{"items": []any{
		map[string]any{"product_id": shampoo, "qty": 50}, map[string]any{"product_id": foam, "qty": 600}}})
	expect(t, rec, http.StatusCreated, "применение")
	applied := decode[struct {
		RunID     int64                    `json:"run_id"`
		Movements []catalog.Movement       `json:"movements"`
		Products  []catalog.ProductSummary `json:"products"`
		Shortages []struct {
			ProductID int64 `json:"product_id"`
			Requested int64 `json:"requested"`
			Consumed  int64 `json:"consumed"`
		} `json:"shortages"`
		Cost *int64 `json:"cost"`
	}](t, rec)
	if applied.RunID == 0 || len(applied.Movements) != 2 || len(applied.Products) != 2 ||
		len(applied.Shortages) != 1 || applied.Shortages[0].Consumed != 500 || applied.Cost == nil || *applied.Cost != 55000 {
		t.Fatalf("применение: %+v", applied)
	}
	if applied.Movements[0].RecipeRunID == nil || *applied.Movements[0].RecipeRunID != applied.RunID {
		t.Fatalf("run_id в движении: %+v", applied.Movements[0])
	}
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(applied.Movements)}),
		http.StatusOK, "отмена применения")

	id2 := stkCreate(c, "/api/recipes", map[string]any{"name": "Стёкла", "items": []any{
		map[string]any{"product_id": foam, "qty": 5}}})
	expect(t, c.do(http.MethodPost, "/api/recipes/reorder", map[string]any{"ids": []int64{id2, id}}),
		http.StatusNoContent, "порядок")
	rec = c.do(http.MethodPut, stkPath("/api/recipes/", id, ""), map[string]any{"name": "Стёкла", "items": []any{
		map[string]any{"product_id": foam, "qty": 5}}})
	expect(t, rec, http.StatusConflict, "повтор имени")
	if decode[errBody](t, rec).Field != "name" {
		t.Fatalf("поле ошибки: %s", rec.Body.String())
	}
	rec = c.do(http.MethodGet, "/api/recipes", nil)
	expect(t, rec, http.StatusOK, "список шаблонов")
	list := decode[struct {
		Recipes []recipe `json:"recipes"`
	}](t, rec).Recipes
	if len(list) != 2 || list[0].Name != "Стёкла" {
		t.Fatalf("список шаблонов: %+v", list)
	}
	expect(t, c.do(http.MethodDelete, stkPath("/api/recipes/", id, ""), nil), http.StatusNoContent, "удаление")
	expect(t, c.do(http.MethodGet, stkPath("/api/recipes/", id, ""), nil), http.StatusNotFound, "удалённый")
}
