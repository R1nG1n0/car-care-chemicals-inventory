package catalog

import (
	"errors"
	"reflect"
	"testing"
)

// purposeID — id назначения пользователя по имени (из стартового набора).
func (f *catFixture) purposeID(user int64, name string) int64 {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRow(`SELECT id FROM purposes WHERE user_id = ? AND name = ?`, user, name).Scan(&id); err != nil {
		f.t.Fatalf("назначение %q: %v", name, err)
	}
	return id
}

// pproduct — позиция с назначениями, остатком stock (0 — нет экземпляров) и оценкой.
func (f *catFixture) pproduct(name string, purposes []int64, stock int64, rating *int64) int64 {
	f.t.Helper()
	p, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: name, Unit: "ml",
		PackageSize: 1000, PurposeIDs: purposes, Rating: rating}, 1_792_000_000)
	if err != nil {
		f.t.Fatal(err)
	}
	if stock > 0 {
		f.unit(f.u1, p.ID, nil, nil, 1000, stock, "sealed", nil)
	}
	return p.ID
}

func ids(list []ProductSummary) []int64 {
	out := []int64{}
	for _, p := range list {
		out = append(out, p.ID)
	}
	return out
}

func TestPurposesCRUDAndSeed(t *testing.T) {
	f := newFixture(t)
	list, err := f.svc.Purposes(f.ctx, f.u1, false)
	if err != nil || len(list) != 7 || list[0].Name != "ЛКП" || list[6].Name != "текстиль" {
		t.Fatalf("стартовый набор: %+v %v", list, err)
	}
	p, err := f.svc.CreatePurpose(f.ctx, f.u1, "  Резина ")
	if err != nil || p.Name != "Резина" || p.Sort != 8 {
		t.Fatalf("создание: %+v %v", p, err)
	}
	// Дубль без учёта регистра — и для кириллицы.
	for _, name := range []string{"резина", "ЛКП", "лкп", "СТЕКЛО"} {
		if _, err := f.svc.CreatePurpose(f.ctx, f.u1, name); !errors.Is(err, ErrConflict) {
			t.Fatalf("дубль %q: %v", name, err)
		}
	}
	for _, name := range []string{"", "   ", string(make([]rune, 41))} {
		if _, err := f.svc.CreatePurpose(f.ctx, f.u1, name); !errors.Is(err, ErrInvalid) {
			t.Fatalf("имя %q: %v", name, err)
		}
	}
	// У второго пользователя то же имя свободно.
	if _, err := f.svc.CreatePurpose(f.ctx, f.u2, "Резина"); err != nil {
		t.Fatal(err)
	}
	// Переименование и архив; из архива — в конец.
	name := "Колёсные диски"
	disks := f.purposeID(f.u1, "диски")
	if u, err := f.svc.UpdatePurpose(f.ctx, f.u1, disks, RefPatch{Name: &name}); err != nil || u.Name != name {
		t.Fatalf("переименование: %+v %v", u, err)
	}
	if _, err := f.svc.UpdatePurpose(f.ctx, f.u1, disks, RefPatch{Name: ptr("лкп")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("переименование в дубль: %v", err)
	}
	if u, err := f.svc.UpdatePurpose(f.ctx, f.u1, disks, RefPatch{Archived: ptr(true)}); err != nil || !u.Archived {
		t.Fatalf("архив: %+v %v", u, err)
	}
	if act, _ := f.svc.Purposes(f.ctx, f.u1, false); len(act) != 7 {
		t.Fatalf("действующих %d, ожидалось 7", len(act))
	}
	if all, _ := f.svc.Purposes(f.ctx, f.u1, true); len(all) != 8 {
		t.Fatalf("с архивом %d, ожидалось 8", len(all))
	}
	if u, err := f.svc.UpdatePurpose(f.ctx, f.u1, disks, RefPatch{Archived: ptr(false)}); err != nil || u.Sort <= p.Sort {
		t.Fatalf("возврат из архива: %+v %v", u, err)
	}
	// Порядок: ровно все действующие.
	act, _ := f.svc.Purposes(f.ctx, f.u1, false)
	rev := []int64{}
	for i := len(act) - 1; i >= 0; i-- {
		rev = append(rev, act[i].ID)
	}
	if err := f.svc.ReorderPurposes(f.ctx, f.u1, rev); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.svc.Purposes(f.ctx, f.u1, false); got[0].ID != rev[0] {
		t.Fatalf("порядок не применился: %+v", got)
	}
	if err := f.svc.ReorderPurposes(f.ctx, f.u1, rev[1:]); !errors.Is(err, ErrInvalid) {
		t.Fatalf("неполный порядок: %v", err)
	}
}

func TestPurposesIsolation(t *testing.T) {
	f := newFixture(t)
	other := f.purposeID(f.u2, "ЛКП")
	if _, err := f.svc.UpdatePurpose(f.ctx, f.u1, other, RefPatch{Name: ptr("Взлом")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужое назначение: %v", err)
	}
	mine := f.purposeID(f.u1, "ЛКП")
	if err := f.svc.ReorderPurposes(f.ctx, f.u1, []int64{other, mine}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("порядок с чужим: %v", err)
	}
	// Позиция не принимает чужое назначение при создании и правке; несуществующее — так же.
	_, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "X", Unit: "ml", PackageSize: 1,
		PurposeIDs: []int64{other}}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("создание с чужим назначением: %v", err)
	}
	p := f.pproduct("Свой", []int64{mine}, 0, nil)
	for _, bad := range [][]int64{{other}, {mine, other}, {999999}} {
		if _, err := f.svc.UpdateProduct(f.ctx, f.u1, p, ProductPatch{PurposeIDs: &bad}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("правка с %v: %v", bad, err)
		}
	}
	if s, _ := f.svc.Summary(f.ctx, f.u1, p); !reflect.DeepEqual(s.PurposeIDs, []int64{mine}) {
		t.Fatalf("после отказов назначения %v", s.PurposeIDs)
	}
	// Чужие пользователи не видят друг друга в счётчике и списке.
	if list, _ := f.svc.Purposes(f.ctx, f.u2, false); list[0].ProductsCount != 0 {
		t.Fatalf("чужая позиция в счётчике: %+v", list[0])
	}
}

func TestProductNewFieldsAndValidation(t *testing.T) {
	f := newFixture(t)
	leather, plastic := f.purposeID(f.u1, "кожа"), f.purposeID(f.u1, "пластик")
	ph := int64(73)
	p, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Крем", Unit: "ml", PackageSize: 250,
		PurposeIDs: []int64{plastic, leather, leather}, PhX10: &ph, Incompatible: "  с кислотой ",
		Instruction: "Нанести тонким слоем", Rating: ptr(int64(4)), DoNotBuy: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{leather, plastic}
	if leather > plastic {
		want = []int64{plastic, leather}
	}
	if !reflect.DeepEqual(p.PurposeIDs, want) || p.PH == nil || *p.PH != 7.3 || p.Rating == nil || *p.Rating != 4 || !p.DoNotBuy {
		t.Fatalf("сводка: %+v", p)
	}
	card, err := f.svc.Card(f.ctx, f.u1, p.ID)
	if err != nil || card.Incompatible != "с кислотой" || card.Instruction != "Нанести тонким слоем" {
		t.Fatalf("карточка: %+v %v", card, err)
	}
	if d, err := f.svc.Product(f.ctx, f.u1, p.ID); err != nil || d.Product.Instruction != card.Instruction {
		t.Fatalf("детали: %+v %v", d.Product, err)
	}
	// Правка частями не сбрасывает остальное; pH снимается null-ом (PhSet).
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{Name: ptr("Крем 2")}); err != nil {
		t.Fatal(err)
	}
	card, _ = f.svc.Card(f.ctx, f.u1, p.ID)
	if card.Name != "Крем 2" || card.PH == nil || card.Instruction == "" || len(card.PurposeIDs) != 2 || !card.DoNotBuy {
		t.Fatalf("после правки имени: %+v", card)
	}
	empty := []int64{}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, p.ID, ProductPatch{PhSet: true, RatingSet: true, PurposeIDs: &empty,
		DoNotBuy: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	card, _ = f.svc.Card(f.ctx, f.u1, p.ID)
	if card.PH != nil || card.Rating != nil || len(card.PurposeIDs) != 0 || card.DoNotBuy {
		t.Fatalf("после сброса: %+v", card)
	}
	// Проверки.
	for name, in := range map[string]ProductInput{
		"ph 141":        {PhX10: ptr(int64(141))},
		"ph -1":         {PhX10: ptr(int64(-1))},
		"оценка 0":      {Rating: ptr(int64(0))},
		"оценка 6":      {Rating: ptr(int64(6))},
		"несовместимое": {Incompatible: repeat("н", 301)},
		"инструкция":    {Instruction: repeat("и", 2001)},
	} {
		in.CategoryID, in.Name, in.Unit, in.PackageSize = f.cat1, "Y", "ml", 1
		if _, err := f.svc.CreateProduct(f.ctx, f.u1, in, 1); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// Граничные значения допустимы.
	if _, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "Z", Unit: "ml", PackageSize: 1,
		PhX10: ptr(int64(140)), Incompatible: repeat("н", 300), Instruction: repeat("и", 2000), Rating: ptr(int64(1))}, 1); err != nil {
		t.Fatal(err)
	}
	// PhX10: десятичное → десятые.
	for in, want := range map[float64]int64{0: 0, 7: 70, 7.3: 73, 14: 140, 0.1: 1} {
		if got, err := PhX10(in); err != nil || got != want {
			t.Fatalf("PhX10(%v) = %d %v", in, got, err)
		}
	}
	for _, in := range []float64{-0.1, 14.1, 7.25, 7.33} {
		if _, err := PhX10(in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("PhX10(%v): %v", in, err)
		}
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestArchivedPurposeStaysButNotOfferedForNew(t *testing.T) {
	f := newFixture(t)
	glass, tires := f.purposeID(f.u1, "стекло"), f.purposeID(f.u1, "шины")
	p := f.pproduct("Очиститель", []int64{glass, tires}, 0, nil)
	if _, err := f.svc.UpdatePurpose(f.ctx, f.u1, glass, RefPatch{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	// Архивное остаётся у позиции и в форме с тем же списком не мешает.
	same := []int64{glass, tires}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, p, ProductPatch{PurposeIDs: &same, Name: ptr("Очиститель 2")}); err != nil {
		t.Fatalf("правка с уже привязанным архивным: %v", err)
	}
	if s, _ := f.svc.Summary(f.ctx, f.u1, p); len(s.PurposeIDs) != 2 {
		t.Fatalf("архивное назначение пропало: %v", s.PurposeIDs)
	}
	// Новой позиции архивное не назначить.
	_, err := f.svc.CreateProduct(f.ctx, f.u1, ProductInput{CategoryID: f.cat1, Name: "N", Unit: "ml", PackageSize: 1,
		PurposeIDs: []int64{glass}}, 1)
	var ce *Error
	if !errors.Is(err, ErrInvalid) || !errors.As(err, &ce) || ce.Field != "purpose_ids" {
		t.Fatalf("архивное новой позиции: %v", err)
	}
	// Привязка архивного к другой позиции — тоже отказ.
	q := f.pproduct("Другая", nil, 0, nil)
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, q, ProductPatch{PurposeIDs: &[]int64{glass}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("архивное другой позиции: %v", err)
	}
	// Слишком много назначений.
	many := make([]int64, MaxProductPurposes+1)
	for i := range many {
		many[i] = int64(1000 + i)
	}
	if _, err := f.svc.UpdateProduct(f.ctx, f.u1, q, ProductPatch{PurposeIDs: &many}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("слишком много назначений: %v", err)
	}
}

func TestSearchAndFilterByPurpose(t *testing.T) {
	f := newFixture(t)
	leather, glass := f.purposeID(f.u1, "кожа"), f.purposeID(f.u1, "стекло")
	a := f.pproduct("Крем", []int64{leather}, 0, nil)
	b := f.pproduct("Очиститель окон", []int64{glass}, 0, nil)
	c := f.pproduct("Универсал", []int64{leather, glass}, 0, nil)
	f.pproduct("Шампунь", nil, 0, nil)

	got, err := f.svc.Products(f.ctx, f.u1, ProductFilter{Query: "КОЖА"})
	if err != nil || !reflect.DeepEqual(ids(got), []int64{a, c}) {
		t.Fatalf("поиск «КОЖА»: %v %v", ids(got), err)
	}
	// Совпадение по названию позиции работает по-прежнему.
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{Query: "окон"}); !reflect.DeepEqual(ids(got), []int64{b}) {
		t.Fatalf("поиск «окон»: %v", ids(got))
	}
	// Фильтр по назначению сочетается с поиском и категорией.
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{PurposeID: glass}); !reflect.DeepEqual(ids(got), []int64{b, c}) {
		t.Fatalf("фильтр по стеклу: %v", ids(got))
	}
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{PurposeID: glass, Query: "универ"}); !reflect.DeepEqual(ids(got), []int64{c}) {
		t.Fatalf("стекло + «универ»: %v", ids(got))
	}
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{PurposeID: f.purposeID(f.u1, "шины")}); len(got) != 0 {
		t.Fatalf("шины: %v", ids(got))
	}
	// Чужой id назначения не находит ничего и не раскрывает чужое.
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{PurposeID: f.purposeID(f.u2, "кожа")}); len(got) != 0 {
		t.Fatalf("чужое назначение в фильтре: %v", ids(got))
	}
	// Поиск находит и по архивному назначению.
	if _, err := f.svc.UpdatePurpose(f.ctx, f.u1, leather, RefPatch{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if got, _ = f.svc.Products(f.ctx, f.u1, ProductFilter{Query: "кожа"}); len(got) != 2 {
		t.Fatalf("поиск по архивному назначению: %v", ids(got))
	}
}

func TestRatingSet(t *testing.T) {
	f := newFixture(t)
	p := f.pproduct("Шампунь", nil, 0, nil)
	s, err := f.svc.SetRating(f.ctx, f.u1, p, true, ptr(int64(5)), nil)
	if err != nil || s.Rating == nil || *s.Rating != 5 || s.DoNotBuy {
		t.Fatalf("оценка: %+v %v", s, err)
	}
	// Только «Не брать больше» — оценка остаётся.
	if s, err = f.svc.SetRating(f.ctx, f.u1, p, false, nil, ptr(true)); err != nil || s.Rating == nil || !s.DoNotBuy {
		t.Fatalf("пометка: %+v %v", s, err)
	}
	// Снять оценку.
	if s, err = f.svc.SetRating(f.ctx, f.u1, p, true, nil, nil); err != nil || s.Rating != nil || !s.DoNotBuy {
		t.Fatalf("снятие оценки: %+v %v", s, err)
	}
	for _, bad := range []int64{0, 6, -1} {
		if _, err := f.svc.SetRating(f.ctx, f.u1, p, true, ptr(bad), nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("оценка %d: %v", bad, err)
		}
	}
	if _, err := f.svc.SetRating(f.ctx, f.u2, p, true, ptr(int64(3)), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая позиция: %v", err)
	}
}

func TestAnalogsRulesAndOrder(t *testing.T) {
	f := newFixture(t)
	leather, plastic, glass := f.purposeID(f.u1, "кожа"), f.purposeID(f.u1, "пластик"), f.purposeID(f.u1, "стекло")
	me := f.pproduct("Я", []int64{leather, plastic}, 0, nil)

	// Кандидаты: имя → (назначения, остаток, оценка).
	two5 := f.pproduct("Б два общих, 5", []int64{leather, plastic}, 100, ptr(int64(5)))
	two3 := f.pproduct("В два общих, 3", []int64{leather, plastic, glass}, 100, ptr(int64(3)))
	twoNone := f.pproduct("Г два общих, без оценки", []int64{plastic, leather}, 900, nil)
	one5 := f.pproduct("Д одно общее, 5", []int64{leather}, 100, ptr(int64(5)))
	oneMore := f.pproduct("Е одно общее, 5, больше", []int64{plastic}, 500, ptr(int64(5)))
	// Не подходят.
	f.pproduct("Нет остатка", []int64{leather}, 0, ptr(int64(5)))
	f.pproduct("Другое назначение", []int64{glass}, 100, nil)
	f.pproduct("Без назначений", nil, 100, nil)
	bought := f.pproduct("Не брать больше", []int64{leather}, 100, ptr(int64(5)))
	f.svc.SetRating(f.ctx, f.u1, bought, false, nil, ptr(true))
	arch := f.pproduct("В архиве", []int64{leather}, 100, nil)
	f.svc.UpdateProduct(f.ctx, f.u1, arch, ProductPatch{Archived: ptr(true)})
	// Чужая позиция с тем же назначением.
	if _, err := f.svc.CreateProduct(f.ctx, f.u2, ProductInput{CategoryID: f.cat2, Name: "Чужая", Unit: "ml", PackageSize: 1,
		PurposeIDs: []int64{f.purposeID(f.u2, "кожа")}}, 1); err != nil {
		t.Fatal(err)
	}

	got, err := f.svc.Analogs(f.ctx, f.u1, me, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Два общих: оценка 5, затем 3, затем без оценки; одно общее: оценка 5, больший остаток первым.
	if want := []int64{two5, two3, twoNone, oneMore, one5}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("порядок %v, ожидался %v", ids(got), want)
	}
	// Предел.
	if got, _ = f.svc.Analogs(f.ctx, f.u1, me, AnalogsLimit); len(got) != 5 {
		t.Fatalf("до пяти: %d", len(got))
	}
	if brief, err := f.svc.AnalogBriefs(f.ctx, f.u1, me, ShoppingAnalogsLimit); err != nil || len(brief) != 3 ||
		brief[0].ID != two5 || brief[0].Total != 100 || brief[0].Unit != "ml" {
		t.Fatalf("краткие: %+v %v", brief, err)
	}
	// Позиция без назначений — пустой список (не nil), чужая — 404.
	none := f.pproduct("Одинокая", nil, 0, nil)
	if got, err = f.svc.Analogs(f.ctx, f.u1, none, 5); err != nil || len(got) != 0 {
		t.Fatalf("без назначений: %v %v", got, err)
	}
	if _, err := f.svc.Analogs(f.ctx, f.u2, me, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая позиция: %v", err)
	}
	// Сама позиция помечена «не брать» — аналоги всё равно считаются для неё.
	f.svc.SetRating(f.ctx, f.u1, me, false, nil, ptr(true))
	if got, _ = f.svc.Analogs(f.ctx, f.u1, me, 10); len(got) != 5 {
		t.Fatalf("для помеченной позиции аналогов %d", len(got))
	}
}

func TestShoppingSkipsDoNotBuyAndShowsAnalogs(t *testing.T) {
	f := newFixture(t)
	leather := f.purposeID(f.u1, "кожа")
	low := f.pproduct("Ниже минимума", []int64{leather}, 100, nil)
	hidden := f.pproduct("Не брать больше", []int64{leather}, 100, nil)
	for _, id := range []int64{low, hidden} {
		f.exec(`UPDATE products SET min_stock = 500 WHERE id = ?`, id)
	}
	home := f.pproduct("Есть дома", []int64{leather}, 700, ptr(int64(4)))
	if _, err := f.svc.SetRating(f.ctx, f.u1, hidden, false, nil, ptr(true)); err != nil {
		t.Fatal(err)
	}
	// Ручной пункт про помеченную позицию остаётся.
	if _, err := f.svc.AddShopping(f.ctx, f.u1, &hidden, "", 1); err != nil {
		t.Fatal(err)
	}

	list, err := f.svc.Shopping(f.ctx, f.u1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Low) != 1 || list.Low[0].Product.ID != low {
		t.Fatalf("ниже минимума: %+v", list.Low)
	}
	if len(list.Manual) != 1 {
		t.Fatalf("ручные пункты: %+v", list.Manual)
	}
	// «Есть дома» — ещё и «Не брать больше» не предлагается; low у сводки честный.
	if a := list.Low[0].Analogs; len(a) != 1 || a[0].ID != home || a[0].Total != 700 || a[0].Name != "Есть дома" {
		t.Fatalf("аналоги: %+v", list.Low[0].Analogs)
	}
	p, _ := f.svc.Summary(f.ctx, f.u1, hidden)
	if !p.Low || !p.DoNotBuy {
		t.Fatalf("сводка помеченной: low=%v do_not_buy=%v", p.Low, p.DoNotBuy)
	}
	if got, _ := f.svc.Products(f.ctx, f.u1, ProductFilter{Filter: FilterLow}); len(got) != 2 {
		t.Fatalf("фильтр «мало» должен показывать и помеченную: %v", ids(got))
	}
	// Без аналогов — пустой массив, не null.
	f.exec(`DELETE FROM product_purposes WHERE user_id = ? AND product_id = ?`, f.u1, home)
	list, _ = f.svc.Shopping(f.ctx, f.u1, 2)
	if list.Low[0].Analogs == nil || len(list.Low[0].Analogs) != 0 {
		t.Fatalf("analogs: %#v", list.Low[0].Analogs)
	}
}
