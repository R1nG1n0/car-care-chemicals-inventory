package web

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

type catLocUnits struct {
	Units []catalog.InventoryUnit `json:"units"`
}

// ФТ-10.1: экземпляры места и отметка «проверено».
func TestCatInventoryHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	locA := decode[catalog.Location](t, c.do(http.MethodPost, "/api/locations", map[string]any{"name": "Полка"})).ID
	locB := decode[catalog.Location](t, c.do(http.MethodPost, "/api/locations", map[string]any{"name": "Коробка"})).ID
	pB := catProduct(t, c, map[string]any{"category_id": cat, "name": "Бета", "unit": "ml", "package_size": 500})
	pA := catProduct(t, c, map[string]any{"category_id": cat, "name": "Альфа", "brand": "Koch", "unit": "ml", "package_size": 1000})
	pOld := catProduct(t, c, map[string]any{"category_id": cat, "name": "Старая", "unit": "pcs", "package_size": 1})

	e.catUnit(pB.ID, locA, 500, 500, "sealed", nil)
	open := e.catUnit(pA.ID, locA, 1000, 320, "open", 1_791_000_000)
	e.catUnit(pA.ID, locA, 1000, 0, "finished", nil)
	e.catUnit(pA.ID, locB, 1000, 1000, "sealed", nil)
	e.catUnit(pOld.ID, locA, 1, 1, "sealed", nil)
	e.catExec(`UPDATE products SET archived = 1 WHERE id = ?`, pOld.ID)

	rec := c.do(http.MethodGet, fmt.Sprintf("/api/locations/%d/units", locA), nil)
	expect(t, rec, http.StatusOK, "экземпляры места")
	got := decode[catLocUnits](t, rec).Units
	if len(got) != 2 || got[0].ID != open || got[0].Product.Name != "Альфа" || got[0].Product.Brand != "Koch" ||
		got[0].Product.Unit != "ml" || got[0].Product.PackageSize != 1000 || got[0].RemainingQty != 320 ||
		got[1].Product.Name != "Бета" || got[1].Product.ID != pB.ID {
		t.Fatalf("экземпляры места: %+v", got)
	}

	// Отметка: сначала null, потом время сервера; в списке мест тоже.
	list := decode[catLocations](t, c.do(http.MethodGet, "/api/locations", nil)).Locations
	for _, l := range list {
		if l.CheckedAt != nil {
			t.Fatalf("checked_at до проверки: %+v", l)
		}
	}
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/locations/%d/checked", locA), nil)
	expect(t, rec, http.StatusOK, "отметка проверки")
	l := decode[catalog.Location](t, rec)
	if l.CheckedAt == nil || *l.CheckedAt != 1_792_000_000 || l.ID != locA {
		t.Fatalf("отметка: %+v", l)
	}
	for _, l := range decode[catLocations](t, c.do(http.MethodGet, "/api/locations", nil)).Locations {
		if (l.ID == locA) != (l.CheckedAt != nil) {
			t.Fatalf("checked_at в списке: %+v", l)
		}
	}

	// Архивное место: проверять нельзя (409); несуществующее и не число — 404.
	e.catExec(`UPDATE locations SET archived = 1 WHERE id = ?`, locB)
	expect(t, c.do(http.MethodPost, fmt.Sprintf("/api/locations/%d/checked", locB), nil), http.StatusConflict, "архивное место")
	expect(t, c.do(http.MethodPost, "/api/locations/99999/checked", nil), http.StatusNotFound, "нет места")
	expect(t, c.do(http.MethodGet, "/api/locations/x/units", nil), http.StatusNotFound, "id не число")
	expect(t, c.do(http.MethodGet, "/api/locations/99999/units", nil), http.StatusNotFound, "нет места")
	expect(t, c.do(http.MethodGet, fmt.Sprintf("/api/locations/%d/checked", locA), nil), http.StatusMethodNotAllowed, "GET checked")
}
