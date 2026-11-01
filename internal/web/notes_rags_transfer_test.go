package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Экспорт и импорт версии 4: заметки, ресурс и тряпки (ФТ-18.5, ФТ-19.6).
func TestTransferV4NotesAndRags(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	cat := stkCategory(a, "Тест")
	ml := stkProduct(a, cat, "Шампунь", "ml", 500)
	rag := ragProduct(a, cat, 50)
	stkBuy(a, rag, 3, 100, nil)
	recipe := stkCreate(a, "/api/recipes", map[string]any{"name": "Мойка",
		"items": []any{map[string]any{"product_id": ml, "qty": 5}}})
	stkCreate(a, "/api/notes", map[string]any{"title": "Про шампунь", "body": "Строка 1\nСтрока 2",
		"product_ids": []int64{ml, rag}, "recipe_ids": []int64{recipe}})
	stkCreate(a, "/api/notes", map[string]any{"title": "Без связей"})
	r1 := ragStart(t, a, rag, map[string]any{}).Rag
	r2 := ragStart(t, a, rag, map[string]any{"label": "Синяя"}).Rag
	ragWash(t, a, "/api/rags/wash", r1.ID, r2.ID)
	ragWash(t, a, "/api/rags/wash", r1.ID)
	expect(t, a.do(http.MethodPost, stkPath("/api/rags/", r2.ID, "/retire"), map[string]any{"reason": "damaged"}), http.StatusOK, "списание")

	file := trExport(t, a)
	if file.Version != 4 || len(file.Notes) != 2 || len(file.Rags) != 2 {
		t.Fatalf("экспорт: версия %d, заметок %d, тряпок %d", file.Version, len(file.Notes), len(file.Rags))
	}
	var wear *int64
	for _, p := range file.Products {
		if p.ID == rag {
			wear = p.WearLimit
		}
	}
	if wear == nil || *wear != 50 || len(file.Notes[0].ProductIDs) != 2 || len(file.Notes[0].RecipeIDs) != 1 ||
		file.Rags[0].Washes != 2 || file.Rags[1].Status != "retired" || *file.Rags[1].RetireReason != "damaged" ||
		file.Rags[0].ConsumeMovementID == nil {
		t.Fatalf("содержимое: %+v %+v", file.Notes, file.Rags)
	}

	b := stkSignIn(e, "friend")
	expect(t, b.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт v4")
	notes := decode[noteList](t, b.do(http.MethodGet, "/api/notes", nil)).Notes
	if len(notes) != 2 {
		t.Fatalf("заметки после импорта: %+v", notes)
	}
	var linked catalog.Note
	for _, n := range notes {
		if n.Title == "Про шампунь" {
			linked = n
		}
	}
	if linked.Body != "Строка 1\nСтрока 2" || len(linked.ProductIDs) != 2 || len(linked.RecipeIDs) != 1 {
		t.Fatalf("связи после импорта: %+v", linked)
	}
	back := trExport(t, b)
	var newRag int64
	for _, p := range back.Products {
		if p.WearLimit != nil {
			newRag = p.ID
		}
	}
	if newRag == 0 || len(back.Rags) != 2 {
		t.Fatalf("после импорта: ресурс %d, тряпок %d", newRag, len(back.Rags))
	}
	all := decode[ragList](t, b.do(http.MethodGet, stkPath("/api/products/", newRag, "/rags?all=1"), nil)).Rags
	if len(all) != 2 || all[0].Washes != 2 || all[0].Level != "ok" || all[1].Label != "Синяя" || all[1].Status != "retired" {
		t.Fatalf("тряпки после импорта: %+v", all)
	}
	// Номер продолжается после импорта.
	stkBuy(b, newRag, 1, 100, nil)
	if s := ragStart(t, b, newRag, map[string]any{}); s.Rag.Number != 3 {
		t.Fatalf("номер после импорта: %d", s.Rag.Number)
	}
}

// Файлы версий 1–3 без заметок, ресурса и тряпок импортируются; плохие данные версии 4 отклоняются.
func TestTransferV4OldVersionsAndValidation(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	cat := stkCategory(a, "Тест")
	rag := ragProduct(a, cat, 10)
	stkBuy(a, rag, 1, 100, nil)
	stkProduct(a, cat, "Шампунь", "ml", 500)
	ragStart(t, a, rag, map[string]any{})
	stkCreate(a, "/api/notes", map[string]any{"title": "Н"})
	raw, _ := json.Marshal(trExport(t, a))

	mut := func(fn func(m map[string]any)) []byte {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		fn(m)
		out, _ := json.Marshal(m)
		return out
	}
	for _, ver := range []int{1, 2, 3} {
		old := mut(func(m map[string]any) {
			m["version"] = ver
			delete(m, "notes")
			delete(m, "rags")
			for _, p := range m["products"].([]any) {
				delete(p.(map[string]any), "wear_limit")
			}
		})
		u := stkSignIn(e, "old"+string(rune('0'+ver)))
		expect(t, trRaw(u, "/api/import", old), http.StatusOK, "импорт старой версии")
		got := trExport(t, u)
		if got.Version != 4 || len(got.Notes) != 0 || len(got.Rags) != 0 {
			t.Fatalf("версия %d: заметок %d, тряпок %d", ver, len(got.Notes), len(got.Rags))
		}
		for _, p := range got.Products {
			if p.WearLimit != nil {
				t.Fatalf("версия %d: ресурс %v", ver, *p.WearLimit)
			}
		}
	}

	bad := map[string]func(m map[string]any){
		"ресурс у позиции в мл": func(m map[string]any) {
			for _, p := range m["products"].([]any) {
				if p.(map[string]any)["unit"] == "ml" {
					p.(map[string]any)["wear_limit"] = 5
				}
			}
		},
		"ресурс 1001": func(m map[string]any) {
			for _, p := range m["products"].([]any) {
				if p.(map[string]any)["unit"] == "pcs" {
					p.(map[string]any)["wear_limit"] = 1001
				}
			}
		},
		"заметка без заголовка": func(m map[string]any) {
			m["notes"].([]any)[0].(map[string]any)["title"] = " "
		},
		"заметка с позицией вне файла": func(m map[string]any) {
			m["notes"].([]any)[0].(map[string]any)["product_ids"] = []int64{98765}
		},
		"заметка с шаблоном вне файла": func(m map[string]any) {
			m["notes"].([]any)[0].(map[string]any)["recipe_ids"] = []int64{98765}
		},
		"тряпка с позицией вне файла": func(m map[string]any) {
			m["rags"].([]any)[0].(map[string]any)["product_id"] = 98765
		},
		"тряпка у позиции в мл": func(m map[string]any) {
			for _, p := range m["products"].([]any) {
				if p.(map[string]any)["unit"] == "ml" {
					m["rags"].([]any)[0].(map[string]any)["product_id"] = p.(map[string]any)["id"]
				}
			}
		},
		"тряпка со статусом": func(m map[string]any) {
			m["rags"].([]any)[0].(map[string]any)["status"] = "lost"
		},
		"тряпка с длинной меткой": func(m map[string]any) {
			m["rags"].([]any)[0].(map[string]any)["label"] = "яяяяяяяяяяяяяяяяяяяяя"
		},
	}
	n := 0
	for name, fn := range bad {
		n++
		u := stkSignIn(e, "bad"+strconv.Itoa(n))
		rec := trRaw(u, "/api/import", mut(fn))
		expect(t, rec, http.StatusBadRequest, name)
		if got := trExport(t, u); len(got.Products) != 0 || len(got.Notes) != 0 {
			t.Fatalf("%s: после отказа есть данные", name)
		}
	}
}
