package stock

import (
	"sync"
	"testing"
	"time"
)

func TestConsumeOrderOpenCarryFinish(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	// Новая покупка создана раньше старой по id, но дата у неё позже:
	// вскрываться должна старая по дате.
	newer := e.stkBuy(uid, p, "2026-10-05", 1, 50000, nil)[0]
	older := e.stkBuy(uid, p, "2026-10-01", 1, 50000, nil)[0]

	res, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.OpenedUnitIDs) != 1 || res.OpenedUnitIDs[0] != older || len(res.FinishedUnitIDs) != 0 {
		t.Fatalf("вскрытие: %+v", res)
	}
	stkWant(t, e.stkUnit(older), 400, "open")
	stkWant(t, e.stkUnit(newer), 500, "sealed")
	if res.Cost == nil || *res.Cost != 10000 {
		t.Fatalf("себестоимость %v", res.Cost)
	}

	// Сценарий П-5: расход больше остатка вскрытого — добор из следующего.
	e.clock.Add(time.Minute)
	res, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 600})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.MovementIDs) != 2 || len(res.FinishedUnitIDs) != 1 || res.FinishedUnitIDs[0] != older ||
		len(res.OpenedUnitIDs) != 1 || res.OpenedUnitIDs[0] != newer || res.Consumed != 600 {
		t.Fatalf("добор: %+v", res)
	}
	if r := e.stkUnit(older); r.Status != "finished" || r.Remaining != 0 || !r.Finished.Valid {
		t.Fatalf("первая бутылка: %+v", r)
	}
	stkWant(t, e.stkUnit(newer), 300, "open")
	// Оба движения одного нажатия — с одним временем.
	if n := e.stkCount(`SELECT COUNT(DISTINCT created_at) FROM movements WHERE id IN (?, ?)`,
		res.MovementIDs[0], res.MovementIDs[1]); n != 1 {
		t.Fatalf("разное время движений одного расхода: %d", n)
	}

	// Нехватка — 409 с available, ничего не меняется.
	before := e.stkCount(`SELECT COUNT(*) FROM movements`)
	_, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 301})
	se := stkIs[*ShortageError](t, err, "нехватка")
	if se.Available != 300 || se.Error() != "Не хватает: есть 300 мл" {
		t.Fatalf("нехватка: %+v %q", se, se.Error())
	}
	stkWant(t, e.stkUnit(newer), 300, "open")
	if e.stkCount(`SELECT COUNT(*) FROM movements`) != before {
		t.Fatal("при нехватке записались движения")
	}
	_, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 0})
	stkIs[*ValidationError](t, err, "qty 0")
}

func TestConsumeOpenFirstAndExplicitUnit(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Салфетки", "pcs", 10)
	units := e.stkBuy(uid, p, "2026-10-01", 3, 1000, nil)
	// Вскрываем третий явно.
	res, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 2, UnitID: &units[2]})
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenedUnitIDs[0] != units[2] {
		t.Fatalf("явный экземпляр: %+v", res)
	}
	// Без unit_id — сначала вскрытый, а не старший запечатанный.
	if _, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 9}); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(units[2]), 0, "finished")
	stkWant(t, e.stkUnit(units[0]), 9, "open")
	stkWant(t, e.stkUnit(units[1]), 10, "sealed")
	// С unit_id добор идёт дальше по общему правилу.
	if _, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 12, UnitID: &units[1]}); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(units[1]), 0, "finished")
	stkWant(t, e.stkUnit(units[0]), 7, "open")

	// Закончившийся экземпляр — 409, экземпляр другой позиции — 404.
	_, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1, UnitID: &units[1]})
	stkIs[*ConflictError](t, err, "законченный экземпляр")
	p2 := e.stkProduct(uid, cat, "Другая", "pcs", 1)
	_, err = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p2, Qty: 1, UnitID: &units[0]})
	if !IsNotFound(err) {
		t.Fatalf("экземпляр другой позиции: %v", err)
	}
}

func TestRevert(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	units := e.stkBuy(uid, p, "2026-10-01", 2, 1000, nil)

	// Отмена расхода с добором: оба экземпляра снова sealed.
	res, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 600})
	if err != nil {
		t.Fatal(err)
	}
	products, err := e.svc.Revert(e.ctx, uid, res.MovementIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0] != p {
		t.Fatalf("позиции отмены: %v", products)
	}
	for _, u := range units {
		r := e.stkUnit(u)
		stkWant(t, r, 500, "sealed")
		if r.Opened.Valid || r.Finished.Valid {
			t.Fatalf("после отмены остались отметки: %+v", r)
		}
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'consume'`); n != 0 {
		t.Fatalf("движения не удалены: %d", n)
	}

	// Два расхода, отменяется второй — экземпляр остаётся open.
	first, _ := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 100})
	second, _ := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 50})
	if _, err := e.svc.Revert(e.ctx, uid, second.MovementIDs); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(units[0]), 400, "open")

	// Списание всего и отмена — written_off → open.
	if err := e.svc.Writeoff(e.ctx, uid, units[0], nil, "Испорчено"); err != nil {
		t.Fatal(err)
	}
	var wo int64
	if err := e.db.QueryRow(`SELECT id FROM movements WHERE type = 'writeoff'`).Scan(&wo); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Revert(e.ctx, uid, []int64{wo}); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(units[0]), 400, "open")

	// Все или ничего: чужое движение — 404, своё не отменяется.
	uid2, cat2 := e.stkUser("b")
	p2 := e.stkProduct(uid2, cat2, "Чужая", "ml", 100)
	e.stkBuy(uid2, p2, "2026-10-01", 1, 0, nil)
	foreign, err := e.svc.Consume(e.ctx, uid2, ConsumeInput{ProductID: p2, Qty: 10})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Revert(e.ctx, uid, append(first.MovementIDs, foreign.MovementIDs...))
	if !IsNotFound(err) {
		t.Fatalf("чужое движение: %v", err)
	}
	stkWant(t, e.stkUnit(units[0]), 400, "open")
	stkWant(t, e.stkUnit(foreign.OpenedUnitIDs[0]), 90, "open")

	// Покупку отменить нельзя.
	var buy int64
	if err := e.db.QueryRow(`SELECT id FROM movements WHERE type = 'purchase' AND user_id = ? LIMIT 1`, uid).Scan(&buy); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Revert(e.ctx, uid, []int64{buy})
	stkIs[*ConflictError](t, err, "отмена покупки")

	// Отмена первого расхода: других движений нет — снова sealed.
	if _, err := e.svc.Revert(e.ctx, uid, first.MovementIDs); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(units[0]), 500, "sealed")
}

func TestRevertAdjustAndInitialStock(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	u, err := e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: p, RemainingQty: 500})
	if err != nil {
		t.Fatal(err)
	}
	// Ввод остатка не считается вскрытием: после отмены расхода — sealed.
	res, _ := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 40})
	if _, err := e.svc.Revert(e.ctx, uid, res.MovementIDs); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(u), 500, "sealed")

	// Корректировка до 0 и её отмена: finished → sealed (других движений нет).
	if err := e.svc.Adjust(e.ctx, uid, u, 0, ""); err != nil {
		t.Fatal(err)
	}
	adj := e.stkCount(`SELECT MAX(id) FROM movements WHERE type = 'adjust'`)
	if _, err := e.svc.Revert(e.ctx, uid, []int64{adj}); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(u), 500, "sealed")

	// Отмена ввода остатка после расхода выводит остаток за 0 — 409.
	if _, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 100}); err != nil {
		t.Fatal(err)
	}
	initial := e.stkCount(`SELECT MIN(id) FROM movements WHERE unit_id = ?`, u)
	_, err = e.svc.Revert(e.ctx, uid, []int64{initial})
	stkIs[*ConflictError](t, err, "остаток ниже нуля")
	stkWant(t, e.stkUnit(u), 400, "open")
}

// Гонка: два одновременных расхода с одного экземпляра не уводят остаток в
// минус и не теряют списание.
func TestConsumeRace(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 100)
	u := e.stkBuy(uid, p, "2026-10-01", 1, 0, nil)[0]

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 30})
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if _, short := err.(*ShortageError); !short {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	}
	r := e.stkUnit(u)
	if ok != 3 || r.Remaining != 10 {
		t.Fatalf("успешных %d, остаток %d; ожидалось 3 и 10", ok, r.Remaining)
	}
	if sum := e.stkCount(`SELECT -SUM(qty_delta) FROM movements WHERE type = 'consume'`); sum != 90 {
		t.Fatalf("сумма расхода в журнале %d", sum)
	}
}
