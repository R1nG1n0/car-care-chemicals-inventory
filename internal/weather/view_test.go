package weather

import (
	"testing"
	"time"
)

func TestBuildFixture(t *testing.T) {
	f, err := ParseForecast(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	v := Build(f, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)) // 15:00 в Москве
	if len(v.Days) != 7 || v.Days[0].Date != "2026-10-30" || v.Days[0].Weekday != "пт" || v.Days[6].Weekday != "чт" {
		t.Fatalf("дни: %+v", v.Days)
	}
	if len(v.TodayHours) != 24 || v.TodayHours[15].Time != "15:00" || v.TodayHours[15].Temp != 15.4 {
		t.Fatalf("часы: %d", len(v.TodayHours))
	}
	if v.Days[1].PrecipMM != 2.7 || v.Days[1].Wash.Level != Bad {
		t.Fatalf("второй день: %+v", v.Days[1])
	}
	if v.Best.Wash == nil || *v.Best.Wash != "2026-11-01" || v.Best.Coat == nil || *v.Best.Coat != "2026-11-01" {
		t.Fatalf("лучшие дни: wash=%v coat=%v", v.Best.Wash, v.Best.Coat)
	}
}

func TestBuildDropsPastDays(t *testing.T) {
	f, _ := ParseForecast(fixture(t))
	v := Build(f, time.Date(2026, 11, 2, 9, 0, 0, 0, msk))
	if v.Days[0].Date != "2026-11-02" || len(v.Days) != 4 {
		t.Fatalf("дни: %d, первый %s", len(v.Days), v.Days[0].Date)
	}
	if len(v.TodayHours) != 24 || v.TodayHours[0].Time != "00:00" {
		t.Fatalf("часы: %d", len(v.TodayHours))
	}
	// Прогноз устарел больше недели: показывать нечего, но структура цела.
	v = Build(f, time.Date(2026, 12, 1, 9, 0, 0, 0, msk))
	if v.Days == nil || v.TodayHours == nil || len(v.Days) != 0 {
		t.Fatalf("пустой прогноз: %+v", v)
	}
}

func TestBuildLimitsDays(t *testing.T) {
	f := Forecast{Loc: msk}
	for i := 0; i < 9; i++ {
		f.Days = append(f.Days, Day{Date: time.Date(2026, 11, 10+i, 0, 0, 0, 0, msk).Format("2006-01-02")})
	}
	f.Hours = synth(nil)
	if v := Build(f, time.Date(2026, 11, 10, 7, 0, 0, 0, msk)); len(v.Days) != MaxDays {
		t.Fatalf("дней: %d", len(v.Days))
	}
}
