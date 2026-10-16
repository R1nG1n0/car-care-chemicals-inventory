package web

import (
	"net/http"
	"slices"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// accLocations — действующие места пользователя в порядке sort.
func accLocations(t *testing.T, c *client) []catalog.Location {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/locations", nil)
	expect(t, rec, http.StatusOK, "список мест")
	return decode[catLocations](t, rec).Locations
}

// accHas — есть ли позиция в списке.
func accHas(list []catalog.ProductSummary, product int64) bool {
	for _, p := range list {
		if p.ID == product {
			return true
		}
	}
	return false
}

// П-8 и П-14: перемещение меняет место на главной; переименование и смена
// порядка мест не уводят экземпляры; место с экземплярами не архивируется.
func TestAccMoveRenameReorderLocation(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	p := stkProduct(c, stkCategory(c, "Шампуни тест"), "Koch Chemie Gsf", "ml", 500)
	box := stkLocation(c, "Коробка 3")
	shelf := stkLocation(c, "Полка")
	line := accLine(p, 2, 119000)
	line["location_id"] = box
	units := accBuy(t, c, map[string]any{"date": "2026-10-02", "lines": []any{line}}).Purchase.Lines[0].UnitIDs

	move := func(unit, to int64) {
		t.Helper()
		rec := c.do(http.MethodPost, stkPath("/api/units/", unit, "/move"), map[string]any{"location_id": to})
		expect(t, rec, http.StatusOK, "перемещение")
		if u := decode[catalog.Unit](t, rec); u.LocationID == nil || *u.LocationID != to {
			t.Fatalf("экземпляр после перемещения: %+v", u)
		}
	}
	locIDs := func() []int64 {
		t.Helper()
		ids := slices.Clone(accFind(t, c, p).LocationIDs)
		slices.Sort(ids)
		return ids
	}
	byLoc := func(loc int64) []catalog.ProductSummary {
		t.Helper()
		return accListed(t, c, stkPath("?location_id=", loc, ""))
	}

	// Одна бутылка переехала — позиция в обоих местах.
	move(units[0], shelf)
	if got, want := locIDs(), []int64{min(box, shelf), max(box, shelf)}; !slices.Equal(got, want) {
		t.Fatalf("location_ids после первого перемещения: %v, ожидалось %v", got, want)
	}
	if !accHas(byLoc(shelf), p) || !accHas(byLoc(box), p) {
		t.Fatal("фильтр по месту не находит позицию в обоих местах")
	}
	moves := 0
	for _, m := range accMovements(t, c, p) {
		if m.Type == "move" && m.UnitID == units[0] && m.FromLocationID != nil && *m.FromLocationID == box &&
			m.ToLocationID != nil && *m.ToLocationID == shelf && m.QtyDelta == 0 {
			moves++
		}
	}
	if moves != 1 {
		t.Fatal("в журнале нет движения move из коробки на полку")
	}

	// Вторая тоже — в коробке позиции больше нет.
	move(units[1], shelf)
	if got := locIDs(); !slices.Equal(got, []int64{shelf}) {
		t.Fatalf("location_ids после второго перемещения: %v", got)
	}
	if accHas(byLoc(box), p) || !accHas(byLoc(shelf), p) {
		t.Fatal("фильтр по месту не отражает новое место")
	}

	// Переименование и порядок.
	rec := c.do(http.MethodPatch, stkPath("/api/locations/", shelf, ""), map[string]any{"name": "Полка верхняя"})
	expect(t, rec, http.StatusOK, "переименование места")
	if l := decode[catalog.Location](t, rec); l.Name != "Полка верхняя" || l.UnitsCount != 2 {
		t.Fatalf("место после переименования: %+v", l)
	}
	before := accLocations(t, c)
	order := make([]int64, 0, len(before))
	for i := len(before) - 1; i >= 0; i-- {
		order = append(order, before[i].ID)
	}
	expect(t, c.do(http.MethodPost, "/api/locations/reorder", map[string]any{"ids": order}), http.StatusNoContent, "порядок мест")
	after := accLocations(t, c)
	gotOrder := make([]int64, len(after))
	for i, l := range after {
		gotOrder[i] = l.ID
		if l.ID == shelf && (l.Name != "Полка верхняя" || l.UnitsCount != 2) {
			t.Fatalf("место после смены порядка: %+v", l)
		}
		if l.ID == box && l.UnitsCount != 0 {
			t.Fatalf("в коробке остались экземпляры: %+v", l)
		}
	}
	if !slices.Equal(gotOrder, order) {
		t.Fatalf("порядок мест %v, ожидался %v", gotOrder, order)
	}
	if !accHas(byLoc(shelf), p) || !slices.Equal(locIDs(), []int64{shelf}) {
		t.Fatal("после переименования и смены порядка позиция потеряла место")
	}
	for _, u := range accDetail(t, c, p).Units {
		if u.LocationID == nil || *u.LocationID != shelf {
			t.Fatalf("экземпляр ушёл из места: %+v", u)
		}
	}

	// Архив места с экземплярами — 409, место остаётся действующим.
	rec = c.do(http.MethodPatch, stkPath("/api/locations/", shelf, ""), map[string]any{"archived": true})
	expect(t, rec, http.StatusConflict, "архив места с экземплярами")
	if msg := decode[errBody](t, rec).Error; msg != "В месте лежат экземпляры: 2. Сначала переложите их" {
		t.Fatalf("текст ошибки архива: %q", msg)
	}
	still := false
	for _, l := range accLocations(t, c) {
		still = still || l.ID == shelf
	}
	if !still {
		t.Fatal("место с экземплярами ушло в архив")
	}
	// Пустое место архивируется.
	expect(t, c.do(http.MethodPatch, stkPath("/api/locations/", box, ""), map[string]any{"archived": true}),
		http.StatusOK, "архив пустого места")
}

// П-12: last_consume_qty — последнее ручное списание; шаблон его не меняет;
// после отмены последнего расхода последним становится предыдущий.
func TestAccLastConsumeQty(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	p := stkProduct(c, stkCategory(c, "Шампуни тест"), "Koch Chemie Gsf", "ml", 500)
	accBuy(t, c, map[string]any{"date": "2026-10-02", "lines": []any{accLine(p, 3, 119000)}})

	check := func(want int64, what string) {
		t.Helper()
		if got := accLast(accDetail(t, c, p).Product); got != want {
			t.Fatalf("%s: last_consume_qty в карточке %d, ожидалось %d", what, got, want)
		}
		if got := accLast(accFind(t, c, p)); got != want {
			t.Fatalf("%s: last_consume_qty в списке %d, ожидалось %d", what, got, want)
		}
	}
	check(-1, "до расхода")
	accConsume(t, c, p, 30)
	check(30, "после первого расхода")
	if got := accLast(accConsume(t, c, p, 70).Product); got != 70 {
		t.Fatalf("ответ второго расхода: last_consume_qty %d", got)
	}
	check(70, "после второго расхода")
	// Расход с добором из следующей бутылки — одна группа, сумма движений.
	big := accConsume(t, c, p, 450)
	if len(big.Movements) != 2 {
		t.Fatalf("расход 450 должен затронуть две бутылки: %+v", big.Movements)
	}
	check(450, "после расхода с добором")

	recipe := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка", "items": []any{
		map[string]any{"product_id": p, "qty": 25}}})
	rec := c.do(http.MethodPost, stkPath("/api/recipes/", recipe, "/apply"), map[string]any{"items": []any{
		map[string]any{"product_id": p, "qty": 25}}})
	expect(t, rec, http.StatusCreated, "применение шаблона")
	applied := decode[accApplyBody](t, rec)
	if len(applied.Products) != 1 || accLast(applied.Products[0]) != 450 {
		t.Fatalf("шаблон изменил last_consume_qty в ответе: %+v", applied.Products)
	}
	check(450, "после применения шаблона")

	// Отмена последнего ручного расхода: последним становится предыдущий (70).
	rec = c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(big.Movements)})
	expect(t, rec, http.StatusOK, "отмена последнего расхода")
	if ps := decode[catProducts](t, rec).Products; len(ps) != 1 || accLast(ps[0]) != 70 {
		t.Fatalf("ответ отмены: %+v", ps)
	}
	check(70, "после отмены последнего расхода")

	// Отмена шаблона на last_consume_qty тоже не влияет.
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(applied.Movements)}),
		http.StatusOK, "отмена шаблона")
	check(70, "после отмены шаблона")
}

// П-13: шаблон «Мойка кузова» из 4 позиций, одной не хватает — одно применение
// списывает остальное полностью, нехватка в shortages, себестоимость сходится
// с движениями; отмена возвращает все остатки.
func TestAccRecipeWithShortage(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "ringino")
	cat := stkCategory(c, "Мойка тест")
	type pos struct {
		id, qty, stock int64
	}
	mk := func(name string, size, price, qty int64) pos {
		id := stkProduct(c, cat, name, "ml", size)
		accBuy(t, c, map[string]any{"date": "2026-10-02", "lines": []any{accLine(id, 1, price)}})
		return pos{id: id, qty: qty, stock: size}
	}
	items := []pos{
		mk("Шампунь", 1000, 100000, 40),
		mk("Активная пена", 1000, 80000, 100),
		mk("Чернитель шин", 500, 60000, 30),
		mk("Воск", 100, 90000, 150), // не хватает: есть 100
	}
	short := items[3]
	body := []any{}
	for _, it := range items {
		body = append(body, map[string]any{"product_id": it.id, "qty": it.qty})
	}
	recipe := stkCreate(c, "/api/recipes", map[string]any{"name": "Мойка кузова", "items": body})

	rec := c.do(http.MethodGet, stkPath("/api/recipes/", recipe, ""), nil)
	expect(t, rec, http.StatusOK, "шаблон")
	r := decode[struct {
		Recipe struct {
			Cost  *int64 `json:"cost"`
			Items []struct {
				ProductID int64 `json:"product_id"`
				Short     bool  `json:"short"`
			} `json:"items"`
		} `json:"recipe"`
	}](t, rec).Recipe
	if len(r.Items) != 4 || r.Cost == nil || *r.Cost <= 0 {
		t.Fatalf("шаблон до применения: %+v", r)
	}
	for _, it := range r.Items {
		if it.Short != (it.ProductID == short.id) {
			t.Fatalf("признак нехватки в шаблоне: %+v", r.Items)
		}
	}

	rec = c.do(http.MethodPost, stkPath("/api/recipes/", recipe, "/apply"), map[string]any{"items": body})
	expect(t, rec, http.StatusCreated, "применение одним запросом")
	a := decode[accApplyBody](t, rec)
	if a.RunID == 0 || len(a.Products) != 4 || len(a.Movements) != 4 {
		t.Fatalf("применение: %+v", a)
	}
	if len(a.Shortages) != 1 || a.Shortages[0].ProductID != short.id || a.Shortages[0].Requested != 150 ||
		a.Shortages[0].Consumed != 100 {
		t.Fatalf("нехватка: %+v", a.Shortages)
	}
	var sum int64
	for _, m := range a.Movements {
		if m.Type != "consume" || m.RecipeRunID == nil || *m.RecipeRunID != a.RunID || m.Cost == nil {
			t.Fatalf("движение шаблона: %+v", m)
		}
		sum += *m.Cost
	}
	if a.Cost == nil || *a.Cost <= 0 || *a.Cost != sum {
		t.Fatalf("себестоимость применения %v, сумма движений %d", a.Cost, sum)
	}
	// 40 из 1000 по 100000 + 100 из 1000 по 80000 + 30 из 500 по 60000 + 100 из 100 по 90000.
	if *a.Cost != 4000+8000+3600+90000 {
		t.Fatalf("себестоимость %d, ожидалось %d", *a.Cost, 4000+8000+3600+90000)
	}
	for _, it := range items {
		want := it.stock - it.qty
		if it.id == short.id {
			want = 0
		}
		if got := accFind(t, c, it.id).Stock.Total; got != want {
			t.Fatalf("остаток позиции %d после шаблона: %d, ожидалось %d", it.id, got, want)
		}
	}
	rec = c.do(http.MethodGet, stkPath("/api/recipes/", recipe, ""), nil)
	if used := decode[struct {
		Recipe struct {
			LastUsedAt *int64 `json:"last_used_at"`
		} `json:"recipe"`
	}](t, rec).Recipe.LastUsedAt; used == nil || *used != e.clock.Now().Unix() {
		t.Fatalf("last_used_at после применения: %v", used)
	}

	// Отмена применения возвращает все остатки, бутылки снова запечатаны.
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": stkIDs(a.Movements)}),
		http.StatusOK, "отмена применения")
	for _, it := range items {
		p := accFind(t, c, it.id)
		if p.Stock.Total != it.stock || p.Stock.SealedCount != 1 {
			t.Fatalf("после отмены шаблона позиция %d: %+v", it.id, p.Stock)
		}
	}
}
