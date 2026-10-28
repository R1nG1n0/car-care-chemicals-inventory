package bot

import (
	"strings"
	"testing"
)

func notifications(e *env, chat int64) []sentMsg {
	var out []sentMsg
	for _, m := range e.fake.sent() {
		if m.Chat == chat && strings.HasPrefix(m.Text, "Заканчивается") {
			out = append(out, m)
		}
	}
	return out
}

// ФТ-11.7.2: одно уведомление на одно пересечение порога.
func TestLowNotificationOncePerCrossing(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	shop := e.exec(`INSERT INTO stores (user_id, name) VALUES (?, 'Автохим')`, a.id)
	p := e.product(a, "Шампунь", "Gyeon", "ml", 500, ptr(int64(500)), "")
	e.buy(a, p, 2, 119000, &shop, nil)

	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 0 {
		t.Fatal("остаток выше минимума — уведомления быть не должно")
	}

	// Расход в боте: 1000 → 450 < 500.
	e.say(a, "шампунь 550")
	e.bot.CheckLow(e.ctx)
	n := notifications(e, 1001)
	if len(n) != 1 {
		t.Fatalf("уведомлений %d, ожидалось 1", len(n))
	}
	contains(t, n[0].Text, "Шампунь (Gyeon)", "Осталось 450 мл, минимум 500 мл", "1 190 ₽ · Автохим")
	if _, ok := n[0].button("Куплено"); !ok {
		t.Fatalf("нет кнопки «Куплено»: %+v", n[0].Kb)
	}

	// Повторные проверки и дальнейшее падение — без новых уведомлений.
	e.bot.CheckLow(e.ctx)
	e.say(a, "шампунь 100")
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 1 {
		t.Fatal("на то же пересечение порога пришло повторное уведомление")
	}

	// Остаток поднялся до минимума (покупка) — состояние сбрасывается; новое падение — новое уведомление.
	e.buy(a, p, 1, 119000, &shop, nil)
	e.bot.CheckLow(e.ctx)
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 0 {
		t.Fatal("позиция вышла из «низких», состояние должно очиститься")
	}
	e.say(a, "шампунь 450")
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 2 {
		t.Fatalf("после нового падения ожидалось 2 уведомления, есть %d", len(notifications(e, 1001)))
	}
}

// Падение из-за изменения минимума на сайте тоже вызывает уведомление.
func TestLowNotificationOnMinChange(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(100)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.bot.CheckLow(e.ctx)
	e.exec(`UPDATE products SET min_stock = 800 WHERE id = ?`, p)
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 1 {
		t.Fatal("ожидалось одно уведомление")
	}
}

// ФТ-11.7.3: при подключении уже низкие позиции уведомлений не вызывают.
func TestLowNotificationSeededOnLink(t *testing.T) {
	e := newEnv(t)
	a := e.newUser("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	code, _, _ := e.bot.Links().NewCode(e.ctx, a.id)
	e.say(a, "/start "+code)
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 0 {
		t.Fatal("уже низкая позиция не должна вызывать уведомление при подключении")
	}
	if e.count(`SELECT COUNT(*) FROM bot_low_state WHERE user_id = ?`, a.id) != 1 {
		t.Fatal("состояние должно быть заполнено")
	}
	// А вот новая позиция, упавшая позже, — вызывает.
	q := e.product(a, "Воск", "", "ml", 300, ptr(int64(200)), "")
	e.buy(a, q, 1, 1000, nil, nil)
	e.say(a, "воск 150")
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 1 {
		t.Fatal("ожидалось уведомление по воску")
	}
}

// Telegram недоступен — уведомление не теряется и приходит позже ровно одно.
func TestLowNotificationRetriesAfterOutage(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.fake.failAll = true
	e.bot.CheckLow(e.ctx)
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 0 {
		t.Fatal("при сбое отправки состояние записывать нельзя")
	}
	e.fake.failAll = false
	e.bot.CheckLow(e.ctx)
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 1 {
		t.Fatalf("уведомлений %d", len(notifications(e, 1001)))
	}
}

// Бот заблокирован пользователем (403) — не повторяем бесконечно.
func TestLowNotificationBlockedChat(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.fake.blocked[1001] = true
	e.bot.CheckLow(e.ctx)
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 1 {
		t.Fatal("для заблокированного чата состояние записывается, чтобы не повторять")
	}
}

// Уведомление идёт только своему пользователю и только про его позиции.
func TestNotificationIsolation(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	b := e.linked("friend", 2002)
	pa := e.product(a, "Секрет А", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, pa, 1, 1000, nil, nil)
	pb := e.product(b, "Секрет Б", "", "ml", 500, ptr(int64(100)), "")
	e.buy(b, pb, 1, 1000, nil, nil)
	e.bot.CheckLow(e.ctx)
	if n := notifications(e, 1001); len(n) != 1 || !strings.Contains(n[0].Text, "Секрет А") {
		t.Fatalf("у первого: %+v", n)
	}
	if n := notifications(e, 2002); len(n) != 0 {
		t.Fatalf("у второго ничего не должно прийти: %+v", n)
	}
	// Кнопка «Куплено» из уведомления первого, нажатая вторым, чужую позицию не трогает.
	m := notifications(e, 1001)[0]
	e.clickData(b, m, "b:"+itoa(pa))
	if strings.Contains(e.fake.last().Text, "Секрет А") {
		t.Fatalf("второму показана чужая позиция: %s", e.fake.last().Text)
	}
	contains(t, e.fake.last().Text, "Не найдено")
}

// Сбой отправки по одной позиции не мешает остальным.
func TestLowNotificationPartialFailure(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	for _, name := range []string{"Альфа", "Бета"} {
		p := e.product(a, name, "", "ml", 500, ptr(int64(900)), "")
		e.buy(a, p, 1, 1000, nil, nil)
	}
	e.fake.failText = "Альфа"
	e.bot.CheckLow(e.ctx)
	n := notifications(e, 1001)
	if len(n) != 1 || !strings.Contains(n[0].Text, "Бета") {
		t.Fatalf("уведомления: %+v", n)
	}
	e.fake.failText = ""
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 2 {
		t.Fatal("сбойная позиция должна прийти при следующей проверке")
	}
}

func TestLowNotificationChatNotFoundIsTerminal(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.fake.chatNotFound = true
	e.bot.CheckLow(e.ctx)
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 1 {
		t.Fatal("chat not found — терминальная ошибка, состояние записывается")
	}
}

// Привязка и засев состояния неделимы для проверки минимумов (ФТ-11.7.3).
func TestLinkSeedNotRacingWithLowCheck(t *testing.T) {
	for i := 0; i < 20; i++ {
		e := newEnv(t)
		a := e.newUser("owner", 1001)
		p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
		e.buy(a, p, 1, 1000, nil, nil)
		code, _, _ := e.bot.Links().NewCode(e.ctx, a.id)
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				default:
					e.bot.CheckLow(e.ctx)
				}
			}
		}()
		e.say(a, "/start "+code)
		close(stop)
		<-done
		e.bot.CheckLow(e.ctx)
		if len(notifications(e, 1001)) != 0 {
			t.Fatalf("итерация %d: уведомление сразу после привязки", i)
		}
	}
}

// linkPurpose даёт позиции назначение с таким именем (создаёт, если нет).
func (e *env) linkPurpose(u user, product int64, name string) {
	e.t.Helper()
	e.exec(`INSERT OR IGNORE INTO purposes (user_id, name) VALUES (?, ?)`, u.id, name)
	e.exec(`INSERT INTO product_purposes (user_id, product_id, purpose_id)
		SELECT ?, ?, id FROM purposes WHERE user_id = ? AND name = ?`, u.id, product, u.id, name)
}

// ФТ-15.4.2: по «Не брать больше» уведомления нет и состояние не копится;
// после снятия пометки позиция, всё ещё ниже минимума, вызывает уведомление.
func TestLowNotificationSkipsDoNotBuy(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(800)), "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.exec(`UPDATE products SET do_not_buy = 1 WHERE id = ?`, p)

	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 0 {
		t.Fatal("по «Не брать больше» пришло уведомление")
	}
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 0 {
		t.Fatal("состояние минимумов копится по «Не брать больше»")
	}
	e.exec(`UPDATE products SET do_not_buy = 0 WHERE id = ?`, p)
	e.bot.CheckLow(e.ctx)
	if len(notifications(e, 1001)) != 1 {
		t.Fatal("после снятия пометки ожидалось уведомление")
	}
	// Пометили после уведомления — состояние очищается.
	e.exec(`UPDATE products SET do_not_buy = 1 WHERE id = ?`, p)
	e.bot.CheckLow(e.ctx)
	if e.count(`SELECT COUNT(*) FROM bot_low_state`) != 0 {
		t.Fatal("состояние должно очищаться, когда позиция помечена")
	}
}

// ФТ-15.5.3: в уведомлении — «Есть дома для того же» с аналогами.
func TestLowNotificationAnalogs(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	low := e.product(a, "Кондиционер кожи", "", "ml", 500, ptr(int64(500)), "")
	e.linkPurpose(a, low, "кожа")
	e.buy(a, low, 1, 1000, nil, nil)
	home := e.product(a, "Крем для кожи", "Chemical Guys", "ml", 250, nil, "")
	e.linkPurpose(a, home, "кожа")
	e.buy(a, home, 1, 500, nil, nil)
	empty := e.product(a, "Пустой", "", "ml", 250, nil, "") // нет в наличии
	e.linkPurpose(a, empty, "кожа")

	e.exec(`UPDATE products SET min_stock = 800 WHERE id = ?`, low)
	e.bot.CheckLow(e.ctx)
	n := notifications(e, 1001)
	if len(n) != 1 {
		t.Fatalf("уведомлений %d, ожидалось 1", len(n))
	}
	contains(t, n[0].Text, "Есть дома для того же: Крем для кожи (Chemical Guys)")
	if strings.Contains(n[0].Text, "Пустой") {
		t.Fatalf("в аналогах позиция без остатка: %q", n[0].Text)
	}
}
