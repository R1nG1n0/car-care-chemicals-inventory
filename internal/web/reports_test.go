package web

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/report"
)

// rptSetup заводит данные: две категории, магазин, две позиции, покупки в
// сентябре и октябре, расход и списание. Часы теста — 14 октября 2026, 17:46 UTC.
func rptSetup(t *testing.T) (c *client, p1, p2 int64) {
	t.Helper()
	e := newEnv(t, false)
	c = stkSignIn(e, "owner")
	k1 := stkCategory(c, "Отчёт: шампуни")
	k2 := stkCreate(c, "/api/categories", map[string]any{"name": "Отчёт: салфетки", "color": "olive"})
	store := stkCreate(c, "/api/stores", map[string]any{"name": "Автохим", "url": ""})
	p1 = stkProduct(c, k1, "Шампунь", "ml", 500)
	p2 = stkProduct(c, k2, "Салфетка", "pcs", 1)
	buy := accBuy(t, c, map[string]any{"date": "2026-09-30", "store_id": store, "lines": []any{accLine(p1, 2, 119000)}})
	accBuy(t, c, map[string]any{"date": "2026-10-02", "lines": []any{accLine(p2, 3, 5000)}})
	accConsume(t, c, p1, 100)
	accConsume(t, c, p2, 2)
	unit := buy.Purchase.Lines[0].UnitIDs[1]
	expect(t, c.do(http.MethodPost, stkPath("/api/units/", unit, "/writeoff"), map[string]any{"qty": 50, "reason": "Испорчено"}),
		http.StatusOK, "списание")
	return c, p1, p2
}

func rptGet[T any](t *testing.T, c *client, kind, query string) T {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/reports/"+kind+"?"+query, nil)
	expect(t, rec, http.StatusOK, kind)
	return decode[T](t, rec)
}

func TestReportSpending(t *testing.T) {
	c, _, _ := rptSetup(t)
	got := rptGet[report.Spending](t, c, "spending", "from=2026-09-01&to=2026-10-31")
	if got.Total != 253000 || got.PurchasesCount != 2 {
		t.Fatalf("итог %d, покупок %d", got.Total, got.PurchasesCount)
	}
	if len(got.ByCategory) != 2 || got.ByCategory[0].Name != "Отчёт: шампуни" || got.ByCategory[0].Total != 238000 ||
		got.ByCategory[0].Color != "blue" || got.ByCategory[1].Total != 15000 || got.ByCategory[1].Color != "olive" {
		t.Fatalf("по категориям: %+v", got.ByCategory)
	}
	if len(got.ByStore) != 2 || got.ByStore[0].Name != "Автохим" || got.ByStore[0].StoreID == nil ||
		got.ByStore[1].StoreID != nil || got.ByStore[1].Name != report.NoStoreName || got.ByStore[1].Total != 15000 {
		t.Fatalf("по магазинам: %+v", got.ByStore)
	}
	if len(got.ByMonth) != 2 || got.ByMonth[0] != (report.MonthTotal{Month: "2026-09", Total: 238000}) ||
		got.ByMonth[1] != (report.MonthTotal{Month: "2026-10", Total: 15000}) {
		t.Fatalf("по месяцам: %+v", got.ByMonth)
	}

	oct := rptGet[report.Spending](t, c, "spending", "from=2026-10-01&to=2026-10-31")
	if oct.Total != 15000 || len(oct.ByMonth) != 1 || len(oct.ByCategory) != 1 {
		t.Fatalf("октябрь: %+v", oct)
	}
}

func TestReportSpendingEmptyPeriodHasZeroMonths(t *testing.T) {
	c, _, _ := rptSetup(t)
	got := rptGet[report.Spending](t, c, "spending", "from=2025-01-01&to=2025-03-31")
	if got.Total != 0 || len(got.ByCategory) != 0 || len(got.ByStore) != 0 || len(got.ByMonth) != 3 {
		t.Fatalf("пустой период: %+v", got)
	}
}

func TestReportConsumption(t *testing.T) {
	c, p1, p2 := rptSetup(t)
	got := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-01&to=2026-10-31")
	// 100 мл × 119000 / 500 = 23800; 2 шт × 5000 = 10000.
	if got.TotalCost != 33800 || len(got.ByProduct) != 2 {
		t.Fatalf("расход: %+v", got)
	}
	a, b := got.ByProduct[0], got.ByProduct[1]
	if a.ProductID != p1 || a.Qty != 100 || a.Unit != "ml" || a.Cost == nil || *a.Cost != 23800 ||
		b.ProductID != p2 || b.Qty != 2 || b.Unit != "pcs" || b.Cost == nil || *b.Cost != 10000 {
		t.Fatalf("по позициям: %+v", got.ByProduct)
	}
	if len(got.ByMonth) != 1 || got.ByMonth[0] != (report.MonthUse{Month: "2026-10", Cost: 33800, QtyML: 100, QtyPcs: 2}) {
		t.Fatalf("по месяцам: %+v", got.ByMonth)
	}
	// Списание — отдельно, в расход не входит: 50 мл × 119000 / 500 = 11900.
	if got.Writeoffs.Cost != 11900 || len(got.Writeoffs.ByProduct) != 1 || got.Writeoffs.ByProduct[0].Qty != 50 {
		t.Fatalf("списания: %+v", got.Writeoffs)
	}
}

func TestReportConsumptionRevertedAndTimezone(t *testing.T) {
	c, p1, _ := rptSetup(t)
	// Движения 14.10 17:46 UTC; при +12 ч это уже 15.10.
	day14 := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-14&to=2026-10-14")
	day15 := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-15&to=2026-10-15")
	if day14.TotalCost != 33800 || day15.TotalCost != 0 {
		t.Fatalf("UTC: 14-го %d, 15-го %d", day14.TotalCost, day15.TotalCost)
	}
	day14 = rptGet[report.Consumption](t, c, "consumption", "from=2026-10-14&to=2026-10-14&tz=720")
	day15 = rptGet[report.Consumption](t, c, "consumption", "from=2026-10-15&to=2026-10-15&tz=720")
	if day14.TotalCost != 0 || day15.TotalCost != 33800 {
		t.Fatalf("tz=720: 14-го %d, 15-го %d", day14.TotalCost, day15.TotalCost)
	}
	// Отменённый расход из отчёта пропадает.
	m := accConsume(t, c, p1, 10).Movements[0].ID
	before := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-01&to=2026-10-31")
	if before.ByProduct[0].Qty != 110 {
		t.Fatalf("после расхода: %+v", before.ByProduct)
	}
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": []int64{m}}), http.StatusOK, "отмена")
	after := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-01&to=2026-10-31")
	if after.ByProduct[0].Qty != 100 {
		t.Fatalf("после отмены: %+v", after.ByProduct)
	}
}

// Корректировка вниз — отдельный блок «corrections», в расход не входит; вверх не учитывается.
func TestReportConsumptionCorrections(t *testing.T) {
	c, p1, _ := rptSetup(t)
	buy := accBuy(t, c, map[string]any{"date": "2026-10-10", "lines": []any{accLine(p1, 1, 100000)}})
	unit := buy.Purchase.Lines[0].UnitIDs[0]
	adjust := func(qty int64) {
		expect(t, c.do(http.MethodPost, stkPath("/api/units/", unit, "/adjust"), map[string]any{"remaining_qty": qty, "reason": "Инвентаризация"}),
			http.StatusOK, "корректировка")
	}
	adjust(400) // -100 мл: 100 × 100000 / 500 = 20000
	adjust(450) // +50 мл — вверх, не показывается
	got := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-01&to=2026-10-31")
	if got.TotalCost != 33800 || got.Writeoffs.Cost != 11900 {
		t.Fatalf("расход и списания не должны меняться: %+v", got)
	}
	cr := got.Corrections
	if cr.Cost != 20000 || len(cr.ByProduct) != 1 || cr.ByProduct[0].ProductID != p1 || cr.ByProduct[0].Qty != 100 ||
		cr.ByProduct[0].Cost == nil || *cr.ByProduct[0].Cost != 20000 {
		t.Fatalf("корректировки: %+v", cr)
	}
	empty := rptGet[report.Consumption](t, c, "consumption", "from=2026-01-01&to=2026-01-31")
	if empty.Corrections.ByProduct == nil || len(empty.Corrections.ByProduct) != 0 {
		t.Fatalf("пустые корректировки должны быть []: %+v", empty.Corrections)
	}
}

func TestReportConsumptionWithoutPrice(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	p := stkProduct(c, stkCategory(c, "Отчёт: К"), "Без цены", "ml", 500)
	expect(t, c.do(http.MethodPost, "/api/units", map[string]any{"product_id": p, "remaining_qty": 300}), http.StatusCreated, "остаток")
	accConsume(t, c, p, 120)
	got := rptGet[report.Consumption](t, c, "consumption", "from=2026-10-01&to=2026-10-31")
	if len(got.ByProduct) != 1 || got.ByProduct[0].Qty != 120 || got.ByProduct[0].Cost != nil || got.TotalCost != 0 {
		t.Fatalf("без цены: %+v", got)
	}
}

func TestReportValidation(t *testing.T) {
	c, _, _ := rptSetup(t)
	cases := []struct{ name, query, field string }{
		{"нет дат", "", "from"},
		{"нет конца", "from=2026-01-01", "to"},
		{"не дата", "from=вчера&to=2026-01-01", "from"},
		{"31 февраля", "from=2026-02-31&to=2026-03-01", "from"},
		{"конец раньше начала", "from=2026-05-02&to=2026-05-01", "to"},
		{"больше 5 лет", "from=2020-01-01&to=2025-01-02", "to"},
		{"tz не число", "from=2026-01-01&to=2026-01-02&tz=abc", "tz"},
		{"tz вне диапазона", "from=2026-01-01&to=2026-01-02&tz=900", "tz"},
	}
	for _, kind := range []string{"spending", "consumption"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				rec := c.do(http.MethodGet, "/api/reports/"+kind+"?"+tc.query, nil)
				expect(t, rec, http.StatusBadRequest, tc.query)
				if f := decode[errBody](t, rec).Field; f != tc.field {
					t.Fatalf("field %q, ожидался %q", f, tc.field)
				}
			})
		}
	}
	// Ровно 5 лет — можно.
	q := url.Values{"from": {"2021-01-01"}, "to": {"2026-01-01"}}
	expect(t, c.do(http.MethodGet, "/api/reports/spending?"+q.Encode(), nil), http.StatusOK, "5 лет")
	expect(t, e0(t).do(http.MethodGet, "/api/reports/spending?from=2026-01-01&to=2026-01-02", nil), http.StatusUnauthorized, "без входа")
	expect(t, c.do(http.MethodPost, "/api/reports/spending", nil), http.StatusMethodNotAllowed, "метод")
}

func e0(t *testing.T) *client { return newEnv(t, false).client() }
