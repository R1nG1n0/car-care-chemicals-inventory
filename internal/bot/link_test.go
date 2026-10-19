package bot

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

func TestLinkByCode(t *testing.T) {
	e := newEnv(t)
	a := e.newUser("owner", 1001)
	code, exp, err := e.bot.Links().NewCode(e.ctx, a.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) > 64 || exp != e.clock.Now().Add(10*time.Minute).Unix() {
		t.Fatalf("код %q, срок %d", code, exp)
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			t.Fatalf("символ %q недопустим в start", r)
		}
	}
	// В базе только хеш.
	var hash string
	if err := e.db.QueryRow(`SELECT hash FROM telegram_link_codes`).Scan(&hash); err != nil || hash == code || len(hash) != 64 {
		t.Fatalf("хеш %q, ошибка %v", hash, err)
	}

	e.say(a, "/start "+code)
	contains(t, e.fake.last().Text, "Готово", "owner")
	link, ok, _ := e.bot.Links().ByUser(e.ctx, a.id)
	if !ok || link.TGUserID != 1001 || link.ChatID != 1001 {
		t.Fatalf("привязка: %+v %v", link, ok)
	}

	// Код одноразовый.
	e.say(a, "/start "+code)
	contains(t, e.fake.last().Text, "недействительна")
}

func TestLinkCodeExpiresAndOnlyPrivate(t *testing.T) {
	e := newEnv(t)
	a := e.newUser("owner", 1001)
	code, _, _ := e.bot.Links().NewCode(e.ctx, a.id)
	e.clock.Set(e.clock.Now().Add(11 * time.Minute))
	e.say(a, "/start "+code)
	contains(t, e.fake.last().Text, "недействительна")
	if _, ok, _ := e.bot.Links().ByUser(e.ctx, a.id); ok {
		t.Fatal("просроченный код привязал аккаунт")
	}

	// В группе код не принимается и ответа нет.
	code, _, _ = e.bot.Links().NewCode(e.ctx, a.id)
	before := e.fake.count()
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: 99, Message: &telegram.Message{
		Text: "/start " + code, From: &telegram.User{ID: 1001}, Chat: telegram.Chat{ID: -500, Type: "group"}}})
	if e.fake.count() != before {
		t.Fatal("бот ответил в группе")
	}
	if _, ok, _ := e.bot.Links().ByUser(e.ctx, a.id); ok {
		t.Fatal("в группе привязать нельзя")
	}
	// Новый код гасит предыдущий.
	c2, _, _ := e.bot.Links().NewCode(e.ctx, a.id)
	if _, err := e.bot.Links().Redeem(e.ctx, code, 1001, 1001, "x"); !errors.Is(err, ErrBadCode) {
		t.Fatalf("прежний код должен погаснуть: %v", err)
	}
	if _, err := e.bot.Links().Redeem(e.ctx, c2, 1001, 1001, "x"); err != nil {
		t.Fatal(err)
	}
}

// Один аккаунт Telegram — один пользователь; новая привязка заменяет старую.
func TestRelinkReplaces(t *testing.T) {
	e := newEnv(t)
	a := e.newUser("first", 1001)
	b := e.newUser("second", 1001)
	ca, _, _ := e.bot.Links().NewCode(e.ctx, a.id)
	cb, _, _ := e.bot.Links().NewCode(e.ctx, b.id)
	e.say(a, "/start "+ca)
	e.say(b, "/start "+cb)
	if e.count(`SELECT COUNT(*) FROM telegram_links`) != 1 {
		t.Fatal("должна остаться одна привязка")
	}
	link, ok, _ := e.bot.Links().ByTelegram(e.ctx, 1001)
	if !ok || link.UserID != b.id {
		t.Fatalf("аккаунт должен принадлежать второму: %+v", link)
	}
	if _, ok, _ := e.bot.Links().ByUser(e.ctx, a.id); ok {
		t.Fatal("у первого привязки быть не должно")
	}
}

func TestUnlinkedGetsOnlyHint(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Секретный шампунь", "", "ml", 500, nil, "4600051000057")
	e.buy(a, p, 1, 1000, nil, nil)

	stranger := user{tg: 777}
	for _, text := range []string{"секретный", "секретный 50", "4600051000057", "/list", "/help", "Купить", "/start"} {
		e.say(stranger, text)
		m := e.fake.last()
		contains(t, m.Text, "Ещё → Профиль → Telegram")
		if len(m.Text) > 600 || containsAny(m.Text, "Секретный", "ниже минимума") {
			t.Fatalf("непривязанному %q показаны данные: %s", text, m.Text)
		}
	}
	// Кнопки тоже: чужой нажимает кнопку из сообщения владельца.
	e.say(a, "секретный")
	card := e.fake.last()
	before := e.total(a, p)
	e.clickData(stranger, card, "c:"+itoa(p)+":50")
	if e.total(a, p) != before {
		t.Fatal("непривязанный списал остаток")
	}
	contains(t, e.fake.last().Text, "Ещё → Профиль → Telegram")
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Привязка не раскрывается в группе, даже если привязанный пользователь пишет там.
func TestLinkedUserInGroupIgnored(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.product(a, "Шампунь", "", "ml", 500, nil, "")
	before := e.fake.count()
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: 5, Message: &telegram.Message{
		Text: "шампунь", From: &telegram.User{ID: 1001}, Chat: telegram.Chat{ID: -42, Type: "supergroup"}}})
	if e.fake.count() != before {
		t.Fatal("бот ответил в группе")
	}
	// Нажатие кнопки из чужого (не привязанного) чата.
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: 6, CallbackQuery: &telegram.CallbackQuery{
		ID: "x", From: telegram.User{ID: 1001}, Data: "c:1:10",
		Message: &telegram.Message{MessageID: 1, Chat: telegram.Chat{ID: -42, Type: "supergroup"}}}})
	if e.fake.count() != before {
		t.Fatal("бот ответил на кнопку в группе")
	}
}
