package weather

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSrc struct {
	calls atomic.Int32
	fail  atomic.Bool
	gate  chan struct{} // не nil — Forecast ждёт закрытия
}

func (s *fakeSrc) Forecast(ctx context.Context, lat, lon float64) (Forecast, error) {
	s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	if s.fail.Load() {
		return Forecast{}, errors.New("источник недоступен")
	}
	return Forecast{Loc: msk, Days: []Day{{Date: "2026-11-10"}}}, nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestCache(src Fetcher) (*Cache, *clock) {
	ck := &clock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, msk)}
	return NewCache(src, ck.Now), ck
}

// waitFor ждёт условие, которое выполняет фоновое обновление.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

func TestCacheRefreshesOncePerHour(t *testing.T) {
	src := &fakeSrc{}
	c, ck := newTestCache(src)
	ctx := context.Background()
	first, err := c.Get(ctx, 48.023, 37.802)
	if err != nil || first.Stale || !first.FetchedAt.Equal(ck.Now()) {
		t.Fatalf("первый: %+v, %v", first, err)
	}
	ck.Add(59 * time.Minute)
	if _, err := c.Get(ctx, 48.023, 37.802); err != nil || src.calls.Load() != 1 {
		t.Fatalf("за час повторный запрос наружу: %d, %v", src.calls.Load(), err)
	}
	// Через час прежний прогноз отдаётся сразу, свежий подтягивается в фоне.
	ck.Add(2 * time.Minute)
	r, err := c.Get(ctx, 48.023, 37.802)
	if err != nil || r.Stale || !r.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("сразу после часа: %+v, %v", r, err)
	}
	waitFor(t, "фоновое обновление", func() bool {
		r, _ := c.Get(ctx, 48.023, 37.802)
		return r.FetchedAt.Equal(ck.Now())
	})
	if src.calls.Load() != 2 {
		t.Fatalf("запросов наружу: %d", src.calls.Load())
	}
}

// Обновление в фоне не задерживает ответ и не плодит запросы.
func TestCacheRevalidatesInBackground(t *testing.T) {
	src := &fakeSrc{gate: make(chan struct{})}
	close(src.gate)
	c, ck := newTestCache(src)
	ctx := context.Background()
	old, _ := c.Get(ctx, 1, 1)
	ck.Add(2 * time.Hour)
	src.gate = make(chan struct{}) // следующий запрос зависнет
	for i := 0; i < 5; i++ {
		done := make(chan Result, 1)
		go func() { r, _ := c.Get(ctx, 1, 1); done <- r }()
		select {
		case r := <-done:
			if r.Stale || !r.FetchedAt.Equal(old.FetchedAt) {
				t.Fatalf("ответ из памяти: %+v", r)
			}
		case <-time.After(time.Second):
			t.Fatal("Get ждёт фоновый запрос")
		}
	}
	waitFor(t, "запрос ушёл", func() bool { return src.calls.Load() == 2 })
	time.Sleep(20 * time.Millisecond)
	if src.calls.Load() != 2 {
		t.Fatalf("фоновых запросов: %d, ожидался 1", src.calls.Load()-1)
	}
	close(src.gate)
	waitFor(t, "свежий прогноз", func() bool {
		r, _ := c.Get(ctx, 1, 1)
		return r.FetchedAt.Equal(ck.Now())
	})
}

func TestCacheKeyRoundsToHundredth(t *testing.T) {
	src := &fakeSrc{}
	c, _ := newTestCache(src)
	ctx := context.Background()
	for _, p := range [][2]float64{{48.0231, 37.8019}, {48.0249, 37.8049}, {48.02, 37.80}} {
		if _, err := c.Get(ctx, p[0], p[1]); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls.Load() != 1 {
		t.Fatalf("одно место запрошено %d раз", src.calls.Load())
	}
	if _, err := c.Get(ctx, 48.04, 37.80); err != nil || src.calls.Load() != 2 {
		t.Fatalf("другое место: вызовов %d, %v", src.calls.Load(), err)
	}
}

func TestCacheStaleOnFailure(t *testing.T) {
	src := &fakeSrc{}
	c, ck := newTestCache(src)
	ctx := context.Background()
	good, _ := c.Get(ctx, 1, 1)
	src.fail.Store(true)
	// Старше часа, но моложе 3 часов: отдаётся без пометки, обновление в фоне не удаётся.
	ck.Add(2 * time.Hour)
	r, err := c.Get(ctx, 1, 1)
	if err != nil || r.Stale || !r.FetchedAt.Equal(good.FetchedAt) {
		t.Fatalf("2 часа: %+v, %v", r, err)
	}
	waitFor(t, "фоновая попытка", func() bool { return src.calls.Load() == 2 })
	// Старше 3 часов — Stale.
	ck.Add(2 * time.Hour)
	r, err = c.Get(ctx, 1, 1)
	if err != nil || !r.Stale || !r.FetchedAt.Equal(good.FetchedAt) || len(r.Forecast.Days) != 1 {
		t.Fatalf("4 часа: %+v, %v", r, err)
	}
	// После сбоя минуту источник не дёргают.
	waitFor(t, "вторая попытка", func() bool { return src.calls.Load() == 3 })
	calls := src.calls.Load()
	ck.Add(30 * time.Second)
	if r, err := c.Get(ctx, 1, 1); err != nil || !r.Stale {
		t.Fatalf("в паузе после сбоя: %+v, %v", r, err)
	}
	time.Sleep(20 * time.Millisecond)
	if src.calls.Load() != calls {
		t.Fatalf("в паузе ходили наружу: %d вместо %d", src.calls.Load(), calls)
	}
	// Источник ожил: после паузы прогноз обновляется, Stale уходит.
	src.fail.Store(false)
	ck.Add(40 * time.Second)
	_, _ = c.Get(ctx, 1, 1)
	waitFor(t, "выздоровление", func() bool {
		r, err := c.Get(ctx, 1, 1)
		return err == nil && !r.Stale && r.FetchedAt.Equal(ck.Now())
	})
}

func TestCacheFailureWithoutData(t *testing.T) {
	src := &fakeSrc{}
	src.fail.Store(true)
	c, ck := newTestCache(src)
	_, err := c.Get(context.Background(), 1, 1)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ошибка: %v", err)
	}
	calls := src.calls.Load()
	if _, err := c.Get(context.Background(), 1, 1); !errors.Is(err, ErrUnavailable) || src.calls.Load() != calls {
		t.Fatalf("в паузе после сбоя ходили наружу: %d", src.calls.Load())
	}
	ck.Add(2 * time.Minute)
	src.fail.Store(false)
	if _, err := c.Get(context.Background(), 1, 1); err != nil {
		t.Fatalf("после паузы: %v", err)
	}
}

func TestCacheCollapsesConcurrentRequests(t *testing.T) {
	src := &fakeSrc{gate: make(chan struct{})}
	c, _ := newTestCache(src)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Get(context.Background(), 48.02, 37.80)
			errs <- err
		}()
	}
	// Дождаться, пока запрос ушёл, и дать остальным встать в очередь.
	for src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(src.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if src.calls.Load() != 1 {
		t.Fatalf("запросов наружу: %d, ожидался 1", src.calls.Load())
	}
}

func TestCacheWaiterCancel(t *testing.T) {
	src := &fakeSrc{gate: make(chan struct{})}
	c, _ := newTestCache(src)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Get(ctx, 1, 1); done <- err }()
	for src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("ошибка: %v", err)
	}
	// Запрос не оборван: другой ожидающий получает результат.
	close(src.gate)
	if _, err := c.Get(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if src.calls.Load() != 1 {
		t.Fatalf("запросов наружу: %d", src.calls.Load())
	}
}

func TestCacheBounded(t *testing.T) {
	src := &fakeSrc{}
	c, ck := newTestCache(src)
	for i := 0; i < maxEntries+50; i++ {
		ck.Add(time.Second)
		if _, err := c.Get(context.Background(), float64(i%80)+float64(i/80)*0.5, float64(i)*0.1); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n > maxEntries {
		t.Fatalf("мест в памяти: %d", n)
	}
}
