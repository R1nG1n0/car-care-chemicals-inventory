package weather

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxNameRunes — предел длины названия места.
const MaxNameRunes = 80

// Service — погода для сайта и бота: место пользователя, кэш прогноза, поиск.
type Service struct {
	client *Client
	cache  *Cache
	places *Places
	def    *Place
	now    func() time.Time
}

// ServiceOptions — настройки сервиса.
type ServiceOptions struct {
	DB *sql.DB
	// Default — место по умолчанию из конфигурации; nil — нет.
	Default *Place
	Client  *Client
	// Now — часы; nil — time.Now.
	Now func() time.Time
}

// NewService собирает сервис.
func NewService(opt ServiceOptions) *Service {
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	cl := opt.Client
	if cl == nil {
		cl = NewClient(Options{})
	}
	s := &Service{client: cl, cache: NewCache(cl, now), places: NewPlaces(opt.DB), now: now}
	if opt.Default != nil {
		d := *opt.Default
		d.Custom = false
		s.def = &d
	}
	return s
}

// Now — текущее время сервиса.
func (s *Service) Now() time.Time { return s.now() }

// PlaceFor — место пользователя: своё, иначе по умолчанию; ok=false — погода
// ему выключена (ФТ-16.1.4).
func (s *Service) PlaceFor(ctx context.Context, userID int64) (Place, bool, error) {
	if pl, ok, err := s.places.Get(ctx, userID); err != nil || ok {
		return pl, ok, err
	}
	if s.def != nil {
		return *s.def, true, nil
	}
	return Place{}, false, nil
}

// SetPlace запоминает место пользователя. Название очищается и проверяется.
func (s *Service) SetPlace(ctx context.Context, userID int64, pl Place) (Place, error) {
	pl.Name = strings.TrimSpace(pl.Name)
	pl.Custom = true
	return pl, s.places.Set(ctx, userID, pl)
}

// ResetPlace возвращает пользователя к месту по умолчанию.
func (s *Service) ResetPlace(ctx context.Context, userID int64) error {
	return s.places.Delete(ctx, userID)
}

// ValidName — название места: 1–80 символов без управляющих.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < 1 || n > MaxNameRunes {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Forecast возвращает прогноз места из кэша.
func (s *Service) Forecast(ctx context.Context, pl Place) (Result, error) {
	return s.cache.Get(ctx, pl.Lat, pl.Lon)
}

// Search ищет города по названию.
func (s *Service) Search(ctx context.Context, q string) ([]Found, error) {
	return s.client.Search(ctx, q)
}
