package bot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

const nbsp = " "

// group разбивает число на разряды неразрывными пробелами: 1250 → «1 250».
func group(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(nbsp)
		}
		b.WriteRune(r)
	}
	if neg {
		return "−" + b.String()
	}
	return b.String()
}

// money — копейки в виде «1 234,50 ₽» (целые рубли без копеек), как на сайте.
func money(kopecks int64) string {
	s := group(kopecks / 100)
	if k := kopecks % 100; k != 0 {
		s += fmt.Sprintf(",%02d", k)
	}
	return s + nbsp + "₽"
}

// qty — количество с единицей: «450 мл», «3 шт».
func qty(unit string, n int64) string {
	if unit == "pcs" {
		return group(n) + nbsp + "шт"
	}
	return group(n) + nbsp + "мл"
}

var rublesRe = regexp.MustCompile(`^(\d+)(?:[.,](\d{0,2}))?$`)

// parseRubles разбирает «2 450», «2450,5», «2450.50 ₽» в копейки (как
// parseRubles на сайте). ok=false — не число.
func parseRubles(input string) (int64, bool) {
	s := strings.Join(strings.Fields(input), "")
	s = strings.TrimSuffix(s, "₽")
	s = strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(s), "руб."), "руб")
	m := rublesRe.FindStringSubmatch(s)
	if m == nil || len(m[1]) > 9 {
		return 0, false
	}
	rub, _ := strconv.ParseInt(m[1], 10, 64)
	frac := m[2]
	for len(frac) < 2 {
		frac += "0"
	}
	kop, _ := strconv.ParseInt(frac, 10, 64)
	return rub*100 + kop, true
}

// bold — жирный текст, содержимое экранируется.
func bold(s string) string { return "<b>" + telegram.Esc(s) + "</b>" }

// label — подпись на кнопке: не длиннее n символов.
func label(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// productTitle — «Название (бренд)».
func productTitle(name, brand string) string {
	if brand == "" {
		return name
	}
	return name + " (" + brand + ")"
}

// plural выбирает форму слова: 1 позиция, 2 позиции, 5 позиций.
func plural(n int, one, few, many string) string {
	n100 := n % 100
	if n100 >= 11 && n100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
