package bot

import (
	"strings"
	"testing"
)

// ТБ-5 для бота: чужие id в кнопках и ключи ожидающих действий ничего не дают.
func TestIsolationBetweenUsers(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	b := e.linked("friend", 2002)
	pa := e.product(a, "Секретный шампунь", "Тайнобренд", "ml", 500, ptr(int64(900)), "4600051000057")
	e.buy(a, pa, 1, 1000, nil, nil)
	pb := e.product(b, "Мой шампунь", "", "ml", 500, nil, "")
	e.buy(b, pb, 1, 1000, nil, nil)
	itemA := e.exec(`INSERT INTO shopping_items (user_id, title, created_at) VALUES (?, 'Тайная глина', 1)`, a.id)

	// Поиск, штрихкод и список второго не показывают чужое.
	for _, q := range []string{"секретный", "тайнобренд", "4600051000057", "/list", "шампунь"} {
		e.say(b, q)
		got := e.fake.last().Text
		if strings.Contains(got, "Секретный") || strings.Contains(got, "Тайная") || strings.Contains(got, "Тайнобренд") {
			t.Fatalf("запрос %q: второй видит чужое: %s", q, got)
		}
	}

	// Чужие id в callback_data: расход, карточка, упаковки, пункт списка.
	card := e.fake.last()
	for _, data := range []string{
		"c:" + itoa(pa) + ":50", "p:" + itoa(pa), "b:" + itoa(pa), "n:" + itoa(pa) + ":1", "m:" + itoa(itemA), "r:" + itoa(itemA),
	} {
		before := e.fake.count()
		e.clickData(b, card, data)
		for _, m := range e.fake.sent()[before:] {
			if strings.Contains(m.Text, "Секретный") || strings.Contains(m.Text, "Тайная") {
				t.Fatalf("callback %q раскрыл чужое: %s", data, m.Text)
			}
		}
	}
	if e.total(a, pa) != 500 {
		t.Fatalf("остаток первого изменился: %d", e.total(a, pa))
	}
	if e.count(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NULL`, itemA) != 1 {
		t.Fatal("пункт первого изменён вторым")
	}

	// Ключ отмены первого, использованный вторым.
	e.say(a, "секретный 100")
	undo := e.fake.last()
	btn, _ := undo.button("Отменить")
	e.clickData(b, undo, btn.Data)
	if e.total(a, pa) != 400 {
		t.Fatal("второй отменил расход первого")
	}
	e.clickData(a, undo, btn.Data)
	if e.total(a, pa) != 500 {
		t.Fatal("владелец должен отменить свой расход")
	}

	// Ключ отмены покупки первого.
	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено: Секретный")
	e.click(a, *e.fake.msgs[list.ID], "1")
	e.click(a, *e.fake.msgs[list.ID], "Подтвердить")
	buyMsg := *e.fake.msgs[list.ID]
	d, ok := buyMsg.button("Отменить")
	if !ok {
		t.Fatal("нет кнопки отмены покупки")
	}
	e.clickData(b, buyMsg, d.Data)
	if e.count(`SELECT COUNT(*) FROM purchases WHERE user_id = ?`, a.id) != 2 {
		t.Fatal("второй отменил покупку первого")
	}
}

// Цена, введённая текстом, относится только к черновику того же пользователя.
func TestPriceInputBelongsToChat(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	b := e.linked("friend", 2002)
	pa := e.product(a, "Шампунь", "", "ml", 500, ptr(int64(900)), "")
	e.buy(a, pa, 1, 1000, nil, nil)
	e.say(a, "/list")
	list := e.fake.last()
	e.click(a, list, "Куплено")
	e.click(a, *e.fake.msgs[list.ID], "1")
	e.click(a, *e.fake.msgs[list.ID], "Другая цена")
	// Второй пишет число — это не цена первого.
	e.say(b, "500")
	if e.count(`SELECT COUNT(*) FROM purchases WHERE user_id = ?`, a.id) != 1 {
		t.Fatal("сообщение второго стало ценой первого")
	}
}
