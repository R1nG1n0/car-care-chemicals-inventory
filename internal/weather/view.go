package weather

import (
	"math"
	"time"
)

// Дни недели для подписи (индекс — time.Weekday).
var weekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// DayView — сутки прогноза с оценкой (поле days ответа /api/weather).
type DayView struct {
	Date       string  `json:"date"`
	Weekday    string  `json:"weekday"`
	TMin       float64 `json:"t_min"`
	TMax       float64 `json:"t_max"`
	PrecipMM   float64 `json:"precip_mm"`
	PrecipProb int     `json:"precip_prob"`
	Code       int     `json:"code"`
	Wash       Rating  `json:"wash"`
	Coat       Rating  `json:"coat"`
}

// HourView — один час сегодняшнего дня (поле today_hours).
type HourView struct {
	Time       string  `json:"time"`
	Temp       float64 `json:"temp"`
	PrecipMM   float64 `json:"precip_mm"`
	PrecipProb int     `json:"precip_prob"`
	Wind       float64 `json:"wind"`
	Humidity   int     `json:"humidity"`
	Code       int     `json:"code"`
}

// Best — ближайший хороший день (ГГГГ-ММ-ДД) для мойки и покрытия или nil.
type Best struct {
	Wash *string `json:"wash"`
	Coat *string `json:"coat"`
}

// View — прогноз, готовый к показу: дни с оценкой, сегодня по часам, лучшие дни.
type View struct {
	Days       []DayView  `json:"days"`
	TodayHours []HourView `json:"today_hours"`
	Best       Best       `json:"best"`
}

// Build превращает прогноз в View на момент now. Дни, которые уже прошли
// (старый прогноз без связи), отбрасываются.
func Build(f Forecast, now time.Time) View {
	now = now.In(f.Loc)
	today := now.Format("2006-01-02")
	v := View{Days: []DayView{}, TodayHours: []HourView{}}
	for _, d := range f.Days {
		if d.Date < today {
			continue
		}
		if len(v.Days) >= MaxDays {
			break
		}
		a := Assess(f.Hours, d.Date, now)
		v.Days = append(v.Days, DayView{
			Date: d.Date, Weekday: weekday(d.Date), TMin: d.TMin, TMax: d.TMax,
			PrecipMM: round1(d.Precip), PrecipProb: d.Prob, Code: d.Code, Wash: a.Wash, Coat: a.Coat,
		})
		date := d.Date
		if v.Best.Wash == nil && a.Wash.Level == Good {
			v.Best.Wash = &date
		}
		if v.Best.Coat == nil && a.Coat.Level == Good {
			v.Best.Coat = &date
		}
	}
	for _, h := range f.Hours {
		if h.Time.Format("2006-01-02") != today {
			continue
		}
		v.TodayHours = append(v.TodayHours, HourView{
			Time: h.Time.Format("15:04"), Temp: h.Temp, PrecipMM: round1(h.Precip), PrecipProb: h.Prob,
			Wind: h.Wind, Humidity: h.Humidity, Code: h.Code,
		})
	}
	return v
}

func weekday(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return weekdays[t.Weekday()]
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
