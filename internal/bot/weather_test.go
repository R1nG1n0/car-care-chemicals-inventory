package bot

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/weather"
)

// weatherEnv подключает к боту погоду на имитации Open-Meteo (httptest).
func weatherEnv(t *testing.T, def *weather.Place) (*env, *atomic.Bool, *tgClock) {
	t.Helper()
	fixture, err := os.ReadFile("../weather/testdata/forecast_donetsk.json")
	if err != nil {
		t.Fatal(err)
	}
	down := &atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)
	msk, _ := time.LoadLocation("Europe/Moscow")
	clock := &tgClock{t: time.Date(2026, 10, 30, 12, 0, 0, 0, msk)}
	e := newEnv(t)
	e.bot.weather = weather.NewService(weather.ServiceOptions{
		DB: e.db, Default: def, Now: clock.Now,
		Client: weather.NewClient(weather.Options{BaseURL: srv.URL, GeocodingURL: srv.URL}),
	})
	return e, down, clock
}

func TestWeatherCommand(t *testing.T) {
	e, _, _ := weatherEnv(t, &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802})
	a := e.linked("owner", 1001)
	for _, say := range []string{"/weather", "погода", "Погода", "/weather@garage_test_bot"} {
		e.say(a, say)
		contains(t, e.fake.last().Text, "Погода: <b>Донецк</b>", "Сегодня</b>, пт 30.10", "Завтра</b>, сб 31.10",
			"🚿 Мойка ❌ — дождь завтра в 01:00", "🧴 Покрытие", "Мойка: лучше вс 01.11",
			"Покрытие: лучше вс 01.11", "Open-Meteo.com")
	}
	// Вчерашний день и послезавтра в текст не попадают.
	if containsAny(e.fake.last().Text, "Суббота", "вс 01.11, ", "Воскресенье") {
		t.Fatalf("лишнее в ответе: %s", e.fake.last().Text)
	}
}

func TestWeatherCommandTomorrowReasons(t *testing.T) {
	e, _, _ := weatherEnv(t, &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802})
	a := e.linked("owner", 1001)
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "🧴 Покрытие ❌ — дождь в 08:00")
}

func TestWeatherOwnPlace(t *testing.T) {
	e, _, _ := weatherEnv(t, &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802})
	a := e.linked("owner", 1001)
	b := e.linked("friend", 1002)
	e.exec(`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (?, 'Ростов <1>', 47.2, 39.7)`, a.id)
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Ростов &lt;1&gt;") // название места экранируется для HTML
	e.say(b, "/weather")
	m := e.fake.last().Text
	contains(t, m, "Донецк")
	if containsAny(m, "Ростов") {
		t.Fatalf("чужое место: %s", m)
	}
}

func TestWeatherNotConfigured(t *testing.T) {
	e, _, _ := weatherEnv(t, nil)
	a := e.linked("owner", 1001)
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Погода не настроена")

	// Бот без сервиса погоды (как до v1.8) отвечает так же.
	e.bot.weather = nil
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Погода не настроена")
}

func TestWeatherUnavailableAndStale(t *testing.T) {
	e, down, clock := weatherEnv(t, &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802})
	a := e.linked("owner", 1001)
	down.Store(true)
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Погода сейчас недоступна")
	if containsAny(e.fake.last().Text, "127.0.0.1", "http") {
		t.Fatalf("в ответе адрес источника: %s", e.fake.last().Text)
	}

	down.Store(false)
	clock.Set(clock.Now().Add(2 * time.Minute))
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Сегодня")
	if containsAny(e.fake.last().Text, "Нет связи") {
		t.Fatal("свежий прогноз помечен устаревшим")
	}
	down.Store(true)
	clock.Set(clock.Now().Add(4 * time.Hour))
	e.say(a, "/weather")
	contains(t, e.fake.last().Text, "Нет связи с источником", "прогноз от 30.10 12:02")
}

func TestWeatherNeedsLink(t *testing.T) {
	e, _, _ := weatherEnv(t, &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802})
	e.say(user{tg: 4242}, "/weather")
	contains(t, e.fake.last().Text, "Откройте сайт")
}

func TestHelpMentionsWeather(t *testing.T) {
	e, _, _ := weatherEnv(t, nil)
	a := e.linked("owner", 1001)
	e.say(a, "/help")
	contains(t, e.fake.last().Text, "/weather")
}
