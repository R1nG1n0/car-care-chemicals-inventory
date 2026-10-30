package catalog

import (
	"errors"
	"strings"
	"testing"
)

func dilNames(p ProductSummary) []string {
	out := []string{}
	for _, d := range p.Dilutions {
		out = append(out, d.Name)
	}
	return out
}

func TestProductDilutions(t *testing.T) {
	f := newFixture(t)
	in := ProductInput{CategoryID: f.cat1, Name: "Концентрат", Unit: "ml", PackageSize: 1000,
		Dilutions: []DilutionInput{{Name: " Предмойка ", Ratio: 10}, {Name: "Салон", Ratio: 20}}}
	p, err := f.svc.CreateProduct(f.ctx, f.u1, in, 1_792_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if got := dilNames(p); len(got) != 2 || got[0] != "Предмойка" || got[1] != "Салон" || p.Dilutions[1].Ratio != 20 || p.Dilutions[1].Sort != 1 {
		t.Fatalf("создание: %+v", p.Dilutions)
	}

	// Не передан — без изменений; в списке тоже есть.
	p2, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{Note: strPtr("x")})
	if err != nil || len(p2.Dilutions) != 2 {
		t.Fatalf("без изменений: %+v %v", p2.Dilutions, err)
	}
	list, err := f.svc.Products(f.ctx, f.u1, ProductFilter{})
	if err != nil || len(list) != 1 || len(list[0].Dilutions) != 2 {
		t.Fatalf("список: %+v %v", list, err)
	}

	// Полная замена в новом порядке; id сохраняется у переданных.
	salon, pre := p.Dilutions[1].ID, p.Dilutions[0].ID
	repl := []DilutionInput{{ID: salon, Name: "Салон", Ratio: 25}, {Name: "Колёса", Ratio: 5}}
	p3, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{Dilutions: &repl})
	if err != nil {
		t.Fatal(err)
	}
	if got := dilNames(p3); len(got) != 2 || got[0] != "Салон" || got[1] != "Колёса" || p3.Dilutions[0].ID != salon || p3.Dilutions[0].Ratio != 25 {
		t.Fatalf("замена: %+v", p3.Dilutions)
	}
	var n int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM product_dilutions WHERE id = ?`, pre).Scan(&n)
	if n != 0 {
		t.Fatal("старая пропорция не удалена")
	}

	// Пустой список — удалить все.
	empty := []DilutionInput{}
	p4, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{Dilutions: &empty})
	if err != nil || len(p4.Dilutions) != 0 || p4.Dilutions == nil {
		t.Fatalf("очистка: %+v %v", p4.Dilutions, err)
	}

	// Проверки ввода.
	many := make([]DilutionInput, 11)
	for i := range many {
		many[i] = DilutionInput{Name: "П", Ratio: 2}
	}
	for name, list := range map[string][]DilutionInput{
		"11 пропорций":   many,
		"пустое имя":     {{Name: "  ", Ratio: 2}},
		"длинное имя":    {{Name: strings.Repeat("я", 41), Ratio: 2}},
		"пропорция 0":    {{Name: "А", Ratio: 0}},
		"пропорция 1001": {{Name: "А", Ratio: 1001}},
	} {
		l := list
		_, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{Dilutions: &l})
		var ce *Error
		if !errors.As(err, &ce) || ce.Kind != ErrInvalid || ce.Field != "dilutions" {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Позиция в шт: пропорции — 400.
	_, err = f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Салфетки", Unit: "pcs",
		PackageSize: 10, Dilutions: []DilutionInput{{Name: "А", Ratio: 2}}}, 1)
	var ce *Error
	if !errors.As(err, &ce) || ce.Field != "dilutions" {
		t.Fatalf("шт: %v", err)
	}
	pcs, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Салфетки", Unit: "pcs", PackageSize: 10}, 1)
	if err != nil || pcs.Dilutions == nil || len(pcs.Dilutions) != 0 {
		t.Fatalf("шт без пропорций: %+v %v", pcs.Dilutions, err)
	}
	one := []DilutionInput{{Name: "А", Ratio: 2}}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, pcs.ID, ProductPatch{Dilutions: &one}); !errors.As(err, &ce) || ce.Field != "dilutions" {
		t.Fatalf("PATCH шт: %v", err)
	}

	// Смена единицы на шт (экземпляров нет) без списка снимает пропорции.
	ml, _ := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Ещё", Unit: "ml", PackageSize: 100,
		Dilutions: one}, 1)
	got, err := f.svc.UpdateProduct(f.ctx, f.u1, ml.ID, ProductPatch{Unit: strPtr("pcs")})
	if err != nil || len(got.Dilutions) != 0 {
		t.Fatalf("смена единицы: %+v %v", got.Dilutions, err)
	}
}

func TestDilutionsIsolation(t *testing.T) {
	f := newFixture(t)
	a, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Чужой", Unit: "ml", PackageSize: 100,
		Dilutions: []DilutionInput{{Name: "Тайная", Ratio: 7}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.CreateProduct(f.ctx, f.u2, ProductInput{CategoryID: f.cat2, Name: "Свой", Unit: "ml", PackageSize: 100}, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Чужой id пропорции в списке своей позиции — 404, ничего не меняется.
	l := []DilutionInput{{ID: a.Dilutions[0].ID, Name: "Взлом", Ratio: 2}}
	_, err = f.svc.UpdateProduct(f.ctx, f.u2, b.ID, ProductPatch{Dilutions: &l})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая пропорция: %v", err)
	}
	// Чужая пропорция той же позиции другого пользователя не меняется.
	l = []DilutionInput{{Name: "Взлом", Ratio: 2}}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u2, a.ID, ProductPatch{Dilutions: &l}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая позиция: %v", err)
	}
	got, _ := f.svc.Summary(f.ctx, f.u1, a.ID)
	if len(got.Dilutions) != 1 || got.Dilutions[0].Name != "Тайная" {
		t.Fatalf("у владельца: %+v", got.Dilutions)
	}
	// Удаление позиции убирает пропорции.
	if _, err := f.db.Exec(`DELETE FROM products WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM product_dilutions`).Scan(&n)
	if n != 0 {
		t.Fatalf("пропорций после удаления позиции: %d", n)
	}
}

func strPtr(s string) *string { return &s }
