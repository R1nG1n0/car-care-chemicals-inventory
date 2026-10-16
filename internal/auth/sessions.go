package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// maxUserAgent — сколько символов User-Agent сохраняется вместе с сессией.
const maxUserAgent = 256

// Session — сессия пользователя. Token заполнен только при создании:
// в базе лежит хеш.
type Session struct {
	Token     string
	TokenHash string
	UserID    int64
	ExpiresAt time.Time
}

// CreateSession создаёт сессию на SessionTTL. Заодно удаляются просроченные
// сессии всех пользователей: вход редок, таблица маленькая, отдельная
// фоновая чистка не нужна.
func (s *Service) CreateSession(ctx context.Context, userID int64, userAgent string) (Session, error) {
	token, hash, err := newToken()
	if err != nil {
		return Session{}, err
	}
	if r := []rune(userAgent); len(r) > maxUserAgent {
		userAgent = string(r[:maxUserAgent])
	}
	if _, err := s.DeleteExpiredSessions(ctx); err != nil {
		return Session{}, err
	}
	now := s.now().Unix()
	exp := now + int64(SessionTTL/time.Second)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)`, hash, userID, now, now, exp, userAgent); err != nil {
		return Session{}, fmt.Errorf("создание сессии: %w", err)
	}
	return Session{Token: token, TokenHash: hash, UserID: userID, ExpiresAt: time.Unix(exp, 0)}, nil
}

// ResolveSession находит сессию по токену из cookie и её пользователя.
// Если с последнего продления прошло больше SessionRefresh, срок сдвигается
// на SessionTTL от текущего момента, а extended = true: обработчику нужно
// заново выставить cookie.
func (s *Service) ResolveSession(ctx context.Context, token string) (sess Session, u User, extended bool, err error) {
	if token == "" {
		return Session{}, User{}, false, ErrNoSession
	}
	hash := hashToken(token)
	now := s.now().Unix()

	var lastSeen, exp, created int64
	err = s.db.QueryRowContext(ctx, `
		SELECT s.user_id, s.last_seen_at, s.expires_at, u.login, u.is_owner, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`, hash).
		Scan(&u.ID, &lastSeen, &exp, &u.Login, &u.IsOwner, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, false, ErrNoSession
	}
	if err != nil {
		return Session{}, User{}, false, fmt.Errorf("поиск сессии: %w", err)
	}
	if exp <= now {
		return Session{}, User{}, false, ErrNoSession
	}
	u.CreatedAt = time.Unix(created, 0)

	if now-lastSeen > int64(SessionRefresh/time.Second) {
		exp = now + int64(SessionTTL/time.Second)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`, now, exp, hash); err != nil {
			return Session{}, User{}, false, fmt.Errorf("продление сессии: %w", err)
		}
		extended = true
	}
	return Session{TokenHash: hash, UserID: u.ID, ExpiresAt: time.Unix(exp, 0)}, u, extended, nil
}

// DeleteSession удаляет сессию по токену. Несуществующая — не ошибка.
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token)); err != nil {
		return fmt.Errorf("удаление сессии: %w", err)
	}
	return nil
}

// DeleteUserSessions удаляет все сессии пользователя, кроме keepTokenHash
// (пусто — удаляются все).
func (s *Service) DeleteUserSessions(ctx context.Context, userID int64, keepTokenHash string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepTokenHash); err != nil {
		return fmt.Errorf("удаление сессий пользователя: %w", err)
	}
	return nil
}

// DeleteExpiredSessions удаляет просроченные сессии и возвращает их число.
func (s *Service) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, s.now().Unix())
	if err != nil {
		return 0, fmt.Errorf("чистка просроченных сессий: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
