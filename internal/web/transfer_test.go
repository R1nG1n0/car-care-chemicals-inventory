package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

// Экспорт и импорт (ФТ-10.3): адреса, формат файла, изоляция.

// trRaw отправляет сырое тело (битый файл, огромный файл) от имени клиента.
func trRaw(c *client, target string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	req.RemoteAddr = c.remoteAddr
	req.Header.Set("X-Requested-With", "garage")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.token})
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	return rec
}

func trGet(t *testing.T, c *client, target string) string {
	t.Helper()
	rec := c.do(http.MethodGet, target, nil)
	expect(t, rec, http.StatusOK, target)
	return rec.Body.String()
}

func trExport(t *testing.T, c *client) transfer.File {
	t.Helper()
	return decode[transfer.File](t, c.do(http.MethodGet, "/api/export.json", nil))
}

// trSeed наполняет склад пользователя: места, магазин, покупки, расход,
// шаблон с применением, перемещение, списание, список покупок, шаги кнопок.
func trSeed(t *testing.T, c *client) {
	t.Helper()
	cat := catFirstCategory(t, c)
	shelf := stkLocation(c, "Полка 1")
	box := stkLocation(c, "Коробка")
	shop := stkCreate(c, "/api/stores", map[string]any{"name": "Авто-Мир", "url": "https://example.com/shop"})
	foam := catProduct(t, c, map[string]any{"category_id": cat, "name": "Пена активная", "brand": "Grass",
		"unit": "ml", "package_size": 1000, "min_stock": 500, "barcode": "4600000000011", "note": "Для мойки"}).ID
	cloth := catProduct(t, c, map[string]any{"category_id": cat, "name": "Салфетка полировочная", "unit": "pcs", "package_size": 1}).ID
	// v1.5: свой порядок и архив назначений, свойства, оценка (ФТ-15.6.1).
	purposes := decode[purposeList](t, c.do(http.MethodGet, "/api/purposes", nil)).Purposes
	extra := stkCreate(c, "/api/purposes", map[string]any{"name": "Рога и копыта"})
	expect(t, c.do(http.MethodPatch, stkPath("/api/products/", foam, ""), map[string]any{
		"purpose_ids": []int64{purposes[0].ID, purposes[2].ID, extra}, "ph": 5.5, "incompatible": "с хлором",
		"instruction": "Разбавить 1:10", "rating": 4}), http.StatusOK, "свойства позиции")
	// Архивное назначение остаётся у позиции и уезжает в файл.
	expect(t, c.do(http.MethodPatch, stkPath("/api/purposes/", purposes[2].ID, ""), map[string]any{"archived": true}),
		http.StatusOK, "архив назначения")
	expect(t, c.do(http.MethodPatch, stkPath("/api/products/", cloth, "/rating"), map[string]any{"do_not_buy": true}),
		http.StatusOK, "не брать больше")

	buy := func(date string, store any, lines ...any) {
		expect(t, c.do(http.MethodPost, "/api/purchases", map[string]any{"date": date, "store_id": store,
			"note": "Заметка " + date, "lines": lines}), http.StatusCreated, "покупка")
	}
	l := accLine(foam, 3, 119050)
	l["location_id"] = shelf
	buy("2026-09-14", shop, l, accLine(cloth, 5, 15000))
	buy("2026-10-02", nil, accLine(foam, 1, 99000))

	accConsume(t, c, foam, 250)
	accConsume(t, c, foam, 900)
	rid := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка кузова",
		"items": []any{map[string]any{"product_id": foam, "qty": 100}, map[string]any{"product_id": cloth, "qty": 1}}})
	expect(t, c.do(http.MethodPost, stkPath("/api/recipes/", rid, "/apply"), map[string]any{"items": []any{
		map[string]any{"product_id": foam, "qty": 100}, map[string]any{"product_id": cloth, "qty": 1}}}),
		http.StatusCreated, "применение шаблона")

	units := decode[catDetail](t, c.do(http.MethodGet, stkPath("/api/products/", cloth, ""), nil)).Units
	expect(t, c.do(http.MethodPost, stkPath("/api/units/", units[0].ID, "/move"), map[string]any{"location_id": box}),
		http.StatusOK, "перемещение")
	expect(t, c.do(http.MethodPost, stkPath("/api/units/", units[1].ID, "/writeoff"), map[string]any{"reason": "Порвана"}),
		http.StatusOK, "списание")
	stkCreate(c, "/api/shopping", map[string]any{"title": "Глина для кузова"})
	expect(t, c.do(http.MethodPut, "/api/auth/settings", map[string]any{"step_small": 5, "step_large": 25}),
		http.StatusOK, "шаги")
}

// trBatchShape — группировка движений: для каждого движения индекс первого в его группе.
func trBatchShape(t *testing.T, f transfer.File) []int {
	t.Helper()
	pos := map[int64]int{}
	for i, m := range f.Movements {
		pos[m.ID] = i
	}
	out := make([]int, len(f.Movements))
	for i, m := range f.Movements {
		if m.BatchID == nil {
			t.Fatalf("движение %d без batch_id", i)
		}
		p, ok := pos[*m.BatchID]
		if !ok {
			t.Fatalf("движение %d: batch_id %d вне файла", i, *m.BatchID)
		}
		out[i] = p
	}
	return out
}

func TestTransferRoundTrip(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)

	rec := a.do(http.MethodGet, "/api/export.json", nil)
	expect(t, rec, http.StatusOK, "экспорт")
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="garage-owner-2026-10-14.json"` {
		t.Fatalf("Content-Disposition: %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type: %q", ct)
	}
	fileA := decode[transfer.File](t, rec)
	if fileA.Format != "garage-export" || fileA.Version != 2 || fileA.User.Login != "owner" || fileA.ExportedAt == "" {
		t.Fatalf("шапка файла: %+v", fileA)
	}
	if len(fileA.Products) != 2 || len(fileA.Purchases) != 2 || len(fileA.Movements) < 8 || len(fileA.Recipes) != 1 ||
		len(fileA.RecipeRuns) != 1 || len(fileA.Shopping) != 1 || len(fileA.Locations) != 2 || len(fileA.Stores) != 1 ||
		len(fileA.Units) == 0 || fileA.Settings == nil || fileA.Settings.StepLarge != 25 {
		t.Fatalf("состав файла: %+v", transfer.Count(&fileA))
	}
	for _, secret := range []string{"password", "token", "session", "invite"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), secret) {
			t.Fatalf("в экспорте есть %q", secret)
		}
	}

	// Другу: импорт в пустой склад (со стандартными категориями).
	b := stkSignIn(e, "friend")
	imp := b.do(http.MethodPost, "/api/import", fileA)
	expect(t, imp, http.StatusOK, "импорт")
	got := decode[struct {
		Imported transfer.Counts `json:"imported"`
	}](t, imp).Imported
	if want := transfer.Count(&fileA); got != want {
		t.Fatalf("счётчики %+v, ожидались %+v", got, want)
	}

	for _, target := range []string{"/api/export/products.csv", "/api/export/purchases.csv", "/api/export/movements.csv"} {
		if x, y := trGet(t, a, target), trGet(t, b, target); x != y {
			t.Fatalf("%s различается:\n%s\n---\n%s", target, x, y)
		}
	}
	fileB := trExport(t, b)
	if x, y := transfer.Count(&fileA), transfer.Count(&fileB); x != y {
		t.Fatalf("состав после импорта: %+v и %+v", x, y)
	}
	if !reflect.DeepEqual(trBatchShape(t, fileA), trBatchShape(t, fileB)) {
		t.Fatalf("группы движений не совпали: %v и %v", trBatchShape(t, fileA), trBatchShape(t, fileB))
	}
	if !reflect.DeepEqual(fileA.Purposes, fileB.Purposes) {
		// id в базе другие: сравниваем по именам, порядку и архиву.
		names := func(f transfer.File) []string {
			var out []string
			for _, p := range f.Purposes {
				out = append(out, fmt.Sprintf("%s/%d/%v", p.Name, p.Sort, p.Archived))
			}
			return out
		}
		if !reflect.DeepEqual(names(fileA), names(fileB)) {
			t.Fatalf("назначения: %v и %v", names(fileA), names(fileB))
		}
	}
	for i := range fileA.Products {
		x, y := fileA.Products[i], fileB.Products[i]
		if len(x.PurposeIDs) != len(y.PurposeIDs) || !reflect.DeepEqual(x.PH, y.PH) || x.Incompatible != y.Incompatible ||
			x.Instruction != y.Instruction || !reflect.DeepEqual(x.Rating, y.Rating) || x.DoNotBuy != y.DoNotBuy {
			t.Fatalf("поля версии 2 позиции %q: %+v и %+v", x.Name, x, y)
		}
	}
	if foam := fileA.Products[0]; len(foam.PurposeIDs) != 3 || foam.PH == nil || *foam.PH != 5.5 || foam.Rating == nil ||
		*foam.Rating != 4 || !fileA.Products[1].DoNotBuy {
		t.Fatalf("экспорт не содержит новых полей: %+v", fileA.Products)
	}
	if len(fileB.Categories) != len(fileA.Categories) {
		t.Fatalf("категории слились неверно: %d и %d", len(fileB.Categories), len(fileA.Categories))
	}
	if fileB.Settings == nil || *fileB.Settings != *fileA.Settings {
		t.Fatalf("шаги кнопок: %+v", fileB.Settings)
	}
	for _, target := range []string{"/api/shopping", "/api/recipes"} {
		x, y := trGet(t, a, target), trGet(t, b, target)
		if norm(x) != norm(y) {
			t.Fatalf("%s различается:\n%s\n---\n%s", target, x, y)
		}
	}
	// Остатки и журнал одной позиции — те же, что у владельца.
	pa, pb := accListed(t, a, ""), accListed(t, b, "")
	if len(pa) != len(pb) {
		t.Fatalf("позиции: %d и %d", len(pa), len(pb))
	}
	for i := range pa {
		if pa[i].Name != pb[i].Name || !reflect.DeepEqual(pa[i].Stock, pb[i].Stock) || pa[i].Low != pb[i].Low {
			t.Fatalf("позиция %d: %+v и %+v", i, pa[i], pb[i])
		}
	}
	// Данные владельца не тронуты.
	if after := trExport(t, a); !reflect.DeepEqual(after, fileA) {
		t.Fatal("импорт изменил данные владельца")
	}
}

// norm убирает из JSON id и служебные числа, оставляя тексты и количества.
func norm(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	var strip func(any) any
	strip = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			for k, val := range t {
				if k == "id" || strings.HasSuffix(k, "_id") || strings.HasSuffix(k, "_ids") || k == "created_at" || k == "last_used_at" {
					delete(t, k)
					continue
				}
				t[k] = strip(val)
			}
		case []any:
			for i := range t {
				t[i] = strip(t[i])
			}
		}
		return x
	}
	out, _ := json.Marshal(strip(v))
	return string(out)
}

func TestImportRefusesNonEmptyWarehouse(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	file := trExport(t, a)

	for name, prep := range map[string]func(c *client){
		"product": func(c *client) { stkProduct(c, catFirstCategory(t, c), "Своя", "ml", 100) },
		"purchase": func(c *client) {
			p := stkProduct(c, catFirstCategory(t, c), "Своя", "ml", 100)
			stkBuy(c, p, 1, 100, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := stkSignIn(e, "friend-"+name)
			prep(b)
			before := trGet(t, b, "/api/export/products.csv")
			rec := b.do(http.MethodPost, "/api/import", file)
			expect(t, rec, http.StatusConflict, "импорт в непустой склад")
			if msg := decode[errBody](t, rec).Error; !strings.Contains(msg, "пустой склад") {
				t.Fatalf("текст отказа: %q", msg)
			}
			if trGet(t, b, "/api/export/products.csv") != before {
				t.Fatal("после отказа данные изменились")
			}
		})
	}
	// Одни только свои места и магазины — склад пуст, импорт идёт.
	c := stkSignIn(e, "third")
	stkLocation(c, "Полка 1")
	expect(t, c.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт при своих местах")
	if n := len(trExport(t, c).Locations); n != 2 {
		t.Fatalf("места: %d, ожидалось 2 (слияние по имени)", n)
	}
	// Повторный импорт — уже непустой склад.
	expect(t, c.do(http.MethodPost, "/api/import", file), http.StatusConflict, "повторный импорт")
}

func TestImportRejectsBadFiles(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	good := trExport(t, a)
	raw, _ := json.Marshal(good)

	mutate := func(fn func(m map[string]any)) []byte {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		fn(m)
		out, _ := json.Marshal(m)
		return out
	}
	item := func(m map[string]any, list string, i int) map[string]any {
		return m[list].([]any)[i].(map[string]any)
	}
	cases := []struct {
		name  string
		body  []byte
		code  int
		field string
	}{
		{"не JSON", []byte("это не json"), 400, ""},
		{"пустой файл", nil, 400, ""},
		{"JSON с хвостом", append(append([]byte{}, raw...), []byte(" {}")...), 400, ""},
		{"неверный тип поля", mutate(func(m map[string]any) { item(m, "products", 0)["name"] = 5 }), 400, "products.name"},
		{"чужой формат", mutate(func(m map[string]any) { m["format"] = "other" }), 400, "format"},
		{"версия 3", mutate(func(m map[string]any) { m["version"] = 3 }), 400, "version"},
		{"без версии", mutate(func(m map[string]any) { delete(m, "version") }), 400, "version"},
		{"пустое название позиции", mutate(func(m map[string]any) { item(m, "products", 1)["name"] = "  " }), 400, "products[1].name"},
		{"слишком длинное название", mutate(func(m map[string]any) { item(m, "products", 0)["name"] = strings.Repeat("я", 81) }), 400, "products[0].name"},
		{"единица", mutate(func(m map[string]any) { item(m, "products", 0)["unit"] = "kg" }), 400, "products[0].unit"},
		{"категория вне файла", mutate(func(m map[string]any) { item(m, "products", 0)["category_id"] = 987654 }), 400, "products[0].category_id"},
		{"позиция строки покупки вне файла", mutate(func(m map[string]any) {
			item(m, "purchases", 0)["lines"].([]any)[0].(map[string]any)["product_id"] = 987654
		}), 400, "purchases[0].lines[0].product_id"},
		{"магазин вне файла", mutate(func(m map[string]any) { item(m, "purchases", 0)["store_id"] = 987654 }), 400, "purchases[0].store_id"},
		{"плохая дата покупки", mutate(func(m map[string]any) { item(m, "purchases", 1)["date"] = "02.10.2026" }), 400, "purchases[1].date"},
		{"цена вне диапазона", mutate(func(m map[string]any) {
			item(m, "purchases", 0)["lines"].([]any)[0].(map[string]any)["price_per_package"] = -1
		}), 400, "purchases[0].lines[0].price_per_package"},
		{"экземпляр чужой позиции", mutate(func(m map[string]any) { item(m, "units", 0)["product_id"] = 987654 }), 400, "units[0].product_id"},
		{"остаток больше объёма", mutate(func(m map[string]any) {
			u := item(m, "units", 0)
			u["remaining_qty"] = u["initial_qty"].(float64) + 1
		}), 400, "units[0].remaining_qty"},
		{"движение чужого экземпляра", mutate(func(m map[string]any) { item(m, "movements", 0)["unit_id"] = 987654 }), 400, "movements[0].unit_id"},
		{"неизвестный тип движения", mutate(func(m map[string]any) { item(m, "movements", 0)["type"] = "steal" }), 400, "movements[0].type"},
		{"повтор id", mutate(func(m map[string]any) { item(m, "products", 1)["id"] = item(m, "products", 0)["id"] }), 400, "products[1].id"},
		{"повтор названия категории", mutate(func(m map[string]any) {
			item(m, "categories", 1)["name"] = strings.ToUpper(item(m, "categories", 0)["name"].(string))
		}), 400, "categories[1].name"},
		{"цвет", mutate(func(m map[string]any) { item(m, "categories", 0)["color"] = "pink" }), 400, "categories[0].color"},
		{"шаблон с позицией вне файла", mutate(func(m map[string]any) {
			item(m, "recipes", 0)["items"].([]any)[0].(map[string]any)["product_id"] = 987654
		}), 400, "recipes[0].items[0].product_id"},
		{"шаги кнопок", mutate(func(m map[string]any) { m["settings"] = map[string]any{"step_small": 50, "step_large": 5} }), 400, "settings.step_large"},
	}
	b := stkSignIn(e, "friend")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := trRaw(b, "/api/import", tc.body)
			expect(t, rec, tc.code, tc.name)
			got := decode[errBody](t, rec)
			if got.Error == "" || got.Field != tc.field {
				t.Fatalf("ошибка %+v, ожидалось поле %q", got, tc.field)
			}
			if tc.field != "" && !strings.Contains(got.Error, tc.field) && !strings.HasPrefix(tc.field, "products.") {
				t.Fatalf("в тексте %q нет места %q", got.Error, tc.field)
			}
		})
	}
	// Ни одна из неудач ничего не записала: корректный файл всё ещё импортируется.
	if n := len(trExport(t, b).Products); n != 0 {
		t.Fatalf("после отказов в складе %d позиций", n)
	}
	expect(t, b.do(http.MethodPost, "/api/import", good), http.StatusOK, "корректный файл после отказов")
}

// Необязательное фото позиции не ломает импорт ни с ним, ни без него (ФТ-10.4).
func TestImportOptionalPhoto(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	file := trExport(t, a)
	expect(t, stkSignIn(e, "friend").do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт без фото")
	file.Products[0].Photo = nil
	expect(t, stkSignIn(e, "friend2").do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт без поля photo")
}

func TestImportSizeLimitOnlyOnImport(t *testing.T) {
	e := newEnv(t, false)
	b := stkSignIn(e, "friend")
	big := []byte(`{"format":"garage-export","version":1,"pad":"` + strings.Repeat("a", 101<<20) + `"}`)
	rec := trRaw(b, "/api/import", big)
	expect(t, rec, http.StatusRequestEntityTooLarge, "файл больше 100 МБ")
	if msg := decode[errBody](t, rec).Error; !strings.Contains(msg, "100 МБ") {
		t.Fatalf("текст: %q", msg)
	}
	// Файл крупнее обычного предела, но меньше 100 МБ, проходит чтение (отказ — по содержанию).
	mid := []byte(`{"format":"garage-export","version":1,"pad":"` + strings.Repeat("a", 5<<20) + `"}`)
	if rec := trRaw(b, "/api/import", mid); rec.Code != http.StatusOK {
		t.Fatalf("5 МБ: код %d, %s", rec.Code, rec.Body.String())
	}
	// Остальные адреса по-прежнему принимают только крошечные тела.
	huge := []byte(`{"name":"` + strings.Repeat("a", 100<<10) + `","color":"blue"}`)
	rec = trRaw(b, "/api/categories", huge)
	expect(t, rec, http.StatusRequestEntityTooLarge, "общий предел тела")
}

func TestCSVFormatAndInjection(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	cat := catFirstCategory(t, a)
	evil := []string{"=1+1", "+7", "-2+3", "@SUM(A1)"}
	var first int64
	for i, name := range evil {
		id := catProduct(t, a, map[string]any{"category_id": cat, "name": name, "brand": "=cmd|' /C calc'!A0",
			"unit": "ml", "package_size": 100, "note": "-note"}).ID
		if i == 0 {
			first = id
		}
	}
	store := stkCreate(a, "/api/stores", map[string]any{"name": "@Магазин", "url": ""})
	expect(t, a.do(http.MethodPost, "/api/purchases", map[string]any{"date": "2026-03-05", "store_id": store,
		"note": "=ШАГ", "lines": []any{accLine(first, 2, 119000)}}), http.StatusCreated, "покупка")
	accConsume(t, a, first, 30)

	rec := a.do(http.MethodGet, "/api/export/products.csv", nil)
	expect(t, rec, http.StatusOK, "products.csv")
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="garage-owner-products-2026-10-14.csv"` {
		t.Fatalf("Content-Disposition: %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type: %q", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "\xEF\xBB\xBFНазвание;Бренд;") {
		t.Fatalf("нет BOM или заголовка: %q", body[:40])
	}
	for _, cell := range []string{"'=1+1;", "'+7;", "'-2+3;", "'@SUM(A1);", "'=cmd|"} {
		if !strings.Contains(body, cell) {
			t.Fatalf("в products.csv нет %q:\n%s", cell, body)
		}
	}
	for _, line := range strings.Split(body, "\r\n")[1:] {
		for _, cell := range strings.Split(line, ";") {
			if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
				t.Fatalf("ячейка %q начинается с опасного символа: %s", cell, line)
			}
		}
	}

	pur := trGet(t, a, "/api/export/purchases.csv")
	want := "05.03.2026;'@Магазин;'=1+1;'=cmd|' /C calc'!A0;2;1190,00;2380,00;'=ШАГ"
	if !strings.Contains(pur, want) || !strings.HasPrefix(pur, "\xEF\xBB\xBFДата;Магазин;Позиция;") {
		t.Fatalf("purchases.csv:\n%s", pur)
	}
	mov := a.do(http.MethodGet, "/api/export/movements.csv?tz=180", nil)
	expect(t, mov, http.StatusOK, "movements.csv")
	m := mov.Body.String()
	// 1_792_000_000 = 2026-10-14 21:46 UTC → 00:46 11.10 в МСК; расход отрицателен и не экранирован.
	if !strings.Contains(m, "14.10.2026 20:46;Расход;'=1+1;") || !strings.Contains(m, ";-30;мл;") {
		t.Fatalf("movements.csv:\n%s", m)
	}
	if !strings.Contains(m, ";Покупка;'=1+1;") {
		t.Fatalf("movements.csv, покупка:\n%s", m)
	}
	expect(t, a.do(http.MethodGet, "/api/export/movements.csv?tz=abc", nil), http.StatusBadRequest, "tz")
}

func TestExportIsolationAndAccess(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	b := stkSignIn(e, "friend")
	pb := stkProduct(b, catFirstCategory(t, b), "Свой шампунь", "ml", 100)
	stkBuy(b, pb, 1, 500, nil)

	targets := []string{"/api/export.json", "/api/export/products.csv", "/api/export/purchases.csv", "/api/export/movements.csv"}
	for _, target := range targets {
		body := trGet(t, b, target)
		for _, leak := range []string{"Пена активная", "Салфетка полировочная", "Авто-Мир", "Полка 1", "Мойка кузова", "Глина", "Порвана", "Grass"} {
			if strings.Contains(body, leak) {
				t.Fatalf("%s у второго содержит чужое %q", target, leak)
			}
		}
		if !strings.Contains(body, "Свой шампунь") {
			t.Fatalf("%s у второго не содержит своего", target)
		}
		expect(t, e.client().do(http.MethodGet, target, nil), http.StatusUnauthorized, "без входа "+target)
		expect(t, b.do(http.MethodPost, target, nil), http.StatusMethodNotAllowed, "POST "+target)
	}
	if f := trExport(t, b); f.User.Login != "friend" || len(f.Products) != 1 {
		t.Fatalf("экспорт второго: %+v", transfer.Count(&f))
	}
	// Импорт без входа и без CSRF-заголовка отклоняется.
	expect(t, e.client().do(http.MethodPost, "/api/import", map[string]any{}), http.StatusUnauthorized, "импорт без входа")
	noCSRF := e.client()
	noCSRF.login("owner", "password-owner")
	noCSRF.noCSRF = true
	expect(t, noCSRF.do(http.MethodPost, "/api/import", map[string]any{}), http.StatusForbidden, "импорт без CSRF")
	expect(t, a.do(http.MethodGet, "/api/import", nil), http.StatusMethodNotAllowed, "GET /api/import")
}

// Файл версии 1 (без назначений и новых полей) импортируется: стартовые
// назначения пользователя остаются, у позиций новые поля пусты.
func TestImportVersion1File(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	raw, _ := json.Marshal(trExport(t, a))
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["version"] = 1
	delete(m, "purposes")
	for _, p := range m["products"].([]any) {
		for _, k := range []string{"purpose_ids", "ph", "incompatible", "instruction", "rating", "do_not_buy"} {
			delete(p.(map[string]any), k)
		}
	}
	old, _ := json.Marshal(m)

	b := stkSignIn(e, "friend")
	expect(t, trRaw(b, "/api/import", old), http.StatusOK, "импорт файла версии 1")
	got := trExport(t, b)
	if len(got.Purposes) != 7 || got.Version != 2 {
		t.Fatalf("назначения после импорта v1: %+v", got.Purposes)
	}
	for _, p := range got.Products {
		if len(p.PurposeIDs) != 0 || p.PH != nil || p.Rating != nil || p.DoNotBuy || p.Incompatible != "" || p.Instruction != "" {
			t.Fatalf("у позиции %q новые поля не пусты: %+v", p.Name, p)
		}
	}
}

// Новые поля файла проверяются так же, как ввод через API.
func TestImportRejectsBadV2Fields(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	raw, _ := json.Marshal(trExport(t, a))
	mutate := func(fn func(p, m map[string]any)) []byte {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		fn(m["products"].([]any)[0].(map[string]any), m)
		out, _ := json.Marshal(m)
		return out
	}
	b := stkSignIn(e, "friend")
	for name, tc := range map[string]struct {
		body  []byte
		field string
	}{
		"pH 15":           {mutate(func(p, _ map[string]any) { p["ph"] = 15 }), "products[0].ph"},
		"pH два знака":    {mutate(func(p, _ map[string]any) { p["ph"] = 7.25 }), "products[0].ph"},
		"оценка 6":        {mutate(func(p, _ map[string]any) { p["rating"] = 6 }), "products[0].rating"},
		"длинная инстр.":  {mutate(func(p, _ map[string]any) { p["instruction"] = strings.Repeat("я", 2001) }), "products[0].instruction"},
		"длинное несовм.": {mutate(func(p, _ map[string]any) { p["incompatible"] = strings.Repeat("я", 301) }), "products[0].incompatible"},
		"назначение вне файла": {mutate(func(p, _ map[string]any) { p["purpose_ids"] = []int{987654} }),
			"products[0].purpose_ids[0]"},
		"повтор назначения": {mutate(func(p, m map[string]any) {
			id := m["purposes"].([]any)[0].(map[string]any)["id"]
			p["purpose_ids"] = []any{id, id}
		}), "products[0].purpose_ids[1]"},
		"повтор имени назначения": {mutate(func(_, m map[string]any) {
			m["purposes"].([]any)[1].(map[string]any)["name"] = strings.ToUpper(m["purposes"].([]any)[0].(map[string]any)["name"].(string))
		}), "purposes[1].name"},
	} {
		rec := trRaw(b, "/api/import", tc.body)
		expect(t, rec, http.StatusBadRequest, name)
		if got := decode[errBody](t, rec); got.Field != tc.field {
			t.Fatalf("%s: поле %q, ожидалось %q (%s)", name, got.Field, tc.field, got.Error)
		}
	}
}

// CSV позиций: в конце колонки «Назначения», «pH», «Оценка», «Не брать больше».
func TestCSVProductsNewColumns(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	cat := catFirstCategory(t, a)
	leather, glass := purposeByName(t, a, "кожа"), purposeByName(t, a, "стекло")
	catProduct(t, a, map[string]any{"category_id": cat, "name": "Крем", "unit": "ml", "package_size": 100,
		"purpose_ids": []int64{leather, glass}, "ph": 5.5, "rating": 4, "do_not_buy": true})
	catProduct(t, a, map[string]any{"category_id": cat, "name": "Голая", "unit": "ml", "package_size": 100})

	body := trGet(t, a, "/api/export/products.csv")
	lines := strings.Split(strings.TrimSpace(body), "\r\n")
	if !strings.HasSuffix(lines[0], ";В архиве;Назначения;pH;Оценка;Не брать больше") {
		t.Fatalf("заголовок: %q", lines[0])
	}
	var cream, bare string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "Крем;") {
			cream = l
		}
		if strings.HasPrefix(l, "Голая;") {
			bare = l
		}
	}
	if !strings.HasSuffix(cream, ";нет;стекло, кожа;5,5;4;да") {
		t.Fatalf("строка с полями: %q", cream)
	}
	if !strings.HasSuffix(bare, ";нет;;;;нет") {
		t.Fatalf("строка без полей: %q", bare)
	}
}
