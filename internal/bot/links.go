// Package bot — Telegram-бот garage (ФТ-11): привязка аккаунта, остатки,
// быстрый расход, список покупок и уведомления о минимуме.
//
// Бот работает с теми же сервисами catalog и stock, что и сайт, и всегда с
// user_id из привязки чата — никогда из текста сообщения или кнопки.
package bot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// CodeTTL — срок действия кода привязки (ФТ-11.2.1).
const CodeTTL = 10 * time.Minute

// ErrBadCode — код привязки неверный, просрочен или уже использован.
var ErrBadCode = errors.New("код привязки недействителен")

// Link — привязка Telegram-аккаунта к пользователю сайта.
type Link struct {
	UserID   int64
	TGUserID int64
	ChatID   int64
	TGName   string
	LinkedAt int64
}

// Links — привязки аккаунтов и одноразовые коды в базе.
type Links struct {
	db  *sql.DB
	now func() time.Time
}

// NewLinks создаёт хранилище привязок. now — часы; nil — time.Now.
func NewLinks(db *sql.DB, now func() time.Time) *Links {
	if now == nil {
		now = time.Now
	}
	return &Links{db: db, now: now}
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// NewCode выдаёт одноразовый код привязки для пользователя: 32 случайных
// байта в base64url (подходит для параметра start). В базе — только хеш.
// Прежние неиспользованные коды пользователя гаснут.
func (l *Links) NewCode(ctx context.Context, userID int64) (code string, expiresAt int64, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", 0, fmt.Errorf("код привязки: %w", err)
	}
	code = base64.RawURLEncoding.EncodeToString(raw)
	now := l.now()
	expiresAt = now.Add(CodeTTL).Unix()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback() //nolint:errcheck // после Commit это no-op
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM telegram_link_codes WHERE user_id = ? OR expires_at <= ?`, userID, now.Unix()); err != nil {
		return "", 0, fmt.Errorf("очистка кодов привязки: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO telegram_link_codes (hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashCode(code), userID, expiresAt); err != nil {
		return "", 0, fmt.Errorf("запись кода привязки: %w", err)
	}
	return code, expiresAt, tx.Commit()
}

// Redeem принимает код из /start: связывает Telegram-аккаунт с пользователем
// сайта. Код одноразовый (тратится, даже если просрочен). Один аккаунт
// Telegram — один пользователь, новая привязка заменяет старую; у вытесненного
// пользователя гаснет и состояние уведомлений.
func (l *Links) Redeem(ctx context.Context, code string, tgUserID, chatID int64, tgName string) (Link, error) {
	now := l.now().Unix()
	link := Link{TGUserID: tgUserID, ChatID: chatID, TGName: tgName, LinkedAt: now}
	err := inTx(ctx, l.db, func(tx *sql.Tx) error {
		var expires int64
		h := hashCode(code)
		err := tx.QueryRowContext(ctx,
			`SELECT user_id, expires_at FROM telegram_link_codes WHERE hash = ?`, h).Scan(&link.UserID, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBadCode
		}
		if err != nil {
			return fmt.Errorf("чтение кода привязки: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM telegram_link_codes WHERE hash = ?`, h); err != nil {
			return fmt.Errorf("удаление кода привязки: %w", err)
		}
		if expires <= now {
			return ErrBadCode
		}
		// Этот Telegram-аккаунт мог быть привязан к другому пользователю.
		if _, err := tx.ExecContext(ctx, `DELETE FROM bot_low_state WHERE user_id IN
			(SELECT user_id FROM telegram_links WHERE tg_user_id = ? AND user_id <> ?)`, tgUserID, link.UserID); err != nil {
			return fmt.Errorf("очистка состояния: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM telegram_links WHERE tg_user_id = ? OR user_id = ?`, tgUserID, link.UserID); err != nil {
			return fmt.Errorf("замена привязки: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO telegram_links (user_id, tg_user_id, chat_id, tg_name, linked_at)
			VALUES (?, ?, ?, ?, ?)`, link.UserID, tgUserID, chatID, tgName, now); err != nil {
			return fmt.Errorf("запись привязки: %w", err)
		}
		return nil
	})
	if err != nil {
		return Link{}, err
	}
	return link, nil
}

// ByTelegram — привязка по Telegram-аккаунту; ok=false — не привязан.
func (l *Links) ByTelegram(ctx context.Context, tgUserID int64) (Link, bool, error) {
	return l.one(ctx, `WHERE tg_user_id = ?`, tgUserID)
}

// ByUser — привязка пользователя сайта; ok=false — Telegram не подключён.
func (l *Links) ByUser(ctx context.Context, userID int64) (Link, bool, error) {
	return l.one(ctx, `WHERE user_id = ?`, userID)
}

func (l *Links) one(ctx context.Context, where string, arg int64) (Link, bool, error) {
	var k Link
	err := l.db.QueryRowContext(ctx,
		`SELECT user_id, tg_user_id, chat_id, tg_name, linked_at FROM telegram_links `+where, arg).
		Scan(&k.UserID, &k.TGUserID, &k.ChatID, &k.TGName, &k.LinkedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, false, nil
	}
	if err != nil {
		return Link{}, false, fmt.Errorf("привязка Telegram: %w", err)
	}
	return k, true, nil
}

// All — все привязки (для проверки минимумов).
func (l *Links) All(ctx context.Context) ([]Link, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT user_id, tg_user_id, chat_id, tg_name, linked_at FROM telegram_links ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("привязки Telegram: %w", err)
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var k Link
		if err := rows.Scan(&k.UserID, &k.TGUserID, &k.ChatID, &k.TGName, &k.LinkedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Unlink отключает Telegram пользователя: привязка, его коды и состояние
// уведомлений. Нет привязки — не ошибка.
func (l *Links) Unlink(ctx context.Context, userID int64) error {
	return inTx(ctx, l.db, func(tx *sql.Tx) error {
		for _, q := range []string{
			`DELETE FROM telegram_links WHERE user_id = ?`,
			`DELETE FROM telegram_link_codes WHERE user_id = ?`,
			`DELETE FROM bot_low_state WHERE user_id = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, userID); err != nil {
				return fmt.Errorf("отключение Telegram: %w", err)
			}
		}
		return nil
	})
}

// inTx выполняет fn в транзакции. Соединение в базе одно (store.Open), поэтому
// внутри fn нельзя обращаться к *sql.DB.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // после Commit это no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
