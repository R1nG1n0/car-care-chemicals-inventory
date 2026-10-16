// Package catalog — справочники (категории, места, магазины), позиции,
// сводка остатков по позициям и список покупок (ФТ-2, ФТ-3, ФТ-6, ФТ-7).
//
// Все методы принимают userID и видят только данные этого пользователя (ТБ-5).
// Пакет не знает про HTTP: обработчики в internal/web переводят ошибки
// (*Error с видом ErrInvalid, ErrNotFound, ErrConflict) в коды ответа.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Виды ошибок. Проверять через errors.Is: конкретная ошибка — *Error
// с текстом для человека.
var (
	// ErrInvalid — ввод не прошёл проверку (ТБ-7), 400 с полем.
	ErrInvalid = errors.New("неверный ввод")
	// ErrNotFound — записи нет или она чужая (одинаково, ТБ-5), 404.
	ErrNotFound = errors.New("не найдено")
	// ErrConflict — нарушение правила данных (дубль имени, архив места
	// с экземплярами, смена единицы), 409.
	ErrConflict = errors.New("конфликт")
)

// Error — ошибка с текстом для пользователя. Kind — один из ErrInvalid,
// ErrNotFound, ErrConflict; Field — имя поля формы (может быть пустым).
type Error struct {
	Kind    error
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Kind }

func invalid(field, msg string) error {
	return &Error{Kind: ErrInvalid, Field: field, Message: msg}
}

func notFound(msg string) error {
	return &Error{Kind: ErrNotFound, Message: msg}
}

func conflict(field, msg string) error {
	return &Error{Kind: ErrConflict, Field: field, Message: msg}
}

// Тексты «не найдено» для разных записей.
const (
	msgProductNotFound  = "Позиция не найдена"
	msgCategoryNotFound = "Категория не найдена"
	msgLocationNotFound = "Место не найдено"
	msgStoreNotFound    = "Магазин не найден"
	msgUnitNotFound     = "Экземпляр не найден"
	msgShoppingNotFound = "Пункт списка не найден"
)

// querier — общее у *sql.DB и *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Service — справочники, позиции и список покупок.
type Service struct {
	db *sql.DB
	q  querier
}

// New создаёт сервис над базой.
func New(db *sql.DB) *Service {
	return &Service{db: db, q: db}
}

// WithTx возвращает сервис, чьи чтения идут внутри транзакции tx.
// Нужен, чтобы читать сводки до Commit: у базы одно соединение
// (store.Open), и запрос мимо открытой транзакции ждал бы её вечно.
// Изменяющие методы такого сервиса тоже работают в tx.
func (s *Service) WithTx(tx *sql.Tx) *Service {
	return &Service{db: s.db, q: tx}
}

// inTx выполняет fn в транзакции. Если сервис уже привязан к транзакции
// (WithTx), fn работает в ней, а фиксирует её владелец.
func (s *Service) inTx(ctx context.Context, fn func(q querier) error) error {
	if _, ok := s.q.(*sql.Tx); ok {
		return fn(s.q)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func isUniqueViolation(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// placeholders — «?, ?, ?» для n аргументов IN (…).
func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}

// idArgs — userID и затем ids как аргументы запроса.
func idArgs(userID int64, ids []int64) []any {
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}

// divRound — a×b÷c с округлением к ближайшему (a, b ≥ 0, c > 0).
func divRound(a, b, c int64) int64 {
	return (a*b + c/2) / c
}

func nullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

func nullString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
