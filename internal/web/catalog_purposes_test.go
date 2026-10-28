package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Назначения, свойства позиции, оценка и аналоги (ФТ-15).

type purposeList struct {
	Purposes []catalog.Purpose `json:"purposes"`
}

type analogList struct {
	Analogs []catalog.ProductSummary `json:"analogs"`
}

func purposeByName(t *testing.T, c *client, name string) int64 {
	t.Helper()
	for _, p := range decode[purposeList](t, c.do(http.MethodGet, "/api/purposes?archived=1", nil)).Purposes {
		if p.Name == name {
			return p.ID
		}
	}
	t.Fatalf("нет назначения %q", name)
	return 0
}

func TestCatPurposesHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")

	list := decode[purposeList](t, c.do(http.MethodGet, "/api/purposes", nil)).Purposes
	if len(list) != 7 || list[0].Name != "ЛКП" || list[6].Name != "текстиль" || list[0].ProductsCount != 0 {
		t.Fatalf("стартовый набор: %+v", list)
	}
	rec := c.do(http.MethodPost, "/api/purposes", map[string]any{"name": "Резина"})
	expect(t, rec, http.StatusCreated, "новое назначение")
	rubber := decode[catalog.Purpose](t, rec)
	if rubber.Sort <= list[6].Sort || rubber.Archived {
		t.Fatalf("новое не в конце: %+v", rubber)
	}
	catExpectField(t, c, http.MethodPost, "/api/purposes", map[string]any{"name": ""}, 400, "name")
	catExpectField(t, c, http.MethodPost, "/api/purposes", map[string]any{"name": strings.Repeat("я", 41)}, 400, "name")
	catExpectField(t, c, http.MethodPost, "/api/purposes", map[string]any{"name": "лкп"}, 409, "name")
	expect(t, c.do(http.MethodPost, "/api/purposes", "{"), http.StatusBadRequest, "битый JSON")

	target := fmt.Sprintf("/api/purposes/%d", rubber.ID)
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"name": "Стекло"}, 409, "name")
	rec = c.do(http.MethodPatch, target, map[string]any{"name": "Резина и шины", "archived": true})
	expect(t, rec, http.StatusOK, "правка")
	if got := decode[catalog.Purpose](t, rec); got.Name != "Резина и шины" || !got.Archived {
		t.Fatalf("правка: %+v", got)
	}
	if n := len(decode[purposeList](t, c.do(http.MethodGet, "/api/purposes", nil)).Purposes); n != 7 {
		t.Fatalf("архивное в списке действующих: %d", n)
	}
	if n := len(decode[purposeList](t, c.do(http.MethodGet, "/api/purposes?archived=1", nil)).Purposes); n != 8 {
		t.Fatalf("archived=1: %d", n)
	}
	expect(t, c.do(http.MethodPatch, "/api/purposes/abc", map[string]any{}), http.StatusNotFound, "id не число")
	expect(t, c.do(http.MethodPatch, "/api/purposes/999999", map[string]any{"name": "x"}), http.StatusNotFound, "нет такого")

	ids := []int64{}
	for i := len(list) - 1; i >= 0; i-- {
		ids = append(ids, list[i].ID)
	}
	catExpectField(t, c, http.MethodPost, "/api/purposes/reorder", map[string]any{"ids": ids[1:]}, 400, "ids")
	expect(t, c.do(http.MethodPost, "/api/purposes/reorder", map[string]any{"ids": ids}), http.StatusNoContent, "порядок")
	if got := decode[purposeList](t, c.do(http.MethodGet, "/api/purposes", nil)).Purposes; got[0].Name != "текстиль" {
		t.Fatalf("порядок не применился: %+v", got[0])
	}
	// Новый пользователь получает такой же стартовый набор, у первого он свой.
	d := e.catLogin("friend")
	if got := decode[purposeList](t, d.do(http.MethodGet, "/api/purposes", nil)).Purposes; len(got) != 7 || got[0].Name != "ЛКП" {
		t.Fatalf("набор нового пользователя: %+v", got)
	}
}

func TestCatProductNewFieldsHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	leather, glass := purposeByName(t, c, "кожа"), purposeByName(t, c, "стекло")

	rec := c.do(http.MethodPost, "/api/products", map[string]any{"category_id": cat, "name": "Крем", "unit": "ml",
		"package_size": 250, "purpose_ids": []int64{leather, glass}, "ph": 7.3, "incompatible": "с кислотой",
		"instruction": "Тонким слоем", "rating": 4, "do_not_buy": false})
	expect(t, rec, http.StatusCreated, "создание")
	p := decode[struct {
		catalog.ProductCard
	}](t, rec).ProductCard
	if len(p.PurposeIDs) != 2 || p.PH == nil || *p.PH != 7.3 || p.Rating == nil || *p.Rating != 4 || p.DoNotBuy ||
		p.Incompatible != "с кислотой" || p.Instruction != "Тонким слоем" {
		t.Fatalf("ответ создания: %+v", p)
	}
	target := fmt.Sprintf("/api/products/%d", p.ID)
	// Карточка: те же поля внутри product.
	d := decode[struct {
		Product catalog.ProductCard `json:"product"`
	}](t, c.do(http.MethodGet, target, nil))
	if d.Product.Instruction != "Тонким слоем" || d.Product.PH == nil {
		t.Fatalf("карточка: %+v", d.Product)
	}
	// В списке — сводка без длинных текстов.
	body := c.do(http.MethodGet, "/api/products", nil).Body.String()
	if strings.Contains(body, "Тонким слоем") || strings.Contains(body, "incompatible") ||
		!strings.Contains(body, `"purpose_ids"`) || !strings.Contains(body, `"ph":7.3`) {
		t.Fatalf("список позиций: %s", body)
	}
	// Частичная правка, снятие pH и оценки null-ом, очистка назначений.
	rec = c.do(http.MethodPatch, target, map[string]any{"ph": nil, "rating": nil, "purpose_ids": []int64{}})
	expect(t, rec, http.StatusOK, "сброс")
	if q := decode[catalog.ProductCard](t, rec); q.PH != nil || q.Rating != nil || len(q.PurposeIDs) != 0 || q.Instruction == "" {
		t.Fatalf("после сброса: %+v", q)
	}
	// Ошибки: коды и поля.
	for _, tc := range []struct {
		body  map[string]any
		code  int
		field string
	}{
		{map[string]any{"ph": 14.5}, 400, "ph"},
		{map[string]any{"ph": -1}, 400, "ph"},
		{map[string]any{"ph": 7.25}, 400, "ph"},
		{map[string]any{"ph": "кислый"}, 400, "ph"},
		{map[string]any{"rating": 0}, 400, "rating"},
		{map[string]any{"rating": 6}, 400, "rating"},
		{map[string]any{"rating": "много"}, 400, "rating"},
		{map[string]any{"incompatible": strings.Repeat("я", 301)}, 400, "incompatible"},
		{map[string]any{"instruction": strings.Repeat("я", 2001)}, 400, "instruction"},
		{map[string]any{"purpose_ids": []int64{999999}}, 404, ""},
	} {
		rec := c.do(http.MethodPatch, target, tc.body)
		expect(t, rec, tc.code, fmt.Sprintf("%v", tc.body))
		if got := decode[errBody](t, rec); got.Field != tc.field || got.Error == "" {
			t.Fatalf("%v: ошибка %+v", tc.body, got)
		}
	}
	// Архивное назначение: новой позиции нельзя (400), у старой остаётся.
	expect(t, c.do(http.MethodPatch, fmt.Sprintf("/api/purposes/%d", glass), map[string]any{"archived": true}), http.StatusOK, "архив")
	catExpectField(t, c, http.MethodPost, "/api/products", map[string]any{"category_id": cat, "name": "N", "unit": "ml",
		"package_size": 1, "purpose_ids": []int64{glass}}, 400, "purpose_ids")
}

func TestCatRatingEndpointHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	p := catProduct(t, c, map[string]any{"category_id": catFirstCategory(t, c), "name": "Шампунь", "unit": "ml", "package_size": 500})
	target := fmt.Sprintf("/api/products/%d/rating", p.ID)

	rec := c.do(http.MethodPatch, target, map[string]any{"rating": 5})
	expect(t, rec, http.StatusOK, "оценка")
	if got := decode[catalog.ProductCard](t, rec); got.Rating == nil || *got.Rating != 5 || got.DoNotBuy {
		t.Fatalf("оценка: %+v", got)
	}
	// Только пометка — оценка остаётся.
	rec = c.do(http.MethodPatch, target, map[string]any{"do_not_buy": true})
	if got := decode[catalog.ProductCard](t, rec); got.Rating == nil || *got.Rating != 5 || !got.DoNotBuy {
		t.Fatalf("пометка: %+v", got)
	}
	// Снять оценку и пометку.
	rec = c.do(http.MethodPatch, target, map[string]any{"rating": nil, "do_not_buy": false})
	if got := decode[catalog.ProductCard](t, rec); got.Rating != nil || got.DoNotBuy {
		t.Fatalf("снятие: %+v", got)
	}
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"rating": 6}, 400, "rating")
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"rating": 0}, 400, "rating")
	catExpectField(t, c, http.MethodPatch, target, map[string]any{"rating": "x"}, 400, "rating")
	expect(t, c.do(http.MethodPatch, "/api/products/999999/rating", map[string]any{"rating": 3}), http.StatusNotFound, "нет позиции")
	expect(t, c.do(http.MethodPatch, "/api/products/abc/rating", map[string]any{"rating": 3}), http.StatusNotFound, "id не число")
	expect(t, c.do(http.MethodPatch, target, "{"), http.StatusBadRequest, "битый JSON")
}

func TestCatSearchByPurposeHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	leather, glass := purposeByName(t, c, "кожа"), purposeByName(t, c, "стекло")
	mk := func(name string, purposes ...int64) {
		catProduct(t, c, map[string]any{"category_id": cat, "name": name, "unit": "ml", "package_size": 100, "purpose_ids": purposes})
	}
	mk("Крем", leather)
	mk("Окна", glass)
	mk("Универсал", leather, glass)
	mk("Шампунь")

	if got := catNames(t, c, "?q=кожа"); strings.Join(got, ",") != "Крем,Универсал" {
		t.Fatalf("q=кожа: %v", got)
	}
	if got := catNames(t, c, fmt.Sprintf("?purpose_id=%d", glass)); strings.Join(got, ",") != "Окна,Универсал" {
		t.Fatalf("purpose_id=стекло: %v", got)
	}
	if got := catNames(t, c, fmt.Sprintf("?purpose_id=%d&q=крем", leather)); strings.Join(got, ",") != "Крем" {
		t.Fatalf("purpose_id + q: %v", got)
	}
	catExpectField(t, c, http.MethodGet, "/api/products?purpose_id=abc", nil, 400, "purpose_id")
}

func TestCatAnalogsAndShoppingHTTP(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("ringino")
	cat := catFirstCategory(t, c)
	leather := purposeByName(t, c, "кожа")
	mk := func(name string, min any, extra map[string]any) catalog.ProductSummary {
		body := map[string]any{"category_id": cat, "name": name, "unit": "ml", "package_size": 500,
			"purpose_ids": []int64{leather}, "min_stock": min}
		for k, v := range extra {
			body[k] = v
		}
		return catProduct(t, c, body)
	}
	low := mk("Кондиционер", 800, nil)
	home := mk("Крем", nil, map[string]any{"rating": 5})
	hidden := mk("Старый крем", 800, map[string]any{"do_not_buy": true})
	e.catPurchase(low.ID, nil, "2026-10-01", 100000, 1)
	e.catPurchase(home.ID, nil, "2026-10-01", 100000, 1)
	e.catPurchase(hidden.ID, nil, "2026-10-01", 100000, 1)

	got := decode[analogList](t, c.do(http.MethodGet, fmt.Sprintf("/api/products/%d/analogs", low.ID), nil)).Analogs
	if len(got) != 1 || got[0].ID != home.ID || got[0].Stock.Total != 500 {
		t.Fatalf("аналоги: %+v", got)
	}
	if body := c.do(http.MethodGet, fmt.Sprintf("/api/products/%d/analogs", home.ID), nil).Body.String(); !strings.Contains(body, `"analogs":[{`) {
		t.Fatalf("аналоги крема: %s", body)
	}
	expect(t, c.do(http.MethodGet, "/api/products/999999/analogs", nil), http.StatusNotFound, "нет позиции")

	// Список покупок: помеченная позиция не в low; у остальных — analogs.
	list := decode[shoppingWithAnalogs](t, c.do(http.MethodGet, "/api/shopping", nil))
	if len(list.Low) != 1 || list.Low[0].Product.ID != low.ID || len(list.Low[0].Analogs) != 1 ||
		list.Low[0].Analogs[0].ID != home.ID || list.Low[0].Analogs[0].Total != 500 || list.Low[0].Analogs[0].Name != "Крем" {
		t.Fatalf("список покупок: %+v", list)
	}
	// Фильтр «мало» на главной по-прежнему показывает помеченную: low честный.
	if got := catNames(t, c, "?filter=low"); strings.Join(got, ",") != "Кондиционер,Старый крем" {
		t.Fatalf("filter=low: %v", got)
	}
}

type shoppingWithAnalogs struct {
	Low []struct {
		Product catalog.ProductSummary `json:"product"`
		Analogs []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Brand string `json:"brand"`
			Unit  string `json:"unit"`
			Total int64  `json:"total"`
		} `json:"analogs"`
	} `json:"low"`
}
