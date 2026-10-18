package stock

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

// stkClock — подменяемые часы.
type stkClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stkClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *stkClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// stkEnv — склад на настоящей SQLite: пользователь, категория, место.
type stkEnv struct {
	t     *testing.T
	db    *sql.DB
	svc   *Service
	clock *stkClock
	ctx   context.Context
}

func stkSetup(t *testing.T) *stkEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	c := &stkClock{t: time.Unix(1_792_000_000, 0)}
	return &stkEnv{t: t, db: db.DB, svc: New(db.DB, c.Now), clock: c, ctx: context.Background()}
}

func (e *stkEnv) exec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// stkUser создаёт пользователя с категорией; возвращает id пользователя и категории.
func (e *stkEnv) stkUser(login string) (userID, categoryID int64) {
	e.t.Helper()
	userID = e.exec(`INSERT INTO users (login, password_hash) VALUES (?, 'x')`, login)
	categoryID = e.exec(`INSERT INTO categories (user_id, name, color) VALUES (?, 'Шампуни', 'blue')`, userID)
	return userID, categoryID
}

func (e *stkEnv) stkProduct(userID, categoryID int64, name, unit string, size int64) int64 {
	e.t.Helper()
	return e.exec(`INSERT INTO products (user_id, category_id, name, unit, package_size) VALUES (?, ?, ?, ?, ?)`,
		userID, categoryID, name, unit, size)
}

func (e *stkEnv) stkLocation(userID int64, name string) int64 {
	e.t.Helper()
	return e.exec(`INSERT INTO locations (user_id, name) VALUES (?, ?)`, userID, name)
}

// stkBuy покупает packages упаковок позиции и возвращает id экземпляров.
func (e *stkEnv) stkBuy(userID, productID int64, date string, packages, price int64, loc *int64) []int64 {
	e.t.Helper()
	id, err := e.svc.CreatePurchase(e.ctx, userID, PurchaseInput{Date: date, Lines: []PurchaseLineInput{
		{ProductID: productID, Packages: packages, PricePerPackage: price, LocationID: loc}}})
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := e.svc.Purchase(e.ctx, userID, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return p.Lines[0].UnitIDs
}

type stkUnitRow struct {
	Remaining int64
	Status    string
	Location  sql.NullInt64
	Opened    sql.NullInt64
	Finished  sql.NullInt64
}

func (e *stkEnv) stkUnit(id int64) stkUnitRow {
	e.t.Helper()
	var u stkUnitRow
	if err := e.db.QueryRow(`SELECT remaining_qty, status, location_id, opened_at, finished_at FROM units WHERE id = ?`, id).
		Scan(&u.Remaining, &u.Status, &u.Location, &u.Opened, &u.Finished); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *stkEnv) stkCount(q string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func stkWant(t *testing.T, u stkUnitRow, remaining int64, status string) {
	t.Helper()
	if u.Remaining != remaining || u.Status != status {
		t.Fatalf("экземпляр: остаток %d, статус %s; ожидалось %d, %s", u.Remaining, u.Status, remaining, status)
	}
}

func stkIs[T error](t *testing.T, err error, what string) T {
	t.Helper()
	var target T
	if !errors.As(err, &target) {
		t.Fatalf("%s: ошибка %v (%T), ожидалась %T", what, err, err, target)
	}
	return target
}

func TestPurchaseCreatesUnitsAndMovements(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	box := e.stkLocation(uid, "Коробка")
	shelf := e.stkLocation(uid, "Полка")
	shop := e.exec(`INSERT INTO stores (user_id, name) VALUES (?, 'Автохим')`, uid)
	item := e.exec(`INSERT INTO shopping_items (user_id, product_id, created_at) VALUES (?, ?, 1)`, uid, p)
	other := e.exec(`INSERT INTO shopping_items (user_id, title, created_at) VALUES (?, 'Глина', 1)`, uid)

	id, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-02", StoreID: &shop, Lines: []PurchaseLineInput{
		{ProductID: p, Packages: 2, PricePerPackage: 119000, LocationID: &box}}})
	if err != nil {
		t.Fatal(err)
	}
	pur, err := e.svc.Purchase(e.ctx, uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if pur.Total != 238000 || pur.Store == nil || *pur.Store != "Автохим" || len(pur.Lines) != 1 ||
		len(pur.Lines[0].UnitIDs) != 2 || pur.CreatedAt != e.clock.Now().Unix() {
		t.Fatalf("покупка: %+v", pur)
	}
	for _, u := range pur.Lines[0].UnitIDs {
		r := e.stkUnit(u)
		stkWant(t, r, 500, "sealed")
		if r.Location.Int64 != box {
			t.Fatalf("место экземпляра %d: %v", u, r.Location)
		}
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'purchase' AND qty_delta = 500 AND to_location_id = ?`, box); n != 2 {
		t.Fatalf("движений purchase: %d", n)
	}
	if e.stkCount(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NOT NULL`, item) != 1 ||
		e.stkCount(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NULL`, other) != 1 {
		t.Fatal("пункт списка покупок не отмечен или отмечен лишний")
	}

	// Место по умолчанию — последнего экземпляра позиции.
	units := e.stkBuy(uid, p, "2026-10-03", 1, 100, nil)
	if r := e.stkUnit(units[0]); r.Location.Int64 != box {
		t.Fatalf("место по умолчанию: %v", r.Location)
	}
	if err := e.svc.Move(e.ctx, uid, units[0], shelf); err != nil {
		t.Fatal(err)
	}
	units = e.stkBuy(uid, p, "2026-10-04", 1, 100, nil)
	if r := e.stkUnit(units[0]); r.Location.Int64 != shelf {
		t.Fatalf("место по умолчанию после перемещения: %v", r.Location)
	}
	// Позиция без экземпляров — без места.
	p2 := e.stkProduct(uid, cat, "Новая", "pcs", 1)
	if r := e.stkUnit(e.stkBuy(uid, p2, "2026-10-04", 1, 100, nil)[0]); r.Location.Valid {
		t.Fatalf("у новой позиции место %v", r.Location)
	}

	list, err := e.svc.Purchases(e.ctx, uid, 0, 2)
	if err != nil || len(list) != 2 || list[0].ID <= list[1].ID {
		t.Fatalf("список покупок: %+v, %v", list, err)
	}
	next, err := e.svc.Purchases(e.ctx, uid, list[1].ID, 50)
	if err != nil || len(next) != 2 || next[1].ID != id || next[1].Total != 238000 || next[1].LinesCount != 1 {
		t.Fatalf("вторая страница: %+v, %v", next, err)
	}
}

func TestPurchaseValidation(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	line := PurchaseLineInput{ProductID: p, Packages: 1, PricePerPackage: 1}
	cases := []struct {
		in    PurchaseInput
		field string
	}{
		{PurchaseInput{Date: "02.10.2026", Lines: []PurchaseLineInput{line}}, "date"},
		{PurchaseInput{Date: "2026-10-02"}, "lines"},
		{PurchaseInput{Date: "2026-10-02", Lines: []PurchaseLineInput{{ProductID: p, Packages: 101}}}, "packages"},
		{PurchaseInput{Date: "2026-10-02", Lines: []PurchaseLineInput{{ProductID: p, Packages: 1, PricePerPackage: -1}}}, "price_per_package"},
	}
	for _, c := range cases {
		_, err := e.svc.CreatePurchase(e.ctx, uid, c.in)
		if ve := stkIs[*ValidationError](t, err, c.field); ve.Field != c.field {
			t.Fatalf("поле %s, ожидалось %s", ve.Field, c.field)
		}
	}
	// Вторая строка с чужой позицией откатывает всю покупку.
	uid2, cat2 := e.stkUser("b")
	foreign := e.stkProduct(uid2, cat2, "Чужая", "ml", 100)
	_, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-02",
		Lines: []PurchaseLineInput{line, {ProductID: foreign, Packages: 1}}})
	if !IsNotFound(err) {
		t.Fatalf("чужая позиция: %v", err)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM purchases`); n != 0 {
		t.Fatalf("покупка не откатилась: %d", n)
	}
}

func TestAddUnitMoveAdjustWriteoff(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 1000)
	box := e.stkLocation(uid, "Коробка")

	full, err := e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: p, RemainingQty: 1000, LocationID: &box})
	if err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(full), 1000, "sealed")
	initial := int64(500)
	part, err := e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: p, RemainingQty: 200, InitialQty: &initial})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.stkUnit(part); r.Status != "open" || !r.Opened.Valid || r.Location.Int64 != box {
		t.Fatalf("неполный остаток: %+v", r)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'adjust' AND reason = 'Начальный остаток'`); n != 2 {
		t.Fatalf("движений ввода остатка: %d", n)
	}
	_, err = e.svc.AddUnit(e.ctx, uid, AddUnitInput{ProductID: p, RemainingQty: 501, InitialQty: &initial})
	stkIs[*ValidationError](t, err, "остаток больше объёма")

	// Перемещение.
	shelf := e.stkLocation(uid, "Полка")
	if err := e.svc.Move(e.ctx, uid, full, shelf); err != nil {
		t.Fatal(err)
	}
	if r := e.stkUnit(full); r.Location.Int64 != shelf {
		t.Fatalf("после перемещения место %v", r.Location)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'move' AND qty_delta = 0 AND from_location_id = ? AND to_location_id = ?`, box, shelf); n != 1 {
		t.Fatalf("движение move: %d", n)
	}
	e.exec(`UPDATE locations SET archived = 1 WHERE id = ?`, box)
	stkIs[*ValidationError](t, e.svc.Move(e.ctx, uid, full, box), "архивное место")

	// Корректировка: sealed → open, 0 → finished, > 0 у finished → open.
	if _, err := e.svc.Adjust(e.ctx, uid, full, 900, "Пролил"); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(full), 900, "open")
	if _, err := e.svc.Adjust(e.ctx, uid, full, 0, ""); err != nil {
		t.Fatal(err)
	}
	if r := e.stkUnit(full); r.Status != "finished" || !r.Finished.Valid {
		t.Fatalf("после корректировки в 0: %+v", r)
	}
	stkIs[*ConflictError](t, e.svc.Move(e.ctx, uid, full, shelf), "перемещение законченного")
	if _, err := e.svc.Adjust(e.ctx, uid, full, 300, ""); err != nil {
		t.Fatal(err)
	}
	if r := e.stkUnit(full); r.Status != "open" || r.Finished.Valid || r.Remaining != 300 {
		t.Fatalf("после корректировки законченного: %+v", r)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE unit_id = ? AND type = 'adjust'`, full); n != 4 {
		t.Fatalf("движений adjust: %d", n)
	}
	stkIs[*ValidationError](t, second(e.svc.Adjust(e.ctx, uid, full, 1001, "")), "больше объёма")

	// Списание: часть, больше остатка, всё.
	ten := int64(10)
	if _, err := e.svc.Writeoff(e.ctx, uid, full, &ten, "Испорчено"); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(full), 290, "open")
	big := int64(291)
	if se := stkIs[*ShortageError](t, second(e.svc.Writeoff(e.ctx, uid, full, &big, "Испорчено")), "больше остатка"); se.Available != 290 {
		t.Fatalf("available %d", se.Available)
	}
	stkIs[*ValidationError](t, second(e.svc.Writeoff(e.ctx, uid, full, nil, " ")), "без причины")
	if _, err := e.svc.Writeoff(e.ctx, uid, full, nil, "Ошибка ввода"); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(full), 0, "written_off")
	// Корректировка списанного возвращает его в open.
	if _, err := e.svc.Adjust(e.ctx, uid, full, 50, ""); err != nil {
		t.Fatal(err)
	}
	stkWant(t, e.stkUnit(full), 50, "open")
}

// second отбрасывает первое значение (id движения) и оставляет ошибку.
func second(_ int64, err error) error { return err }
