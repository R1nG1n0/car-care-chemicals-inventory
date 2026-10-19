package transfer

import "testing"

func TestCSVText(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"обычный":   "обычный",
		"=1+1":      "'=1+1",
		"+7":        "'+7",
		"-5":        "'-5",
		"@a":        "'@a",
		"\tтаб":     "'\tтаб",
		"\rвозврат": "'\rвозврат",
		"а=б":       "а=б",
		"'уже":      "'уже",
	} {
		if got := csvText(in); got != want {
			t.Errorf("csvText(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestFormats(t *testing.T) {
	if got := rubles(119050); got != "1190,50" {
		t.Errorf("rubles: %q", got)
	}
	if got := rubles(5); got != "0,05" {
		t.Errorf("rubles: %q", got)
	}
	if got := ruDate("2026-03-05"); got != "05.03.2026" {
		t.Errorf("ruDate: %q", got)
	}
	if got := ruDateTime(1_792_000_000, 180); got != "14.10.2026 20:46" {
		t.Errorf("ruDateTime: %q", got)
	}
}
