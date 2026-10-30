package weather

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Level — уровень оценки дня.
type Level string

// Уровни оценки (ФТ-16.2.3).
const (
	Good Level = "good"
	Meh  Level = "meh"
	Bad  Level = "bad"
)

// Rating — оценка дня для одного дела.
type Rating struct {
	Level   Level    `json:"level"`
	Reasons []string `json:"reasons"`
}

// Assessment — оценка дня: мойка и покрытие.
type Assessment struct {
	Wash Rating `json:"wash"`
	Coat Rating `json:"coat"`
}

// Assess оценивает день date (ГГГГ-ММ-ДД) по часам прогноза. now — текущий
// момент: прошедшие часы сегодняшнего светлого времени не учитываются.
func Assess(hours []Hour, date string, now time.Time) Assessment {
	var win []Hour
	for _, h := range hours {
		if h.Time.Format("2006-01-02") == date && h.Time.Hour() >= WindowFrom && h.Time.Hour() < WindowTo &&
			h.Time.Add(time.Hour).After(now) {
			win = append(win, h)
		}
	}
	if len(win) == 0 {
		reason := "нет данных о погоде"
		if date == now.Format("2006-01-02") {
			reason = "светлое время уже закончилось"
		}
		r := Rating{Level: Bad, Reasons: []string{reason}}
		return Assessment{Wash: r, Coat: r}
	}
	start := win[0].Time
	day := time.Date(start.Year(), start.Month(), start.Day(), WindowTo, 0, 0, 0, start.Location())
	return Assessment{
		Wash: assessWash(win, hours, day),
		Coat: assessCoat(win, hours, start),
	}
}

func assessWash(win, all []Hour, windowEnd time.Time) Rating {
	var b rater
	b.rain(win, win[0].Time)
	minT, maxWind := win[0].Temp, 0.0
	for _, h := range win {
		minT = math.Min(minT, h.Temp)
		maxWind = math.Max(maxWind, h.Wind)
	}
	if minT <= WashMinTemp {
		b.bad(coldReason(minT))
	}
	if maxWind >= WashMaxWind {
		b.bad(fmt.Sprintf("ветер %d м/с", int(math.Round(maxWind))))
	}
	// Машина должна остаться чистой: сутки после окна без дождя.
	if h, ok := firstRain(all, windowEnd, windowEnd.Add(WashAfterHours*time.Hour)); ok {
		b.bad(rainReason(h, win[0].Time))
	}
	b.noData(win, windowEnd, windowEnd.Add(WashAfterHours*time.Hour), all)
	b.edge(all, windowEnd.Add(WashAfterHours*time.Hour), reasonNoForecast24)
	b.prob(win)
	return b.rating()
}

func assessCoat(win, all []Hour, _ time.Time) Rating {
	var b rater
	start := win[0].Time
	if run, humidity, ok := coatWindow(win); ok {
		start = run[0].Time
		if humidity >= CoatHumMeh {
			b.meh(fmt.Sprintf("влажность %d %%", humidity))
		}
		b.prob(run)
	} else {
		b.bad(coatNoWindowReason(win))
	}
	// Сутки и ещё сутки после начала нанесения без осадков.
	if h, ok := firstRain(all, start, start.Add(CoatAfterHours*time.Hour)); ok {
		b.bad(rainReason(h, win[0].Time))
	}
	b.noData(win, start, start.Add(CoatAfterHours*time.Hour), all)
	b.edge(all, start.Add(CoatAfterHours*time.Hour), reasonNoForecast48)
	return b.rating()
}

func coatTempOK(h Hour) bool { return h.Temp >= CoatMinTemp && h.Temp <= CoatMaxTemp }

// coatWindow ищет в светлых часах окно для нанесения: CoatWindowHours часов
// подряд с подходящей температурой и влажностью ниже CoatHumMeh; нет такого —
// с влажностью до CoatHumBad включительно (берётся окно с меньшей влажностью).
// Возвращает окно и наибольшую влажность в нём.
func coatWindow(win []Hour) (run []Hour, humidity int, ok bool) {
	for _, limit := range []int{CoatHumMeh - 1, CoatHumBad} {
		best := -1
		for _, r := range runs(win, func(h Hour) bool { return coatTempOK(h) && h.Humidity <= limit }) {
			m := 0
			for _, h := range r {
				m = max(m, h.Humidity)
			}
			if best < 0 || m < best {
				run, best = r, m
			}
		}
		if run != nil {
			return run, best, true
		}
	}
	return nil, 0, false
}

// runs — отрезки подряд идущих часов, подходящих под ok, не короче CoatWindowHours.
func runs(hours []Hour, ok func(Hour) bool) [][]Hour {
	var out [][]Hour
	start := -1
	flush := func(end int) {
		if start >= 0 && end-start >= CoatWindowHours {
			out = append(out, hours[start:end])
		}
		start = -1
	}
	for i, h := range hours {
		if !ok(h) || (i > 0 && start >= 0 && !h.Time.Equal(hours[i-1].Time.Add(time.Hour))) {
			flush(i)
		}
		if ok(h) && start < 0 {
			start = i
		}
	}
	flush(len(hours))
	return out
}

// coatNoWindowReason — причина по часу, самому близкому к норме.
func coatNoWindowReason(win []Hour) string {
	best, bestScore := win[0], math.MaxFloat64
	for _, h := range win {
		temp := math.Max(0, math.Max(CoatMinTemp-h.Temp, h.Temp-CoatMaxTemp))
		hum := math.Max(0, float64(h.Humidity-(CoatHumMeh-1)))
		if score := temp/CoatMinTemp + hum/10; score < bestScore {
			best, bestScore = h, score
		}
	}
	switch {
	case best.Temp < CoatMinTemp:
		return coldReason(best.Temp) + " весь день"
	case best.Temp > CoatMaxTemp:
		return "жара " + Temp(best.Temp) + " весь день"
	case best.Humidity > CoatHumBad:
		return fmt.Sprintf("влажность весь день выше %d %%", CoatHumBad)
	case best.Humidity >= CoatHumMeh:
		return fmt.Sprintf("влажность %d %%", best.Humidity)
	}
	return fmt.Sprintf("нет окна на %d часа подряд", CoatWindowHours)
}

// rater собирает причины двух степеней.
type rater struct{ bads, mehs []string }

func (r *rater) bad(s string) { r.bads = append(r.bads, s) }
func (r *rater) meh(s string) { r.mehs = append(r.mehs, s) }

// rain: осадки в самом светлом окне — «дождь в 15:00».
func (r *rater) rain(win []Hour, day time.Time) {
	if h, ok := firstRain(win, win[0].Time, win[len(win)-1].Time.Add(time.Hour)); ok {
		r.bad(rainReason(h, day))
	}
}

// prob: вероятность осадков 30–50 % в светлое время — мягкая причина.
func (r *rater) prob(win []Hour) {
	best := 0
	for _, h := range win {
		best = max(best, h.Prob)
	}
	if best >= ProbMeh && best <= ProbBad {
		r.meh(fmt.Sprintf("вероятность осадков %d %%", best))
	}
}

// edge: окно «после» выходит за последний час прогноза — не выше «так себе».
func (r *rater) edge(all []Hour, need time.Time, reason string) {
	if len(all) > 0 && need.After(all[len(all)-1].Time.Add(time.Hour)) {
		r.meh(reason)
	}
}

// noData: у часа окна или «после» осадки неизвестны — мягкая причина.
func (r *rater) noData(win []Hour, from, to time.Time, all []Hour) {
	for _, h := range win {
		if h.NoPrecip {
			r.meh(reasonNoPrecipData)
			return
		}
	}
	for _, h := range all {
		if h.NoPrecip && !h.Time.Before(from) && h.Time.Before(to) {
			r.meh(reasonNoPrecipData)
			return
		}
	}
}

func (r *rater) rating() Rating {
	switch {
	case len(r.bads) > 0:
		return Rating{Level: Bad, Reasons: uniq(r.bads, MaxReasons)}
	case len(r.mehs) > 0:
		return Rating{Level: Meh, Reasons: uniq(r.mehs, MaxReasons)}
	}
	return Rating{Level: Good, Reasons: []string{}}
}

func uniq(in []string, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] && len(out) < limit {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// isRain: осадки за час не меньше порога или вероятность выше ProbBad.
func isRain(h Hour) bool { return h.Precip >= RainMM || h.Prob > ProbBad }

// firstRain — первый час с осадками в [from, to).
func firstRain(hours []Hour, from, to time.Time) (Hour, bool) {
	for _, h := range hours {
		if !h.Time.Before(from) && h.Time.Before(to) && isRain(h) {
			return h, true
		}
	}
	return Hour{}, false
}

func isSnow(code int) bool { return (code >= 71 && code <= 77) || code == 85 || code == 86 }

// rainReason: «дождь в 15:00», «снег завтра в 12:00», «дождь послезавтра в 03:00».
func rainReason(h Hour, day time.Time) string {
	what := "дождь"
	if isSnow(h.Code) {
		what = "снег"
	}
	d0 := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	d1 := time.Date(h.Time.Year(), h.Time.Month(), h.Time.Day(), 0, 0, 0, 0, day.Location())
	when := ""
	switch int(math.Round(d1.Sub(d0).Hours() / 24)) {
	case 0:
	case 1:
		when = " завтра"
	default:
		when = " послезавтра"
	}
	return fmt.Sprintf("%s%s в %02d:00", what, when, h.Time.Hour())
}

func coldReason(t float64) string {
	if t < 0 && math.Round(t) < 0 {
		return "мороз " + Temp(t)
	}
	return "холодно " + Temp(t)
}

// Temp: «−3°», «+12°» (настоящий минус, как в интерфейсе).
func Temp(t float64) string {
	n := int(math.Round(t))
	switch {
	case n < 0:
		return fmt.Sprintf("−%d°", -n)
	case n > 0:
		return fmt.Sprintf("+%d°", n)
	}
	return "0°"
}

// Join склеивает причины для одной строки.
func Join(reasons []string) string { return strings.Join(reasons, ", ") }
