package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/weather"
)

const (
	hintWeatherOff   = "Погода не настроена: выберите свой город на сайте, в профиле."
	hintWeatherError = "Погода сейчас недоступна, попробуйте чуть позже."
)

var levelEmoji = map[weather.Level]string{weather.Good: "✅", weather.Meh: "🟡", weather.Bad: "❌"}

// cmdWeather — /weather: оценка на сегодня и завтра и ближайшие хорошие дни
// (ФТ-16.3.3). Место берётся у привязанного пользователя.
func (b *Bot) cmdWeather(ctx context.Context, link Link, chatID int64) {
	if b.weather == nil {
		b.send(ctx, chatID, hintWeatherOff, nil)
		return
	}
	place, ok, err := b.weather.PlaceFor(ctx, link.UserID)
	if err != nil {
		b.log.Error("telegram: место погоды", "err", err)
		b.send(ctx, chatID, hintWeatherError, nil)
		return
	}
	if !ok {
		b.send(ctx, chatID, hintWeatherOff, nil)
		return
	}
	res, err := b.weather.Forecast(ctx, place)
	if err != nil {
		b.log.Warn("telegram: прогноз погоды недоступен", "err", err)
		b.send(ctx, chatID, hintWeatherError, nil)
		return
	}
	b.send(ctx, chatID, b.weatherText(place, res), nil)
}

// weatherText — текст ответа /weather.
func (b *Bot) weatherText(place weather.Place, res weather.Result) string {
	now := b.weather.Now().In(res.Forecast.Loc)
	view := weather.Build(res.Forecast, now)
	text := "🌤 Погода: " + bold(place.Name)
	if res.Stale {
		text += "\n⚠️ Нет связи с источником, прогноз от " + res.FetchedAt.In(res.Forecast.Loc).Format("02.01 15:04") + "."
	}
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	shown := 0
	for _, d := range view.Days {
		if d.Date != today && d.Date != tomorrow {
			continue
		}
		shown++
		head := "Сегодня"
		if d.Date != today {
			head = "Завтра"
		}
		text += fmt.Sprintf("\n\n%s, %s %s: %s…%s", bold(head), d.Weekday, shortDate(d.Date), weather.Temp(d.TMin), weather.Temp(d.TMax))
		text += "\n" + ratingLine("🚿 Мойка", d.Wash)
		text += "\n" + ratingLine("🧴 Покрытие", d.Coat)
	}
	if shown == 0 {
		return hintWeatherError
	}
	text += "\n\n" + bestLine("🚿 Мойка", view.Best.Wash, today, tomorrow)
	text += "\n" + bestLine("🧴 Покрытие", view.Best.Coat, today, tomorrow)
	return text + "\n\nПогода: Open-Meteo.com"
}

func ratingLine(label string, r weather.Rating) string {
	line := label + " " + levelEmoji[r.Level]
	if len(r.Reasons) > 0 {
		line += " — " + telegram.Esc(weather.Join(r.Reasons))
	}
	return line
}

func bestLine(label string, date *string, today, tomorrow string) string {
	if date == nil {
		return label + ": хорошего дня на неделе нет"
	}
	return label + ": лучше " + dayLabel(*date, today, tomorrow)
}

// dayLabel: «сегодня», «завтра» или «чт 30.10».
func dayLabel(date, today, tomorrow string) string {
	switch date {
	case today:
		return "сегодня"
	case tomorrow:
		return "завтра"
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return weekdayShort[t.Weekday()] + " " + t.Format("02.01")
}

// shortDate: «30.10».
func shortDate(date string) string {
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t.Format("02.01")
	}
	return date
}

var weekdayShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
