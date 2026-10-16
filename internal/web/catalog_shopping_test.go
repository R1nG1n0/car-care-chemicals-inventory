package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

func TestCatShoppingHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	cheap := decode[catalog.Store](t, c.do(http.MethodPost, "/api/stores", map[string]any{"name": "Дешёвый"}))
	dear := decode[catalog.Store](t, c.do(http.MethodPost, "/api/stores", map[string]any{"name": "Дорогой"}))
	p := catProduct(t, c, map[string]any{"category_id": cat, "name": "Gsf", "unit": "ml", "package_size": 1000, "min_stock": 5000})
	catProduct(t, c, map[string]any{"category_id": cat, "name": "Без минимума", "unit": "ml", "package_size": 1000})
	never := catProduct(t, c, map[string]any{"category_id": cat, "name": "Не покупали", "unit": "pcs", "package_size": 1, "min_stock": 1})
	e.catPurchase(p.ID, cheap.ID, "2026-09-01", 100000, 1_791_000_000)
	e.catPurchase(p.ID, dear.ID, "2026-09-20", 150000, 1_791_500_000)

	// Добавление: позиция, текст, ни того ни другого — 400.
	rec := c.do(http.MethodPost, "/api/shopping", map[string]any{"product_id": p.ID})
	expect(t, rec, http.StatusCreated, "пункт с позицией")
	item := decode[catalog.ShoppingItem](t, rec)
	if item.Product == nil || item.Product.ID != p.ID || item.DoneAt != nil || item.CreatedAt != 1_792_000_000 {
		t.Fatalf("пункт: %+v", item)
	}
	e.clock.Add(time.Second)
	rec = c.do(http.MethodPost, "/api/shopping", map[string]any{"title": "Глина для кузова"})
	expect(t, rec, http.StatusCreated, "текстовый пункт")
	text := decode[catalog.ShoppingItem](t, rec)
	if text.ProductID != nil || text.Product != nil || !strings.Contains(rec.Body.String(), `"product":null`) {
		t.Fatalf("текстовый пункт: %s", rec.Body.String())
	}
	catExpectField(t, c, http.MethodPost, "/api/shopping", map[string]any{}, 400, "title")
	catExpectField(t, c, http.MethodPost, "/api/shopping", map[string]any{"title": strings.Repeat("г", 81)}, 400, "title")
	expect(t, c.do(http.MethodPost, "/api/shopping", map[string]any{"product_id": 99999}), http.StatusNotFound, "нет позиции")

	rec = c.do(http.MethodGet, "/api/shopping", nil)
	expect(t, rec, http.StatusOK, "список покупок")
	list := decode[catalog.ShoppingList](t, rec)
	if len(list.Low) != 2 || list.Low[0].Product.ID != p.ID || list.Low[1].Product.ID != never.ID {
		t.Fatalf("low: %+v", list.Low)
	}
	if l := list.Low[0]; *l.LastPrice != 150000 || *l.LastStore != "Дорогой" || *l.CheapestStore != "Дешёвый" {
		t.Fatalf("цены low: %+v", l)
	}
	if l := list.Low[1]; l.LastPrice != nil || l.LastStore != nil || l.CheapestStore != nil {
		t.Fatalf("low без покупок: %+v", l)
	}
	if len(list.Manual) != 2 || list.Manual[0].ID != text.ID || list.Manual[1].ID != item.ID {
		t.Fatalf("manual — новые сверху: %+v", list.Manual)
	}

	// Покупка мимо POST /api/purchases (прямой SQL) пункт не отмечает:
	// GET только читает.
	e.clock.Add(time.Minute)
	e.catPurchase(p.ID, nil, "2026-10-15", 120000, e.clock.Now().Unix())
	list = decode[catalog.ShoppingList](t, c.do(http.MethodGet, "/api/shopping", nil))
	if list.Manual[1].DoneAt != nil {
		t.Fatalf("GET отметил пункт: %+v", list.Manual[1])
	}

	// Покупка через API отмечает пункт её позиции в той же транзакции;
	// текстовый не трогается.
	e.clock.Add(time.Minute)
	expect(t, c.do(http.MethodPost, "/api/purchases", map[string]any{"date": "2026-10-16",
		"lines": []map[string]any{{"product_id": p.ID, "packages": 1, "price_per_package": 120000}}}),
		http.StatusCreated, "покупка")
	list = decode[catalog.ShoppingList](t, c.do(http.MethodGet, "/api/shopping", nil))
	if d := list.Manual[1].DoneAt; d == nil || *d != e.clock.Now().Unix() || list.Manual[0].DoneAt != nil {
		t.Fatalf("отметка покупкой: %+v", list.Manual)
	}

	// Ручные отметки.
	target := fmt.Sprintf("/api/shopping/%d", text.ID)
	catExpectField(t, c, http.MethodPatch, target, map[string]any{}, 400, "done")
	rec = c.do(http.MethodPatch, target, map[string]any{"done": true})
	expect(t, rec, http.StatusOK, "вычеркнуть")
	if got := decode[catalog.ShoppingItem](t, rec); got.DoneAt == nil {
		t.Fatalf("вычеркнуть: %+v", got)
	}
	e.clock.Add(time.Second)
	rec = c.do(http.MethodPatch, fmt.Sprintf("/api/shopping/%d", item.ID), map[string]any{"done": false})
	expect(t, rec, http.StatusOK, "вернуть купленный")
	if got := decode[catalog.ShoppingItem](t, rec); got.DoneAt != nil || got.CreatedAt != e.clock.Now().Unix() {
		t.Fatalf("вернуть купленный: %+v", got)
	}
	list = decode[catalog.ShoppingList](t, c.do(http.MethodGet, "/api/shopping", nil))
	if list.Manual[0].ID != item.ID || list.Manual[0].DoneAt != nil {
		t.Fatalf("снятая отметка вернулась: %+v", list.Manual)
	}
	// Снятая отметка с некупленного пункта тоже поднимает его наверх.
	e.clock.Add(time.Second)
	rec = c.do(http.MethodPatch, target, map[string]any{"done": false})
	expect(t, rec, http.StatusOK, "вернуть текстовый")
	if got := decode[catalog.ShoppingItem](t, rec); got.DoneAt != nil || got.CreatedAt != e.clock.Now().Unix() {
		t.Fatalf("вернуть текстовый: %+v", got)
	}
	list = decode[catalog.ShoppingList](t, c.do(http.MethodGet, "/api/shopping", nil))
	if list.Manual[0].ID != text.ID || list.Manual[1].ID != item.ID {
		t.Fatalf("снятый пункт не наверху: %+v", list.Manual)
	}
	// И снова вычеркнуть — для проверки скрытия старых выполненных.
	expect(t, c.do(http.MethodPatch, target, map[string]any{"done": true}), http.StatusOK, "вычеркнуть снова")

	// Выполненные старше 7 дней скрываются.
	e.clock.Add(8 * 24 * time.Hour)
	list = decode[catalog.ShoppingList](t, c.do(http.MethodGet, "/api/shopping", nil))
	if len(list.Manual) != 1 || list.Manual[0].ID != item.ID {
		t.Fatalf("старый выполненный: %+v", list.Manual)
	}

	expect(t, c.do(http.MethodDelete, target, nil), http.StatusNoContent, "удаление")
	expect(t, c.do(http.MethodDelete, target, nil), http.StatusNotFound, "повторное удаление")
}
