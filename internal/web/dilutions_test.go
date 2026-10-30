package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

// Пропорции позиции и раствор в расходе (ФТ-17): API целиком.

func dilProduct(t *testing.T, c *client, cat int64, dil any) int64 {
	t.Helper()
	return stkCreate(c, "/api/products", map[string]any{"category_id": cat, "name": "Концентрат", "unit": "ml",
		"package_size": 500, "dilutions": dil})
}

func TestProductDilutionsAPI(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	id := dilProduct(t, c, cat, []any{
		map[string]any{"name": "Предмойка", "ratio": 10}, map[string]any{"name": "Салон", "ratio": 20}})

	detail := decode[struct {
		Product catalog.ProductCard `json:"product"`
	}](t, c.do(http.MethodGet, stkPath("/api/products/", id, ""), nil))
	d := detail.Product.Dilutions
	if len(d) != 2 || d[0].Name != "Предмойка" || d[0].Ratio != 10 || d[1].Sort != 1 || d[0].ID == 0 {
		t.Fatalf("карточка: %+v", d)
	}
	list := decode[catProducts](t, c.do(http.MethodGet, "/api/products", nil)).Products
	if len(list) != 1 || len(list[0].Dilutions) != 2 {
		t.Fatalf("список: %+v", list)
	}
	// В сыром JSON поле всегда массив.
	if body := c.do(http.MethodGet, "/api/products", nil).Body.String(); !strings.Contains(body, `"dilutions":[{"id"`) {
		t.Fatalf("JSON списка: %s", body)
	}

	// Полная замена в переданном порядке; id сохраняется; без поля — без изменений.
	expect(t, c.do(http.MethodPatch, stkPath("/api/products/", id, ""), map[string]any{"note": "x"}), http.StatusOK, "без dilutions")
	rec := c.do(http.MethodPatch, stkPath("/api/products/", id, ""), map[string]any{"dilutions": []any{
		map[string]any{"name": "Колёса", "ratio": 5}, map[string]any{"id": d[0].ID, "name": "Предмойка", "ratio": 15}}})
	expect(t, rec, http.StatusOK, "замена")
	got := decode[catalog.ProductCard](t, rec).Dilutions
	if len(got) != 2 || got[0].Name != "Колёса" || got[1].ID != d[0].ID || got[1].Ratio != 15 {
		t.Fatalf("после замены: %+v", got)
	}
	got = decode[catalog.ProductCard](t, c.do(http.MethodPatch, stkPath("/api/products/", id, ""), map[string]any{"dilutions": []any{}})).Dilutions
	if got == nil || len(got) != 0 {
		t.Fatalf("очистка: %+v", got)
	}

	// Ошибки: поле dilutions.
	pcs := stkProduct(c, cat, "Салфетки", "pcs", 10)
	many := make([]any, 11)
	for i := range many {
		many[i] = map[string]any{"name": "П", "ratio": 2}
	}
	for name, body := range map[string]any{
		"11":          many,
		"имя":         []any{map[string]any{"name": "", "ratio": 2}},
		"пропорция 0": []any{map[string]any{"name": "А", "ratio": 0}},
		"1001":        []any{map[string]any{"name": "А", "ratio": 1001}},
	} {
		rec := c.do(http.MethodPatch, stkPath("/api/products/", id, ""), map[string]any{"dilutions": body})
		expect(t, rec, http.StatusBadRequest, name)
		if f := decode[struct {
			Field string `json:"field"`
		}](t, rec).Field; f != "dilutions" {
			t.Fatalf("%s: поле %q", name, f)
		}
	}
	rec = c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"dilutions": []any{map[string]any{"name": "А", "ratio": 2}}})
	expect(t, rec, http.StatusBadRequest, "шт с пропорциями")
	rec = c.do(http.MethodPost, "/api/products", map[string]any{"category_id": cat, "name": "Тряпка", "unit": "pcs",
		"package_size": 1, "dilutions": []any{map[string]any{"name": "А", "ratio": 2}}})
	expect(t, rec, http.StatusBadRequest, "создание шт с пропорциями")
}

func TestConsumeSolutionAPI(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := stkProduct(c, cat, "Концентрат", "ml", 100)
	pcs := stkProduct(c, cat, "Салфетки", "pcs", 10)
	stkBuy(c, p, 2, 10000, nil)
	stkBuy(c, pcs, 1, 1000, nil)

	rec := c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p,
		"solution": map[string]any{"volume_ml": 1000, "ratio": 10}})
	expect(t, rec, http.StatusCreated, "расход раствором")
	res := decode[stkConsumeBody](t, rec)
	if res.Solution == nil || res.Solution.Concentrate != 91 || res.Solution.VolumeML != 1000 || res.Solution.Ratio != 10 {
		t.Fatalf("solution: %+v", res.Solution)
	}
	if len(res.Movements) != 1 || res.Movements[0].QtyDelta != -91 || res.Movements[0].SolutionML == nil ||
		*res.Movements[0].SolutionML != 1000 || *res.Movements[0].SolutionRatio != 10 {
		t.Fatalf("движения: %+v", res.Movements)
	}
	if res.Product.Stock.Total != 109 {
		t.Fatalf("остаток %d", res.Product.Stock.Total)
	}

	// Обычный расход: solution null, поля раствора null.
	rec = c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 5})
	expect(t, rec, http.StatusCreated, "обычный")
	if body := rec.Body.String(); !strings.Contains(body, `"solution":null`) || !strings.Contains(body, `"solution_ml":null`) {
		t.Fatalf("обычный расход: %s", body)
	}

	// Расход растянулся на две бутылки: оба движения несут раствор.
	rec = c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p,
		"solution": map[string]any{"volume_ml": 1100, "ratio": 10}})
	expect(t, rec, http.StatusCreated, "две бутылки")
	two := decode[stkConsumeBody](t, rec)
	if len(two.Movements) != 2 {
		t.Fatalf("движений %d", len(two.Movements))
	}
	for _, m := range two.Movements {
		if m.SolutionML == nil || *m.SolutionML != 1100 || *m.SolutionRatio != 10 {
			t.Fatalf("раствор движения: %+v", m)
		}
	}

	// Журнал позиции показывает раствор; отмена работает как раньше.
	j := decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, c.do(http.MethodGet, stkPath("/api/movements?product_id=", p, ""), nil)).Movements
	var withSol int
	for _, m := range j {
		if m.SolutionML != nil {
			withSol++
		}
	}
	if withSol != 3 {
		t.Fatalf("в журнале движений с раствором %d, ожидалось 3", withSol)
	}
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(two.Movements)}), http.StatusOK, "отмена")
	detail := decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, c.do(http.MethodGet, stkPath("/api/products/", p, ""), nil))
	withSol = 0
	for _, m := range detail.Movements {
		if m.SolutionML != nil {
			withSol++
		}
	}
	if withSol != 1 {
		t.Fatalf("в карточке после отмены движений с раствором %d", withSol)
	}

	// Ошибки.
	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"шт", map[string]any{"product_id": pcs, "solution": map[string]any{"volume_ml": 100, "ratio": 10}}, "solution"},
		{"оба", map[string]any{"product_id": p, "qty": 5, "solution": map[string]any{"volume_ml": 100, "ratio": 10}}, "qty"},
		{"ни одного", map[string]any{"product_id": p}, "qty"},
		{"объём 0", map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 0, "ratio": 10}}, "solution"},
		{"объём 100001", map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 100001, "ratio": 10}}, "solution"},
		{"пропорция 0", map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 100, "ratio": 0}}, "solution"},
		{"пропорция 1001", map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 100, "ratio": 1001}}, "solution"},
	} {
		rec := c.do(http.MethodPost, "/api/consume", tc.body)
		expect(t, rec, http.StatusBadRequest, tc.name)
		if f := decode[struct {
			Field string `json:"field"`
		}](t, rec).Field; f != tc.field {
			t.Errorf("%s: поле %q, ожидалось %q", tc.name, f, tc.field)
		}
	}
}

func TestConsumeSolutionIdempotentAndOccurredAt(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := stkProduct(c, cat, "Концентрат", "ml", 500)
	stkBuy(c, p, 1, 10000, nil)
	at := e.clock.Now().Add(-time.Hour).Unix()
	body := map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 1000, "ratio": 10},
		"client_op_id": "op-solution-0001", "occurred_at": at}
	first := c.do(http.MethodPost, "/api/consume", body)
	expect(t, first, http.StatusCreated, "первый")
	again := c.do(http.MethodPost, "/api/consume", body)
	expect(t, again, http.StatusOK, "повтор")
	if again.Header().Get("X-Garage-Replayed") != "1" || again.Body.String() != first.Body.String() {
		t.Fatalf("повтор не совпал:\n%s\n%s", first.Body.String(), again.Body.String())
	}
	res := decode[stkConsumeBody](t, first)
	if res.Solution == nil || res.Solution.Concentrate != 91 || res.Movements[0].CreatedAt != at || res.Product.Stock.Total != 409 {
		t.Fatalf("итог: %+v", res)
	}
}

func TestRecipeSolutionAPI(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := stkProduct(c, cat, "Концентрат", "ml", 500)
	pcs := stkProduct(c, cat, "Салфетки", "pcs", 10)
	stkBuy(c, p, 1, 10000, nil)
	stkBuy(c, pcs, 1, 1000, nil)

	rid := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка", "items": []any{
		map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 1000, "ratio": 10}},
		map[string]any{"product_id": pcs, "qty": 2}}})
	type item struct {
		Qty      int64 `json:"qty"`
		Solution *struct {
			VolumeML int64 `json:"volume_ml"`
			Ratio    int64 `json:"ratio"`
		} `json:"solution"`
	}
	r := decode[struct {
		Recipe struct {
			Items []item `json:"items"`
		} `json:"recipe"`
	}](t, c.do(http.MethodGet, stkPath("/api/recipes/", rid, ""), nil)).Recipe
	if r.Items[0].Qty != 91 || r.Items[0].Solution == nil || r.Items[0].Solution.VolumeML != 1000 || r.Items[1].Solution != nil {
		t.Fatalf("шаблон: %+v", r.Items)
	}

	// Применение: строка с раствором пересчитывается, qty игнорируется.
	rec := c.do(http.MethodPost, stkPath("/api/recipes/", rid, "/apply"), map[string]any{"items": []any{
		map[string]any{"product_id": p, "qty": 1, "solution": map[string]any{"volume_ml": 500, "ratio": 1}},
		map[string]any{"product_id": pcs, "qty": 1}}})
	expect(t, rec, http.StatusCreated, "применение")
	ap := decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, rec)
	if len(ap.Movements) != 2 || ap.Movements[0].QtyDelta != -250 || *ap.Movements[0].SolutionML != 500 || ap.Movements[1].SolutionML != nil {
		t.Fatalf("движения: %+v", ap.Movements)
	}

	// Ошибки: шт с раствором — 400 solution.
	rec = c.do(http.MethodPost, "/api/recipes", map[string]any{"name": "Плохой", "items": []any{
		map[string]any{"product_id": pcs, "solution": map[string]any{"volume_ml": 100, "ratio": 2}}}})
	expect(t, rec, http.StatusBadRequest, "шт с раствором в шаблоне")
}

func TestTransferV3AndOldVersions(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	cat := stkCategory(a, "Тест")
	p := dilProduct(t, a, cat, []any{map[string]any{"name": "Салон", "ratio": 20}, map[string]any{"name": "Кузов", "ratio": 3}})
	stkBuy(a, p, 1, 10000, nil)
	expect(t, a.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p,
		"solution": map[string]any{"volume_ml": 1000, "ratio": 10}}), http.StatusCreated, "расход")
	stkCreate(a, "/api/recipes", map[string]any{"name": "Р", "items": []any{
		map[string]any{"product_id": p, "solution": map[string]any{"volume_ml": 500, "ratio": 4}}}})

	file := trExport(t, a)
	if file.Version != 3 || len(file.Products[0].Dilutions) != 2 || file.Products[0].Dilutions[1].Name != "Кузов" ||
		file.Movements[len(file.Movements)-1].SolutionML == nil || file.Recipes[0].Items[0].SolutionRatio == nil {
		t.Fatalf("экспорт v3: %+v", file)
	}

	b := stkSignIn(e, "friend")
	expect(t, b.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт v3")
	back := trExport(t, b)
	if len(back.Products[0].Dilutions) != 2 || back.Products[0].Dilutions[0].Ratio != 20 {
		t.Fatalf("пропорции после импорта: %+v", back.Products[0].Dilutions)
	}
	var withSol int
	for _, m := range back.Movements {
		if m.SolutionML != nil && *m.SolutionML == 1000 && *m.SolutionRatio == 10 {
			withSol++
		}
	}
	if withSol != 1 || *back.Recipes[0].Items[0].SolutionML != 500 || back.Recipes[0].Items[0].Qty != 100 {
		t.Fatalf("раствор после импорта: движений %d, строка %+v", withSol, back.Recipes[0].Items[0])
	}
	csv := b.do(http.MethodGet, "/api/export/movements.csv?tz=0", nil).Body.String()
	if !strings.Contains(csv, ";Стоимость, руб;Раствор, мл;Пропорция 1:N") || !strings.Contains(csv, ";1000;10") {
		t.Fatalf("movements.csv:\n%s", csv)
	}

	// Файлы версий 1 и 2: новых полей нет — импортируются без них.
	raw, _ := json.Marshal(file)
	for _, ver := range []int{1, 2} {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["version"] = ver
		for _, pr := range m["products"].([]any) {
			delete(pr.(map[string]any), "dilutions")
		}
		for _, mv := range m["movements"].([]any) {
			delete(mv.(map[string]any), "solution_ml")
			delete(mv.(map[string]any), "solution_ratio")
		}
		for _, rc := range m["recipes"].([]any) {
			for _, it := range rc.(map[string]any)["items"].([]any) {
				delete(it.(map[string]any), "solution_ml")
				delete(it.(map[string]any), "solution_ratio")
			}
		}
		old, _ := json.Marshal(m)
		u := stkSignIn(e, "old"+string(rune('0'+ver)))
		expect(t, trRaw(u, "/api/import", old), http.StatusOK, "импорт старой версии")
		got := trExport(t, u)
		if got.Version != 3 || len(got.Products[0].Dilutions) != 0 || got.Movements[len(got.Movements)-1].SolutionML != nil {
			t.Fatalf("версия %d после импорта: %+v", ver, got.Products[0])
		}
	}

	// Проверка диапазонов в файле.
	n := 0
	for name, mut := range map[string]func(*transfer.File){
		"пропорция 0":      func(f *transfer.File) { f.Products[0].Dilutions[0].Ratio = 0 },
		"пропорция 1001":   func(f *transfer.File) { f.Products[0].Dilutions[0].Ratio = 1001 },
		"имя пустое":       func(f *transfer.File) { f.Products[0].Dilutions[0].Name = " " },
		"объём в движении": func(f *transfer.File) { v := int64(100001); f.Movements[len(f.Movements)-1].SolutionML = &v },
		"только объём":     func(f *transfer.File) { f.Movements[len(f.Movements)-1].SolutionRatio = nil },
		"пропорция строки": func(f *transfer.File) { v := int64(0); f.Recipes[0].Items[0].SolutionRatio = &v },
		"раствор без пары": func(f *transfer.File) { f.Recipes[0].Items[0].SolutionML = nil },
	} {
		var f transfer.File
		_ = json.Unmarshal(raw, &f)
		mut(&f)
		n++
		u := stkSignIn(e, fmt.Sprintf("chk%d", n))
		expect(t, u.do(http.MethodPost, "/api/import", f), http.StatusBadRequest, name)
	}
}
