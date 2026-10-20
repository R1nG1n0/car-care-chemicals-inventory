package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Пределы идемпотентности и времени с телефона (ФТ-12.4).
const (
	opKindConsume = "consume"
	opKindApply   = "apply"

	opTTL       = 90 * 24 * time.Hour // сколько хранятся номера операций
	occurredMax = 5 * time.Minute     // насколько время может быть в будущем
	occurredMin = 30 * 24 * time.Hour // насколько давним оно может быть
)

var opIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// Op — необязательные поля запроса расхода без связи: номер операции для
// безопасного повтора и время, когда расходовали на самом деле.
type Op struct {
	ClientOpID string `json:"client_op_id"`
	OccurredAt *int64 `json:"occurred_at"`
}

// validate проверяет формат номера операции.
func (o Op) validate() error {
	if o.ClientOpID != "" && !opIDRe.MatchString(o.ClientOpID) {
		return &ValidationError{Field: "client_op_id",
			Message: "Номер операции — 8–64 символа: латинские буквы, цифры, «_» и «-»"}
	}
	return nil
}

// at — время движений: occurred_at, если оно не дальше 5 минут в будущее и не
// старше 30 дней, иначе now (молча, ФТ-12.4.2).
func (o Op) at(now time.Time) int64 {
	if o.OccurredAt == nil {
		return now.Unix()
	}
	t := *o.OccurredAt
	if t > now.Add(occurredMax).Unix() || t < now.Add(-occurredMin).Unix() {
		return now.Unix()
	}
	return t
}

// Render собирает JSON ответа внутри транзакции записи: чтения идут через
// tx (у базы одно соединение). При client_op_id ответ сохраняется вместе
// со списанием.
type Render[R any] func(tx *sql.Tx, res R) ([]byte, error)

// replayed ищет сохранённый ответ операции. Номер с другим видом операции —
// ConflictError. Нет записи — nil.
func replayed(ctx context.Context, tx *sql.Tx, userID int64, opID, kind string) ([]byte, error) {
	var gotKind, body string
	err := tx.QueryRowContext(ctx,
		`SELECT kind, response FROM client_ops WHERE user_id = ? AND op_id = ?`, userID, opID).Scan(&gotKind, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("чтение номера операции: %w", err)
	}
	if gotKind != kind {
		return nil, &ConflictError{Field: "client_op_id", Message: "Этот номер операции уже использован для другого действия"}
	}
	return []byte(body), nil
}

// remember сохраняет ответ операции и заодно удаляет номера старше 90 дней.
func remember(ctx context.Context, tx *sql.Tx, userID int64, opID, kind string, body []byte, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM client_ops WHERE created_at < ?`,
		now.Add(-opTTL).Unix()); err != nil {
		return fmt.Errorf("очистка номеров операций: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO client_ops (user_id, op_id, kind, response, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, opID, kind, string(body), now.Unix()); err != nil {
		return fmt.Errorf("запись номера операции: %w", err)
	}
	return nil
}

// runOp выполняет exec в транзакции с идемпотентностью. Без номера операции —
// просто exec. С номером: уже есть — exec не вызывается, возвращается
// сохранённый ответ (replay = true); иначе после exec ответ собирается
// render (если задан) и при номере операции сохраняется в той же транзакции.
func runOp[R any](ctx context.Context, s *Service, userID int64, op Op, kind string,
	exec func(tx *sql.Tx, at int64) (R, error), render Render[R]) (res R, body []byte, replay bool, err error) {
	if err = op.validate(); err != nil {
		return
	}
	now := s.now()
	at := op.at(now)
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if op.ClientOpID != "" {
			saved, err := replayed(ctx, tx, userID, op.ClientOpID, kind)
			if err != nil {
				return err
			}
			if saved != nil {
				body, replay = saved, true
				return nil
			}
		}
		var err error
		if res, err = exec(tx, at); err != nil {
			return err
		}
		if render == nil {
			return nil
		}
		if body, err = render(tx, res); err != nil {
			return err
		}
		if op.ClientOpID == "" {
			return nil
		}
		return remember(ctx, tx, userID, op.ClientOpID, kind, body, now)
	})
	return
}
