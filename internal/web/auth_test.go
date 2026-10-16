package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// env — сервер на настоящей SQLite во временном каталоге.
type env struct {
	t     *testing.T
	h     http.Handler
	svc   *auth.Service
	clock *testClock
}

func newEnv(t *testing.T, insecure bool) *env {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	c := &testClock{t: time.Unix(1_792_000_000, 0)}
	svc := auth.New(db.DB, auth.Options{Now: c.Now, Params: auth.Params{Time: 1, Memory: 64, Threads: 1}})
	srv := New(Options{
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:         "test",
		Auth:            svc,
		Limiter:         auth.NewLimiter(auth.LimitAttempts, auth.LimitWindow, c.Now),
		InsecureCookies: insecure,
		DB:              db.DB,
		Now:             c.Now,
	})
	return &env{t: t, h: srv.Handler(), svc: svc, clock: c}
}

func (e *env) createUser(login, password string, owner bool) auth.User {
	e.t.Helper()
	u, err := e.svc.CreateUser(context.Background(), login, password, owner)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

// client — браузер одного пользователя: помнит cookie сессии.
type client struct {
	e          *env
	token      string
	remoteAddr string
	header     http.Header
	noCSRF     bool
}

func (e *env) client() *client {
	return &client{e: e, remoteAddr: "192.0.2.10:40000", header: http.Header{}}
}

func (c *client) do(method, target string, body any) *httptest.ResponseRecorder {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, rd)
	req.RemoteAddr = c.remoteAddr
	for k, v := range c.header {
		req.Header[k] = v
	}
	if !c.noCSRF && method != http.MethodGet {
		req.Header.Set("X-Requested-With", "garage")
	}
	if c.token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.token})
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie {
			if ck.MaxAge < 0 {
				c.token = ""
			} else {
				c.token = ck.Value
			}
		}
	}
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, code int, what string) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("%s: код %d, ожидался %d; тело %s", what, rec.Code, code, rec.Body.String())
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("разбор ответа %q: %v", rec.Body.String(), err)
	}
	return v
}

type meBody struct {
	User     auth.User     `json:"user"`
	Settings auth.Settings `json:"settings"`
}

type errBody struct {
	Error      string `json:"error"`
	Field      string `json:"field"`
	RetryAfter int    `json:"retry_after"`
}

type createdInvite struct {
	Invite inviteJSON `json:"invite"`
	URL    string     `json:"url"`
}

type inviteList struct {
	Invites []inviteJSON `json:"invites"`
}

func (c *client) login(login, password string) {
	c.e.t.Helper()
	expect(c.e.t, c.do(http.MethodPost, "/api/auth/login", credentials{login, password}), http.StatusOK, "вход "+login)
}

func tokenFromURL(t *testing.T, u string) string {
	t.Helper()
	i := strings.Index(u, "/invite/")
	if i < 0 {
		t.Fatalf("в ссылке нет /invite/: %s", u)
	}
	return u[i+len("/invite/"):]
}

// Полный сценарий этапа 1.
func TestFullScenario(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)

	owner := e.client()
	expect(t, owner.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "me без входа")

	rec := owner.do(http.MethodPost, "/api/auth/login", credentials{"RinGino", "owner-password"})
	expect(t, rec, http.StatusOK, "вход владельца")
	me := decode[meBody](t, rec)
	if !me.User.IsOwner || me.User.Login != "ringino" || me.Settings != auth.DefaultSettings {
		t.Fatalf("ответ входа: %+v", me)
	}

	rec = owner.do(http.MethodGet, "/api/auth/me", nil)
	expect(t, rec, http.StatusOK, "me")
	if got := decode[meBody](t, rec); got.User.ID != me.User.ID {
		t.Fatalf("me: %+v", got)
	}

	owner.header.Set("Host", "garage.north-flow.online")
	rec = owner.do(http.MethodPost, "/api/invites", nil)
	expect(t, rec, http.StatusCreated, "создание приглашения")
	inv := decode[createdInvite](t, rec)
	if inv.Invite.Status != "active" || inv.Invite.ExpiresAt-inv.Invite.CreatedAt != int64(auth.InviteTTL/time.Second) {
		t.Fatalf("приглашение: %+v", inv.Invite)
	}
	token := tokenFromURL(t, inv.URL)

	rec = owner.do(http.MethodGet, "/api/invites", nil)
	expect(t, rec, http.StatusOK, "список приглашений")
	if list := decode[inviteList](t, rec); len(list.Invites) != 1 || list.Invites[0].ID != inv.Invite.ID {
		t.Fatalf("список: %+v", list)
	}
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("в списке виден токен приглашения")
	}

	friend := e.client()
	friend.remoteAddr = "192.0.2.20:40000"
	rec = friend.do(http.MethodGet, "/api/invites/by-token/"+token, nil)
	expect(t, rec, http.StatusOK, "проверка приглашения")
	if got := decode[struct {
		ExpiresAt int64 `json:"expires_at"`
	}](t, rec); got.ExpiresAt != inv.Invite.ExpiresAt {
		t.Fatalf("срок: %d", got.ExpiresAt)
	}

	expect(t, friend.do(http.MethodPost, "/api/invites/by-token/"+token+"/accept", credentials{"RINGINO", "friend-password"}),
		http.StatusConflict, "занятый логин")
	rec = friend.do(http.MethodPost, "/api/invites/by-token/"+token+"/accept", credentials{"f", "friend-password"})
	expect(t, rec, http.StatusBadRequest, "короткий логин")
	if decode[errBody](t, rec).Field != "login" {
		t.Fatal("нет field: login")
	}
	rec = friend.do(http.MethodPost, "/api/invites/by-token/"+token+"/accept", credentials{"friend", "friend-password"})
	expect(t, rec, http.StatusCreated, "принятие приглашения")
	fme := decode[meBody](t, rec)
	if fme.User.IsOwner || fme.User.Login != "friend" || friend.token == "" {
		t.Fatalf("приглашённый: %+v, cookie %q", fme, friend.token)
	}
	expect(t, friend.do(http.MethodPost, "/api/invites/by-token/"+token+"/accept", credentials{"friend2", "friend-password"}),
		http.StatusNotFound, "повторное принятие")
	expect(t, friend.do(http.MethodGet, "/api/invites/by-token/"+token, nil), http.StatusNotFound, "использованное приглашение")

	// Друг входит заново с другого устройства.
	friend2 := e.client()
	friend2.login("friend", "friend-password")
	expect(t, friend2.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "me друга")

	// Друг не владелец: приглашения ему недоступны.
	expect(t, friend2.do(http.MethodGet, "/api/invites", nil), http.StatusForbidden, "список приглашений другом")
	expect(t, friend2.do(http.MethodPost, "/api/invites", nil), http.StatusForbidden, "создание приглашения другом")
	expect(t, friend2.do(http.MethodDelete, fmt.Sprintf("/api/invites/%d", inv.Invite.ID), nil), http.StatusForbidden, "отзыв другом")

	// Список владельца показывает использованное приглашение.
	rec = owner.do(http.MethodGet, "/api/invites", nil)
	if list := decode[inviteList](t, rec); list.Invites[0].Status != "used" {
		t.Fatalf("статус после принятия: %+v", list)
	}

	// Выход со всех устройств: обе сессии друга недействительны, cookie стёрта.
	rec = friend2.do(http.MethodPost, "/api/auth/logout-all", nil)
	expect(t, rec, http.StatusNoContent, "logout-all")
	if friend2.token != "" {
		t.Fatal("cookie не стёрта")
	}
	expect(t, friend.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "первая сессия друга после logout-all")
	expect(t, owner.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "сессия владельца после logout-all друга")

	// Обычный выход.
	expect(t, owner.do(http.MethodPost, "/api/auth/logout", nil), http.StatusNoContent, "logout")
	expect(t, owner.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "me после logout")
	expect(t, owner.do(http.MethodPost, "/api/auth/logout", nil), http.StatusNoContent, "logout без сессии")
}

func TestInviteURL(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)

	cases := []struct {
		name, remote, xfp, xri, want string
	}{
		{"loopback без прокси", "127.0.0.1:5000", "", "", "http://garage.test/invite/"},
		{"nginx с X-Forwarded-Proto", "127.0.0.1:5000", "https", "10.211.7.5", "https://garage.test/invite/"},
		{"nginx с X-Real-IP", "127.0.0.1:5000", "", "10.211.7.5", "https://garage.test/invite/"},
		{"чужой адрес с заголовком", "192.0.2.1:5000", "https", "", "http://garage.test/invite/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := e.client()
			c.remoteAddr = tc.remote
			c.login("ringino", "owner-password")
			c.header.Set("Host", "garage.test")
			if tc.xfp != "" {
				c.header.Set("X-Forwarded-Proto", tc.xfp)
			}
			if tc.xri != "" {
				c.header.Set("X-Real-IP", tc.xri)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/invites", nil)
			req.Host = "garage.test"
			req.RemoteAddr = c.remoteAddr
			req.Header = c.header.Clone()
			req.Header.Set("X-Requested-With", "garage")
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.token})
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, req)
			expect(t, rec, http.StatusCreated, "создание")
			if u := decode[createdInvite](t, rec).URL; !strings.HasPrefix(u, tc.want) || len(u) != len(tc.want)+43 {
				t.Fatalf("ссылка %s, ожидалось начало %s", u, tc.want)
			}
		})
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	c := e.client()
	c.login("ringino", "owner-password")

	c.noCSRF = true
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodPost, "/api/auth/logout-all"},
		{http.MethodPost, "/api/invites"},
		{http.MethodPut, "/api/auth/settings"},
		{http.MethodDelete, "/api/invites/1"},
	} {
		rec := c.do(tc.method, tc.path, credentials{"ringino", "owner-password"})
		expect(t, rec, http.StatusForbidden, tc.method+" "+tc.path+" без заголовка")
		if decode[errBody](t, rec).Error != "Запрос отклонён" {
			t.Fatalf("текст ошибки: %s", rec.Body.String())
		}
	}
	c.header.Set("X-Requested-With", "XMLHttpRequest")
	expect(t, c.do(http.MethodPost, "/api/invites", nil), http.StatusForbidden, "чужое значение заголовка")

	// GET без заголовка работает, сессия жива.
	expect(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "GET без заголовка")
}

func TestMethodNotAllowed(t *testing.T) {
	e := newEnv(t, false)
	c := e.client()
	rec := c.do(http.MethodGet, "/api/auth/login", nil)
	expect(t, rec, http.StatusMethodNotAllowed, "GET login")
	if rec.Header().Get("Allow") != "POST" {
		t.Fatalf("Allow %q", rec.Header().Get("Allow"))
	}
	rec = c.do(http.MethodPut, "/api/invites", nil)
	expect(t, rec, http.StatusMethodNotAllowed, "PUT invites")
	if rec.Header().Get("Allow") != "GET, HEAD, POST" {
		t.Fatalf("Allow %q", rec.Header().Get("Allow"))
	}
	expect(t, c.do(http.MethodGet, "/api/invites/abc", nil), http.StatusMethodNotAllowed, "GET invites/{id}")
}

func TestUnauthorized(t *testing.T) {
	e := newEnv(t, false)
	c := e.client()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/auth/logout-all"},
		{http.MethodPost, "/api/auth/password"},
		{http.MethodPut, "/api/auth/settings"},
		{http.MethodGet, "/api/invites"},
		{http.MethodPost, "/api/invites"},
		{http.MethodDelete, "/api/invites/1"},
	} {
		rec := c.do(tc.method, tc.path, nil)
		expect(t, rec, http.StatusUnauthorized, tc.method+" "+tc.path)
		if decode[errBody](t, rec).Error != "Нужно войти" {
			t.Fatalf("текст: %s", rec.Body.String())
		}
	}
	c.token = "поддельный"
	expect(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "поддельная cookie")
	if c.token != "" {
		t.Fatal("недействительная cookie не стёрта")
	}
}

func TestLoginErrorsAreUniform(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	c := e.client()
	a := c.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "wrong-password"})
	b := c.do(http.MethodPost, "/api/auth/login", credentials{"nobody", "wrong-password"})
	expect(t, a, http.StatusUnauthorized, "неверный пароль")
	expect(t, b, http.StatusUnauthorized, "нет логина")
	if a.Body.String() != b.Body.String() || decode[errBody](t, a).Error != "Неверный логин или пароль" {
		t.Fatalf("ответы различаются: %s / %s", a.Body.String(), b.Body.String())
	}
	expect(t, c.do(http.MethodPost, "/api/auth/login", "не объект"), http.StatusBadRequest, "не JSON-объект")
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)

	c := e.client()
	for i := range auth.LimitAttempts {
		expect(t, c.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "wrong-password"}), http.StatusUnauthorized, fmt.Sprintf("попытка %d", i+1))
		e.clock.Add(30 * time.Second)
	}
	// Шестая — 429, даже с верным паролем.
	rec := c.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "owner-password"})
	expect(t, rec, http.StatusTooManyRequests, "шестая попытка")
	body := decode[errBody](t, rec)
	// Первая неудача 2,5 минуты назад, окно 15 минут — ждать 12,5 минуты.
	if body.RetryAfter != 750 || rec.Header().Get("Retry-After") != "750" {
		t.Fatalf("retry_after %d, Retry-After %q", body.RetryAfter, rec.Header().Get("Retry-After"))
	}
	if body.Error != "Слишком много попыток. Повторите через 13 минут" {
		t.Fatalf("текст: %q", body.Error)
	}

	// Логин заблокирован и с другого адреса.
	other := e.client()
	other.remoteAddr = "198.51.100.7:1"
	expect(t, other.do(http.MethodPost, "/api/auth/login", credentials{"RINGINO", "owner-password"}), http.StatusTooManyRequests, "тот же логин с другого адреса")

	// После окна вход проходит, успех сбрасывает счётчик логина.
	e.clock.Add(13 * time.Minute)
	other.login("ringino", "owner-password")
	expect(t, other.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "wrong-password"}), http.StatusUnauthorized, "после сброса")
}

func TestRateLimitPerAddress(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("victim", "owner-password", false)
	c := e.client()
	for i := range auth.LimitAttempts {
		expect(t, c.do(http.MethodPost, "/api/auth/login", credentials{fmt.Sprintf("user%d", i), "wrong-password"}), http.StatusUnauthorized, "перебор логинов")
	}
	expect(t, c.do(http.MethodPost, "/api/auth/login", credentials{"victim", "owner-password"}), http.StatusTooManyRequests, "адрес заблокирован")
	// Другой адрес тем же логином входит.
	other := e.client()
	other.remoteAddr = "198.51.100.7:1"
	other.login("victim", "owner-password")
}

// X-Forwarded-For с не-loopback адреса не учитывается: сменой заголовка
// ограничение не обойти. С loopback (nginx) — учитывается.
func TestForwardedForTrust(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("victim", "owner-password", false)

	direct := e.client()
	direct.remoteAddr = "203.0.113.5:4000"
	for i := range auth.LimitAttempts {
		direct.header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		direct.header.Set("X-Real-IP", fmt.Sprintf("10.0.1.%d", i))
		expect(t, direct.do(http.MethodPost, "/api/auth/login", credentials{fmt.Sprintf("u%d", i), "wrong-password"}), http.StatusUnauthorized, "перебор")
	}
	direct.header.Set("X-Forwarded-For", "10.9.9.9")
	direct.header.Set("X-Real-IP", "10.9.9.9")
	expect(t, direct.do(http.MethodPost, "/api/auth/login", credentials{"victim", "owner-password"}), http.StatusTooManyRequests, "заголовки не помогли")

	// За nginx разные клиенты различаются по X-Real-IP / X-Forwarded-For.
	proxied := e.client()
	proxied.remoteAddr = "127.0.0.1:5000"
	for i := range auth.LimitAttempts {
		proxied.header.Set("X-Real-IP", "10.211.7.10")
		expect(t, proxied.do(http.MethodPost, "/api/auth/login", credentials{fmt.Sprintf("p%d", i), "wrong-password"}), http.StatusUnauthorized, "перебор через nginx")
	}
	expect(t, proxied.do(http.MethodPost, "/api/auth/login", credentials{"victim", "owner-password"}), http.StatusTooManyRequests, "клиент за nginx заблокирован")
	proxied.header.Del("X-Real-IP")
	proxied.header.Set("X-Forwarded-For", "6.6.6.6, 10.211.7.11")
	proxied.login("victim", "owner-password")

	// Левый адрес X-Forwarded-For подставлен клиентом и не учитывается.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 10.211.7.11")
	if ip := clientIP(req); ip != "10.211.7.11" {
		t.Fatalf("адрес %s", ip)
	}
	req.RemoteAddr = "[::1]:1"
	req.Header.Set("X-Real-IP", "10.211.7.12")
	if ip := clientIP(req); ip != "10.211.7.12" {
		t.Fatalf("адрес за IPv6 loopback %s", ip)
	}
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatal("нет cookie сессии")
	return nil
}

func TestCookieFlags(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		t.Run("insecure="+strconv.FormatBool(insecure), func(t *testing.T) {
			e := newEnv(t, insecure)
			e.createUser("ringino", "owner-password", false)
			c := e.client()
			rec := c.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "owner-password"})
			expect(t, rec, http.StatusOK, "вход")
			ck := sessionCookieFrom(t, rec)
			if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" || ck.MaxAge != 30*24*3600 {
				t.Fatalf("флаги cookie: %+v", ck)
			}
			if ck.Secure == insecure {
				t.Fatalf("Secure = %v при insecure_cookies = %v", ck.Secure, insecure)
			}
			raw := rec.Header().Get("Set-Cookie")
			for _, part := range []string{"HttpOnly", "SameSite=Strict", "Max-Age=2592000", "Path=/"} {
				if !strings.Contains(raw, part) {
					t.Fatalf("в Set-Cookie нет %s: %s", part, raw)
				}
			}
			if strings.Contains(raw, "Secure") == insecure {
				t.Fatalf("Set-Cookie: %s", raw)
			}

			rec = c.do(http.MethodPost, "/api/auth/logout", nil)
			ck = sessionCookieFrom(t, rec)
			if ck.MaxAge >= 0 || ck.Value != "" || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
				t.Fatalf("cookie не стёрта: %s", rec.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestSessionExpiryAndRefresh(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	c := e.client()
	c.login("ringino", "owner-password")

	// Меньше часа — cookie не переустанавливается.
	e.clock.Add(30 * time.Minute)
	rec := c.do(http.MethodGet, "/api/auth/me", nil)
	expect(t, rec, http.StatusOK, "me")
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("cookie переустановлена раньше часа")
	}
	// Больше часа — срок продлён, cookie выставлена заново.
	e.clock.Add(31 * time.Minute)
	rec = c.do(http.MethodGet, "/api/auth/me", nil)
	expect(t, rec, http.StatusOK, "me через час")
	if ck := sessionCookieFrom(t, rec); ck.MaxAge != 30*24*3600 {
		t.Fatalf("продление cookie: %+v", ck)
	}

	// 29 дней без работы — сессия жива (срок считается от продления).
	e.clock.Add(29 * 24 * time.Hour)
	expect(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "через 29 дней")

	// Ещё 31 день без работы — сессия истекла.
	e.clock.Add(31 * 24 * time.Hour)
	expect(t, c.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "просроченная сессия")
}

func TestPasswordChange(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	phone, laptop := e.client(), e.client()
	phone.login("ringino", "owner-password")
	laptop.login("ringino", "owner-password")

	rec := phone.do(http.MethodPost, "/api/auth/password", passwordRequest{Current: "owner-password", New: "short"})
	expect(t, rec, http.StatusBadRequest, "короткий новый пароль")
	if decode[errBody](t, rec).Field != "new" {
		t.Fatalf("field: %s", rec.Body.String())
	}
	rec = phone.do(http.MethodPost, "/api/auth/password", passwordRequest{Current: "wrong-password", New: "brand-new-password"})
	expect(t, rec, http.StatusForbidden, "неверный текущий пароль")
	if b := decode[errBody](t, rec); b.Field != "current" || b.Error != "Текущий пароль неверный" {
		t.Fatalf("ответ: %+v", b)
	}
	expect(t, phone.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "сессия жива после 403")

	expect(t, phone.do(http.MethodPost, "/api/auth/password", passwordRequest{Current: "owner-password", New: "brand-new-password"}),
		http.StatusNoContent, "смена пароля")
	expect(t, phone.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "текущая сессия")
	expect(t, laptop.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized, "другая сессия сброшена")

	expect(t, e.client().do(http.MethodPost, "/api/auth/login", credentials{"ringino", "owner-password"}), http.StatusUnauthorized, "старый пароль")
	e.client().login("ringino", "brand-new-password")
}

func TestPasswordChangeRateLimited(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	c := e.client()
	c.login("ringino", "owner-password")
	for range auth.LimitAttempts {
		expect(t, c.do(http.MethodPost, "/api/auth/password", passwordRequest{Current: "guess-guess", New: "brand-new-password"}), http.StatusForbidden, "перебор")
	}
	expect(t, c.do(http.MethodPost, "/api/auth/password", passwordRequest{Current: "owner-password", New: "brand-new-password"}), http.StatusTooManyRequests, "после 5 неудач")
	other := e.client()
	other.remoteAddr = "198.51.100.9:1"
	expect(t, other.do(http.MethodPost, "/api/auth/login", credentials{"ringino", "owner-password"}), http.StatusTooManyRequests, "вход тоже заблокирован")
}

func TestSettings(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	e.createUser("friend", "friend-password", false)
	c := e.client()
	c.login("ringino", "owner-password")

	rec := c.do(http.MethodPut, "/api/auth/settings", auth.Settings{StepSmall: 5, StepLarge: 25})
	expect(t, rec, http.StatusOK, "сохранение")
	if got := decode[auth.Settings](t, rec); got.StepSmall != 5 || got.StepLarge != 25 {
		t.Fatalf("ответ: %+v", got)
	}
	if got := decode[meBody](t, c.do(http.MethodGet, "/api/auth/me", nil)); got.Settings.StepLarge != 25 {
		t.Fatalf("me: %+v", got)
	}
	for _, tc := range []struct {
		body  any
		field string
	}{
		{auth.Settings{StepSmall: 0, StepLarge: 50}, "step_small"},
		{auth.Settings{StepSmall: 10, StepLarge: 5001}, "step_large"},
		{auth.Settings{StepSmall: 60, StepLarge: 50}, "step_large"},
	} {
		rec := c.do(http.MethodPut, "/api/auth/settings", tc.body)
		expect(t, rec, http.StatusBadRequest, fmt.Sprintf("%+v", tc.body))
		if decode[errBody](t, rec).Field != tc.field {
			t.Fatalf("field: %s", rec.Body.String())
		}
	}
	expect(t, c.do(http.MethodPut, "/api/auth/settings", map[string]any{"step_small": 1.5, "step_large": 50}), http.StatusBadRequest, "дробное")

	f := e.client()
	f.login("friend", "friend-password")
	if got := decode[meBody](t, f.do(http.MethodGet, "/api/auth/me", nil)); got.Settings != auth.DefaultSettings {
		t.Fatalf("настройки соседа изменились: %+v", got.Settings)
	}
}

// Изоляция (ТБ-5): второй владелец не видит и не может отозвать
// приглашения первого; приглашённый видит только себя.
func TestIsolation(t *testing.T) {
	e := newEnv(t, false)
	e.createUser("ringino", "owner-password", false)
	e.createUser("second", "second-password", true)

	first := e.client()
	first.login("ringino", "owner-password")
	inv := decode[createdInvite](t, first.do(http.MethodPost, "/api/invites", nil))

	second := e.client()
	second.login("second", "second-password")
	rec := second.do(http.MethodGet, "/api/invites", nil)
	expect(t, rec, http.StatusOK, "список второго")
	if list := decode[inviteList](t, rec); len(list.Invites) != 0 {
		t.Fatalf("второй видит чужие приглашения: %+v", list)
	}
	expect(t, second.do(http.MethodDelete, fmt.Sprintf("/api/invites/%d", inv.Invite.ID), nil), http.StatusNotFound, "отзыв чужого")
	expect(t, second.do(http.MethodDelete, "/api/invites/999", nil), http.StatusNotFound, "несуществующее")
	expect(t, second.do(http.MethodDelete, "/api/invites/abc", nil), http.StatusNotFound, "id не число")

	// Приглашение первого не пострадало и отзывается им самим.
	token := tokenFromURL(t, inv.URL)
	expect(t, e.client().do(http.MethodGet, "/api/invites/by-token/"+token, nil), http.StatusOK, "приглашение цело")
	expect(t, first.do(http.MethodDelete, fmt.Sprintf("/api/invites/%d", inv.Invite.ID), nil), http.StatusNoContent, "отзыв своего")
	expect(t, e.client().do(http.MethodGet, "/api/invites/by-token/"+token, nil), http.StatusNotFound, "отозванное")
	expect(t, e.client().do(http.MethodPost, "/api/invites/by-token/"+token+"/accept", credentials{"late", "late-password"}), http.StatusNotFound, "принятие отозванного")

	// me каждого — только свой пользователь; logout-all одного не трогает другого.
	if got := decode[meBody](t, second.do(http.MethodGet, "/api/auth/me", nil)); got.User.Login != "second" {
		t.Fatalf("me второго: %+v", got)
	}
	expect(t, second.do(http.MethodPost, "/api/auth/logout-all", nil), http.StatusNoContent, "logout-all второго")
	expect(t, first.do(http.MethodGet, "/api/auth/me", nil), http.StatusOK, "первый остался в системе")
}

func TestInviteByTokenUnknown(t *testing.T) {
	e := newEnv(t, false)
	rec := e.client().do(http.MethodGet, "/api/invites/by-token/nope", nil)
	expect(t, rec, http.StatusNotFound, "неизвестный токен")
	if decode[errBody](t, rec).Error != "Приглашение не найдено или уже недействительно" {
		t.Fatalf("текст: %s", rec.Body.String())
	}
}

func TestPluralMinutes(t *testing.T) {
	for n, want := range map[int]string{1: "минуту", 2: "минуты", 5: "минут", 11: "минут", 12: "минут", 15: "минут", 21: "минуту", 22: "минуты"} {
		if got := pluralMinutes(n); got != want {
			t.Errorf("%d: %s, ожидалось %s", n, got, want)
		}
	}
}
