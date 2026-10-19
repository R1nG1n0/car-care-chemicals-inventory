package bot

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

const testToken = "424242:AAH-secret_TOKEN-value"

type btn struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
	URL  string `json:"url"`
}

// sentMsg — сообщение бота в имитации Telegram (с учётом правок).
type sentMsg struct {
	ID   int64
	Chat int64
	Text string
	Kb   [][]btn
}

// fakeTG — имитация Bot API на httptest.
type fakeTG struct {
	t   *testing.T
	mu  sync.Mutex
	seq int64

	msgs         map[int64]*sentMsg
	order        []int64 // id сообщений в порядке отправки
	answers      []string
	cmds         []string
	files        map[string][]byte
	offsets      []int64
	queue        [][]map[string]any // партии для getUpdates
	failAll      bool               // все вызовы отвечают ошибкой
	blocked      map[int64]bool     // чаты, где sendMessage даёт 403
	failText     string             // sendMessage с этим текстом даёт 500
	chatNotFound bool               // sendMessage даёт 400 chat not found
	failCommands bool               // setMyCommands даёт ошибку
	srv          *httptest.Server
}

func newFakeTG(t *testing.T) *fakeTG {
	f := &fakeTG{t: t, msgs: map[int64]*sentMsg{}, files: map[string][]byte{}, blocked: map[int64]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTG) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(result any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}
	fail := func(code int, desc string) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": code, "description": desc})
	}
	if f.failAll {
		fail(500, "boom "+testToken)
		return
	}
	if p, ok := strings.CutPrefix(r.URL.Path, "/file/bot"+testToken+"/"); ok {
		_, _ = w.Write(f.files[p])
		return
	}
	method, _ := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		ChatID      int64  `json:"chat_id"`
		MessageID   int64  `json:"message_id"`
		Text        string `json:"text"`
		FileID      string `json:"file_id"`
		Offset      int64  `json:"offset"`
		CallbackID  string `json:"callback_query_id"`
		ReplyMarkup struct {
			Rows [][]btn `json:"inline_keyboard"`
		} `json:"reply_markup"`
		Commands []telegram.Command `json:"commands"`
	}
	_ = json.Unmarshal(raw, &body)
	switch method {
	case "getMe":
		reply(map[string]any{"id": 1, "is_bot": true, "first_name": "Гараж", "username": "garage_test_bot"})
	case "setMyCommands":
		if f.failCommands {
			fail(500, "boom")
			return
		}
		for _, c := range body.Commands {
			f.cmds = append(f.cmds, c.Command)
		}
		reply(true)
	case "getUpdates":
		f.offsets = append(f.offsets, body.Offset)
		if len(f.queue) > 0 {
			batch := f.queue[0]
			f.queue = f.queue[1:]
			reply(batch)
			return
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		f.mu.Lock()
		reply([]any{})
	case "sendMessage":
		if f.chatNotFound {
			fail(400, "Bad Request: chat not found")
			return
		}
		if f.failText != "" && strings.Contains(body.Text, f.failText) {
			fail(500, "boom")
			return
		}
		if f.blocked[body.ChatID] {
			fail(403, "Forbidden: bot was blocked by the user")
			return
		}
		f.seq++
		m := &sentMsg{ID: f.seq, Chat: body.ChatID, Text: body.Text, Kb: body.ReplyMarkup.Rows}
		f.msgs[m.ID] = m
		f.order = append(f.order, m.ID)
		reply(map[string]any{"message_id": m.ID, "chat": map[string]any{"id": body.ChatID, "type": "private"}})
	case "editMessageText":
		m, ok := f.msgs[body.MessageID]
		if !ok {
			fail(400, "message to edit not found")
			return
		}
		m.Text, m.Kb = body.Text, body.ReplyMarkup.Rows
		reply(true)
	case "answerCallbackQuery":
		var p struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &p)
		f.answers = append(f.answers, p.Text)
		reply(true)
	case "getFile":
		reply(map[string]any{"file_id": body.FileID, "file_size": len(f.files[body.FileID]), "file_path": body.FileID})
	default:
		f.t.Errorf("неожиданный метод %s", method)
	}
}

// sent — копии всех сообщений бота по порядку (с учётом правок).
func (f *fakeTG) sent() []sentMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentMsg, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, *f.msgs[id])
	}
	return out
}

func (f *fakeTG) count() int { return len(f.sent()) }

// last — последнее сообщение бота.
func (f *fakeTG) last() sentMsg {
	f.t.Helper()
	s := f.sent()
	if len(s) == 0 {
		f.t.Fatal("бот ничего не отправил")
	}
	return s[len(s)-1]
}

func (m sentMsg) button(contains string) (btn, bool) {
	for _, row := range m.Kb {
		for _, b := range row {
			if strings.Contains(b.Text, contains) {
				return b, true
			}
		}
	}
	return btn{}, false
}

// syncBuf — буфер журнала, безопасный для чтения во время работы бота.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// tgClock — подменяемые часы.
type tgClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tgClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *tgClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// env — бот на временной базе и имитации Telegram.
type env struct {
	t     *testing.T
	db    *sql.DB
	fake  *fakeTG
	bot   *Bot
	stk   *stock.Service
	clock *tgClock
	logs  *syncBuf
	ctx   context.Context
	upd   int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	fake := newFakeTG(t)
	clock := &tgClock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	logs := &syncBuf{}
	b := New(Options{
		DB:      db.DB,
		API:     telegram.New(testToken, telegram.Options{BaseURL: fake.srv.URL}),
		Log:     slog.New(slog.NewTextHandler(logs, nil)),
		Now:     clock.Now,
		SiteURL: "https://garage.example",
	})
	b.minBackoff = 5 * time.Millisecond
	return &env{t: t, db: db.DB, fake: fake, bot: b, stk: stock.New(db.DB, clock.Now), clock: clock, logs: logs, ctx: context.Background()}
}

func (e *env) exec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// user — пользователь сайта с категорией.
type user struct {
	id, cat, tg int64
}

func (e *env) newUser(login string, tg int64) user {
	e.t.Helper()
	id := e.exec(`INSERT INTO users (login, password_hash) VALUES (?, 'x')`, login)
	cat := e.exec(`INSERT INTO categories (user_id, name, color) VALUES (?, 'Шампуни', 'blue')`, id)
	return user{id: id, cat: cat, tg: tg}
}

// linked — пользователь, уже привязанный к Telegram-аккаунту (чат = id аккаунта).
func (e *env) linked(login string, tg int64) user {
	u := e.newUser(login, tg)
	e.exec(`INSERT INTO telegram_links (user_id, tg_user_id, chat_id, tg_name, linked_at) VALUES (?, ?, ?, 'Тест', 1)`,
		u.id, tg, tg)
	return u
}

func (e *env) product(u user, name, brand, unit string, size int64, min *int64, barcode string) int64 {
	e.t.Helper()
	return e.exec(`INSERT INTO products (user_id, category_id, name, brand, unit, package_size, min_stock, barcode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, u.id, u.cat, name, brand, unit, size, min, barcode)
}

func (e *env) buy(u user, product, packages, price int64, storeID, loc *int64) {
	e.t.Helper()
	if _, err := e.stk.CreatePurchase(e.ctx, u.id, stock.PurchaseInput{Date: "2026-10-01", StoreID: storeID,
		Lines: []stock.PurchaseLineInput{{ProductID: product, Packages: packages, PricePerPackage: price, LocationID: loc}}}); err != nil {
		e.t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

// say — сообщение пользователя боту.
func (e *env) say(u user, text string) {
	e.t.Helper()
	e.upd++
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: e.upd, Message: &telegram.Message{
		MessageID: e.upd, Text: text,
		From: &telegram.User{ID: u.tg, FirstName: "Тест"}, Chat: telegram.Chat{ID: u.tg, Type: "private"}}})
}

// click — нажатие кнопки с подписью, содержащей text, в сообщении m.
func (e *env) click(u user, m sentMsg, text string) {
	e.t.Helper()
	b, ok := e.fake.msgs[m.ID].button(text)
	if !ok {
		e.t.Fatalf("в сообщении %q нет кнопки %q (кнопки: %+v)", m.Text, text, e.fake.msgs[m.ID].Kb)
	}
	e.clickData(u, m, b.Data)
}

func (e *env) clickData(u user, m sentMsg, data string) {
	e.t.Helper()
	e.upd++
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: e.upd, CallbackQuery: &telegram.CallbackQuery{
		ID: "cb", From: telegram.User{ID: u.tg, FirstName: "Тест"}, Data: data,
		Message: &telegram.Message{MessageID: m.ID, Chat: telegram.Chat{ID: u.tg, Type: "private"}}}})
}

// total — суммарный остаток позиции.
func (e *env) total(u user, product int64) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(`SELECT COALESCE(SUM(remaining_qty), 0) FROM units WHERE user_id = ? AND product_id = ?
		AND status IN ('sealed', 'open')`, u.id, product).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) count(q string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func contains(t *testing.T, got string, parts ...string) {
	t.Helper()
	got = strings.ReplaceAll(got, "\u00a0", " ") // неразрывные пробелы в тестах пишем обычными
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Fatalf("в тексте нет %q:\n%s", p, got)
		}
	}
}

// newClosedAPI — клиент к уже закрытому серверу (ошибки соединения).
func newClosedAPI(e *env) API {
	return telegram.New(testToken, telegram.Options{BaseURL: e.fake.srv.URL})
}
