package auth

import (
	"sync"
	"time"
)

// Ограничение попыток входа (ТБ-4): 5 неудач за 15 минут.
const (
	LimitAttempts = 5
	LimitWindow   = 15 * time.Minute

	// sweepEvery — раз в сколько неудач чистятся устаревшие ключи,
	// чтобы перебор по множеству адресов не раздувал память.
	sweepEvery = 256
)

// Limiter считает неудачные попытки по ключу (логин, адрес) в скользящем
// окне. Хранится в памяти: после перезапуска счётчики обнуляются, для
// личного сайта за VPN этого достаточно. Потокобезопасен.
type Limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	fails map[string][]time.Time
	calls int
}

// NewLimiter создаёт ограничитель: max неудач за window. now — источник
// времени; nil — time.Now.
func NewLimiter(max int, window time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{max: max, window: window, now: now, fails: make(map[string][]time.Time)}
}

// SetClock подменяет источник времени (для тестов).
func (l *Limiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	l.now = now
	l.mu.Unlock()
}

// Blocked сообщает, исчерпан ли лимит хотя бы по одному из ключей,
// и через сколько можно повторить (наибольшее из ожиданий).
func (l *Limiter) Blocked(keys ...string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	var wait time.Duration
	for _, k := range keys {
		ts := l.prune(k, now)
		if len(ts) >= l.max {
			// Попытка освободится, когда из окна выйдет самая старая
			// из последних max неудач.
			if d := ts[len(ts)-l.max].Add(l.window).Sub(now); d > wait {
				wait = d
			}
		}
	}
	return wait > 0, wait
}

// Fail записывает неудачную попытку по каждому ключу.
func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, k := range keys {
		ts := append(l.prune(k, now), now)
		// Больше max отметок не нужно: для ожидания важны последние max.
		if len(ts) > l.max {
			ts = ts[len(ts)-l.max:]
		}
		l.fails[k] = ts
	}
	l.calls++
	if l.calls%sweepEvery == 0 {
		for k := range l.fails {
			l.prune(k, now)
		}
	}
}

// Reset обнуляет счётчик ключа (успешный вход сбрасывает счётчик логина).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
}

// prune убирает отметки старше окна; пустой ключ удаляется. Вызывать под mu.
func (l *Limiter) prune(key string, now time.Time) []time.Time {
	ts := l.fails[key]
	i := 0
	for i < len(ts) && !ts[i].After(now.Add(-l.window)) {
		i++
	}
	ts = ts[i:]
	if len(ts) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = ts
	return ts
}
