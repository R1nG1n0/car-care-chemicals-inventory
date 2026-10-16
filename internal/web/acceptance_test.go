package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Сквозные проверки критериев приёмки (раздел 8 ТЗ) только через API:
// данные заводятся теми же адресами, что и в интерфейсе, без прямого SQL.
// Продолжение — acceptance_places_test.go, acceptance_isolation_test.go.

// accPurchaseBody — ответ POST /api/purchases и GET /api/purchases/{id}.
type accPurchaseBody struct {
	Purchase struct {
		ID      int64   `json:"id"`
		Date    string  `json:"date"`
		StoreID *int64  `json:"store_id"`
		Store   *string `json:"store"`
		Total   int64   `json:"total"`
		Lines   []struct {
			ProductID       int64   `json:"product_id"`
			Packages        int64   `json:"packages"`
			PricePerPackage int64   `json:"price_per_package"`
			UnitIDs         []int64 `json:"unit_ids"`
		} `json:"lines"`
		CreatedAt int64 `json:"created_at"`
	} `json:"purchase"`
}

type accApplyBody struct {
	RunID     int64                    `json:"run_id"`
	Movements []catalog.Movement       `json:"movements"`
	Products  []catalog.ProductSummary `json:"products"`
	Shortages []struct {
		ProductID int64 `json:"product_id"`
		Requested int64 `json:"requested"`
		Consumed  int64 `json:"consumed"`
	} `json:"shortages"`
	Cost *int64 `json:"cost"`
}

// accBuy — покупка одной строкой через API; возвращает ответ целиком.
func accBuy(t *testing.T, c *client, body map[string]any) accPurchaseBody {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/purchases", body)
	expect(t, rec, http.StatusCreated, "покупка")
	return decode[accPurchaseBody](t, rec)
}

// accLine — строка покупки.
func accLine(product, packages, price int64) map[string]any {
	return map[string]any{"product_id": product, "packages": packages, "price_per_package": price}
}

func accDetail(t *testing.T, c *client, product int64) catDetail {
	t.Helper()
	rec := c.do(http.MethodGet, stkPath("/api/products/", product, ""), nil)
	expect(t, rec, http.StatusOK, "карточка позиции")
	return decode[catDetail](t, rec)
}

// accListed — позиции из GET /api/products с параметрами query.
func accListed(t *testing.T, c *client, query string) []catalog.ProductSummary {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/products"+query, nil)
	expect(t, rec, http.StatusOK, "список позиций "+query)
	return decode[catProducts](t, rec).Products
}

// accFind — позиция из общего списка по id; фатально, если её нет.
func accFind(t *testing.T, c *client, product int64) catalog.ProductSummary {
	t.Helper()
	for _, p := range accListed(t, c, "") {
		if p.ID == product {
			return p
		}
	}
	t.Fatalf("позиции %d нет в списке", product)
	return catalog.ProductSummary{}
}

func accMovements(t *testing.T, c *client, product int64) []catalog.Movement {
	t.Helper()
	rec := c.do(http.MethodGet, stkPath("/api/movements?product_id=", product, ""), nil)
	expect(t, rec, http.StatusOK, "журнал позиции")
	return decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, rec).Movements
}

func accConsume(t *testing.T, c *client, product, qty int64) stkConsumeBody {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": product, "qty": qty})
	expect(t, rec, http.StatusCreated, "расход")
	return decode[stkConsumeBody](t, rec)
}

// accLast — last_consume_qty; -1 означает null.
func accLast(p catalog.ProductSummary) int64 {
	if p.LastConsumeQty == nil {
		return -1
	}
	return *p.LastConsumeQty
}

// П-4: покупка двух бутылок по 500 мл — два запечатанных экземпляра,
// в карточке цена, магазин, дата и цена за 100 мл.
func TestAccPurchaseTwoBottles(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	p := stkProduct(c, stkCategory(c, "Шампуни тест"), "Koch Chemie Gsf", "ml", 500)
	box := stkLocation(c, "Коробка 3")
	shop := stkCreate(c, "/api/stores", map[string]any{"name": "Автохим", "url": ""})

	line := accLine(p, 2, 119000)
	line["location_id"] = box
	got := accBuy(t, c, map[string]any{"date": "2026-10-02", "store_id": shop, "lines": []any{line}})
	pu := got.Purchase
	if pu.ID == 0 || pu.Date != "2026-10-02" || pu.Store == nil || *pu.Store != "Автохим" || pu.Total != 238000 ||
		len(pu.Lines) != 1 || len(pu.Lines[0].UnitIDs) != 2 {
		t.Fatalf("ответ покупки: %+v", pu)
	}
	rec := c.do(http.MethodGet, stkPath("/api/purchases/", pu.ID, ""), nil)
	expect(t, rec, http.StatusOK, "покупка по id")
	if again := decode[accPurchaseBody](t, rec).Purchase; again.Total != 238000 || len(again.Lines[0].UnitIDs) != 2 {
		t.Fatalf("GET покупки: %+v", again)
	}

	d := accDetail(t, c, p)
	if d.Product.Stock.Total != 1000 || d.Product.Stock.SealedCount != 2 || d.Product.Stock.UnitsCount != 2 ||
		d.Product.Stock.OpenQty != nil || d.Product.LastConsumeQty != nil {
		t.Fatalf("остаток после покупки: %+v", d.Product)
	}
	if len(d.Units) != 2 {
		t.Fatalf("экземпляров в карточке %d, ожидалось 2: %+v", len(d.Units), d.Units)
	}
	want := map[int64]bool{pu.Lines[0].UnitIDs[0]: true, pu.Lines[0].UnitIDs[1]: true}
	for _, u := range d.Units {
		if !want[u.ID] || u.Status != "sealed" || u.InitialQty != 500 || u.RemainingQty != 500 || u.OpenedAt != nil ||
			u.UnitPrice == nil || *u.UnitPrice != 119000 || u.LocationID == nil || *u.LocationID != box {
			t.Fatalf("экземпляр в карточке: %+v", u)
		}
	}
	if len(d.Purchases) != 1 {
		t.Fatalf("покупок в карточке %d: %+v", len(d.Purchases), d.Purchases)
	}
	row := d.Purchases[0]
	if row.PurchaseID != pu.ID || row.Date != "2026-10-02" || row.Store == nil || *row.Store != "Автохим" ||
		row.Packages != 2 || row.PricePerPackage != 119000 || row.PricePer100 == nil || *row.PricePer100 != 23800 ||
		row.PricePerUnit != nil {
		t.Fatalf("строка покупки в карточке: %+v", row)
	}
	purchases := 0
	for _, m := range d.Movements {
		if m.Type == "purchase" && m.QtyDelta == 500 {
			purchases++
		}
	}
	if purchases != 2 || len(d.Movements) != 2 {
		t.Fatalf("движения покупки: %+v", d.Movements)
	}
	if listed := accFind(t, c, p); listed.Stock.SealedCount != 2 || len(listed.LocationIDs) != 1 || listed.LocationIDs[0] != box {
		t.Fatalf("строка главного списка: %+v", listed)
	}

	// Для штучной позиции цена — price_per_unit, без price_per_100.
	pcs := stkProduct(c, stkCategory(c, "Салфетки тест"), "Микрофибра", "pcs", 10)
	accBuy(t, c, map[string]any{"date": "2026-10-03", "lines": []any{accLine(pcs, 1, 50000)}})
	r := accDetail(t, c, pcs).Purchases
	if len(r) != 1 || r[0].Store != nil || r[0].PricePerUnit == nil || *r[0].PricePerUnit != 5000 || r[0].PricePer100 != nil {
		t.Fatalf("строка покупки штучной позиции: %+v", r)
	}
}

// П-5: расход 600 мл из двух бутылок по 500 — первая закончилась, вторая
// вскрыта с остатком 400; в журнале два списания; last_consume_qty = 600.
func TestAccConsumeAcrossBottles(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	p := stkProduct(c, stkCategory(c, "Шампуни тест"), "Koch Chemie Gsf", "ml", 500)
	units := accBuy(t, c, map[string]any{"date": "2026-10-02", "lines": []any{accLine(p, 2, 119000)}}).Purchase.Lines[0].UnitIDs

	got := accConsume(t, c, p, 600)
	if len(got.FinishedUnitIDs) != 1 || got.FinishedUnitIDs[0] != units[0] {
		t.Fatalf("закончившиеся: %v, ожидалась первая бутылка %d", got.FinishedUnitIDs, units[0])
	}
	if got.Product.Stock.Total != 400 || got.Product.Stock.OpenQty == nil || *got.Product.Stock.OpenQty != 400 ||
		got.Product.Stock.SealedCount != 0 || accLast(got.Product) != 600 {
		t.Fatalf("позиция в ответе расхода: %+v", got.Product)
	}

	d := accDetail(t, c, p)
	if len(d.Units) != 1 || d.Units[0].ID != units[1] || d.Units[0].Status != "open" || d.Units[0].RemainingQty != 400 ||
		d.Units[0].OpenedAt == nil {
		t.Fatalf("экземпляры после расхода: %+v", d.Units)
	}
	if accLast(d.Product) != 600 || accLast(accFind(t, c, p)) != 600 {
		t.Fatalf("last_consume_qty: карточка %v", d.Product.LastConsumeQty)
	}

	log := accMovements(t, c, p)
	var consumed []catalog.Movement
	for _, m := range log {
		if m.Type == "consume" {
			consumed = append(consumed, m)
		}
	}
	if len(log) != 4 || len(consumed) != 2 {
		t.Fatalf("журнал: %+v", log)
	}
	byUnit := map[int64]int64{}
	for _, m := range consumed {
		byUnit[m.UnitID] += m.QtyDelta
		if m.Cost == nil || *m.Cost <= 0 {
			t.Fatalf("себестоимость движения: %+v", m)
		}
	}
	if byUnit[units[0]] != -500 || byUnit[units[1]] != -100 {
		t.Fatalf("списания по бутылкам: %v", byUnit)
	}
}

// П-6: позиция ниже минимума — в low; после покупки через API уходит из low,
// ручной пункт с этой позицией отмечается выполненным.
func TestAccShoppingClearedByPurchase(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	cat := stkCategory(c, "Шампуни тест")
	p := stkCreate(c, "/api/products", map[string]any{"category_id": cat, "name": "Gyeon Bathe",
		"unit": "ml", "package_size": 500, "min_stock": 300})
	other := stkProduct(c, cat, "Gyeon Foam", "ml", 1000)
	accBuy(t, c, map[string]any{"date": "2026-10-01", "lines": []any{accLine(p, 1, 150000)}})
	accConsume(t, c, p, 300) // осталось 200 < 300

	shopping := func() catalog.ShoppingList {
		rec := c.do(http.MethodGet, "/api/shopping", nil)
		expect(t, rec, http.StatusOK, "список покупок")
		return decode[catalog.ShoppingList](t, rec)
	}
	inLow := func(l catalog.ShoppingList) bool {
		for _, it := range l.Low {
			if it.Product.ID == p {
				return true
			}
		}
		return false
	}
	before := shopping()
	if !inLow(before) || len(before.Low) != 1 || before.Low[0].LastPrice == nil || *before.Low[0].LastPrice != 150000 {
		t.Fatalf("позиция ниже минимума не попала в low: %+v", before)
	}
	if !accFind(t, c, p).Low {
		t.Fatal("в списке позиций нет признака low")
	}

	rec := c.do(http.MethodPost, "/api/shopping", map[string]any{"product_id": p})
	expect(t, rec, http.StatusCreated, "ручной пункт с позицией")
	item := decode[catalog.ShoppingItem](t, rec)
	rec = c.do(http.MethodPost, "/api/shopping", map[string]any{"product_id": other})
	expect(t, rec, http.StatusCreated, "пункт с другой позицией")
	otherItem := decode[catalog.ShoppingItem](t, rec)
	rec = c.do(http.MethodPost, "/api/shopping", map[string]any{"title": "Глина для кузова"})
	expect(t, rec, http.StatusCreated, "пункт без позиции")
	freeItem := decode[catalog.ShoppingItem](t, rec)

	e.clock.Add(time.Hour) // час спустя — чтобы done_at отличался от created_at
	pu := accBuy(t, c, map[string]any{"date": "2026-10-05", "lines": []any{accLine(p, 1, 140000)}}).Purchase

	after := shopping()
	if inLow(after) || len(after.Low) != 0 {
		t.Fatalf("после покупки позиция осталась в low: %+v", after.Low)
	}
	if accFind(t, c, p).Low {
		t.Fatal("после покупки у позиции остался признак low")
	}
	done := map[int64]*int64{}
	for _, it := range after.Manual {
		done[it.ID] = it.DoneAt
	}
	if len(after.Manual) != 3 {
		t.Fatalf("ручные пункты: %+v", after.Manual)
	}
	if d := done[item.ID]; d == nil || *d != pu.CreatedAt || *d != e.clock.Now().Unix() {
		t.Fatalf("пункт с купленной позицией не отмечен моментом покупки: done_at=%v, покупка %d", d, pu.CreatedAt)
	}
	if done[otherItem.ID] != nil || done[freeItem.ID] != nil {
		t.Fatalf("отмечены пункты, которых нет в покупке: %+v", after.Manual)
	}
}
