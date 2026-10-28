package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

// fastParams — дешёвые параметры для тестов: проверяется логика, а не стойкость.
var fastParams = Params{Time: 1, Memory: 64, Threads: 1}

// clock — подменяемое время, общее для сервиса и ограничителя.
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

func newTestService(t *testing.T) (*Service, *clock) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Unix(1_792_000_000, 0)}
	return New(db.DB, Options{Now: c.Now, Params: fastParams}), c
}

func mustUser(t *testing.T, s *Service, login string, owner bool) User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), login, "password-"+login, owner)
	if err != nil {
		t.Fatalf("CreateUser %s: %v", login, err)
	}
	return u
}

func TestHashFormatAndCheck(t *testing.T) {
	s := New(nil, Options{})
	h, err := s.HashPassword("секретный пароль")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=2,p=1$") {
		t.Fatalf("формат хеша: %s", h)
	}
	if ok, err := s.CheckPassword(h, "секретный пароль"); err != nil || !ok {
		t.Fatalf("верный пароль не прошёл: %v %v", ok, err)
	}
	if ok, _ := s.CheckPassword(h, "секретный парол"); ok {
		t.Fatal("неверный пароль прошёл")
	}
	h2, _ := s.HashPassword("секретный пароль")
	if h == h2 {
		t.Fatal("соль не случайна: хеши одинаковые")
	}
}

// Хеш, посчитанный с одними параметрами, проверяется сервисом с другими.
func TestHashParamsStoredInHash(t *testing.T) {
	fast := New(nil, Options{Params: fastParams})
	h, _ := fast.HashPassword("password1")
	if ok, err := New(nil, Options{}).CheckPassword(h, "password1"); err != nil || !ok {
		t.Fatalf("проверка со старыми параметрами: %v %v", ok, err)
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=64,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=64,t=1,p=1$!!$a2V5"} {
		if _, err := fast.CheckPassword(bad, "x"); err == nil {
			t.Errorf("хеш %q принят", bad)
		}
	}
}

func TestValidation(t *testing.T) {
	for _, l := range []string{"ab", strings.Repeat("a", 33), "логин", "a b", "a@b"} {
		if ValidateLogin(l) == nil {
			t.Errorf("логин %q принят", l)
		}
	}
	for _, l := range []string{"abc", "Ringino_1.2-3", strings.Repeat("a", 32)} {
		if err := ValidateLogin(l); err != nil {
			t.Errorf("логин %q отклонён: %v", l, err)
		}
	}
	// Символы, а не байты: 8 кириллических букв — 16 байт, но это 8 символов.
	if err := ValidatePassword("пароль12"); err != nil {
		t.Errorf("8 символов кириллицы отклонены: %v", err)
	}
	if ValidatePassword("1234567") == nil || ValidatePassword(strings.Repeat("я", 129)) == nil {
		t.Error("пароль вне 8–128 символов принят")
	}
	if ValidatePassword(strings.Repeat("я", 128)) != nil {
		t.Error("128 символов (256 байт) отклонены")
	}
	var ve *ValidationError
	if err := ValidateSettings(Settings{StepSmall: 50, StepLarge: 50}); !errors.As(err, &ve) || ve.Field != "step_large" {
		t.Errorf("равные шаги: %v", err)
	}
	if err := ValidateSettings(Settings{StepSmall: 0, StepLarge: 50}); !errors.As(err, &ve) || ve.Field != "step_small" {
		t.Errorf("нулевой шаг: %v", err)
	}
}

func TestCreateUserOwnerAndDefaults(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	first := mustUser(t, s, "Ringino", false)
	second := mustUser(t, s, "friend", false)
	third := mustUser(t, s, "second-owner", true)
	if !first.IsOwner || second.IsOwner || !third.IsOwner {
		t.Fatalf("владельцы: %v %v %v", first.IsOwner, second.IsOwner, third.IsOwner)
	}

	if _, err := s.CreateUser(ctx, "RINGINO", "password123", false); !errors.Is(err, ErrLoginTaken) {
		t.Fatalf("логин без учёта регистра: %v", err)
	}
	var ve *ValidationError
	if _, err := s.CreateUser(ctx, "x", "password123", false); !errors.As(err, &ve) || ve.Field != "login" {
		t.Fatalf("короткий логин: %v", err)
	}

	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE user_id = ?`, second.ID).Scan(&n); err != nil || n != len(defaultCategories) {
		t.Fatalf("категорий по умолчанию %d (%v)", n, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM purposes WHERE user_id = ?`, second.ID).Scan(&n); err != nil || n != len(defaultPurposes) {
		t.Fatalf("назначений по умолчанию %d (%v)", n, err)
	}
	var color string
	if err := s.db.QueryRow(`SELECT color FROM categories WHERE user_id = ? AND name = 'Активная пена'`, second.ID).Scan(&color); err != nil || color != "berry" {
		t.Fatalf("цвет «Активной пены»: %q (%v)", color, err)
	}
	st, err := s.GetSettings(ctx, second.ID)
	if err != nil || st != DefaultSettings {
		t.Fatalf("настройки: %+v %v", st, err)
	}
	if _, err := s.UpdateSettings(ctx, second.ID, Settings{StepSmall: 5, StepLarge: 100}); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.GetSettings(ctx, second.ID); st.StepSmall != 5 || st.StepLarge != 100 {
		t.Fatalf("настройки после изменения: %+v", st)
	}
	if st, _ := s.GetSettings(ctx, first.ID); st != DefaultSettings {
		t.Fatalf("настройки соседа изменились: %+v", st)
	}

	list, err := s.ListUsers(ctx)
	if err != nil || len(list) != 3 || list[0].Login != "Ringino" {
		t.Fatalf("список: %+v %v", list, err)
	}
}

func TestAuthenticate(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	u := mustUser(t, s, "ringino", false)

	got, err := s.Authenticate(ctx, "RINGINO", "password-ringino")
	if err != nil || got.ID != u.ID {
		t.Fatalf("вход: %+v %v", got, err)
	}
	if _, err := s.Authenticate(ctx, "ringino", "wrong-password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("неверный пароль: %v", err)
	}
	if _, err := s.Authenticate(ctx, "nobody", "wrong-password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("нет логина: %v", err)
	}
	if s.dummyHash == "" {
		t.Fatal("для несуществующего логина не считался хеш")
	}
}

// Проверка несуществующего логина занимает сопоставимое время.
func TestAuthenticateTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("медленный тест")
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	s := New(db.DB, Options{Params: Params{Time: 1, Memory: 16 * 1024, Threads: 1}})
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "ringino", "password123", false); err != nil {
		t.Fatal(err)
	}
	s.burnTime("прогрев")

	measure := func(login string) time.Duration {
		start := time.Now()
		for range 5 {
			_, _ = s.Authenticate(ctx, login, "wrong-password")
		}
		return time.Since(start)
	}
	known, unknown := measure("ringino"), measure("nobody")
	if unknown < known/3 {
		t.Fatalf("несуществующий логин проверяется слишком быстро: %v против %v", unknown, known)
	}
}

func TestSetPasswordResetsSessions(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	u := mustUser(t, s, "ringino", false)
	keep, _ := s.CreateSession(ctx, u.ID, "a")
	other, _ := s.CreateSession(ctx, u.ID, "b")

	if err := s.SetPassword(ctx, u.ID, "short", keep.TokenHash); err == nil {
		t.Fatal("короткий пароль принят")
	}
	if err := s.SetPassword(ctx, u.ID, "new-password", keep.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.ResolveSession(ctx, keep.Token); err != nil {
		t.Fatalf("текущая сессия удалена: %v", err)
	}
	if _, _, _, err := s.ResolveSession(ctx, other.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("другая сессия осталась: %v", err)
	}
	if _, err := s.Authenticate(ctx, "ringino", "new-password"); err != nil {
		t.Fatalf("вход с новым паролем: %v", err)
	}
	if ok, _ := s.CheckUserPassword(ctx, u.ID, "password-ringino"); ok {
		t.Fatal("старый пароль всё ещё подходит")
	}

	// CLI: без keep — сбрасываются все.
	if err := s.SetPassword(ctx, u.ID, "newer-password", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.ResolveSession(ctx, keep.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("после сброса всех сессия осталась: %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s, c := newTestService(t)
	ctx := context.Background()
	u := mustUser(t, s, "ringino", false)

	sess, err := s.CreateSession(ctx, u.ID, strings.Repeat("ю", 1000))
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Token) != 43 { // 32 байта в base64url без выравнивания
		t.Fatalf("длина токена %d", len(sess.Token))
	}
	var stored, ua string
	if err := s.db.QueryRow(`SELECT token_hash, user_agent FROM sessions`).Scan(&stored, &ua); err != nil {
		t.Fatal(err)
	}
	if stored == sess.Token || stored != hashToken(sess.Token) {
		t.Fatal("в базе должен лежать хеш токена")
	}
	if n := len([]rune(ua)); n != maxUserAgent {
		t.Fatalf("User-Agent не обрезан: %d", n)
	}

	_, got, ext, err := s.ResolveSession(ctx, sess.Token)
	if err != nil || got.ID != u.ID || ext {
		t.Fatalf("поиск: %+v %v %v", got, ext, err)
	}
	if _, _, _, err := s.ResolveSession(ctx, sess.Token+"x"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("чужой токен: %v", err)
	}

	// Меньше часа — не продлевается.
	c.Add(59 * time.Minute)
	if _, _, ext, _ := s.ResolveSession(ctx, sess.Token); ext {
		t.Fatal("продление раньше часа")
	}
	// Больше часа — продлевается на 30 дней от текущего момента.
	c.Add(2 * time.Minute)
	got2, _, ext, err := s.ResolveSession(ctx, sess.Token)
	if err != nil || !ext {
		t.Fatalf("продление: %v %v", ext, err)
	}
	if want := c.Now().Add(SessionTTL); !got2.ExpiresAt.Equal(want) {
		t.Fatalf("срок %v, ожидался %v", got2.ExpiresAt, want)
	}
	// Сразу после продления — снова не продлевается.
	if _, _, ext, _ := s.ResolveSession(ctx, sess.Token); ext {
		t.Fatal("повторное продление")
	}

	// Просроченная сессия не находится и вычищается при следующем входе.
	c.Add(SessionTTL)
	if _, _, _, err := s.ResolveSession(ctx, sess.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("просроченная сессия: %v", err)
	}
	if _, err := s.CreateSession(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 1 {
		t.Fatalf("просроченная сессия не удалена, сессий %d", n)
	}
}

func TestDeleteSessions(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	a := mustUser(t, s, "alice", false)
	b := mustUser(t, s, "bob", false)
	a1, _ := s.CreateSession(ctx, a.ID, "")
	a2, _ := s.CreateSession(ctx, a.ID, "")
	a3, _ := s.CreateSession(ctx, a.ID, "")
	b1, _ := s.CreateSession(ctx, b.ID, "")

	if err := s.DeleteSession(ctx, a1.Token); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession(ctx, "нет-такого"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUserSessions(ctx, a.ID, a2.TokenHash); err != nil {
		t.Fatal(err)
	}
	alive := func(sess Session) bool { _, _, _, err := s.ResolveSession(ctx, sess.Token); return err == nil }
	if alive(a1) || !alive(a2) || alive(a3) || !alive(b1) {
		t.Fatalf("после удаления: a1=%v a2=%v a3=%v b1=%v", alive(a1), alive(a2), alive(a3), alive(b1))
	}
	if err := s.DeleteUserSessions(ctx, a.ID, ""); err != nil {
		t.Fatal(err)
	}
	if alive(a2) || !alive(b1) {
		t.Fatal("удаление всех сессий пользователя задело не те")
	}
}

func TestInvites(t *testing.T) {
	s, c := newTestService(t)
	ctx := context.Background()
	owner := mustUser(t, s, "ringino", false)
	other := mustUser(t, s, "other-owner", true)

	inv, token, err := s.CreateInvite(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Status != InviteActive || !inv.ExpiresAt.Equal(c.Now().Add(InviteTTL)) {
		t.Fatalf("приглашение: %+v", inv)
	}
	if exp, err := s.CheckInvite(ctx, token); err != nil || !exp.Equal(inv.ExpiresAt) {
		t.Fatalf("проверка: %v %v", exp, err)
	}
	var stored string
	_ = s.db.QueryRow(`SELECT token_hash FROM invites WHERE id = ?`, inv.ID).Scan(&stored)
	if stored != hashToken(token) {
		t.Fatal("в базе должен лежать хеш токена приглашения")
	}

	// Чужое приглашение не отзывается и не видно.
	if err := s.RevokeInvite(ctx, other.ID, inv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("отзыв чужого: %v", err)
	}
	if list, _ := s.ListInvites(ctx, other.ID); len(list) != 0 {
		t.Fatalf("чужие приглашения видны: %+v", list)
	}

	// Принятие: пользователь создан, приглашение использовано.
	u, err := s.AcceptInvite(ctx, token, "friend", "friend-password")
	if err != nil {
		t.Fatal(err)
	}
	if u.IsOwner {
		t.Fatal("приглашённый стал владельцем")
	}
	var usedBy int64
	_ = s.db.QueryRow(`SELECT used_by FROM invites WHERE id = ?`, inv.ID).Scan(&usedBy)
	if usedBy != u.ID {
		t.Fatalf("used_by = %d", usedBy)
	}
	if _, err := s.AcceptInvite(ctx, token, "friend2", "friend-password"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("повторное принятие: %v", err)
	}
	// По мёртвой ссылке — ошибка приглашения раньше ошибок полей.
	if _, err := s.AcceptInvite(ctx, token, "", "x"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("мёртвая ссылка с плохим вводом: %v", err)
	}
	if _, err := s.CheckInvite(ctx, token); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("использованное приглашение действует: %v", err)
	}
	if err := s.RevokeInvite(ctx, owner.ID, inv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("отзыв использованного: %v", err)
	}

	// Отзыв и истечение.
	revoked, rtoken, _ := s.CreateInvite(ctx, owner.ID)
	if err := s.RevokeInvite(ctx, owner.ID, revoked.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptInvite(ctx, rtoken, "friend3", "friend-password"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("отозванное принято: %v", err)
	}
	c.Add(time.Second)
	_, etoken, _ := s.CreateInvite(ctx, owner.ID)
	c.Add(InviteTTL)
	if _, err := s.CheckInvite(ctx, etoken); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("истёкшее действует: %v", err)
	}
	c.Add(time.Second)
	_, _, _ = s.CreateInvite(ctx, owner.ID)

	list, err := s.ListInvites(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	for _, i := range list {
		statuses = append(statuses, i.Status)
	}
	// Новые сверху: активное, истёкшее, отозванное, использованное.
	if got := strings.Join(statuses, ","); got != "active,expired,revoked,used" {
		t.Fatalf("статусы %s", got)
	}

	// Логин занят — приглашение не тратится.
	_, ftoken, _ := s.CreateInvite(ctx, owner.ID)
	if _, err := s.AcceptInvite(ctx, ftoken, "FRIEND", "friend-password"); !errors.Is(err, ErrLoginTaken) {
		t.Fatalf("занятый логин: %v", err)
	}
	if _, err := s.CheckInvite(ctx, ftoken); err != nil {
		t.Fatalf("после ошибки приглашение испорчено: %v", err)
	}
}

func TestInviteListLimit(t *testing.T) {
	s, c := newTestService(t)
	ctx := context.Background()
	owner := mustUser(t, s, "ringino", false)
	for range inviteListLimit + 5 {
		c.Add(time.Second)
		if _, _, err := s.CreateInvite(ctx, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.ListInvites(ctx, owner.ID)
	if len(list) != inviteListLimit || !list[0].CreatedAt.After(list[1].CreatedAt) {
		t.Fatalf("список: %d записей", len(list))
	}
}

// Два одновременных принятия одного приглашения: успешно ровно одно.
func TestAcceptInviteRace(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	owner := mustUser(t, s, "ringino", false)
	_, token, _ := s.CreateInvite(ctx, owner.ID)

	const n = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ok   int
		errs []error
	)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.AcceptInvite(ctx, token, "friend"+string(rune('a'+i)), "friend-password")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else {
				errs = append(errs, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 {
		t.Fatalf("успешных принятий %d, ожидалось 1", ok)
	}
	for _, err := range errs {
		if !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("неожиданная ошибка: %v", err)
		}
	}
	var users int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users)
	if users != 2 {
		t.Fatalf("пользователей %d, ожидалось 2", users)
	}
}

func TestLimiter(t *testing.T) {
	c := &clock{t: time.Unix(1_792_000_000, 0)}
	l := NewLimiter(LimitAttempts, LimitWindow, c.Now)

	for i := range LimitAttempts {
		if b, _ := l.Blocked("login:a", "ip:1"); b {
			t.Fatalf("заблокировано после %d неудач", i)
		}
		l.Fail("login:a", "ip:1")
		c.Add(time.Minute)
	}
	b, wait := l.Blocked("login:a")
	if !b {
		t.Fatal("нет блокировки после 5 неудач")
	}
	// Первая неудача была 5 минут назад, окно 15 минут — ждать 10.
	if wait != 10*time.Minute {
		t.Fatalf("ожидание %v", wait)
	}
	// Блокировка по адресу действует и для другого логина.
	if b, _ := l.Blocked("login:b", "ip:1"); !b {
		t.Fatal("адрес не заблокирован")
	}
	// Сброс логина не снимает блокировку адреса.
	l.Reset("login:a")
	if b, _ := l.Blocked("login:a"); b {
		t.Fatal("сброс не сработал")
	}
	if b, _ := l.Blocked("ip:1"); !b {
		t.Fatal("сброс логина снял блокировку адреса")
	}
	// Окно скользящее: через 10 минут первая неудача выходит из окна.
	c.Add(10 * time.Minute)
	if b, _ := l.Blocked("ip:1"); b {
		t.Fatal("блокировка не снялась после окна")
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := NewLimiter(LimitAttempts, LimitWindow, nil)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := "ip:" + string(rune('a'+i%5))
			for range 100 {
				l.Fail(key)
				l.Blocked(key, "login:x")
				if i%7 == 0 {
					l.Reset("ip:reset")
				}
			}
		}()
	}
	wg.Wait()
	if b, _ := l.Blocked("ip:b"); !b {
		t.Fatal("после множества неудач нет блокировки")
	}
}
