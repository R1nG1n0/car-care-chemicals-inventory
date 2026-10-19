// Package report — отчёты по покупкам и расходу (ФТ-10.2).
//
// Только чтение: все запросы фильтруются по user_id (ТБ-5). Пакет не знает
// про HTTP: ошибки проверки параметров — *ValidationError, обработчик в
// internal/web переводит их в 400 с полем.
package report

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Service строит отчёты по базе.
type Service struct{ db *sql.DB }

// New создаёт сервис.
func New(db *sql.DB) *Service { return &Service{db: db} }

// ValidationError — неверный параметр запроса.
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

const (
	dateLayout = "2006-01-02"
	// maxYears — наибольший период отчёта.
	maxYears = 5
	// maxTZMinutes — наибольшее смещение часового пояса клиента, минуты.
	maxTZMinutes = 14 * 60
)

// Period — границы отчёта, даты включительно.
type Period struct {
	From, To time.Time // 00:00 UTC выбранных дат
	// TZ — смещение часового пояса клиента от UTC в минутах (МСК = 180):
	// по нему даты периода переводятся в секунды Unix для движений.
	TZ int
}

// ParsePeriod проверяет параметры from, to (ГГГГ-ММ-ДД) и tz (минуты, необязательно).
func ParsePeriod(from, to string, tz int) (Period, error) {
	f, err := parseDate("from", from)
	if err != nil {
		return Period{}, err
	}
	t, err := parseDate("to", to)
	if err != nil {
		return Period{}, err
	}
	if t.Before(f) {
		return Period{}, &ValidationError{"to", "Конец периода раньше начала"}
	}
	if t.After(f.AddDate(maxYears, 0, 0)) {
		return Period{}, &ValidationError{"to", fmt.Sprintf("Период не больше %d лет", maxYears)}
	}
	if tz < -maxTZMinutes || tz > maxTZMinutes {
		return Period{}, &ValidationError{"tz", "Неверный часовой пояс"}
	}
	return Period{From: f, To: t, TZ: tz}, nil
}

func parseDate(field, v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, &ValidationError{field, "Укажите дату"}
	}
	d, err := time.Parse(dateLayout, v)
	if err != nil {
		return time.Time{}, &ValidationError{field, "Дата должна быть в виде ГГГГ-ММ-ДД"}
	}
	return d, nil
}

// bounds — [с, по) в секундах Unix с учётом часового пояса клиента.
func (p Period) bounds() (int64, int64) {
	off := int64(p.TZ) * 60
	return p.From.Unix() - off, p.To.AddDate(0, 0, 1).Unix() - off
}

// months — все месяцы периода ГГГГ-ММ по порядку, включая пустые.
func (p Period) months() []string {
	out := []string{}
	cur := time.Date(p.From.Year(), p.From.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cur.After(p.To) {
		out = append(out, cur.Format("2006-01"))
		cur = cur.AddDate(0, 1, 0)
	}
	return out
}

func rowsErr(what string, err error) error { return fmt.Errorf("отчёт, %s: %w", what, err) }

// query выполняет запрос и вызывает scan на каждой строке.
func (s *Service) query(ctx context.Context, what, q string, args []any, scan func(*sql.Rows) error) error {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return rowsErr(what, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return rowsErr(what, err)
		}
	}
	if err := rows.Err(); err != nil {
		return rowsErr(what, err)
	}
	return nil
}
