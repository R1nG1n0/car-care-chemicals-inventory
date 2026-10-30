package weather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/forecast_donetsk.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseForecast(t *testing.T) {
	f, err := ParseForecast(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.Loc.String() != "Europe/Moscow" || len(f.Hours) != 168 || len(f.Days) != 7 {
		t.Fatalf("loc=%v часов=%d дней=%d", f.Loc, len(f.Hours), len(f.Days))
	}
	h := f.Hours[0]
	if h.Time.Format("2006-01-02 15:04 -0700") != "2026-10-30 00:00 +0300" || h.Temp != 10.6 || h.Humidity != 44 || h.Wind != 4.52 {
		t.Fatalf("первый час: %+v", h)
	}
	d := f.Days[1]
	if d.Date != "2026-10-31" || d.TMin != 8.7 || d.TMax != 12.2 || d.Precip != 2.7 || d.Prob != 63 || d.Code != 80 {
		t.Fatalf("второй день: %+v", d)
	}
}

func TestParseForecastBad(t *testing.T) {
	for name, raw := range map[string]string{
		"не JSON":         `<html>`,
		"пусто":           `{}`,
		"разная длина":    `{"timezone":"Europe/Moscow","hourly":{"time":["2026-10-30T00:00"],"temperature_2m":[]},"daily":{"time":["2026-10-30"]}}`,
		"неверное время":  `{"timezone":"Europe/Moscow","hourly":{"time":["вчера"],"temperature_2m":[1],"precipitation_probability":[1],"precipitation":[0],"relative_humidity_2m":[1],"wind_speed_10m":[1],"weather_code":[1]},"daily":{"time":["2026-10-30"],"temperature_2m_min":[1],"temperature_2m_max":[1],"precipitation_sum":[0],"precipitation_probability_max":[0],"weather_code":[1]}}`,
		"нет температуры": `{"timezone":"Europe/Moscow","hourly":{"time":["2026-10-30T00:00"],"temperature_2m":[null],"precipitation_probability":[1],"precipitation":[0],"relative_humidity_2m":[1],"wind_speed_10m":[1],"weather_code":[1]},"daily":{"time":["2026-10-30"],"temperature_2m_min":[1],"temperature_2m_max":[1],"precipitation_sum":[0],"precipitation_probability_max":[0],"weather_code":[1]}}`,
	} {
		if _, err := ParseForecast([]byte(raw)); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: ошибка %v", name, err)
		}
	}
}

func TestClientForecast(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write(fixture(t))
	}))
	defer srv.Close()
	f, err := NewClient(Options{BaseURL: srv.URL + "/"}).Forecast(context.Background(), 48.0231, 37.8019)
	if err != nil || len(f.Days) != 7 {
		t.Fatalf("%v", err)
	}
	for _, want := range []string{"latitude=48.0231", "longitude=37.8019", "timezone=Europe%2FMoscow", "forecast_days=9", "wind_speed_unit=ms"} {
		if !strings.Contains(query, want) {
			t.Errorf("в запросе нет %s: %s", want, query)
		}
	}
}

// Свой пояс из конфигурации уходит в запрос как есть («auto» тоже допустим).
func TestClientTimezoneOption(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write(fixture(t))
	}))
	defer srv.Close()
	if _, err := NewClient(Options{BaseURL: srv.URL, Timezone: "auto"}).Forecast(context.Background(), 48.02, 37.80); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "timezone=auto") {
		t.Errorf("в запросе нет timezone=auto: %s", query)
	}
}

func TestClientErrorsDoNotLeakURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	url := srv.URL
	c := NewClient(Options{BaseURL: url, GeocodingURL: url})
	_, err := c.Forecast(context.Background(), 48.0231, 37.8019)
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "HTTP 500") || strings.Contains(err.Error(), "48.0231") {
		t.Fatalf("HTTP 500: %v", err)
	}
	if _, err := c.Search(context.Background(), "секретный город"); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "секретный") {
		t.Fatalf("геокодер 500: %v", err)
	}
	srv.Close()
	// Сеть недоступна: *url.Error с адресом очищается.
	_, err = c.Forecast(context.Background(), 48.0231, 37.8019)
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "48.0231") || strings.Contains(err.Error(), url) {
		t.Fatalf("сеть: %v", err)
	}
}

func TestClientResponseLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBody+10)))
	}))
	defer srv.Close()
	if _, err := NewClient(Options{BaseURL: srv.URL}).Forecast(context.Background(), 1, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("огромный ответ принят: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := NewClient(Options{BaseURL: srv.URL}).Forecast(ctx, 1, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("зависший источник: %v", err)
	}
}

func TestClientSearch(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[
			{"name":"Донецк","latitude":48.023,"longitude":37.802,"country":"Украина","admin1":"Донецкая область"},
			{"name":"Донецк","latitude":48.3358,"longitude":39.9423,"country":"Россия","admin1":"Ростовская область"},
			{"name":"","latitude":1,"longitude":1},
			{"name":"Мусор","latitude":99,"longitude":1}]}`))
	}))
	defer srv.Close()
	got, err := NewClient(Options{GeocodingURL: srv.URL}).Search(context.Background(), "Донецк")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Admin1 != "Донецкая область" || got[1].Country != "Россия" || got[1].Lat != 48.3358 {
		t.Fatalf("найдено: %+v", got)
	}
	for _, want := range []string{"count=8", "language=ru", "name=%D0%94"} {
		if !strings.Contains(query, want) {
			t.Errorf("в запросе нет %s: %s", want, query)
		}
	}
}

func TestClientSearchEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"generationtime_ms":0.4}`))
	}))
	defer srv.Close()
	got, err := NewClient(Options{GeocodingURL: srv.URL}).Search(context.Background(), "ъъъ")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("пустой результат: %v, %v", got, err)
	}
}

// Место в другом часовом поясе: часы и «сегодня» считаются по местному времени.
func TestLocalTimezone(t *testing.T) {
	body := func(tz string) []byte {
		return []byte(`{"timezone":"` + tz + `","utc_offset_seconds":18000,"hourly":{"time":["2026-11-11T00:00","2026-11-11T01:00"],
			"temperature_2m":[1,2],"precipitation_probability":[0,0],"precipitation":[0,0],"relative_humidity_2m":[50,50],
			"wind_speed_10m":[1,1],"weather_code":[0,0]},
			"daily":{"time":["2026-11-10","2026-11-11"],"temperature_2m_min":[0,0],"temperature_2m_max":[3,3],"precipitation_sum":[0,0],
			"precipitation_probability_max":[0,0],"weather_code":[0,0]}}`)
	}
	for _, tz := range []string{"Asia/Yekaterinburg", "Etc/Unknown"} { // вторая зона неизвестна: берётся смещение из ответа
		f, err := ParseForecast(body(tz))
		if err != nil {
			t.Fatal(err)
		}
		if _, off := f.Hours[0].Time.Zone(); off != 5*3600 {
			t.Fatalf("%s: смещение %d", tz, off)
		}
		// 20:00 UTC 10 ноября — в Екатеринбурге уже 11-е, 01:00.
		v := Build(f, time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC))
		if len(v.Days) != 1 || v.Days[0].Date != "2026-11-11" || len(v.TodayHours) != 2 || v.TodayHours[1].Time != "01:00" {
			t.Fatalf("%s: сегодня по местному времени: %+v", tz, v)
		}
	}
}

func TestParseForecastNullPrecip(t *testing.T) {
	body := func(prob, precip string) []byte {
		return []byte(`{"timezone":"Europe/Moscow","utc_offset_seconds":10800,"hourly":{"time":["2026-11-10T10:00","2026-11-10T11:00"],
			"temperature_2m":[1,2],"precipitation_probability":[` + prob + `],"precipitation":[` + precip + `],"relative_humidity_2m":[50,50],
			"wind_speed_10m":[1,1],"weather_code":[0,0]},
			"daily":{"time":["2026-11-10"],"temperature_2m_min":[0],"temperature_2m_max":[3],"precipitation_sum":[0],
			"precipitation_probability_max":[0],"weather_code":[0]}}`)
	}
	for name, raw := range map[string][]byte{
		"вероятность": body("10,null", "0,0"), "осадки": body("10,10", "0,null"),
	} {
		f, err := ParseForecast(raw)
		if err != nil || len(f.Hours) != 2 || f.Hours[0].NoPrecip || !f.Hours[1].NoPrecip {
			t.Fatalf("%s: %+v, %v", name, f.Hours, err)
		}
		if f.UTCOffset != 10800 {
			t.Fatalf("смещение: %d", f.UTCOffset)
		}
	}
}
