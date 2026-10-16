package catalog

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

// catFixture — настоящая SQLite и два пользователя со справочниками по умолчанию.
type catFixture struct {
	t        *testing.T
	db       *sql.DB
	svc      *Service
	ctx      context.Context
	u1, u2   int64
	cat1     int64 // «Шампуни» первого
	cat2     int64 // «Шампуни» второго
	nextLine int64
}

func newFixture(t *testing.T) *catFixture {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	a := auth.New(db.DB, auth.Options{Now: func() time.Time { return time.Unix(1_792_000_000, 0) },
		Params: auth.Params{Time: 1, Memory: 64, Threads: 1}})
	ctx := context.Background()
	f := &catFixture{t: t, db: db.DB, svc: New(db.DB), ctx: ctx}
	for i, login := range []string{"first", "second"} {
		u, err := a.CreateUser(ctx, login, "password-123", false)
		if err != nil {
			t.Fatal(err)
		}
		var cat int64
		if err := db.QueryRow(`SELECT id FROM categories WHERE user_id = ? AND name = 'Шампуни'`, u.ID).Scan(&cat); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			f.u1, f.cat1 = u.ID, cat
		} else {
			f.u2, f.cat2 = u.ID, cat
		}
	}
	return f
}

func (f *catFixture) exec(q string, args ...any) int64 {
	f.t.Helper()
	res, err := f.db.Exec(q, args...)
	if err != nil {
		f.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f *catFixture) product(user, cat int64, name, brand, unit string, size int64, minStock *int64) int64 {
	f.t.Helper()
	p, err := f.svc.CreateProduct(f.ctx, user, ProductInput{CategoryID: cat, Name: name, Brand: brand, Unit: unit,
		PackageSize: size, MinStock: minStock}, 1_792_000_000)
	if err != nil {
		f.t.Fatal(err)
	}
	return p.ID
}

// purchase — покупка с одной строкой; возвращает id строки.
func (f *catFixture) purchase(user, product int64, store *int64, date string, packages, price, at int64) int64 {
	pid := f.exec(`INSERT INTO purchases (user_id, store_id, date, created_at) VALUES (?, ?, ?, ?)`, user, store, date, at)
	return f.exec(`INSERT INTO purchase_lines (user_id, purchase_id, product_id, packages, price_per_package)
		VALUES (?, ?, ?, ?, ?)`, user, pid, product, packages, price)
}

func (f *catFixture) unit(user, product int64, line, loc *int64, initial, remaining int64, status string, opened *int64) int64 {
	return f.exec(`INSERT INTO units (user_id, product_id, purchase_line_id, location_id, initial_qty, remaining_qty,
		status, opened_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1792000000)`,
		user, product, line, loc, initial, remaining, status, opened)
}

func (f *catFixture) move(user, unit int64, typ string, delta, at int64, run *int64) int64 {
	return f.exec(`INSERT INTO movements (user_id, unit_id, type, qty_delta, recipe_run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, user, unit, typ, delta, run, at)
}

// batch объединяет движения в одну группу (одно нажатие): batch_id = первый id.
func (f *catFixture) batch(ids ...int64) {
	f.t.Helper()
	for _, id := range ids {
		f.exec(`UPDATE movements SET batch_id = ? WHERE id = ?`, ids[0], id)
	}
}

func ptr[T any](v T) *T { return &v }

func TestSummaryStock(t *testing.T) {
	f := newFixture(t)
	loc1, err := f.svc.CreateLocation(f.ctx, f.u1, "Коробка 1")
	if err != nil {
		t.Fatal(err)
	}
	loc2, _ := f.svc.CreateLocation(f.ctx, f.u1, "Полка 2")
	p := f.product(f.u1, f.cat1, "Koch Chemie Gsf", "Koch Chemie", "ml", 1000, ptr[int64](2500))
	line := f.purchase(f.u1, p, nil, "2026-10-01", 3, 119000, 1_792_000_000)
	// Два вскрытых: ранний по opened_at — тот, что с остатком 320.
	late := f.unit(f.u1, p, &line, &loc1.ID, 1000, 900, "open", ptr[int64](1_792_100_500))
	early := f.unit(f.u1, p, &line, &loc2.ID, 1000, 320, "open", ptr[int64](1_792_100_000))
	f.unit(f.u1, p, &line, &loc2.ID, 1000, 1000, "sealed", nil)
	f.unit(f.u1, p, nil, nil, 500, 0, "finished", nil)
	f.unit(f.u1, p, nil, &loc1.ID, 500, 100, "written_off", nil)

	// Последний ручной расход — одно нажатие с добором: два движения одной
	// группы; позже — расход по шаблону, он не считается.
	f.move(f.u1, early, "consume", -15, 1_792_100_000, nil)
	f.batch(f.move(f.u1, early, "consume", -30, 1_792_200_000, nil),
		f.move(f.u1, late, "consume", -10, 1_792_200_000, nil))
	run := f.exec(`INSERT INTO recipe_runs (user_id, created_at) VALUES (?, 1792300000)`, f.u1)
	f.move(f.u1, late, "consume", -70, 1_792_300_000, &run)

	s, err := f.svc.Summary(f.ctx, f.u1, p)
	if err != nil {
		t.Fatal(err)
	}
	st := s.Stock
	if st.Total != 2220 || st.UnitsCount != 3 || st.SealedCount != 1 {
		t.Fatalf("остаток: %+v", st)
	}
	if st.OpenQty == nil || *st.OpenQty != 320 || *st.OpenCapacity != 1000 {
		t.Fatalf("вскрытый: %+v", st)
	}
	if !s.Low {
		t.Fatal("2220 < 2500 — должно быть low")
	}
	if len(s.LocationIDs) != 2 || s.LocationIDs[0] != loc1.ID || s.LocationIDs[1] != loc2.ID {
		t.Fatalf("места: %v", s.LocationIDs)
	}
	if s.LastConsumeQty == nil || *s.LastConsumeQty != 40 {
		t.Fatalf("last_consume_qty: %v", s.LastConsumeQty)
	}

	// Позиция без экземпляров и без минимума.
	empty := f.product(f.u1, f.cat1, "Пустая", "", "pcs", 10, nil)
	e, _ := f.svc.Summary(f.ctx, f.u1, empty)
	if e.Stock.Total != 0 || e.Stock.OpenQty != nil || e.Low || e.LastConsumeQty != nil || e.LocationIDs == nil {
		t.Fatalf("пустая позиция: %+v", e)
	}

	// Чужая — не найдена; Summaries пропускает чужие и повторы, держит порядок.
	if _, err := f.svc.Summary(f.ctx, f.u2, p); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужая позиция: %v", err)
	}
	list, err := f.svc.Summaries(f.ctx, f.u1, []int64{empty, p, 9999, empty})
	if err != nil || len(list) != 2 || list[0].ID != empty || list[1].ID != p {
		t.Fatalf("Summaries: %v %+v", err, list)
	}
}

func TestMovementsAndUnits(t *testing.T) {
	f := newFixture(t)
	p := f.product(f.u1, f.cat1, "Шампунь", "", "ml", 1000, nil)
	line := f.purchase(f.u1, p, nil, "2026-10-01", 1, 119000, 1_792_000_000)
	priced := f.unit(f.u1, p, &line, nil, 1000, 960, "open", ptr[int64](1_792_000_000))
	free := f.unit(f.u1, p, nil, nil, 3, 3, "sealed", nil)
	m1 := f.move(f.u1, priced, "consume", -40, 1_792_000_100, nil)
	m2 := f.move(f.u1, priced, "consume", -3, 1_792_000_200, nil) // 3×119000/1000 = 357
	m3 := f.move(f.u1, free, "adjust", 3, 1_792_000_300, nil)

	list, err := f.svc.MovementsByIDs(f.ctx, f.u1, []int64{m3, m1, m2, 99999})
	if err != nil || len(list) != 3 {
		t.Fatalf("движения: %v %+v", err, list)
	}
	if list[0].ID != m3 || list[0].Cost != nil || list[0].ProductID != p {
		t.Fatalf("без цены: %+v", list[0])
	}
	if *list[1].Cost != 4760 || *list[2].Cost != 357 {
		t.Fatalf("себестоимость: %d %d", *list[1].Cost, *list[2].Cost)
	}
	if got, _ := f.svc.MovementsByIDs(f.ctx, f.u2, []int64{m1}); len(got) != 0 {
		t.Fatalf("второй видит чужое движение: %+v", got)
	}

	u, err := f.svc.UnitByID(f.ctx, f.u1, priced)
	if err != nil || u.UnitPrice == nil || *u.UnitPrice != 119000 || u.Status != "open" || u.OpenedAt == nil {
		t.Fatalf("экземпляр: %v %+v", err, u)
	}
	if u, _ := f.svc.UnitByID(f.ctx, f.u1, free); u.UnitPrice != nil {
		t.Fatalf("экземпляр без покупки с ценой: %+v", u)
	}
	if _, err := f.svc.UnitByID(f.ctx, f.u2, priced); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужой экземпляр: %v", err)
	}

	d, err := f.svc.Product(f.ctx, f.u1, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Units) != 2 || d.Units[0].ID != priced || len(d.Movements) != 3 || d.Movements[0].ID != m3 {
		t.Fatalf("карточка: %+v", d)
	}
	if len(d.Purchases) != 1 || *d.Purchases[0].PricePer100 != 11900 || d.Purchases[0].Store != nil ||
		d.Purchases[0].PricePerUnit != nil {
		t.Fatalf("покупки в карточке: %+v", d.Purchases)
	}
}

// WithTx читает внутри открытой транзакции, а не ждёт единственное соединение.
func TestWithTx(t *testing.T) {
	f := newFixture(t)
	p := f.product(f.u1, f.cat1, "Шампунь", "", "ml", 1000, nil)
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO units (user_id, product_id, initial_qty, remaining_qty) VALUES (?, ?, 500, 500)`,
		f.u1, p); err != nil {
		t.Fatal(err)
	}
	s, err := f.svc.WithTx(tx).Summary(f.ctx, f.u1, p)
	if err != nil || s.Stock.Total != 500 {
		t.Fatalf("сводка в транзакции: %v %+v", err, s.Stock)
	}
}
