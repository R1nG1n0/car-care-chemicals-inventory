package stock

import (
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

func TestPartialRevertBatchExportImport(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("source")
	p := e.stkProduct(uid, cat, "Шампунь", "ml", 500)
	units := e.stkBuy(uid, p, "2026-10-01", 2, 100, nil)
	consumed, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 600})
	if err != nil {
		t.Fatal(err)
	}
	if len(consumed.MovementIDs) != 2 {
		t.Fatalf("движения: %v", consumed.MovementIDs)
	}
	first, last := consumed.MovementIDs[0], consumed.MovementIDs[1]
	other, _ := e.stkUser("other")
	// Даже повреждённая чужая ссылка на тот же ключ не должна изменяться.
	otherCategory := e.exec(`INSERT INTO categories (user_id, name, color) VALUES (?, 'Чужая', 'blue')`, other)
	otherProduct := e.stkProduct(other, otherCategory, "Чужой", "ml", 500)
	otherUnit, err := e.svc.AddUnit(e.ctx, other, AddUnitInput{ProductID: otherProduct, RemainingQty: 500})
	if err != nil {
		t.Fatal(err)
	}
	e.exec(`UPDATE movements SET batch_id = ? WHERE user_id = ?`, first, other)
	if _, err := e.svc.Revert(e.ctx, uid, []int64{first}); err != nil {
		t.Fatal(err)
	}
	if got := e.stkCount(`SELECT batch_id FROM movements WHERE id = ?`, last); got != last {
		t.Fatalf("batch_id=%d, нужен %d", got, last)
	}
	if got := e.stkCount(`SELECT batch_id FROM movements WHERE user_id = ? AND unit_id = ?`, other, otherUnit); got != first {
		t.Fatal("изменена чужая группа")
	}
	stkWant(t, e.stkUnit(units[0]), 500, "sealed")
	stkWant(t, e.stkUnit(units[1]), 400, "open")
	tr := transfer.New(e.db, e.clock.Now)
	exported, err := tr.Export(e.ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	// Получатель без категории: импорт требует пустой склад.
	target := e.exec(`INSERT INTO users (login,password_hash) VALUES ('target','x')`)
	if _, err := tr.Import(e.ctx, target, exported); err != nil {
		t.Fatalf("импорт после частичной отмены: %v", err)
	}
	if got := e.stkCount(`SELECT SUM(remaining_qty) FROM units WHERE user_id = ?`, target); got != 900 {
		t.Fatalf("остаток после импорта: %d", got)
	}
	if got := e.stkCount(`SELECT COUNT(*) FROM units WHERE user_id = ? AND remaining_qty = 500 AND status = 'sealed'`, target); got != 1 {
		t.Fatal("не сохранён полный экземпляр")
	}
	if got := e.stkCount(`SELECT COUNT(*) FROM units WHERE user_id = ? AND remaining_qty = 400 AND status = 'open'`, target); got != 1 {
		t.Fatal("не сохранён вскрытый экземпляр")
	}
}

func TestPartialRevertReanchorsAllSurvivorsAndRollsBack(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "успех"
		if fail {
			name = "откат"
		}
		t.Run(name, func(t *testing.T) {
			e := stkSetup(t)
			uid, cat := e.stkUser("a")
			p := e.stkProduct(uid, cat, "Шампунь", "ml", 500)
			units := e.stkBuy(uid, p, "2026-10-01", 3, 100, nil)
			result, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1100})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.MovementIDs) != 3 {
				t.Fatal(result.MovementIDs)
			}
			first := result.MovementIDs[0]
			if fail {
				// Поздняя корректировка делает возврат первой бутылки невозможным.
				if _, err := e.svc.Adjust(e.ctx, uid, units[0], 1, ""); err != nil {
					t.Fatal(err)
				}
			}
			_, err = e.svc.Revert(e.ctx, uid, []int64{first})
			if fail {
				if err == nil {
					t.Fatal("ожидался конфликт остатка")
				}
				if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE batch_id = ?`, first); n != 3 {
					t.Fatalf("группа изменилась при откате: %d", n)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE batch_id = ?`, result.MovementIDs[1]); n != 2 {
				t.Fatalf("оставшиеся движения: %d", n)
			}
			if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'purchase' AND batch_id = (SELECT MIN(id) FROM movements WHERE type = 'purchase')`); n != 3 {
				t.Fatal("изменена группа покупки")
			}
		})
	}
}
