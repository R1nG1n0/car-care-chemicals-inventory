package bot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func update(id int64, tg int64, text string) map[string]any {
	return map[string]any{"update_id": id, "message": map[string]any{
		"message_id": id, "text": text,
		"from": map[string]any{"id": tg, "first_name": "Тест"},
		"chat": map[string]any{"id": tg, "type": "private"}}}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

// Цикл опроса: команды меню, имя бота, обработка, offset в базе (ФТ-11.1.4).
func TestRunPollsAndStoresOffset(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.fake.queue = [][]map[string]any{{update(41, 1001, "шампунь 100"), update(42, 1001, "шампунь 100")}}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.bot.Run(ctx); close(done) }()
	waitFor(t, "обработка обновлений", func() bool { return e.total(a, p) == 300 })
	waitFor(t, "offset", func() bool {
		v, _ := e.bot.loadOffset(e.ctx)
		return v == 43
	})
	cancel()
	<-done

	if e.bot.BotUsername() != "garage_test_bot" {
		t.Fatalf("имя бота: %q", e.bot.BotUsername())
	}
	if strings.Join(e.fake.cmds, ",") != "start,list,help" {
		t.Fatalf("команды меню: %v", e.fake.cmds)
	}

	// После перезапуска опрос идёт с сохранённого offset, а старые обновления не повторяются.
	e.fake.mu.Lock()
	e.fake.offsets = nil
	e.fake.queue = nil
	e.fake.mu.Unlock()
	b2 := New(Options{DB: e.db, API: e.bot.api, Log: e.bot.log, Now: e.clock.Now})
	b2.minBackoff = time.Millisecond
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { b2.Run(ctx2); close(done2) }()
	waitFor(t, "getUpdates после перезапуска", func() bool {
		e.fake.mu.Lock()
		defer e.fake.mu.Unlock()
		return len(e.fake.offsets) > 0
	})
	cancel2()
	<-done2
	if e.fake.offsets[0] != 43 {
		t.Fatalf("опрос начался с offset %d, ожидалось 43", e.fake.offsets[0])
	}
	if e.total(a, p) != 300 {
		t.Fatal("старые обновления обработаны повторно")
	}
}

// Сбои Telegram не останавливают бота; токен не попадает в журнал (ФТ-11.1.3).
func TestRunSurvivesOutageAndNeverLogsToken(t *testing.T) {
	e := newEnv(t)
	e.fake.failAll = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.bot.Run(ctx); close(done) }()
	waitFor(t, "повторы при сбое", func() bool { return strings.Count(e.logs.String(), "getMe") >= 3 })
	e.fake.mu.Lock()
	e.fake.failAll = false
	e.fake.mu.Unlock()
	waitFor(t, "восстановление", func() bool { return e.bot.BotUsername() != "" })
	cancel()
	<-done

	assertNoToken(t, e.logs.String())

	// Закрытый порт: ошибки url.Error тоже без токена.
	e.fake.srv.Close()
	e2 := New(Options{DB: e.db, API: newClosedAPI(e), Log: e.bot.log, Now: e.clock.Now})
	e2.minBackoff = time.Millisecond
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	e2.Run(ctx2)
	assertNoToken(t, e.logs.String())
	if !strings.Contains(e.logs.String(), "getMe") {
		t.Fatal("ошибки должны журналироваться")
	}
}

func assertNoToken(t *testing.T, logs string) {
	t.Helper()
	for _, s := range []string{testToken, "AAH-secret", "424242", "/bot"} {
		if strings.Contains(logs, s) {
			t.Fatalf("в журнале есть %q:\n%s", s, logs)
		}
	}
}

// Паника в обработчике не роняет бота, offset двигается.
func TestRunPanicDoesNotStopBot(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.fake.queue = [][]map[string]any{{
		{"update_id": 10, "message": map[string]any{"message_id": 1, "text": "x",
			"from": map[string]any{"id": 1001}, "chat": map[string]any{"id": 1001, "type": "private"}}},
	}, {update(11, 1001, "/help")}}
	e.bot.cat = nil // любой поиск паникует
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.bot.Run(ctx); close(done) }()
	waitFor(t, "ответ на /help после паники", func() bool {
		for _, m := range e.fake.sent() {
			if strings.Contains(m.Text, "Что я умею") {
				return true
			}
		}
		return false
	})
	cancel()
	<-done
	_ = a
	if v, _ := e.bot.loadOffset(e.ctx); v != 12 {
		t.Fatalf("offset %d, ожидалось 12", v)
	}
	if !strings.Contains(e.logs.String(), "паника") {
		t.Fatal("паника должна попасть в журнал")
	}
}

// Сбой setMyCommands не блокирует опрос.
func TestRunCommandsFailureDoesNotBlockPolling(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.fake.failCommands = true
	e.fake.queue = [][]map[string]any{{update(1, 1001, "шампунь 100")}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.bot.Run(ctx); close(done) }()
	waitFor(t, "обработка при сбое меню", func() bool { return e.total(a, p) == 400 })
	cancel()
	<-done
	if !strings.Contains(e.logs.String(), "setMyCommands") {
		t.Fatal("сбой должен попасть в журнал")
	}
}
