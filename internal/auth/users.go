package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// defaultPurposes — стартовый набор назначений (ФТ-15.1.2); тот же набор
// миграция 008 засевает существующим пользователям.
var defaultPurposes = []string{"ЛКП", "стекло", "пластик", "кожа", "диски", "шины", "текстиль"}

// defaultCategories — начальный набор категорий (ФТ-2.1, API.md).
var defaultCategories = []struct{ name, color string }{
	{"Шампуни", "blue"},
	{"Активная пена", "berry"},
	{"Очистители", "olive"},
	{"Полироли и защита", "ochre"},
	{"Стёкла", "petrol"},
	{"Кожа и пластик", "plum"},
	{"Шины и диски", "slate"},
	{"Микрофибра", "umber"},
	{"Губки и аппликаторы", "umber"},
	{"Щётки", "umber"},
	{"Прочее", "slate"},
}

// CreateUser создаёт пользователя. Первый пользователь в базе становится
// владельцем; owner делает владельцем явно (флаг -owner команды user add).
func (s *Service) CreateUser(ctx context.Context, login, password string, owner bool) (User, error) {
	if err := ValidateLogin(login); err != nil {
		return User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return User{}, err
	}
	// Хеш считается до транзакции: единственное соединение с базой
	// не должно ждать argon2id.
	hash, err := s.HashPassword(password)
	if err != nil {
		return User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("создание пользователя: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op

	if !owner {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return User{}, fmt.Errorf("создание пользователя: %w", err)
		}
		owner = n == 0
	}
	u, err := s.createUserTx(ctx, tx, login, hash, owner)
	if err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("создание пользователя: фиксация: %w", err)
	}
	return u, nil
}

// createUserTx добавляет пользователя, его настройки и справочники
// по умолчанию внутри уже открытой транзакции.
func (s *Service) createUserTx(ctx context.Context, tx *sql.Tx, login, hash string, owner bool) (User, error) {
	now := s.now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (login, password_hash, is_owner, created_at) VALUES (?, ?, ?, ?)`,
		login, hash, owner, now.Unix())
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrLoginTaken
		}
		return User{}, fmt.Errorf("создание пользователя: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("создание пользователя: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_settings (user_id, step_small, step_large) VALUES (?, ?, ?)`,
		id, DefaultSettings.StepSmall, DefaultSettings.StepLarge); err != nil {
		return User{}, fmt.Errorf("создание настроек пользователя: %w", err)
	}
	for i, c := range defaultCategories {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO categories (user_id, name, color, sort) VALUES (?, ?, ?, ?)`,
			id, c.name, c.color, i+1); err != nil {
			return User{}, fmt.Errorf("создание категорий по умолчанию: %w", err)
		}
	}
	for i, name := range defaultPurposes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO purposes (user_id, name, sort) VALUES (?, ?, ?)`, id, name, i+1); err != nil {
			return User{}, fmt.Errorf("создание назначений по умолчанию: %w", err)
		}
	}
	return User{ID: id, Login: login, IsOwner: owner, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// Authenticate проверяет логин и пароль. Для несуществующего логина время
// тратится так же, как для существующего, а ошибка одна — ErrBadCredentials.
func (s *Service) Authenticate(ctx context.Context, login, password string) (User, error) {
	u, hash, err := s.userWithHash(ctx, login)
	if errors.Is(err, ErrNotFound) {
		s.burnTime(password)
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	ok, err := s.CheckPassword(hash, password)
	if err != nil {
		return User{}, fmt.Errorf("проверка пароля %s: %w", u.Login, err)
	}
	if !ok {
		return User{}, ErrBadCredentials
	}
	return u, nil
}

// UserByLogin ищет пользователя без учёта регистра логина.
func (s *Service) UserByLogin(ctx context.Context, login string) (User, error) {
	u, _, err := s.userWithHash(ctx, login)
	return u, err
}

func (s *Service) userWithHash(ctx context.Context, login string) (User, string, error) {
	var (
		u       User
		hash    string
		created int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, login, is_owner, created_at, password_hash FROM users WHERE login = ?`, login).
		Scan(&u.ID, &u.Login, &u.IsOwner, &created, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("поиск пользователя: %w", err)
	}
	u.CreatedAt = time.Unix(created, 0)
	return u, hash, nil
}

// ListUsers возвращает всех пользователей по порядку создания.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, login, is_owner, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("список пользователей: %w", err)
	}
	defer rows.Close()
	var list []User
	for rows.Next() {
		var (
			u       User
			created int64
		)
		if err := rows.Scan(&u.ID, &u.Login, &u.IsOwner, &created); err != nil {
			return nil, fmt.Errorf("список пользователей: %w", err)
		}
		u.CreatedAt = time.Unix(created, 0)
		list = append(list, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список пользователей: %w", err)
	}
	return list, nil
}

// SetPassword задаёт новый пароль и удаляет все сессии пользователя,
// кроме keepTokenHash (пусто — удаляются все).
func (s *Service) SetPassword(ctx context.Context, userID int64, password, keepTokenHash string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := s.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("смена пароля: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op

	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID)
	if err != nil {
		return fmt.Errorf("смена пароля: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepTokenHash); err != nil {
		return fmt.Errorf("смена пароля: сброс сессий: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("смена пароля: фиксация: %w", err)
	}
	return nil
}

// CheckUserPassword проверяет пароль пользователя (текущий пароль при смене).
func (s *Service) CheckUserPassword(ctx context.Context, userID int64, password string) (bool, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		s.burnTime(password)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("проверка пароля: %w", err)
	}
	return s.CheckPassword(hash, password)
}

// GetSettings возвращает настройки пользователя; если строки нет —
// значения по умолчанию.
func (s *Service) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	var st Settings
	err := s.db.QueryRowContext(ctx,
		`SELECT step_small, step_large FROM user_settings WHERE user_id = ?`, userID).
		Scan(&st.StepSmall, &st.StepLarge)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("чтение настроек: %w", err)
	}
	return st, nil
}

// UpdateSettings проверяет и сохраняет настройки.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, st Settings) (Settings, error) {
	if err := ValidateSettings(st); err != nil {
		return Settings{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (user_id, step_small, step_large) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET step_small = excluded.step_small, step_large = excluded.step_large`,
		userID, st.StepSmall, st.StepLarge); err != nil {
		return Settings{}, fmt.Errorf("сохранение настроек: %w", err)
	}
	return st, nil
}
