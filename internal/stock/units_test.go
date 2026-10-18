package stock

import (
	"errors"
	"testing"
)

// Закончившийся экземпляр не оживает в архивном месте: корректировка и
// отмена движения — 409, экземпляр не меняется. Без места или после возврата
// места из архива — можно.
func TestReviveInArchivedPlace(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	box := e.stkLocation(uid, "Коробка")
	u := e.stkBuy(uid, p, "2026-10-01", 1, 1000, &box)[0]

	res, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 500})
	if err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(u), 0, "finished")
	e.exec(`UPDATE locations SET archived = 1 WHERE id = ?`, box)

	err = second(e.svc.Adjust(e.ctx, uid, u, 100, ""))
	if ce := stkIs[*ConflictError](t, err, "корректировка в архивном месте"); !errors.Is(ce, errUnitPlaceArchived) {
		t.Fatalf("не та ошибка: %v", ce)
	}
	_, err = e.svc.Revert(e.ctx, uid, res.MovementIDs)
	if ce := stkIs[*ConflictError](t, err, "отмена в архивном месте"); !errors.Is(ce, errUnitPlaceArchived) {
		t.Fatalf("не та ошибка: %v", ce)
	}
	stkWant(t, e.stkUnit(u), 0, "finished")
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE unit_id = ? AND type = 'consume'`, u); n != 1 {
		t.Fatalf("движение расхода удалено: %d", n)
	}
	// Корректировка в 0 закончившегося в архивном месте ничего не оживляет.
	if _, err := e.svc.Adjust(e.ctx, uid, u, 0, ""); err != nil {
		t.Fatal(err)
	}

	// После возврата места из архива — отмена проходит.
	e.exec(`UPDATE locations SET archived = 0 WHERE id = ?`, box)
	if _, err := e.svc.Revert(e.ctx, uid, res.MovementIDs); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(u), 500, "sealed")

	// Без места: списанный оживает корректировкой. Другая позиция — у этой
	// место по умолчанию «Коробка».
	p2 := e.stkProduct(uid, cat, "Воск", "ml", 500)
	free, err := e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: p2, RemainingQty: 500})
	if err != nil {
		t.Fatal(err)
	}
	if e.stkUnit(free).Location.Valid {
		t.Fatal("ожидался экземпляр без места")
	}
	if _, err := e.svc.Writeoff(e.ctx, uid, free, nil, "Испорчено"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Adjust(e.ctx, uid, free, 50, ""); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(free), 50, "open")
}

// Корректировка и списание обновляют экземпляр с условием на прежнее
// состояние: если строка не обновилась (её изменили между чтением и
// записью), — 409 и никаких движений. Изменение посередине изображает
// триггер, который пропускает UPDATE.
func TestAdjustWriteoffStaleUnit(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	u := e.stkBuy(uid, p, "2026-10-01", 1, 1000, nil)[0]
	before := e.stkCount(`SELECT COUNT(*) FROM movements`)

	e.exec(`CREATE TRIGGER skip_units BEFORE UPDATE ON units BEGIN SELECT RAISE(IGNORE); END`)
	ten := int64(10)
	for name, err := range map[string]error{
		"списание части":  second(e.svc.Writeoff(e.ctx, uid, u, &ten, "Испорчено")),
		"списание всего":  second(e.svc.Writeoff(e.ctx, uid, u, nil, "Испорчено")),
		"корректировка":   second(e.svc.Adjust(e.ctx, uid, u, 300, "")),
		"корректировка 0": second(e.svc.Adjust(e.ctx, uid, u, 0, "")),
	} {
		if ce := stkIs[*ConflictError](t, err, name); !errors.Is(ce, errUnitChanged) {
			t.Fatalf("%s: не та ошибка: %v", name, ce)
		}
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements`); n != before {
		t.Fatalf("записаны движения: было %d, стало %d", before, n)
	}

	e.exec(`DROP TRIGGER skip_units`)
	if _, err := e.svc.Writeoff(e.ctx, uid, u, &ten, "Испорчено"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Adjust(e.ctx, uid, u, 300, ""); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(u), 300, "open")
}
