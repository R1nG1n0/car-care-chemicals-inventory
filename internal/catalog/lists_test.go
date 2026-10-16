package catalog

import (
	"errors"
	"testing"
)

func TestProductsSearchAndFilters(t *testing.T) {
	f := newFixture(t)
	loc, _ := f.svc.CreateLocation(f.ctx, f.u1, "Коробка")
	shampoo := f.product(f.u1, f.cat1, "Шампунь Активный", "Grass", "ml", 1000, ptr[int64](500))
	foam := f.product(f.u1, f.cat1, "Пена", "КОХ Хеми", "ml", 1000, nil)
	cloth := f.product(f.u1, f.cat1, "Микрофибра", "", "pcs", 10, ptr[int64](2))
	f.exec(`UPDATE products SET barcode = '4601234567890' WHERE id = ?`, cloth)
	f.unit(f.u1, shampoo, nil, &loc.ID, 1000, 300, "open", ptr[int64](1))
	f.unit(f.u1, foam, nil, nil, 1000, 1000, "sealed", nil)
	f.product(f.u2, f.cat2, "Шампунь второго", "", "ml", 500, nil)

	names := func(fl ProductFilter) []string {
		t.Helper()
		list, err := f.svc.Products(f.ctx, f.u1, fl)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, p := range list {
			out = append(out, p.Name)
		}
		return out
	}
	eq := func(what string, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: %v, ожидалось %v", what, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: %v, ожидалось %v", what, got, want)
			}
		}
	}
	eq("все по имени", names(ProductFilter{}), "Микрофибра", "Пена", "Шампунь Активный")
	eq("кириллица без регистра", names(ProductFilter{Query: "шАМП"}), "Шампунь Активный")
	eq("бренд кириллицей", names(ProductFilter{Query: "кох"}), "Пена")
	eq("бренд латиницей", names(ProductFilter{Query: "GRASS"}), "Шампунь Активный")
	eq("штрихкод", names(ProductFilter{Query: "4567"}), "Микрофибра")
	eq("low", names(ProductFilter{Filter: FilterLow}), "Микрофибра", "Шампунь Активный")
	eq("out", names(ProductFilter{Filter: FilterOut}), "Микрофибра")
	eq("in", names(ProductFilter{Filter: FilterIn}), "Пена", "Шампунь Активный")
	eq("место", names(ProductFilter{LocationID: loc.ID}), "Шампунь Активный")
	eq("чужая категория", names(ProductFilter{CategoryID: f.cat2}))
	if _, err := f.svc.Products(f.ctx, f.u1, ProductFilter{Filter: "bad"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("неверный фильтр: %v", err)
	}

	// Архив: из обычного списка пропадает, в archived — только он.
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, foam, ProductPatch{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	eq("архивные", names(ProductFilter{Archived: true}), "Пена")
	eq("без архивных", names(ProductFilter{}), "Микрофибра", "Шампунь Активный")

	brands, _ := f.svc.Brands(f.ctx, f.u1)
	if len(brands) != 2 || brands[0] != "Grass" || brands[1] != "КОХ Хеми" {
		t.Fatalf("бренды: %v", brands)
	}

	// Единицу нельзя менять при экземплярах; минимум сбрасывается в null.
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, shampoo, ProductPatch{Unit: ptr("pcs")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("смена единицы: %v", err)
	}
	p, err := f.svc.UpdateProduct(f.ctx, f.u1, shampoo, ProductPatch{MinStockSet: true})
	if err != nil || p.MinStock != nil || p.Low {
		t.Fatalf("сброс минимума: %v %+v", err, p)
	}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, shampoo, ProductPatch{CategoryID: &f.cat2}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая категория: %v", err)
	}
}

func TestRefs(t *testing.T) {
	f := newFixture(t)
	cats, err := f.svc.Categories(f.ctx, f.u1, false)
	if err != nil || len(cats) != 11 || cats[0].Name != "Шампуни" || cats[0].Color != "blue" {
		t.Fatalf("категории по умолчанию: %v %+v", err, cats)
	}
	c, err := f.svc.CreateCategory(f.ctx, f.u1, "  Глины ", "plum")
	if err != nil || c.Name != "Глины" || c.Sort != cats[len(cats)-1].Sort+1 {
		t.Fatalf("новая категория: %v %+v", err, c)
	}
	if _, err := f.svc.CreateCategory(f.ctx, f.u1, "Шампуни", "blue"); !errors.Is(err, ErrConflict) {
		t.Fatalf("дубль: %v", err)
	}
	if _, err := f.svc.CreateCategory(f.ctx, f.u2, "Глины", "plum"); err != nil {
		t.Fatalf("то же имя у второго: %v", err)
	}
	var e *Error
	if _, err := f.svc.CreateCategory(f.ctx, f.u1, "X", "red"); !errors.As(err, &e) || e.Field != "color" {
		t.Fatalf("цвет: %v", err)
	}

	// Порядок: ровно все действующие.
	ids := []int64{c.ID}
	for _, x := range cats {
		ids = append(ids, x.ID)
	}
	if err := f.svc.ReorderCategories(f.ctx, f.u1, ids[:3]); !errors.Is(err, ErrInvalid) {
		t.Fatalf("неполный порядок: %v", err)
	}
	if err := f.svc.ReorderCategories(f.ctx, f.u1, ids); err != nil {
		t.Fatal(err)
	}
	cats, _ = f.svc.Categories(f.ctx, f.u1, false)
	if cats[0].ID != c.ID {
		t.Fatalf("порядок не применился: %+v", cats[0])
	}

	// Место с экземплярами не архивируется.
	loc, _ := f.svc.CreateLocation(f.ctx, f.u1, "Коробка 3")
	p := f.product(f.u1, f.cat1, "Шампунь", "", "ml", 1000, nil)
	u := f.unit(f.u1, p, nil, &loc.ID, 1000, 1000, "sealed", nil)
	_, err = f.svc.UpdateLocation(f.ctx, f.u1, loc.ID, RefPatch{Archived: ptr(true)})
	if !errors.As(err, &e) || !errors.Is(err, ErrConflict) || e.Message != "В месте лежат экземпляры: 1. Сначала переложите их" {
		t.Fatalf("архив места с экземплярами: %v", err)
	}
	f.exec(`UPDATE units SET status = 'finished', remaining_qty = 0 WHERE id = ?`, u)
	l, err := f.svc.UpdateLocation(f.ctx, f.u1, loc.ID, RefPatch{Archived: ptr(true)})
	if err != nil || !l.Archived || l.UnitsCount != 0 {
		t.Fatalf("архив пустого места: %v %+v", err, l)
	}

	st, err := f.svc.CreateStore(f.ctx, f.u1, "Автохим", "https://avtohim.example")
	if err != nil || st.URL != "https://avtohim.example" {
		t.Fatalf("магазин: %v %+v", err, st)
	}
	if _, err := f.svc.CreateStore(f.ctx, f.u1, "Озон", "ftp://x"); !errors.As(err, &e) || e.Field != "url" {
		t.Fatalf("ссылка: %v", err)
	}
	if _, err := f.svc.UpdateStore(f.ctx, f.u2, st.ID, RefPatch{Name: ptr("Чужой")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужой магазин: %v", err)
	}
}

func TestShoppingAutoMark(t *testing.T) {
	f := newFixture(t)
	s1, _ := f.svc.CreateStore(f.ctx, f.u1, "Дорогой", "")
	s2, _ := f.svc.CreateStore(f.ctx, f.u1, "Дешёвый", "")
	p := f.product(f.u1, f.cat1, "Шампунь", "", "ml", 1000, ptr[int64](100))
	// Дешёвый магазин: 500 мл за 50000 (10000 за 100 мл) против 1000 мл за 120000.
	small := f.purchase(f.u1, p, &s2.ID, "2026-09-01", 1, 50000, 1_791_000_000)
	f.unit(f.u1, p, &small, nil, 500, 0, "finished", nil)
	f.purchase(f.u1, p, &s1.ID, "2026-09-10", 1, 120000, 1_791_100_000)

	it, err := f.svc.AddShopping(f.ctx, f.u1, &p, "", 1_792_000_000)
	if err != nil || it.Product == nil || it.Product.ID != p {
		t.Fatalf("пункт с позицией: %v %+v", err, it)
	}
	text, _ := f.svc.AddShopping(f.ctx, f.u1, nil, "Глина для кузова", 1_792_000_001)
	if _, err := f.svc.AddShopping(f.ctx, f.u1, nil, " ", 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("пустой пункт: %v", err)
	}

	list, err := f.svc.Shopping(f.ctx, f.u1, 1_792_000_010)
	if err != nil || len(list.Low) != 1 || len(list.Manual) != 2 || list.Manual[0].ID != text.ID {
		t.Fatalf("список: %v %+v", err, list)
	}
	low := list.Low[0]
	if *low.LastPrice != 120000 || *low.LastStore != "Дорогой" || *low.CheapestStore != "Дешёвый" {
		t.Fatalf("low: %+v", low)
	}
	if list.Manual[1].DoneAt != nil {
		t.Fatal("старые покупки не должны отмечать новый пункт")
	}

	// Покупка после добавления пункта отмечает его.
	f.purchase(f.u1, p, nil, "2026-10-16", 1, 1, 1_792_000_500)
	list, _ = f.svc.Shopping(f.ctx, f.u1, 1_792_000_600)
	if d := list.Manual[1].DoneAt; d == nil || *d != 1_792_000_500 {
		t.Fatalf("автоотметка: %+v", list.Manual[1])
	}
	// Снятая отметка не возвращается при следующем чтении.
	if _, err := f.svc.SetShoppingDone(f.ctx, f.u1, it.ID, false, 1_792_000_700); err != nil {
		t.Fatal(err)
	}
	list, _ = f.svc.Shopping(f.ctx, f.u1, 1_792_000_800)
	if list.Manual[0].ID != it.ID || list.Manual[0].DoneAt != nil {
		t.Fatalf("снятие отметки: %+v", list.Manual)
	}
	// Выполненные старше 7 дней пропадают.
	if _, err := f.svc.SetShoppingDone(f.ctx, f.u1, text.ID, true, 1_792_000_000); err != nil {
		t.Fatal(err)
	}
	list, _ = f.svc.Shopping(f.ctx, f.u1, 1_792_000_000+8*24*3600)
	if len(list.Manual) != 1 {
		t.Fatalf("старый выполненный не скрыт: %+v", list.Manual)
	}
	if err := f.svc.DeleteShopping(f.ctx, f.u2, it.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("удаление чужого: %v", err)
	}
	if _, err := f.svc.AddShopping(f.ctx, f.u2, &p, "", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая позиция в списке: %v", err)
	}
}
