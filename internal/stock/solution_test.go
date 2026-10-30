package stock

import (
	"database/sql"
	"testing"
)

// Границы расчёта концентрата (ФТ-17.2.2): к ближайшему, половины вверх, не меньше 1.
func TestConcentrate(t *testing.T) {
	for _, c := range []struct{ volume, ratio, want int64 }{
		{1000, 10, 91}, // 90,9
		{500, 1, 250},  // ровно
		{1, 1000, 1},   // 0,001 → минимум 1
		{100000, 1, 50000},
		{100000, 1000, 100}, // 99,9
		{3, 1, 2},           // 1,5 → вверх
		{5, 1, 3},           // 2,5 → вверх
		{1000, 20, 48},      // 47,6
		{10, 9, 1},          // ровно 1
		{15, 9, 2},          // 1,5 → вверх
		{14, 9, 1},          // 1,4
	} {
		if got := Concentrate(c.volume, c.ratio); got != c.want {
			t.Errorf("Concentrate(%d, 1:%d) = %d, ожидалось %d", c.volume, c.ratio, got, c.want)
		}
	}
}

func solMovement(t *testing.T, e *stkEnv, id int64) (ml, ratio sql.NullInt64, delta int64) {
	t.Helper()
	if err := e.db.QueryRow(`SELECT solution_ml, solution_ratio, qty_delta FROM movements WHERE id = ?`, id).
		Scan(&ml, &ratio, &delta); err != nil {
		t.Fatal(err)
	}
	return
}

func TestConsumeSolution(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Концентрат", "ml", 100)
	pcs := e.stkProduct(uid, cat, "Салфетки", "pcs", 10)
	one := e.stkBuy(uid, p, "2026-10-01", 1, 10000, nil)[0]
	e.stkBuy(uid, p, "2026-10-02", 1, 10000, nil)
	e.stkBuy(uid, pcs, "2026-10-02", 1, 1000, nil)

	res, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 550, Ratio: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Consumed != 50 || res.Solution == nil || res.Solution.Concentrate != 50 || res.Solution.VolumeML != 550 || res.Solution.Ratio != 10 {
		t.Fatalf("итог: %+v %+v", res, res.Solution)
	}
	stkWant(t, e.stkUnit(one), 50, "open")
	ml, ratio, delta := solMovement(t, e, res.MovementIDs[0])
	if ml.Int64 != 550 || ratio.Int64 != 10 || delta != -50 {
		t.Fatalf("движение: %v %v %d", ml, ratio, delta)
	}

	// Концентрата больше, чем в одной бутылке: каждое движение несёт тот же раствор.
	res, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 1000, Ratio: 9}})
	if err != nil || len(res.MovementIDs) != 2 || res.Consumed != 100 {
		t.Fatalf("две бутылки: %+v %v", res, err)
	}
	for _, id := range res.MovementIDs {
		if ml, ratio, _ := solMovement(t, e, id); ml.Int64 != 1000 || ratio.Int64 != 9 {
			t.Fatalf("раствор движения %d: %v %v", id, ml, ratio)
		}
	}

	// Обычный расход раствора не несёт.
	res, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 5})
	if err != nil || res.Solution != nil {
		t.Fatalf("обычный: %+v %v", res, err)
	}
	if ml, ratio, _ := solMovement(t, e, res.MovementIDs[0]); ml.Valid || ratio.Valid {
		t.Fatal("у обычного расхода есть раствор")
	}

	// Ошибки ввода.
	bad := []struct {
		name  string
		in    ConsumeInput
		field string
	}{
		{"шт", ConsumeInput{ProductID: pcs, Solution: &Solution{VolumeML: 100, Ratio: 10}}, "solution"},
		{"оба поля", ConsumeInput{ProductID: p, Qty: 5, Solution: &Solution{VolumeML: 100, Ratio: 10}}, "qty"},
		{"ни одного", ConsumeInput{ProductID: p}, "qty"},
		{"объём 0", ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 0, Ratio: 10}}, "solution"},
		{"объём велик", ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 100001, Ratio: 10}}, "solution"},
		{"пропорция 0", ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 100, Ratio: 0}}, "solution"},
		{"пропорция велика", ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 100, Ratio: 1001}}, "solution"},
	}
	for _, c := range bad {
		_, err := e.svc.Consume(e.ctx, uid, c.in)
		if ve := stkIs[*ValidationError](t, err, c.name); ve.Field != c.field {
			t.Errorf("%s: поле %q, ожидалось %q", c.name, ve.Field, c.field)
		}
	}
	// Нехватка — как у обычного расхода.
	_, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 100000, Ratio: 1}})
	stkIs[*ShortageError](t, err, "нехватка")
}

// Раствор + отмена + идемпотентность.
func TestConsumeSolutionOpAndRevert(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Концентрат", "ml", 500)
	unit := e.stkBuy(uid, p, "2026-10-01", 1, 10000, nil)[0]
	in := ConsumeInput{ProductID: p, Solution: &Solution{VolumeML: 1000, Ratio: 10}, Op: Op{ClientOpID: "op-solution-1"}}
	render := func(_ *sql.Tx, res ConsumeResult) ([]byte, error) {
		return []byte(`{"c":` + string(rune('0'+res.Solution.Concentrate/10%10)) + `}`), nil
	}
	res, body, replay, err := e.svc.ConsumeOp(e.ctx, uid, in, render)
	if err != nil || replay || res.Consumed != 91 {
		t.Fatalf("первый: %+v %v", res, err)
	}
	_, body2, replay, err := e.svc.ConsumeOp(e.ctx, uid, in, render)
	if err != nil || !replay || string(body) != string(body2) {
		t.Fatalf("повтор: %q %v %v", body2, replay, err)
	}
	stkWant(t, e.stkUnit(unit), 409, "open")
	if _, err := e.svc.Revert(e.ctx, uid, res.MovementIDs); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(unit), 500, "sealed")
}

func TestRecipeWithSolution(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	conc := e.stkProduct(uid, cat, "Концентрат", "ml", 500)
	pcs := e.stkProduct(uid, cat, "Салфетки", "pcs", 10)
	e.stkBuy(uid, conc, "2026-10-01", 1, 10000, nil)
	e.stkBuy(uid, pcs, "2026-10-01", 1, 1000, nil)

	// qty при растворе игнорируется, концентрат считает сервер.
	id, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Мойка", Items: []ItemInput{
		{ProductID: conc, Qty: 7, Solution: &Solution{VolumeML: 1000, Ratio: 10}}, {ProductID: pcs, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.Recipes(e.ctx, uid, id)
	if err != nil {
		t.Fatal(err)
	}
	it := list[0].Items
	if it[0].Qty != 91 || it[0].Solution == nil || it[0].Solution.VolumeML != 1000 || it[0].Solution.Ratio != 10 || it[1].Solution != nil {
		t.Fatalf("строки: %+v", it)
	}

	// Шт с раствором и неверные диапазоны.
	for _, items := range [][]ItemInput{
		{{ProductID: pcs, Solution: &Solution{VolumeML: 10, Ratio: 1}}},
		{{ProductID: conc, Solution: &Solution{VolumeML: 0, Ratio: 1}}},
		{{ProductID: conc, Solution: &Solution{VolumeML: 10, Ratio: 1001}}},
	} {
		_, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Плохой", Items: items})
		if ve := stkIs[*ValidationError](t, err, "раствор в шаблоне"); ve.Field != "solution" {
			t.Fatalf("поле %q", ve.Field)
		}
	}

	// Применение: строка с раствором пересчитывается, qty игнорируется.
	res, err := e.svc.ApplyRecipe(e.ctx, uid, id, []ItemInput{
		{ProductID: conc, Qty: 1, Solution: &Solution{VolumeML: 500, Ratio: 1}}, {ProductID: pcs, Qty: 1}})
	if err != nil || len(res.MovementIDs) != 2 || len(res.Shortages) != 0 {
		t.Fatalf("применение: %+v %v", res, err)
	}
	ml, ratio, delta := solMovement(t, e, res.MovementIDs[0])
	if ml.Int64 != 500 || ratio.Int64 != 1 || delta != -250 {
		t.Fatalf("движение: %v %v %d", ml, ratio, delta)
	}
	if ml, _, _ := solMovement(t, e, res.MovementIDs[1]); ml.Valid {
		t.Fatal("у штучной строки есть раствор")
	}

	// Нехватка: на складе 250 мл, раствор просит 2 500 — списывается остаток
	// обычным расходом, без раствора (иначе журнал врёт «5 000 мл 1:1 → 250 мл»).
	res, err = e.svc.ApplyRecipe(e.ctx, uid, id, []ItemInput{{ProductID: conc, Solution: &Solution{VolumeML: 5000, Ratio: 1}}})
	if err != nil || len(res.MovementIDs) != 1 || len(res.Shortages) != 1 {
		t.Fatalf("применение с нехваткой: %+v %v", res, err)
	}
	if ml, ratio, delta := solMovement(t, e, res.MovementIDs[0]); ml.Valid || ratio.Valid || delta != -250 {
		t.Fatalf("движение при нехватке: %v %v %d", ml, ratio, delta)
	}

	// Замена шаблона обычной строкой снимает раствор.
	if err := e.svc.UpdateRecipe(e.ctx, uid, id, RecipeInput{Name: "Мойка", Items: []ItemInput{{ProductID: conc, Qty: 30}}}); err != nil {
		t.Fatal(err)
	}
	list, _ = e.svc.Recipes(e.ctx, uid, id)
	if list[0].Items[0].Solution != nil || list[0].Items[0].Qty != 30 {
		t.Fatalf("после замены: %+v", list[0].Items[0])
	}
}
