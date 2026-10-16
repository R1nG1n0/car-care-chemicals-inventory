package stock

import (
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Движения одного запроса — одна группа batch_id = id первого из них;
// разные запросы — разные группы, даже в одну секунду.
func TestMovementBatches(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	q := e.stkProduct(uid, cat, "Салфетки", "pcs", 10)

	// Покупка двух строк: все четыре движения purchase — одна группа.
	if _, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-01", Lines: []PurchaseLineInput{
		{ProductID: p, Packages: 2, PricePerPackage: 1000}, {ProductID: q, Packages: 2, PricePerPackage: 100}}}); err != nil {
		t.Fatal(err)
	}
	if n := e.stkCount(`SELECT COUNT(DISTINCT batch_id) FROM movements WHERE type = 'purchase'`); n != 1 {
		t.Fatalf("групп у покупки: %d", n)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'purchase'
		AND batch_id = (SELECT MIN(id) FROM movements WHERE type = 'purchase')`); n != 4 {
		t.Fatalf("движений в группе покупки: %d", n)
	}

	// Два ручных расхода в одну секунду (часы не двигаются): второй с
	// добором — два движения одной группы, первый — своя группа.
	first, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 100})
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 450})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.MovementIDs) != 2 {
		t.Fatalf("добор: %+v", second)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE id IN (?, ?) AND batch_id = ?`,
		second.MovementIDs[0], second.MovementIDs[1], second.MovementIDs[0]); n != 2 {
		t.Fatal("движения одного расхода в разных группах")
	}
	if n := e.stkCount(`SELECT batch_id FROM movements WHERE id = ?`, first.MovementIDs[0]); n != first.MovementIDs[0] {
		t.Fatalf("группа первого расхода: %d", n)
	}
	sum, err := catalog.New(e.db).Summary(e.ctx, uid, p)
	if err != nil {
		t.Fatal(err)
	}
	if sum.LastConsumeQty == nil || *sum.LastConsumeQty != 450 {
		t.Fatalf("last_consume_qty: %v, ожидалось 450 (без первого расхода той же секунды)", sum.LastConsumeQty)
	}

	// Применение шаблона по двум позициям — одна группа.
	r, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Мойка", Items: []ItemInput{{ProductID: p, Qty: 10}, {ProductID: q, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := e.svc.ApplyRecipe(e.ctx, uid, r, []ItemInput{{ProductID: p, Qty: 10}, {ProductID: q, Qty: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if n := e.stkCount(`SELECT COUNT(DISTINCT batch_id) FROM movements WHERE recipe_run_id = ?`, applied.RunID); n != 1 {
		t.Fatalf("групп у применения шаблона: %d", n)
	}
	// Расход по шаблону не меняет «последний ручной».
	if sum, _ = catalog.New(e.db).Summary(e.ctx, uid, p); *sum.LastConsumeQty != 450 {
		t.Fatalf("last_consume_qty после шаблона: %d", *sum.LastConsumeQty)
	}

	// Одиночные движения (ввод остатка, корректировка, списание, перемещение)
	// — каждое своя группа.
	unit, err := e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: q, RemainingQty: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Adjust(e.ctx, uid, unit, 4, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Writeoff(e.ctx, uid, unit, nil, "Пролил"); err != nil {
		t.Fatal(err)
	}
	active := e.stkCount(`SELECT id FROM units WHERE product_id = ? AND status IN ('sealed', 'open') LIMIT 1`, q)
	if err := e.svc.Move(e.ctx, uid, active, e.stkLocation(uid, "Полка")); err != nil {
		t.Fatal(err)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type IN ('adjust', 'writeoff', 'move')`); n != 4 {
		t.Fatalf("одиночных движений: %d", n)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE batch_id IS NULL
		OR (batch_id <> id AND type IN ('adjust', 'writeoff', 'move'))`); n != 0 {
		t.Fatalf("одиночных движений не в своей группе: %d", n)
	}
}
