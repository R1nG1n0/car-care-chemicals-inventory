package web

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/bot"
)

// fakeBot — бот для тестов привязки.
type fakeBot struct {
	enabled  bool
	username string
}

func (f fakeBot) Enabled() bool       { return f.enabled }
func (f fakeBot) BotUsername() string { return f.username }

type tgStatus struct {
	Enabled      bool    `json:"enabled"`
	BotUsername  *string `json:"bot_username"`
	Linked       bool    `json:"linked"`
	TelegramName *string `json:"telegram_name"`
	LinkedAt     *int64  `json:"linked_at"`
}

type tgLink struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
}

var startURL = regexp.MustCompile(`^https://t\.me/garage_bot\?start=([A-Za-z0-9_-]{1,64})$`)

func TestTelegramDisabled(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	st := decode[tgStatus](t, c.do(http.MethodGet, "/api/telegram", nil))
	if st.Enabled || st.BotUsername != nil || st.Linked || st.TelegramName != nil || st.LinkedAt != nil {
		t.Fatalf("статус выключенного бота: %+v", st)
	}
	rec := c.do(http.MethodPost, "/api/telegram/link", map[string]any{})
	expect(t, rec, http.StatusServiceUnavailable, "ссылка при выключенном боте")
	if !strings.Contains(rec.Body.String(), "Бот не настроен") {
		t.Fatal(rec.Body.String())
	}
}

// Бот включён, но getMe ещё не удался — имени нет, ссылку выдать нельзя.
func TestTelegramUsernameUnknown(t *testing.T) {
	e := newEnvWith(t, false, fakeBot{enabled: true})
	c := stkSignIn(e, "owner")
	st := decode[tgStatus](t, c.do(http.MethodGet, "/api/telegram", nil))
	if !st.Enabled || st.BotUsername != nil {
		t.Fatalf("статус: %+v", st)
	}
	expect(t, c.do(http.MethodPost, "/api/telegram/link", map[string]any{}), http.StatusServiceUnavailable, "без имени бота")
}

func TestTelegramLinkLifecycle(t *testing.T) {
	e := newEnvWith(t, false, fakeBot{enabled: true, username: "garage_bot"})
	c := stkSignIn(e, "owner")
	st := decode[tgStatus](t, c.do(http.MethodGet, "/api/telegram", nil))
	if !st.Enabled || st.BotUsername == nil || *st.BotUsername != "garage_bot" || st.Linked {
		t.Fatalf("статус: %+v", st)
	}

	link := decode[tgLink](t, c.do(http.MethodPost, "/api/telegram/link", map[string]any{}))
	m := startURL.FindStringSubmatch(link.URL)
	if m == nil {
		t.Fatalf("адрес привязки %q", link.URL)
	}
	if link.ExpiresAt != e.clock.Now().Unix()+600 {
		t.Fatalf("срок %d, ожидалось +600 с", link.ExpiresAt)
	}
	// В базе только хеш кода.
	var stored string
	if err := e.db.QueryRow(`SELECT hash FROM telegram_link_codes`).Scan(&stored); err != nil || stored == m[1] {
		t.Fatalf("в базе должен быть хеш, а не код: %q %v", stored, err)
	}

	// Бот принимает код — сайт видит привязку.
	var userID int64
	if err := e.db.QueryRow(`SELECT id FROM users WHERE login = 'owner'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	links := bot.NewLinks(e.db, e.clock.Now)
	if _, err := links.Redeem(context.Background(), m[1], 555, 555, "Иван П"); err != nil {
		t.Fatal(err)
	}
	st = decode[tgStatus](t, c.do(http.MethodGet, "/api/telegram", nil))
	if !st.Linked || st.TelegramName == nil || *st.TelegramName != "Иван П" || st.LinkedAt == nil || *st.LinkedAt != e.clock.Now().Unix() {
		t.Fatalf("статус после привязки: %+v", st)
	}

	// Отключение: 204, привязка и состояние уведомлений исчезают.
	cat := stkCategory(c, "К")
	prod := stkProduct(c, cat, "Шампунь", "ml", 100)
	e.db.Exec(`INSERT INTO bot_low_state (user_id, product_id) VALUES (?, ?)`, userID, prod)
	expect(t, c.do(http.MethodDelete, "/api/telegram", nil), http.StatusNoContent, "отключение")
	st = decode[tgStatus](t, c.do(http.MethodGet, "/api/telegram", nil))
	if st.Linked || st.TelegramName != nil {
		t.Fatalf("после отключения: %+v", st)
	}
	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM bot_low_state`).Scan(&n)
	if n != 0 {
		t.Fatal("состояние уведомлений должно удаляться")
	}
	// Повторное отключение безвредно.
	expect(t, c.do(http.MethodDelete, "/api/telegram", nil), http.StatusNoContent, "повторное отключение")
}

func TestTelegramAccessAndCSRF(t *testing.T) {
	e := newEnvWith(t, false, fakeBot{enabled: true, username: "garage_bot"})
	anon := e.client()
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/api/telegram"}, {http.MethodPost, "/api/telegram/link"}, {http.MethodDelete, "/api/telegram"},
	} {
		if rec := anon.do(tc.method, tc.target, map[string]any{}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s без входа: %d", tc.method, tc.target, rec.Code)
		}
	}
	c := stkSignIn(e, "owner")
	c.noCSRF = true
	for _, tc := range []struct{ method, target string }{
		{http.MethodPost, "/api/telegram/link"}, {http.MethodDelete, "/api/telegram"},
	} {
		if rec := c.do(tc.method, tc.target, map[string]any{}); rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s без CSRF-заголовка: %d", tc.method, tc.target, rec.Code)
		}
	}
	c.noCSRF = false
	expect(t, c.do(http.MethodPut, "/api/telegram", map[string]any{}), http.StatusMethodNotAllowed, "PUT")
	expect(t, c.do(http.MethodGet, "/api/telegram/link", nil), http.StatusMethodNotAllowed, "GET link")
}

// Код привязки одного пользователя не привязывает другого; чужая привязка не видна и не удаляется.
func TestTelegramIsolation(t *testing.T) {
	e := newEnvWith(t, false, fakeBot{enabled: true, username: "garage_bot"})
	a := stkSignIn(e, "owner")
	b := stkSignIn(e, "friend")
	link := decode[tgLink](t, a.do(http.MethodPost, "/api/telegram/link", map[string]any{}))
	code := startURL.FindStringSubmatch(link.URL)[1]
	if _, err := bot.NewLinks(e.db, e.clock.Now).Redeem(context.Background(), code, 555, 555, "Владелец"); err != nil {
		t.Fatal(err)
	}
	if st := decode[tgStatus](t, b.do(http.MethodGet, "/api/telegram", nil)); st.Linked || st.TelegramName != nil {
		t.Fatalf("второй видит чужую привязку: %+v", st)
	}
	expect(t, b.do(http.MethodDelete, "/api/telegram", nil), http.StatusNoContent, "отключение вторым")
	if st := decode[tgStatus](t, a.do(http.MethodGet, "/api/telegram", nil)); !st.Linked {
		t.Fatal("второй отключил привязку владельца")
	}
}
