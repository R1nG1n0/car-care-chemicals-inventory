package bot

import (
	"strings"
	"testing"
)

// «шампунь 1000 1:10» — расход раствора (ФТ-17.2.5).
func TestSolutionConsumeAndUndo(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Шампунь Gsf", "", "ml", 500, nil, "")
	e.buy(a, p, 2, 119000, nil, nil)

	e.say(a, "шампунь 1000 1:10")
	m := e.fake.last()
	contains(t, m.Text, "Списано 91 мл", "Шампунь Gsf", "Раствор 1 000 мл 1:10 → 91 мл концентрата", "Остаток: 909 мл")
	if e.total(a, p) != 909 {
		t.Fatalf("остаток %d", e.total(a, p))
	}
	if n := e.count(`SELECT COUNT(*) FROM movements WHERE solution_ml = 1000 AND solution_ratio = 10 AND qty_delta = -91`); n != 1 {
		t.Fatalf("движение с раствором: %d", n)
	}
	e.click(a, m, "Отменить")
	if e.total(a, p) != 1000 {
		t.Fatalf("после отмены %d", e.total(a, p))
	}

	// Запись через косую черту, с пробелами и заглавными буквами.
	e.say(a, "Шампунь 500 1/1")
	contains(t, e.fake.last().Text, "Списано 250 мл", "Раствор 500 мл 1:1 → 250 мл концентрата")
	e.say(a, "шампунь 500 1 : 4")
	contains(t, e.fake.last().Text, "Списано 100 мл", "1:4")
}

func TestSolutionEdgeCases(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	ml := e.product(a, "Шампунь", "", "ml", 500, nil, "")
	pcs := e.product(a, "Салфетки", "", "pcs", 10, nil, "")
	e.buy(a, ml, 1, 1000, nil, nil)
	e.buy(a, pcs, 1, 500, nil, nil)

	// Позиция в шт — понятная ошибка, ничего не списано.
	e.say(a, "салфетки 100 1:10")
	contains(t, e.fake.last().Text, "мл")
	if e.total(a, pcs) != 10 {
		t.Fatal("списание шт раствором")
	}
	// Пропорция вне диапазона и слишком большой объём.
	e.say(a, "шампунь 100 1:5000")
	if e.total(a, ml) != 500 {
		t.Fatal("списано при пропорции 1:5000")
	}
	e.say(a, "шампунь 999999 1:10")
	contains(t, e.fake.last().Text, "100")
	if e.total(a, ml) != 500 {
		t.Fatal("списано при слишком большом объёме")
	}
	// Прежний сценарий не сломан.
	e.say(a, "шампунь 50")
	contains(t, e.fake.last().Text, "Списано 50 мл")
	if got := e.fake.last().Text; strings.Contains(got, "Раствор") {
		t.Fatalf("у обычного расхода есть раствор: %q", got)
	}
}

func TestSolutionSeveralMatches(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p1 := e.product(a, "Шампунь Gsf", "", "ml", 500, nil, "")
	p2 := e.product(a, "Шампунь Soft99", "", "ml", 500, nil, "")
	e.buy(a, p1, 1, 1000, nil, nil)
	e.buy(a, p2, 1, 1000, nil, nil)

	e.say(a, "шампунь 1000 1:10")
	m := e.fake.last()
	contains(t, m.Text, "несколько")
	if e.total(a, p1) != 500 || e.total(a, p2) != 500 {
		t.Fatal("до выбора ничего не списывается")
	}
	e.click(a, m, "Soft99")
	if e.total(a, p1) != 500 || e.total(a, p2) != 409 {
		t.Fatalf("остатки: %d, %d", e.total(a, p1), e.total(a, p2))
	}
	contains(t, e.fake.last().Text, "Раствор 1 000 мл 1:10 → 91 мл концентрата")
}

// Название позиции с цифрами и дробью остаётся поиском, а не расходом.
func TestSolutionNotConsumeWhenWholePhraseIsProduct(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	p := e.product(a, "Мойка 100 1:10", "", "ml", 500, nil, "")
	e.buy(a, p, 1, 1000, nil, nil)
	e.say(a, "мойка 100 1:10")
	if e.total(a, p) != 500 {
		t.Fatal("списание по ошибке")
	}
}
