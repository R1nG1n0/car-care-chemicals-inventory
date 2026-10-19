package report

import (
	"errors"
	"reflect"
	"testing"
)

func TestParsePeriod(t *testing.T) {
	bad := []struct {
		from, to string
		tz       int
		field    string
	}{
		{"", "2026-01-01", 0, "from"},
		{"2026-01-01", "", 0, "to"},
		{"2026-13-01", "2026-12-01", 0, "from"},
		{"2026-02-01", "2026-01-31", 0, "to"},
		{"2020-01-01", "2025-01-02", 0, "to"},
		{"2026-01-01", "2026-01-02", 841, "tz"},
		{"2026-01-01", "2026-01-02", -841, "tz"},
	}
	for _, c := range bad {
		_, err := ParsePeriod(c.from, c.to, c.tz)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != c.field {
			t.Errorf("%+v: ошибка %v, ожидалось поле %s", c, err, c.field)
		}
	}
	if _, err := ParsePeriod("2021-01-01", "2026-01-01", 180); err != nil {
		t.Fatalf("ровно 5 лет: %v", err)
	}
}

func TestPeriodMonthsAndBounds(t *testing.T) {
	p, _ := ParsePeriod("2025-11-15", "2026-02-03", 180)
	want := []string{"2025-11", "2025-12", "2026-01", "2026-02"}
	if got := p.months(); !reflect.DeepEqual(got, want) {
		t.Fatalf("месяцы %v", got)
	}
	one, _ := ParsePeriod("2026-10-14", "2026-10-14", 180)
	from, to := one.bounds()
	if to-from != 86400 {
		t.Fatalf("сутки: %d", to-from)
	}
	// 14.10 00:00 в МСК (UTC+3) = 13.10 21:00 UTC.
	if from != 1_791_925_200 {
		t.Fatalf("начало %d", from)
	}
}
