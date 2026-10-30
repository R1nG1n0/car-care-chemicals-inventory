package weather

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	// CacheTTL — как часто обновляется прогноз одного места (ФТ-16.1.3).
	CacheTTL = time.Hour
	// StaleAfter — с какого возраста прогноз помечается устаревшим (Stale).
	StaleAfter = 3 * time.Hour
	// failBackoff — пауза перед новой попыткой после сбоя Open-Meteo.
	failBackoff = time.Minute
	// maxEntries — предел мест в памяти: места задают пользователи.
	maxEntries = 200
)

// Result — прогноз места из кэша.
type Result struct {
	Forecast  Forecast
	FetchedAt time.Time
	// Stale — свежий прогноз получить не удалось, это последний удачный.
	Stale bool
}

// Fetcher получает прогноз у источника.
type Fetcher interface {
	Forecast(ctx context.Context, lat, lon float64) (Forecast, error)
}

type entry struct {
	f         Forecast
	fetchedAt time.Time
	has       bool
	lastFail  time.Time
	call      *call // идущий запрос
}

type call struct {
	done chan struct{}
	err  error
}

// Cache держит прогнозы в памяти по месту (координаты с точностью 0,01°).
// Одновременные запросы к одному месту схлопываются в один запрос наружу.
type Cache struct {
	src Fetcher
	now func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
}

// NewCache создаёт кэш; now — часы, nil — time.Now.
func NewCache(src Fetcher, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{src: src, now: now, entries: map[string]*entry{}}
}

func cacheKey(lat, lon float64) (string, float64, float64) {
	lat, lon = math.Round(lat*100)/100, math.Round(lon*100)/100
	return fmt.Sprintf("%.2f,%.2f", lat, lon), lat, lon
}

// Get возвращает прогноз места. Свежее (младше часа) берётся из памяти. Старше
// часа отдаётся сразу, а обновление идёт в фоне (stale-while-revalidate):
// Stale ставится, только когда прогнозу больше StaleAfter. Ждать запрос наружу
// приходится, лишь когда прогноза нет вовсе; не ответил источник — ErrUnavailable.
func (c *Cache) Get(ctx context.Context, lat, lon float64) (Result, error) {
	key, lat, lon := cacheKey(lat, lon)
	c.mu.Lock()
	e := c.entries[key]
	if e == nil {
		e = &entry{}
		c.entries[key] = e
		c.evict(key)
	}
	now := c.now()
	canTry := e.call == nil && (e.lastFail.IsZero() || now.Sub(e.lastFail) >= failBackoff)
	if e.has {
		age := now.Sub(e.fetchedAt)
		if age >= CacheTTL && canTry {
			c.start(e, lat, lon)
		}
		r := c.result(e, age >= StaleAfter)
		c.mu.Unlock()
		return r, nil
	}
	if !canTry && e.call == nil {
		defer c.mu.Unlock()
		return c.failed(e)
	}
	cl := e.call
	if cl == nil {
		cl = c.start(e, lat, lon)
	}
	c.mu.Unlock()

	select {
	case <-cl.done:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl.err == nil {
		return c.result(e, false), nil
	}
	return c.failed(e)
}

// start запускает запрос места; под mu. Запрос идёт сам по себе: отмена
// ожидающего не должна ронять остальных.
func (c *Cache) start(e *entry, lat, lon float64) *call {
	cl := &call{done: make(chan struct{})}
	e.call = cl
	go c.fetch(e, cl, lat, lon)
	return cl
}

func (c *Cache) fetch(e *entry, cl *call, lat, lon float64) {
	f, err := c.src.Forecast(context.Background(), lat, lon)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		e.lastFail = c.now()
	} else {
		e.f, e.fetchedAt, e.has, e.lastFail = f, c.now(), true, time.Time{}
	}
	e.call, cl.err = nil, err
	close(cl.done)
}

func (c *Cache) result(e *entry, stale bool) Result {
	return Result{Forecast: e.f, FetchedAt: e.fetchedAt, Stale: stale}
}

// failed: прогноз не получен — последний удачный или ошибка.
func (c *Cache) failed(e *entry) (Result, error) {
	if e.has {
		return c.result(e, c.now().Sub(e.fetchedAt) >= StaleAfter), nil
	}
	return Result{}, fmt.Errorf("%w: источник не отвечает", ErrUnavailable)
}

// evict выбрасывает самые старые места сверх предела (кроме keep). Под mu.
func (c *Cache) evict(keep string) {
	for len(c.entries) > maxEntries {
		oldest, at := "", time.Time{}
		for k, e := range c.entries {
			if k == keep || e.call != nil {
				continue
			}
			if oldest == "" || e.fetchedAt.Before(at) {
				oldest, at = k, e.fetchedAt
			}
		}
		if oldest == "" {
			return
		}
		delete(c.entries, oldest)
	}
}
