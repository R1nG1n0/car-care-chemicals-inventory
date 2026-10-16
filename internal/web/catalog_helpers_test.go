package web

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

// catEnv — как newEnv, но с доступом к базе: экземпляры, покупки и движения
// тесты каталога создают прямым SQL.
type catEnv struct {
	*env
	db *sql.DB
}

func catNewEnv(t *testing.T) *catEnv {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	c := &testClock{t: time.Unix(1_792_000_000, 0)}
	svc := auth.New(db.DB, auth.Options{Now: c.Now, Params: auth.Params{Time: 1, Memory: 64, Threads: 1}})
	srv := New(Options{
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:         "test",
		Auth:            svc,
		Limiter:         auth.NewLimiter(auth.LimitAttempts, auth.LimitWindow, c.Now),
		InsecureCookies: true,
		DB:              db.DB,
		Now:             c.Now,
	})
	return &catEnv{env: &env{t: t, h: srv.Handler(), svc: svc, clock: c}, db: db.DB}
}

// catLogin создаёт пользователя и возвращает его вошедший клиент.
func (e *catEnv) catLogin(login string) *client {
	e.t.Helper()
	e.createUser(login, login+"-password", false)
	c := e.client()
	c.login(login, login+"-password")
	return c
}

func (e *catEnv) catExec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// catUnit — экземпляр позиции пользователя, которому принадлежит позиция.
func (e *catEnv) catUnit(product int64, loc any, initial, remaining int64, status string, opened any) int64 {
	return e.catExec(`INSERT INTO units (user_id, product_id, location_id, initial_qty, remaining_qty, status, opened_at)
		SELECT user_id, id, ?, ?, ?, ?, ? FROM products WHERE id = ?`, loc, initial, remaining, status, opened, product)
}

// catPurchase — покупка позиции с одной строкой и экземпляром на упаковку.
func (e *catEnv) catPurchase(product int64, store any, date string, price, at int64) int64 {
	pid := e.catExec(`INSERT INTO purchases (user_id, store_id, date, created_at)
		SELECT user_id, ?, ?, ? FROM products WHERE id = ?`, store, date, at, product)
	line := e.catExec(`INSERT INTO purchase_lines (user_id, purchase_id, product_id, packages, price_per_package)
		SELECT user_id, ?, id, 1, ? FROM products WHERE id = ?`, pid, price, product)
	e.catExec(`INSERT INTO units (user_id, product_id, purchase_line_id, initial_qty, remaining_qty)
		SELECT user_id, id, ?, package_size, package_size FROM products WHERE id = ?`, line, product)
	return pid
}

type catCategories struct {
	Categories []catalog.Category `json:"categories"`
}
type catLocations struct {
	Locations []catalog.Location `json:"locations"`
}
type catStores struct {
	Stores []catalog.Store `json:"stores"`
}
type catProducts struct {
	Products []catalog.ProductSummary `json:"products"`
}

// catFirstCategory — id первой действующей категории клиента.
func catFirstCategory(t *testing.T, c *client) int64 {
	t.Helper()
	list := decode[catCategories](t, c.do(http.MethodGet, "/api/categories", nil))
	if len(list.Categories) == 0 {
		t.Fatal("нет категорий по умолчанию")
	}
	return list.Categories[0].ID
}

// catProduct создаёт позицию через API.
func catProduct(t *testing.T, c *client, body map[string]any) catalog.ProductSummary {
	t.Helper()
	rec := c.do(http.MethodPost, "/api/products", body)
	expect(t, rec, http.StatusCreated, fmt.Sprintf("создание позиции %v", body["name"]))
	return decode[catalog.ProductSummary](t, rec)
}

// catExpectField — код и поле ошибки.
func catExpectField(t *testing.T, c *client, method, target string, body any, code int, field string) {
	t.Helper()
	rec := c.do(method, target, body)
	expect(t, rec, code, method+" "+target)
	if got := decode[errBody](t, rec); got.Field != field || got.Error == "" {
		t.Fatalf("%s %s: ошибка %+v, ожидалось поле %q", method, target, got, field)
	}
}
