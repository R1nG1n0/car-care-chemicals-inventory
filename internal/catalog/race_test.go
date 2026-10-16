package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// raceRounds — сколько раз повторяется гонка: окно между проверкой и
// записью узкое, одного раза мало, чтобы поймать его до исправления.
const raceRounds = 200

// raceStep — шаг задержки покупки в TestProductUnitRace.
const raceStep = 4 * time.Microsecond

// hammer через delay и до закрытия stop раз за разом кладёт экземпляры
// (raceUnit), чтобы покупка попала между проверкой и записью проверяемого
// метода. Задержка нужна, когда до проверки метод делает много запросов:
// без неё покупка всегда успевает раньше проверки.
func hammer(ctx context.Context, db *sql.DB, user, product, loc int64, delay time.Duration, stop <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		time.Sleep(delay)
		for {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			if err := raceUnit(ctx, db, user, product, loc); err != nil {
				done <- err
				return
			}
		}
	}()
	return done
}

// raceUnit кладёт экземпляр так же, как покупка в internal/stock: в
// транзакции читает позицию и место и пишет, только если условия ещё
// выполняются (позиция в мл, место действующее).
func raceUnit(ctx context.Context, db *sql.DB, user, product, loc int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // после Commit это no-op
	var unit string
	var archived bool
	if err := tx.QueryRowContext(ctx, `SELECT unit FROM products WHERE user_id = ? AND id = ?`, user, product).Scan(&unit); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT archived FROM locations WHERE user_id = ? AND id = ?`, user, loc).Scan(&archived); err != nil {
		return err
	}
	if unit != "ml" || archived {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO units (user_id, product_id, location_id, initial_qty, remaining_qty)
		VALUES (?, ?, ?, 1000, 1000)`, user, product, loc); err != nil {
		return err
	}
	return tx.Commit()
}

// Архив места и параллельная покупка в него: место не может оказаться в
// архиве с экземплярами sealed/open (ФТ-2.2).
func TestLocationArchiveRace(t *testing.T) {
	f := newFixture(t)
	p := f.product(f.u1, f.cat1, "Шампунь", "", "ml", 1000, nil)
	for i := range raceRounds {
		loc, err := f.svc.CreateLocation(f.ctx, f.u1, fmt.Sprintf("Коробка %d", i))
		if err != nil {
			t.Fatal(err)
		}
		stop := make(chan struct{})
		done := hammer(f.ctx, f.db, f.u1, p, loc.ID, 0, stop)
		_, errArchive := f.svc.UpdateLocation(f.ctx, f.u1, loc.ID, RefPatch{Archived: ptr(true)})
		close(stop)
		if errUnit := <-done; errUnit != nil {
			t.Fatal(errUnit)
		}
		var archived bool
		var units int64
		if err := f.db.QueryRow(`SELECT archived, (SELECT COUNT(*) FROM units WHERE location_id = l.id
			AND status IN ('sealed', 'open')) FROM locations l WHERE id = ?`, loc.ID).Scan(&archived, &units); err != nil {
			t.Fatal(err)
		}
		if archived && units > 0 {
			t.Fatalf("круг %d: место в архиве с экземплярами: %d", i, units)
		}
		if !archived && errArchive == nil {
			t.Fatalf("круг %d: архив прошёл без ошибки, но место не в архиве", i)
		}
	}
}

// Смена единицы и параллельная покупка позиции: у позиции с экземплярами
// единица не меняется (409).
func TestProductUnitRace(t *testing.T) {
	f := newFixture(t)
	loc, err := f.svc.CreateLocation(f.ctx, f.u1, "Полка")
	if err != nil {
		t.Fatal(err)
	}
	for i := range raceRounds {
		p := f.product(f.u1, f.cat1, fmt.Sprintf("Позиция %d", i), "", "ml", 1000, nil)
		stop := make(chan struct{})
		// Задержка растёт от круга к кругу и проходит всё время работы метода.
		done := hammer(f.ctx, f.db, f.u1, p, loc.ID, time.Duration(i)*raceStep, stop)
		_, errUpdate := f.svc.UpdateProduct(f.ctx, f.u1, p, ProductPatch{Unit: ptr("pcs")})
		close(stop)
		if errUnit := <-done; errUnit != nil {
			t.Fatal(errUnit)
		}
		var unit string
		var units int64
		if err := f.db.QueryRow(`SELECT unit, (SELECT COUNT(*) FROM units WHERE product_id = p.id)
			FROM products p WHERE id = ?`, p).Scan(&unit, &units); err != nil {
			t.Fatal(err)
		}
		if unit == "pcs" && units > 0 {
			t.Fatalf("круг %d: единица сменилась у позиции с экземплярами: %d", i, units)
		}
		if unit == "ml" && errUpdate == nil {
			t.Fatalf("круг %d: смена единицы прошла без ошибки, но единица прежняя", i)
		}
	}
}
