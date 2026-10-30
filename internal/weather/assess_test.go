package weather

import (
	"reflect"
	"testing"
	"time"
)

var msk = func() *time.Location {
	l, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		panic(err)
	}
	return l
}()

// synth строит 72 часа с 00:00 дня 2026-11-10 (вторник): ясно, +15°, ветер 3 м/с,
// влажность 50 %, осадков нет. tweak меняет отдельные часы.
func synth(tweak func(h *Hour)) []Hour {
	start := time.Date(2026, 11, 10, 0, 0, 0, 0, msk)
	hours := make([]Hour, 72)
	for i := range hours {
		hours[i] = Hour{Time: start.Add(time.Duration(i) * time.Hour), Temp: 15, Wind: 3, Humidity: 50}
		if tweak != nil {
			tweak(&hours[i])
		}
	}
	return hours
}

// at: час с номером i от начала (0 — сегодня 00:00, 24 — завтра 00:00).
func at(i int, f func(h *Hour)) func(h *Hour) {
	return func(h *Hour) {
		if int(h.Time.Sub(time.Date(2026, 11, 10, 0, 0, 0, 0, msk))/time.Hour) == i {
			f(h)
		}
	}
}

func all(fs ...func(h *Hour)) func(h *Hour) {
	return func(h *Hour) {
		for _, f := range fs {
			f(h)
		}
	}
}

func TestAssess(t *testing.T) {
	morning := time.Date(2026, 11, 10, 7, 0, 0, 0, msk)
	cases := []struct {
		name  string
		hours []Hour
		now   time.Time
		wash  Rating
		coat  Rating
	}{
		{"ясный тёплый день", synth(nil), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"дождь днём", synth(at(15, func(h *Hour) { h.Precip = 0.4 })), morning,
			Rating{Bad, []string{"дождь в 15:00"}}, Rating{Bad, []string{"дождь в 15:00"}}},
		{"мороз", synth(func(h *Hour) { h.Temp = -3 }), morning,
			Rating{Bad, []string{"мороз −3°"}}, Rating{Bad, []string{"мороз −3° весь день"}}},
		{"для мойки +2° уже мало, для покрытия +3° тоже", synth(func(h *Hour) { h.Temp = 2 }), morning,
			Rating{Bad, []string{"холодно +2°"}}, Rating{Bad, []string{"холодно +2° весь день"}}},
		{"+3° — мыть можно, покрытие нельзя", synth(func(h *Hour) { h.Temp = 3 }), morning,
			Rating{Good, []string{}}, Rating{Bad, []string{"холодно +3° весь день"}}},
		{"+5° и +30° — граница покрытия включена", synth(at(10, func(h *Hour) { h.Temp = 5 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"жара в середине дня — окно утром есть", synth(at(14, func(h *Hour) { h.Temp = 33 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"жара весь день", synth(func(h *Hour) { h.Temp = 33 }), morning,
			Rating{Good, []string{}}, Rating{Bad, []string{"жара +33° весь день"}}},
		{"ветер 12 м/с", synth(at(13, func(h *Hour) { h.Wind = 12 })), morning,
			Rating{Bad, []string{"ветер 12 м/с"}}, Rating{Good, []string{}}},
		{"ветер 9,9 м/с — ещё можно", synth(at(13, func(h *Hour) { h.Wind = 9.9 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"завтра дождь в 12:00", synth(at(36, func(h *Hour) { h.Precip = 1 })), morning,
			Rating{Bad, []string{"дождь завтра в 12:00"}}, Rating{Bad, []string{"дождь завтра в 12:00"}}},
		{"вечером дождь в 22:00 — за окном, но в сутках после мойки", synth(at(22, func(h *Hour) { h.Precip = 1 })), morning,
			Rating{Bad, []string{"дождь в 22:00"}}, Rating{Bad, []string{"дождь в 22:00"}}},
		{"дождь завтра в 21:00 не портит мойку (сутки после окна), но портит покрытие (48 ч)", synth(at(45, func(h *Hour) { h.Precip = 1 })), morning,
			Rating{Good, []string{}}, Rating{Bad, []string{"дождь завтра в 21:00"}}},
		{"дождь послезавтра в 05:00 портит покрытие", synth(at(53, func(h *Hour) { h.Precip = 1 })), morning,
			Rating{Good, []string{}}, Rating{Bad, []string{"дождь послезавтра в 05:00"}}},
		{"дождь через 49 ч покрытию не мешает", synth(at(57, func(h *Hour) { h.Precip = 1 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"снег по коду погоды", synth(at(15, func(h *Hour) { h.Precip = 0.5; h.Code = 73 })), morning,
			Rating{Bad, []string{"снег в 15:00"}}, Rating{Bad, []string{"снег в 15:00"}}},
		{"вероятность осадков 40 % — так себе", synth(at(11, func(h *Hour) { h.Prob = 40 })), morning,
			Rating{Meh, []string{"вероятность осадков 40 %"}}, Rating{Meh, []string{"вероятность осадков 40 %"}}},
		{"вероятность 30 % — уже так себе", synth(at(11, func(h *Hour) { h.Prob = 30 })), morning,
			Rating{Meh, []string{"вероятность осадков 30 %"}}, Rating{Meh, []string{"вероятность осадков 30 %"}}},
		{"вероятность 29 % — хорошо", synth(at(11, func(h *Hour) { h.Prob = 29 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"вероятность 70 % — как дождь", synth(at(11, func(h *Hour) { h.Prob = 70 })), morning,
			Rating{Bad, []string{"дождь в 11:00"}}, Rating{Bad, []string{"дождь в 11:00"}}},
		{"влажность 85 % весь день — покрытие так себе", synth(func(h *Hour) { h.Humidity = 85 }), morning,
			Rating{Good, []string{}}, Rating{Meh, []string{"влажность 85 %"}}},
		{"влажность 92 % весь день — покрытие плохо", synth(func(h *Hour) { h.Humidity = 92 }), morning,
			Rating{Good, []string{}}, Rating{Bad, []string{"влажность весь день выше 90 %"}}},
		{"утренний туман, днём сухо — окно есть", synth(func(h *Hour) {
			if h.Time.Hour() < 12 {
				h.Humidity = 95
			}
		}), morning, Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"окно ровно 3 часа (12–14)", synth(func(h *Hour) {
			if h.Time.Hour() < 12 || h.Time.Hour() > 14 {
				h.Humidity = 95
			}
		}), morning, Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"сухих часов только по два подряд — окна нет", synth(func(h *Hour) {
			if hr := h.Time.Hour(); hr != 8 && hr != 9 && hr != 12 && hr != 13 {
				h.Humidity = 95
			}
		}), morning, Rating{Good, []string{}}, Rating{Bad, []string{"нет окна на 3 часа подряд"}}},
		{"окно из влажных 80–90 % часов лучше, чем ничего", synth(func(h *Hour) {
			if h.Time.Hour() < 15 {
				h.Humidity = 95
			} else {
				h.Humidity = 84
			}
		}), morning, Rating{Good, []string{}}, Rating{Meh, []string{"влажность 84 %"}}},
		{"тепло только утром (3 часа) — окно есть", synth(func(h *Hour) {
			if h.Time.Hour() > 10 {
				h.Temp = 3
			}
		}), morning, Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"тепло только 2 часа — окна нет", synth(func(h *Hour) {
			if h.Time.Hour() > 9 {
				h.Temp = 3
			}
		}), morning, Rating{Good, []string{}}, Rating{Bad, []string{"нет окна на 3 часа подряд"}}},
		{"влажность ночью не считается", synth(at(3, func(h *Hour) { h.Humidity = 99 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"дождь ночью (до 8:00) не мешает", synth(at(5, func(h *Hour) { h.Precip = 2 })), morning,
			Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"несколько причин: мороз, ветер и дождь", synth(all(
			func(h *Hour) { h.Temp = -3 }, at(10, func(h *Hour) { h.Wind = 11 }), at(15, func(h *Hour) { h.Precip = 1 }))), morning,
			Rating{Bad, []string{"дождь в 15:00", "мороз −3°", "ветер 11 м/с"}}, Rating{Bad, []string{"мороз −3° весь день", "дождь в 15:00"}}},
		{"мороз около нуля не «мороз 0°»", synth(func(h *Hour) { h.Temp = -0.3 }), morning,
			Rating{Bad, []string{"холодно 0°"}}, Rating{Bad, []string{"холодно 0° весь день"}}},
		{"сегодня после обеда: утренний дождь уже не учитывается", synth(at(9, func(h *Hour) { h.Precip = 3 })),
			time.Date(2026, 11, 10, 14, 30, 0, 0, msk), Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"осадки неизвестны днём — мягкая причина", synth(at(11, func(h *Hour) { h.NoPrecip = true })), morning,
			Rating{Meh, []string{"нет данных об осадках"}}, Rating{Meh, []string{"нет данных об осадках"}}},
		{"осадки неизвестны через 25 ч — мойка хороша, покрытие так себе", synth(at(45, func(h *Hour) { h.NoPrecip = true })), morning,
			Rating{Good, []string{}}, Rating{Meh, []string{"нет данных об осадках"}}},
		{"неизвестные осадки не мешают плохой оценке", synth(all(
			at(11, func(h *Hour) { h.NoPrecip = true }), at(15, func(h *Hour) { h.Precip = 1 }))), morning,
			Rating{Bad, []string{"дождь в 15:00"}}, Rating{Bad, []string{"дождь в 15:00"}}},
		{"сегодня после обеда: окно только среди оставшихся часов", synth(func(h *Hour) {
			if h.Time.Hour() >= 11 {
				h.Humidity = 95
			}
		}), time.Date(2026, 11, 10, 15, 30, 0, 0, msk), Rating{Good, []string{}}, Rating{Bad, []string{"влажность весь день выше 90 %"}}},
		{"сегодня вечером светлое время закончилось", synth(nil), time.Date(2026, 11, 10, 21, 0, 0, 0, msk),
			Rating{Bad, []string{"светлое время уже закончилось"}}, Rating{Bad, []string{"светлое время уже закончилось"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Assess(tc.hours, "2026-11-10", tc.now)
			if !reflect.DeepEqual(got.Wash, tc.wash) {
				t.Errorf("мойка: %+v, ожидалось %+v", got.Wash, tc.wash)
			}
			if !reflect.DeepEqual(got.Coat, tc.coat) {
				t.Errorf("покрытие: %+v, ожидалось %+v", got.Coat, tc.coat)
			}
		})
	}
}

func TestAssessNoData(t *testing.T) {
	got := Assess(synth(nil), "2026-11-20", time.Date(2026, 11, 10, 7, 0, 0, 0, msk))
	if got.Wash.Level != Bad || got.Wash.Reasons[0] != "нет данных о погоде" {
		t.Fatalf("оценка дня без данных: %+v", got)
	}
}

func TestTemp(t *testing.T) {
	for in, want := range map[float64]string{-3: "−3°", -2.6: "−3°", 0: "0°", 0.4: "0°", 12.5: "+13°", 30: "+30°"} {
		if got := Temp(in); got != want {
			t.Errorf("Temp(%v) = %q, ожидалось %q", in, got, want)
		}
	}
}

// Край прогноза: окно «после» не помещается в данные — не выше «так себе».
func TestAssessForecastEdge(t *testing.T) {
	// synth кончается 13.11 в 00:00.
	morning := time.Date(2026, 11, 10, 7, 0, 0, 0, msk)
	cases := []struct {
		date       string
		wash, coat Rating
	}{
		{"2026-11-10", Rating{Good, []string{}}, Rating{Good, []string{}}},
		{"2026-11-11", Rating{Good, []string{}}, Rating{Meh, []string{"нет прогноза на 2 суток вперёд"}}},
		{"2026-11-12", Rating{Meh, []string{"нет прогноза на сутки вперёд"}}, Rating{Meh, []string{"нет прогноза на 2 суток вперёд"}}},
	}
	for _, tc := range cases {
		got := Assess(synth(nil), tc.date, morning)
		if !reflect.DeepEqual(got.Wash, tc.wash) || !reflect.DeepEqual(got.Coat, tc.coat) {
			t.Errorf("%s: мойка %+v, покрытие %+v", tc.date, got, tc)
		}
	}
	// Плохая оценка остаётся плохой, к ней край ничего не добавляет.
	bad := synth(at(14+48, func(h *Hour) { h.Precip = 1 }))
	if got := Assess(bad, "2026-11-12", morning); got.Wash.Level != Bad || len(got.Wash.Reasons) != 1 {
		t.Errorf("плохой день на краю: %+v", got.Wash)
	}
}
