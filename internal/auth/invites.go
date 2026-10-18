package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Статусы приглашения. В базе не хранятся — вычисляются из полей.
const (
	InviteActive  = "active"
	InviteUsed    = "used"
	InviteExpired = "expired"
	InviteRevoked = "revoked"
)

// Invite — приглашение без токена: токен виден один раз, при создании.
type Invite struct {
	ID        int64
	CreatedAt time.Time
	ExpiresAt time.Time
	Status    string
}

// CreateInvite создаёт приглашение на InviteTTL и возвращает его вместе
// с токеном для ссылки.
func (s *Service) CreateInvite(ctx context.Context, ownerID int64) (Invite, string, error) {
	token, hash, err := newToken()
	if err != nil {
		return Invite{}, "", err
	}
	now := s.now().Unix()
	exp := now + int64(InviteTTL/time.Second)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO invites (token_hash, created_by, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hash, ownerID, now, exp)
	if err != nil {
		return Invite{}, "", fmt.Errorf("создание приглашения: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Invite{}, "", fmt.Errorf("создание приглашения: %w", err)
	}
	return Invite{ID: id, CreatedAt: time.Unix(now, 0), ExpiresAt: time.Unix(exp, 0), Status: InviteActive}, token, nil
}

// ListInvites — последние приглашения, созданные пользователем, новые сверху.
func (s *Service) ListInvites(ctx context.Context, ownerID int64) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, expires_at, used_at IS NOT NULL, revoked_at IS NOT NULL
		FROM invites WHERE created_by = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, ownerID, inviteListLimit)
	if err != nil {
		return nil, fmt.Errorf("список приглашений: %w", err)
	}
	defer rows.Close()

	now := s.now().Unix()
	list := []Invite{}
	for rows.Next() {
		var (
			inv           Invite
			created, exp  int64
			used, revoked bool
		)
		if err := rows.Scan(&inv.ID, &created, &exp, &used, &revoked); err != nil {
			return nil, fmt.Errorf("список приглашений: %w", err)
		}
		inv.CreatedAt, inv.ExpiresAt = time.Unix(created, 0), time.Unix(exp, 0)
		switch {
		case used:
			inv.Status = InviteUsed
		case revoked:
			inv.Status = InviteRevoked
		case exp <= now:
			inv.Status = InviteExpired
		default:
			inv.Status = InviteActive
		}
		list = append(list, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список приглашений: %w", err)
	}
	return list, nil
}

// RevokeInvite отзывает своё неиспользованное приглашение. Чужое,
// несуществующее и уже использованное — ErrNotFound. Повторный отзыв
// не ошибка и не меняет время первого.
func (s *Service) RevokeInvite(ctx context.Context, ownerID, inviteID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE invites SET revoked_at = COALESCE(revoked_at, ?)
		WHERE id = ? AND created_by = ? AND used_at IS NULL`,
		s.now().Unix(), inviteID, ownerID)
	if err != nil {
		return fmt.Errorf("отзыв приглашения: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckInvite проверяет токен приглашения и возвращает срок действия.
func (s *Service) CheckInvite(ctx context.Context, token string) (time.Time, error) {
	var exp int64
	err := s.db.QueryRowContext(ctx, `
		SELECT expires_at FROM invites
		WHERE token_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		hashToken(token), s.now().Unix()).Scan(&exp)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrInviteInvalid
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("проверка приглашения: %w", err)
	}
	return time.Unix(exp, 0), nil
}

// AcceptInvite создаёт пользователя по приглашению. Создание пользователя
// и отметка об использовании — одна транзакция, а отметка ставится только
// на ещё не использованное приглашение: из двух одновременных принятий
// успешно ровно одно.
func (s *Service) AcceptInvite(ctx context.Context, token, login, password string) (User, error) {
	// Сначала приглашение: по мёртвой ссылке ошибки полей бессмысленны.
	// Проверка дешёвая и идёт до argon2id — на такую ссылку не тратим время.
	if _, err := s.CheckInvite(ctx, token); err != nil {
		return User{}, err
	}
	if err := ValidateLogin(login); err != nil {
		return User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, err
	}
	hash, err := s.HashPassword(password)
	if err != nil {
		return User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("принятие приглашения: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op

	now := s.now().Unix()
	var inviteID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM invites
		WHERE token_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		hashToken(token), now).Scan(&inviteID)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInviteInvalid
	}
	if err != nil {
		return User{}, fmt.Errorf("принятие приглашения: %w", err)
	}

	u, err := s.createUserTx(ctx, tx, login, hash, false)
	if err != nil {
		return User{}, err
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE invites SET used_by = ?, used_at = ?
		WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL`, u.ID, now, inviteID)
	if err != nil {
		return User{}, fmt.Errorf("принятие приглашения: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return User{}, ErrInviteInvalid
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("принятие приглашения: фиксация: %w", err)
	}
	return u, nil
}
