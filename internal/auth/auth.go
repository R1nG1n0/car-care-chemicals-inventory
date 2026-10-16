// Package auth — учётные записи, сессии, приглашения и ограничение попыток
// входа (ФТ-1, ТБ-2, ТБ-4). Пакет не знает про HTTP: обработчики в
// internal/web вызывают его методы и переводят ошибки в коды ответа.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Сроки из ФТ-1.1 и ФТ-1.3.
const (
	SessionTTL     = 30 * 24 * time.Hour
	SessionRefresh = time.Hour
	InviteTTL      = 7 * 24 * time.Hour

	// inviteListLimit — сколько приглашений показывает список (API.md).
	inviteListLimit = 20
)

// Ошибки, которые обработчики переводят в коды ответа.
var (
	// ErrBadCredentials — неверный логин или пароль. Одна ошибка на оба
	// случая, чтобы по ответу нельзя было узнать, есть ли такой логин.
	ErrBadCredentials = errors.New("неверный логин или пароль")
	ErrLoginTaken     = errors.New("логин занят")
	ErrNotFound       = errors.New("не найдено")
	ErrNoSession      = errors.New("сессии нет или она истекла")
	ErrInviteInvalid  = errors.New("приглашение не найдено или уже недействительно")
)

// ValidationError — ввод не прошёл проверку (ТБ-7). Field — имя поля формы.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// User — учётная запись.
type User struct {
	ID        int64     `json:"id"`
	Login     string    `json:"login"`
	IsOwner   bool      `json:"is_owner"`
	CreatedAt time.Time `json:"-"`
}

// Settings — шаги кнопок количества (ФТ-5.8).
type Settings struct {
	StepSmall int `json:"step_small"`
	StepLarge int `json:"step_large"`
}

// DefaultSettings совпадают со значениями по умолчанию в схеме.
var DefaultSettings = Settings{StepSmall: 10, StepLarge: 50}

// Options — необязательные параметры Service.
type Options struct {
	// Now — источник времени; nil — time.Now. Подменяется в тестах.
	Now func() time.Time
	// Params — параметры argon2id для новых хешей; нулевые — DefaultParams.
	Params Params
	// MaxHashers — сколько хешей считается одновременно; 0 — 2.
	MaxHashers int
}

// Service — учётные записи, сессии и приглашения поверх базы.
type Service struct {
	db     *sql.DB
	now    func() time.Time
	params Params
	// hashers ограничивает одновременные вычисления argon2id: каждое
	// занимает Params.Memory КиБ, на маленьком VPS это важно.
	hashers chan struct{}

	dummyOnce sync.Once
	dummyHash string
}

// New создаёт сервис. db — открытая база с применёнными миграциями.
func New(db *sql.DB, opt Options) *Service {
	s := &Service{db: db, now: opt.Now, params: opt.Params}
	if s.now == nil {
		s.now = time.Now
	}
	if s.params == (Params{}) {
		s.params = DefaultParams
	}
	n := opt.MaxHashers
	if n <= 0 {
		n = 2
	}
	s.hashers = make(chan struct{}, n)
	return s
}

// SetClock подменяет источник времени (для тестов).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// newToken возвращает случайный токен (32 байта в base64url) и его хеш
// для базы. Сам токен в базу не попадает (ТБ-2).
func newToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("генерация токена: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

// hashToken — sha256 токена в hex. Токен случайный и длинный, поэтому
// медленный хеш не нужен: перебор 256 бит невозможен и так.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
