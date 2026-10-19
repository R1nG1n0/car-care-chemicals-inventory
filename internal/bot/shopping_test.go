package bot

import (
	"testing"
	"time"
)

func TestListLowAndManual(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	shop := e.exec(`INSERT INTO stores (user_id, name) VALUES (?, 'Автохим')`, a.id)
	low := e.product(a, "Шампунь", "Gyeon", "ml", 500, ptr(int64(800)), "")
	e.buy(a, low, 1, 119000, &shop, nil)
	ok := e.product(a, "Воск", "", "ml", 300, ptr(int64(100)), "")
	e.buy(a, ok, 1, 50000, nil, nil)
	manual := e.exec(`INSERT INTO shopping_items (user_id, title, created_at) VALUES (?, 'Глина для кузова', 1)`, a.id)

	e.say(a, "/list")
	m := e.fake.last()
	contains(t, m.Text, "Ниже минимума", "Шампунь (Gyeon)", "500 мл из 800 мл", "1 190 ₽ · Автохим", "Ещё в списке", "Глина для кузова")
	if containsAny(m.Text, "Воск") {
		t.Fatal("позиция выше минимума не должна быть в списке")
	}
	// Кнопка «Купить» из текста тоже открывает список.
	e.say(a, "Купить")
	contains(t, e.fake.last().Text, "Ниже минимума")

	// Ручной пункт: «Куплено» и «Вернуть».
	e.click(a, m, "Куплено: Глина")
	m = *e.fake.msgs[m.ID]
	if containsAny(m.Text, "Ещё в списке") {
		t.Fatalf("пункт должен исчезнуть из списка: %s", m.Text)
	}
	if e.count(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NOT NULL`, manual) != 1 {
		t.Fatal("пункт не вычеркнут")
	}
	e.click(a, m, "Вернуть")
	contains(t, e.fake.msgs[m.ID].Text, "Глина для кузова")
	if e.count(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NULL`, manual) != 1 {
		t.Fatal("пункт не возвращён")
	}
}

func TestListEmpty(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.say(a, "/list")
	contains(t, e.fake.last().Text, "нечего")
}

// Покупка по списку: упаковки → подтверждение последней цены → отмена.
func TestBuyLowItemAndUndo(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	shop := e.exec(`INSERT INTO stores (user_id, name) VALUES (?, 'Автохим')`, a.id)
	shelf := e.exec(`INSERT INTO locations (user_id, name) VALUES (?, 'Полка')`, a.id)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(800)), "")
	e.buy(a, p, 1, 119000, &shop, &shelf)
	// 22:30 UTC 19 октября — в Москве уже 20-е.
	e.clock.Set(time.Date(2026, 10, 19, 22, 30, 0, 0, time.UTC))

	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено: Шампунь")
	m := *e.fake.msgs[list.ID]
	contains(t, m.Text, "Сколько упаковок", "500 мл")
	e.click(a, m, "2")
	m = *e.fake.msgs[list.ID]
	contains(t, m.Text, "Цена: 1 190 ₽", "Автохим")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 1 {
		t.Fatal("до подтверждения покупки быть не должно")
	}
	confirm, _ := m.button("Подтвердить")
	confirmData := confirm.Data
	e.click(a, m, "Подтвердить")
	m = *e.fake.msgs[list.ID]
	contains(t, m.Text, "Куплено", "× 2", "2 380 ₽", "Остаток теперь: 1 500 мл")

	var date string
	var store, loc int64
	if err := e.db.QueryRow(`SELECT date, store_id FROM purchases ORDER BY id DESC LIMIT 1`).Scan(&date, &store); err != nil {
		t.Fatal(err)
	}
	if date != "2026-10-20" || store != shop {
		t.Fatalf("дата %s (ожидалась московская 2026-10-20), магазин %d", date, store)
	}
	if err := e.db.QueryRow(`SELECT location_id FROM units ORDER BY id DESC LIMIT 1`).Scan(&loc); err != nil || loc != shelf {
		t.Fatalf("место %d, ошибка %v", loc, err)
	}
	if e.count(`SELECT price_per_package FROM purchase_lines ORDER BY id DESC LIMIT 1`) != 119000 {
		t.Fatal("цена за упаковку")
	}
	// Повторное «Подтвердить» (двойное нажатие) не создаёт вторую покупку и
	// не затирает итог.
	e.clickData(a, m, confirmData)
	if e.count(`SELECT COUNT(*) FROM purchases`) != 2 || e.fake.msgs[list.ID].Text != m.Text {
		t.Fatal("повторное нажатие изменило данные или сообщение")
	}

	e.click(a, m, "Отменить")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 1 || e.total(a, p) != 500 {
		t.Fatalf("покупка не отменена: покупок %d, остаток %d", e.count(`SELECT COUNT(*) FROM purchases`), e.total(a, p))
	}
	contains(t, e.fake.msgs[list.ID].Text, "Покупка отменена")
}

// startOtherPrice доводит диалог до «Другая цена» и возвращает сообщение диалога.
func startOtherPrice(e *env, a user) sentMsg {
	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено")
	e.click(a, *e.fake.msgs[list.ID], "1")
	e.click(a, *e.fake.msgs[list.ID], "Другая цена")
	return list
}

func TestBuyOtherPriceByText(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(800)), "")
	e.buy(a, p, 1, 119000, nil, nil)

	list := startOtherPrice(e, a)
	contains(t, e.fake.msgs[list.ID].Text, "в рублях")
	e.say(a, "1 234,50")
	contains(t, e.fake.last().Text, "Куплено", "1 234,50 ₽")
	if e.count(`SELECT price_per_package FROM purchase_lines ORDER BY id DESC LIMIT 1`) != 123450 {
		t.Fatal("цена должна быть 123450 копеек")
	}
	// Следующее сообщение — уже обычный поиск, а не цена.
	e.say(a, "шампунь")
	contains(t, e.fake.last().Text, "Остаток")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 2 {
		t.Fatal("лишняя покупка")
	}
}

// Не цена — обычный разбор, ожидание снимается (ФТ-11.6.3).
func TestPriceWaitingNonPriceFallsThrough(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(800)), "")
	e.buy(a, p, 1, 119000, nil, nil)
	startOtherPrice(e, a)
	e.say(a, "шампунь")
	contains(t, e.fake.last().Text, "Остаток")
	e.say(a, "300")
	contains(t, e.fake.last().Text, "Ничего не нашлось")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 1 {
		t.Fatal("после снятия ожидания число не должно стать ценой")
	}
}

// Ожидание цены живёт 10 минут.
func TestPriceWaitingExpires(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(800)), "")
	e.buy(a, p, 1, 119000, nil, nil)
	startOtherPrice(e, a)
	e.clock.Set(e.clock.Now().Add(11 * time.Minute))
	e.say(a, "300")
	contains(t, e.fake.last().Text, "Ничего не нашлось")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 1 {
		t.Fatal("просроченное ожидание приняло цену")
	}
}

func TestBuyWithoutHistoryAsksPrice(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Новая", "", "ml", 500, ptr(int64(800)), "")

	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено")
	e.click(a, *e.fake.msgs[list.ID], "3")
	contains(t, e.fake.msgs[list.ID].Text, "Прошлых покупок нет", "цену")
	e.say(a, "300")
	contains(t, e.fake.last().Text, "Куплено", "× 3", "900 ₽")
	if e.total(a, p) != 1500 {
		t.Fatalf("остаток %d", e.total(a, p))
	}
	// Без прошлых покупок магазина и места нет.
	if e.count(`SELECT COUNT(*) FROM purchases WHERE store_id IS NULL`) != 1 {
		t.Fatal("магазин должен быть пуст")
	}
}

func TestUndoPurchaseAfterUseRefused(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(2000)), "")
	e.buy(a, p, 1, 1000, nil, nil)

	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено")
	e.click(a, *e.fake.msgs[list.ID], "1")
	e.click(a, *e.fake.msgs[list.ID], "Подтвердить")
	buyMsg := *e.fake.msgs[list.ID]

	e.say(a, "шампунь 600") // тронули оба экземпляра
	e.click(a, buyMsg, "Отменить")
	contains(t, e.fake.last().Text, "Экземпляры покупки уже использованы — отменить нельзя")
	if e.count(`SELECT COUNT(*) FROM purchases`) != 2 {
		t.Fatal("покупка не должна удаляться")
	}
}

func TestPriceTooBig(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.product(a, "Новая", "", "ml", 500, ptr(int64(800)), "")
	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено")
	e.click(a, *e.fake.msgs[list.ID], "1")
	e.say(a, "2000000")
	contains(t, e.fake.last().Text, "не больше 1 000 000")
	e.say(a, "1000")
	contains(t, e.fake.last().Text, "Куплено")
}

func TestParseRubles(t *testing.T) {
	ok := map[string]int64{"2450": 245000, "2 450": 245000, "2450,5": 245050, "2450.50 ₽": 245050,
		"1 234,05": 123405, "300 руб": 30000, "0": 0, "12,": 1200}
	for in, want := range ok {
		if got, good := parseRubles(in); !good || got != want {
			t.Errorf("%q → %d %v, ожидалось %d", in, got, good, want)
		}
	}
	for _, in := range []string{"", "abc", "1,234", "-5", "12 abc", "1.2.3"} {
		if _, good := parseRubles(in); good {
			t.Errorf("%q не должно разбираться", in)
		}
	}
}

func TestMoneyFormat(t *testing.T) {
	cases := map[int64]string{0: "0 ₽", 100: "1 ₽", 123450: "1 234,50 ₽", 119000: "1 190 ₽", 5: "0,05 ₽", 100000000: "1 000 000 ₽"}
	for in, want := range cases {
		got := money(in)
		if got != want && got != replaceNBSP(want) {
			t.Errorf("money(%d) = %q, ожидалось %q", in, got, want)
		}
	}
}

func replaceNBSP(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r == ' ' {
			out[i] = ' '
		}
	}
	return string(out)
}
