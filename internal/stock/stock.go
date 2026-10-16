// Package stock — покупки, экземпляры, расход, движения и шаблоны расхода
// (ФТ-4, ФТ-5, ФТ-8).
//
// Все методы принимают userID и видят только данные этого пользователя (ТБ-5).
// Пакет только пишет и возвращает идентификаторы и свои объекты; сводки
// позиций, экземпляры и движения в формате API читает пакет catalog.
//
// У базы одно соединение (store.Open), поэтому внутри транзакции нельзя
// обращаться к *sql.DB — только к *sql.Tx, иначе запрос будет ждать вечно.
package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Пределы из docs/API.md (ТБ-7).
const (
	maxQty        = 100000
	maxPrice      = 100000000
	maxPackages   = 100
	maxLines      = 30
	maxReasonLen  = 80
	maxNoteLen    = 500
	maxRecipeName = 60
	maxRevertIDs  = 100
	defaultLimit  = 50
	maxLimit      = 100

	// initialReason — причина движения при вводе имеющегося остатка.
	initialReason = "Начальный остаток"
)

// ValidationError — ввод не прошёл проверку (400). Field — имя поля формы.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// NotFoundError — записи нет или она чужая (404). Сообщение одинаковое в
// обоих случаях, чтобы нельзя было узнать о чужих данных (ТБ-5).
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

// ConflictError — действие противоречит состоянию данных (409).
type ConflictError struct {
	Field   string
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// ShortageError — на складе меньше, чем просят списать (409 с available).
type ShortageError struct {
	Available int64
	Unit      string // ml | pcs
}

func (e *ShortageError) Error() string {
	return fmt.Sprintf("Не хватает: есть %d %s", e.Available, unitLabel(e.Unit))
}

// Готовые ошибки «не найдено».
var (
	errProductNotFound  = &NotFoundError{Message: "Позиция не найдена"}
	errUnitNotFound     = &NotFoundError{Message: "Экземпляр не найден"}
	errLocationNotFound = &NotFoundError{Message: "Место не найдено"}
	errStoreNotFound    = &NotFoundError{Message: "Магазин не найден"}
	errPurchaseNotFound = &NotFoundError{Message: "Покупка не найдена"}
	errMovementNotFound = &NotFoundError{Message: "Движение не найдено"}
	errRecipeNotFound   = &NotFoundError{Message: "Шаблон не найден"}
)

// IsNotFound сообщает, что ошибка — «не найдено».
func IsNotFound(err error) bool {
	var nf *NotFoundError
	return errors.As(err, &nf)
}

func unitLabel(unit string) string {
	if unit == "pcs" {
		return "шт"
	}
	return "мл"
}

// Service — склад поверх базы.
type Service struct {
	db  *sql.DB
	now func() time.Time
}

// New создаёт сервис. now — часы; nil — time.Now.
func New(db *sql.DB, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

// querier — общее у *sql.DB и *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// inTx выполняет fn в транзакции: ошибка — откат, иначе фиксация.
func (s *Service) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}
	return nil
}

// UnitCost — себестоимость движения в копейках: |qty_delta| × цена упаковки
// ÷ initial_qty экземпляра, с округлением к ближайшему. nil — у экземпляра
// нет цены (введён без покупки). Так же считает catalog (Movement.Cost).
func UnitCost(qtyDelta int64, price sql.NullInt64, initialQty int64) *int64 {
	if !price.Valid || initialQty <= 0 {
		return nil
	}
	if qtyDelta < 0 {
		qtyDelta = -qtyDelta
	}
	c := (qtyDelta*price.Int64 + initialQty/2) / initialQty
	return &c
}

// addCost прибавляет c к сумме; сумма остаётся nil, пока не встретится цена.
func addCost(sum *int64, c *int64) *int64 {
	if c == nil {
		return sum
	}
	if sum == nil {
		v := *c
		return &v
	}
	v := *sum + *c
	return &v
}

// cleanText обрезает пробелы и проверяет длину в символах.
func cleanText(s string, minLen, maxLen int, field, msg string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < minLen || n > maxLen {
		return "", &ValidationError{Field: field, Message: msg}
	}
	return s, nil
}

func isUniqueViolation(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// productInfo — то, что складу нужно знать о позиции.
type productInfo struct {
	ID          int64
	Name        string
	Unit        string
	PackageSize int64
}

// product читает свою позицию; чужая или несуществующая — 404.
func product(ctx context.Context, q querier, userID, id int64) (productInfo, error) {
	p := productInfo{ID: id}
	err := q.QueryRowContext(ctx,
		`SELECT name, unit, package_size FROM products WHERE user_id = ? AND id = ?`,
		userID, id).Scan(&p.Name, &p.Unit, &p.PackageSize)
	if errors.Is(err, sql.ErrNoRows) {
		return p, errProductNotFound
	}
	if err != nil {
		return p, fmt.Errorf("чтение позиции: %w", err)
	}
	return p, nil
}

// activeLocation проверяет, что место своё и не в архиве: в архивном месте
// не может лежать экземпляров (API.md, справочники).
func activeLocation(ctx context.Context, q querier, userID, id int64) error {
	var archived bool
	err := q.QueryRowContext(ctx,
		`SELECT archived FROM locations WHERE user_id = ? AND id = ?`, userID, id).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return errLocationNotFound
	}
	if err != nil {
		return fmt.Errorf("чтение места: %w", err)
	}
	if archived {
		return &ValidationError{Field: "location_id", Message: "Место в архиве — выберите другое"}
	}
	return nil
}

// defaultLocation — место последнего экземпляра позиции, если оно не в
// архиве; иначе нет места (ФТ-4.3).
func defaultLocation(ctx context.Context, q querier, userID, productID int64) (sql.NullInt64, error) {
	var loc sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT u.location_id FROM units u
		JOIN locations l ON l.user_id = u.user_id AND l.id = u.location_id
		WHERE u.user_id = ? AND u.product_id = ? AND l.archived = 0
		ORDER BY u.id DESC LIMIT 1`, userID, productID).Scan(&loc)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, nil
	}
	if err != nil {
		return loc, fmt.Errorf("место по умолчанию: %w", err)
	}
	return loc, nil
}

// resolveLocation — место из запроса (своё и действующее) или по умолчанию.
func resolveLocation(ctx context.Context, q querier, userID, productID int64, requested *int64) (sql.NullInt64, error) {
	if requested != nil {
		if err := activeLocation(ctx, q, userID, *requested); err != nil {
			return sql.NullInt64{}, err
		}
		return sql.NullInt64{Int64: *requested, Valid: true}, nil
	}
	return defaultLocation(ctx, q, userID, productID)
}

// insertMovement пишет движение и возвращает его id. Все движения одного
// запроса — одна группа (batch_id): id первого из них. m.batch — общая для
// запроса переменная: 0 — группа ещё не начата, её начинает это движение;
// nil — движение само по себе группа.
func insertMovement(ctx context.Context, q querier, m movementRow) (int64, error) {
	if m.batch == nil {
		m.batch = new(int64)
	}
	res, err := q.ExecContext(ctx, `
		INSERT INTO movements (user_id, unit_id, type, qty_delta, from_location_id, to_location_id,
		                       reason, recipe_run_id, batch_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, 0), ?)`,
		m.userID, m.unitID, m.typ, m.delta, m.from, m.to, m.reason, m.runID, *m.batch, m.at)
	if err != nil {
		return 0, fmt.Errorf("запись движения: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if *m.batch == 0 {
		if _, err := q.ExecContext(ctx, `UPDATE movements SET batch_id = ? WHERE user_id = ? AND id = ?`,
			id, m.userID, id); err != nil {
			return 0, fmt.Errorf("группа движений: %w", err)
		}
		*m.batch = id
	}
	return id, nil
}

type movementRow struct {
	userID, unitID int64
	typ            string
	delta          int64
	from, to       sql.NullInt64
	reason         string
	runID          sql.NullInt64
	batch          *int64 // группа запроса, см. insertMovement
	at             int64
}

// validLimit — размер страницы: 0 — по умолчанию, иначе 1–100.
func validLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultLimit, nil
	}
	if limit < 1 || limit > maxLimit {
		return 0, &ValidationError{Field: "limit", Message: "limit — от 1 до 100"}
	}
	return limit, nil
}
