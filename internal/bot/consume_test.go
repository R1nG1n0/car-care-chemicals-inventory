package bot

import (
	"testing"
)

func TestQuickConsumeAndUndo(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь Gsf", "", "ml", 500, nil, "")
	e.buy(a, p, 2, 119000, nil, nil)

	e.say(a, "шампунь 50")
	m := e.fake.last()
	contains(t, m.Text, "Списано 50 мл", "Шампунь Gsf", "Остаток: 950 мл", "Себестоимость: 119 ₽")
	if e.total(a, p) != 950 {
		t.Fatalf("остаток %d", e.total(a, p))
	}
	e.click(a, m, "Отменить")
	if e.total(a, p) != 1000 {
		t.Fatalf("после отмены остаток %d", e.total(a, p))
	}
	contains(t, e.fake.msgs[m.ID].Text, "Расход отменён", "1 000 мл")
	if len(e.fake.msgs[m.ID].Kb) != 0 {
		t.Fatal("после отмены кнопки должны исчезнуть")
	}
	// Движение действительно удалено из журнала, а экземпляр снова запечатан.
	if n := e.count(`SELECT COUNT(*) FROM movements WHERE type = 'consume'`); n != 0 {
		t.Fatalf("движений расхода: %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM units WHERE status = 'sealed'`); n != 2 {
		t.Fatalf("запечатанных: %d", n)
	}
	// Повторное нажатие безвредно.
	e.clickData(a, m, "u:"+itoa(0))
	if e.total(a, p) != 1000 {
		t.Fatal("повторная отмена изменила остаток")
	}
}

func TestConsumeButtonsAndPieces(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	ml := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	pcs := e.product(a, "Салфетки", "", "pcs", 10, nil, "")
	e.buy(a, ml, 1, 1000, nil, nil)
	e.buy(a, pcs, 1, 500, nil, nil)

	e.say(a, "шампунь")
	e.click(a, e.fake.last(), "−100")
	contains(t, e.fake.last().Text, "Списано 100 мл", "Остаток: 400 мл")

	e.say(a, "салфетки")
	card := e.fake.last()
	if _, ok := card.button("−50"); ok {
		t.Fatal("у штук нет шага −50")
	}
	e.click(a, card, "−5")
	contains(t, e.fake.last().Text, "Списано 5 шт", "Остаток: 5 шт", "Себестоимость: 2,50 ₽")
	// Те же кнопки работают повторно.
	e.click(a, card, "−1")
	contains(t, e.fake.last().Text, "Остаток: 4 шт")
}

func TestConsumeShortage(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.say(a, "шампунь 600")
	contains(t, e.fake.last().Text, "Не хватает", "500 мл", "Ничего не списано")
	if e.total(a, p) != 500 {
		t.Fatal("при нехватке ничего не должно списываться")
	}
	// Слишком большое число для сервиса — понятная ошибка, а не падение.
	e.say(a, "шампунь 999999")
	contains(t, e.fake.last().Text, "Количество")
}

func TestConsumeSeveralMatches(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p1 := e.product(a, "Шампунь Gsf", "", "ml", 500, nil, "")
	p2 := e.product(a, "Шампунь Soft99", "", "ml", 500, nil, "")
	e.buy(a, p1, 1, 1000, nil, nil)
	e.buy(a, p2, 1, 1000, nil, nil)

	e.say(a, "шампунь 50")
	m := e.fake.last()
	contains(t, m.Text, "несколько")
	if e.total(a, p1) != 500 || e.total(a, p2) != 500 {
		t.Fatal("до выбора ничего не списывается")
	}
	soft, _ := m.button("Soft99")
	e.click(a, m, "Soft99")
	if e.total(a, p1) != 500 || e.total(a, p2) != 450 {
		t.Fatalf("остатки: %d, %d", e.total(a, p1), e.total(a, p2))
	}
	if len(e.fake.msgs[m.ID].Kb) != 0 {
		t.Fatal("после выбора клавиатура должна исчезнуть")
	}
	// Повторное нажатие по старой кнопке выбора не списывает ещё раз.
	e.clickData(a, m, soft.Data)
	if e.total(a, p2) != 450 {
		t.Fatalf("двойное списание: %d", e.total(a, p2))
	}
}

// «Мойка 2 в 1»: конец фразы — число, но позиции «Мойка 2 в» нет — это обычный поиск.
func TestTrailingNumberFallsBackToSearch(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Мойка 2 в 1", "", "ml", 500, nil, "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.say(a, "мойка 2 в 1")
	contains(t, e.fake.last().Text, "Мойка 2 в 1", "Остаток: 500 мл")
	if e.total(a, p) != 500 {
		t.Fatal("списание случилось по ошибке")
	}
}

// Расход на сайте и в боте — один и тот же сервис: правила ФТ-5.1 соблюдаются.
func TestConsumeUsesOpenUnitFirst(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	e.buy(a, p, 2, 1000, nil, nil)
	e.say(a, "шампунь 100")
	e.say(a, "шампунь 100")
	if n := e.count(`SELECT COUNT(*) FROM units WHERE status = 'open' AND remaining_qty = 300`); n != 1 {
		t.Fatal("оба расхода должны идти из одного вскрытого экземпляра")
	}
	if n := e.count(`SELECT COUNT(*) FROM units WHERE status = 'sealed'`); n != 1 {
		t.Fatal("второй экземпляр должен остаться запечатанным")
	}
}

// Отмена, если id движения освободился и достался другой записи, не срабатывает.
func TestUndoRefusedWhenMovementChanged(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	e.buy(a, p, 2, 1000, nil, nil)
	e.say(a, "шампунь 50")
	m := e.fake.last()
	var mid, unit int64
	if err := e.db.QueryRow(`SELECT id, unit_id FROM movements WHERE type = 'consume'`).Scan(&mid, &unit); err != nil {
		t.Fatal(err)
	}
	// «Отмена на сайте» и новое движение, занявшее тот же id у другого экземпляра.
	e.exec(`DELETE FROM movements WHERE id = ?`, mid)
	e.exec(`UPDATE units SET remaining_qty = 500, status = 'sealed', opened_at = NULL WHERE id = ?`, unit)
	e.exec(`INSERT INTO movements (id, user_id, unit_id, type, qty_delta) SELECT ?, user_id, id, 'consume', -10 FROM units WHERE id <> ? LIMIT 1`, mid, unit)
	before := e.count(`SELECT COUNT(*) FROM movements`)
	e.click(a, m, "Отменить")
	contains(t, e.fake.last().Text, "уже отменено или изменено")
	if e.count(`SELECT COUNT(*) FROM movements`) != before {
		t.Fatal("чужое движение удалено")
	}
}
