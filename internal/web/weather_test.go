package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/weather"
)

// Погода (ФТ-16): Open-Meteo заменён сервером httptest, настоящих запросов нет.

const (
	geoBody = `{"results":[
		{"name":"Донецк","latitude":48.023,"longitude":37.802,"country":"Украина","admin1":"Донецкая область","timezone":"Europe/Kyiv"},
		{"name":"Донецк","latitude":48.3358,"longitude":39.9423,"country":"Россия","admin1":"Ростовская область","timezone":"Europe/Moscow"}]}`
)

// upstream — имитация Open-Meteo.
type upstream struct {
	srv *httptest.Server

	mu        sync.Mutex
	forecasts int
	searches  int
	lastQuery string
	down      bool
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	fixture, err := os.ReadFile("../weather/testdata/forecast_donetsk.json")
	if err != nil {
		t.Fatal(err)
	}
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.down {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		u.lastQuery = r.URL.RawQuery
		switch r.URL.Path {
		case "/v1/forecast":
			u.forecasts++
			_, _ = w.Write(fixture)
		case "/v1/search":
			u.searches++
			if r.URL.Query().Get("name") == "Нигде" {
				_, _ = w.Write([]byte(`{"generationtime_ms":0.5}`))
				return
			}
			_, _ = w.Write([]byte(geoBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) setDown(v bool) {
	u.mu.Lock()
	u.down = v
	u.mu.Unlock()
}

func (u *upstream) counts() (forecasts, searches int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.forecasts, u.searches
}

func (u *upstream) query() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.lastQuery
}

var donetsk = &weather.Place{Name: "Донецк", Lat: 48.023, Lon: 37.802}

// newWeatherEnv — сервер с погодой; время погоды 30.10.2026 12:00 по Москве
// (дата фикстуры), его можно двигать через clock.
func newWeatherEnv(t *testing.T, def *weather.Place) (*env, *upstream, *testClock) {
	t.Helper()
	up := newUpstream(t)
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{t: time.Date(2026, 10, 30, 12, 0, 0, 0, msk)}
	e := newEnvFull(t, false, nil, func(db *sql.DB) *weather.Service {
		return weather.NewService(weather.ServiceOptions{
			DB:      db,
			Default: def,
			Client:  weather.NewClient(weather.Options{BaseURL: up.srv.URL, GeocodingURL: up.srv.URL}),
			Now:     clock.Now,
		})
	})
	return e, up, clock
}

type weatherBody struct {
	Enabled  bool `json:"enabled"`
	Location *struct {
		Name   string  `json:"name"`
		Lat    float64 `json:"lat"`
		Lon    float64 `json:"lon"`
		Custom bool    `json:"custom"`
	} `json:"location"`
	FetchedAt *int64 `json:"fetched_at"`
	Stale     bool   `json:"stale"`
	UTCOffset int    `json:"utc_offset"`
	Days      []struct {
		Date    string  `json:"date"`
		Weekday string  `json:"weekday"`
		TMin    float64 `json:"t_min"`
		TMax    float64 `json:"t_max"`
		Wash    struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		} `json:"wash"`
		Coat struct {
			Level   string   `json:"level"`
			Reasons []string `json:"reasons"`
		} `json:"coat"`
	} `json:"days"`
	TodayHours []struct {
		Time string `json:"time"`
	} `json:"today_hours"`
	Best struct {
		Wash *string `json:"wash"`
		Coat *string `json:"coat"`
	} `json:"best"`
}

func getWeather(t *testing.T, c *client) weatherBody {
	t.Helper()
	rec := c.do(http.MethodGet, "/api/weather", nil)
	expect(t, rec, http.StatusOK, "GET /api/weather")
	return decode[weatherBody](t, rec)
}

func TestWeatherDisabledWithoutPlace(t *testing.T) {
	e, up, _ := newWeatherEnv(t, nil)
	c := stkSignIn(e, "owner")
	rec := c.do(http.MethodGet, "/api/weather", nil)
	expect(t, rec, http.StatusOK, "погода выключена")
	if got := strings.TrimSpace(rec.Body.String()); got !=
		`{"enabled":false,"location":null,"fetched_at":null,"stale":false,"utc_offset":0,"days":[],"today_hours":[],"best":{"wash":null,"coat":null}}` {
		t.Fatalf("тело: %s", got)
	}
	if f, _ := up.counts(); f != 0 {
		t.Fatalf("при enabled=false был запрос наружу: %d", f)
	}
}

func TestWeatherDefaultPlace(t *testing.T) {
	e, up, clock := newWeatherEnv(t, donetsk)
	c := stkSignIn(e, "owner")
	w := getWeather(t, c)
	if !w.Enabled || w.Location == nil || w.Location.Name != "Донецк" || w.Location.Custom || w.Stale {
		t.Fatalf("ответ: %+v", w)
	}
	if w.FetchedAt == nil || *w.FetchedAt != clock.Now().Unix() {
		t.Fatalf("fetched_at: %v", w.FetchedAt)
	}
	if w.UTCOffset != 10800 {
		t.Fatalf("utc_offset: %d", w.UTCOffset)
	}
	if len(w.Days) != 7 || w.Days[0].Date != "2026-10-30" || w.Days[0].Weekday != "пт" {
		t.Fatalf("дни: %+v", w.Days)
	}
	if len(w.TodayHours) != 24 || w.TodayHours[0].Time != "00:00" || w.TodayHours[15].Time != "15:00" {
		t.Fatalf("часы сегодня: %d", len(w.TodayHours))
	}
	if w.Days[0].Wash.Level == "" || w.Days[0].Coat.Reasons == nil {
		t.Fatalf("оценка: %+v", w.Days[0])
	}
	if w.Best.Wash == nil || *w.Best.Wash != "2026-11-01" {
		t.Fatalf("лучшие дни: %+v", w.Best)
	}
	// Второй запрос — из памяти.
	getWeather(t, c)
	if f, _ := up.counts(); f != 1 {
		t.Fatalf("запросов прогноза: %d", f)
	}
	if !strings.Contains(up.query(), "timezone=Europe%2FMoscow") {
		t.Fatalf("запрос: %s", up.query())
	}
}

func TestWeatherOwnPlaceSetAndReset(t *testing.T) {
	e, up, _ := newWeatherEnv(t, donetsk)
	a := stkSignIn(e, "alice")
	b := stkSignIn(e, "bob")

	rec := a.do(http.MethodPut, "/api/weather/location", map[string]any{"name": "  Ростов-на-Дону ", "lat": 47.2313, "lon": 39.7233})
	expect(t, rec, http.StatusOK, "своё место")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"name":"Ростов-на-Дону","lat":47.2313,"lon":39.7233,"custom":true}` {
		t.Fatalf("тело: %s", got)
	}
	w := getWeather(t, a)
	if w.Location == nil || w.Location.Name != "Ростов-на-Дону" || !w.Location.Custom {
		t.Fatalf("своё место: %+v", w.Location)
	}
	if !strings.Contains(up.query(), "latitude=47.2300") {
		t.Fatalf("прогноз не для своего места: %s", up.query())
	}
	// Изоляция: у второго по-прежнему место по умолчанию, сброс чужого его не задевает.
	if wb := getWeather(t, b); wb.Location == nil || wb.Location.Custom || wb.Location.Name != "Донецк" {
		t.Fatalf("у второго: %+v", wb.Location)
	}
	expect(t, b.do(http.MethodDelete, "/api/weather/location", nil), http.StatusNoContent, "сброс у второго")
	if w := getWeather(t, a); w.Location == nil || !w.Location.Custom {
		t.Fatalf("сброс второго задел первого: %+v", w.Location)
	}
	// Сброс первого.
	expect(t, a.do(http.MethodDelete, "/api/weather/location", nil), http.StatusNoContent, "сброс")
	if w := getWeather(t, a); w.Location == nil || w.Location.Custom || w.Location.Name != "Донецк" {
		t.Fatalf("после сброса: %+v", w.Location)
	}
}

// Без места по умолчанию погоду включает собственный выбор.
func TestWeatherOwnPlaceEnablesWithoutDefault(t *testing.T) {
	e, _, _ := newWeatherEnv(t, nil)
	c := stkSignIn(e, "owner")
	expect(t, c.do(http.MethodPut, "/api/weather/location", map[string]any{"name": "Москва", "lat": 55.75, "lon": 37.62}),
		http.StatusOK, "место")
	if w := getWeather(t, c); !w.Enabled || len(w.Days) != 7 {
		t.Fatalf("ответ: %+v", w)
	}
	expect(t, c.do(http.MethodDelete, "/api/weather/location", nil), http.StatusNoContent, "сброс")
	if w := getWeather(t, c); w.Enabled {
		t.Fatal("после сброса без места по умолчанию погода должна выключиться")
	}
}

func TestWeatherSetLocationValidation(t *testing.T) {
	e, _, _ := newWeatherEnv(t, donetsk)
	c := stkSignIn(e, "owner")
	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"пустое имя", map[string]any{"name": "  ", "lat": 1, "lon": 1}, "name"},
		{"длинное имя", map[string]any{"name": strings.Repeat("я", 81), "lat": 1, "lon": 1}, "name"},
		{"управляющий символ", map[string]any{"name": "a\nb", "lat": 1, "lon": 1}, "name"},
		{"широта", map[string]any{"name": "X", "lat": 90.5, "lon": 1}, "lat"},
		{"нет широты", map[string]any{"name": "X", "lon": 1}, "lat"},
		{"долгота", map[string]any{"name": "X", "lat": 1, "lon": -181}, "lon"},
		{"нет долготы", map[string]any{"name": "X", "lat": 1}, "lon"},
	} {
		rec := c.do(http.MethodPut, "/api/weather/location", tc.body)
		expect(t, rec, http.StatusBadRequest, tc.name)
		if got := decode[struct {
			Field string `json:"field"`
		}](t, rec).Field; got != tc.field {
			t.Fatalf("%s: поле %q, ожидалось %q", tc.name, got, tc.field)
		}
	}
	if w := getWeather(t, c); w.Location.Custom {
		t.Fatal("неверный ввод сохранил место")
	}
}

func TestWeatherUnavailable(t *testing.T) {
	e, up, clock := newWeatherEnv(t, donetsk)
	c := stkSignIn(e, "owner")
	up.setDown(true)
	rec := c.do(http.MethodGet, "/api/weather", nil)
	expect(t, rec, http.StatusBadGateway, "нет прогноза и кэша")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"Прогноз сейчас недоступен"}` {
		t.Fatalf("тело: %s", got)
	}
	if strings.Contains(rec.Body.String(), up.srv.URL) {
		t.Fatal("в ответе адрес источника")
	}

	// Источник ожил — прогноз получен; потом пропал — отдаётся прежний с stale.
	up.setDown(false)
	clock.Add(2 * time.Minute)
	if w := getWeather(t, c); w.Stale {
		t.Fatal("свежий прогноз помечен устаревшим")
	}
	up.setDown(true)
	// Старше часа, но моложе 3 часов: прогноз отдаётся без пометки.
	clock.Add(2 * time.Hour)
	if w := getWeather(t, c); w.Stale || len(w.Days) == 0 {
		t.Fatalf("прогноз 2 часа: stale=%v days=%d", w.Stale, len(w.Days))
	}
	clock.Add(2 * time.Hour)
	w := getWeather(t, c)
	if !w.Stale || len(w.Days) == 0 {
		t.Fatalf("ожидался устаревший прогноз: stale=%v days=%d", w.Stale, len(w.Days))
	}
}

func TestWeatherPlacesSearch(t *testing.T) {
	e, up, _ := newWeatherEnv(t, donetsk)
	c := stkSignIn(e, "owner")
	rec := c.do(http.MethodGet, "/api/weather/places?q=%D0%94%D0%BE%D0%BD%D0%B5%D1%86%D0%BA", nil)
	expect(t, rec, http.StatusOK, "поиск")
	got := decode[struct {
		Places []struct {
			Name    string  `json:"name"`
			Admin1  string  `json:"admin1"`
			Country string  `json:"country"`
			Lat     float64 `json:"lat"`
			Lon     float64 `json:"lon"`
		} `json:"places"`
	}](t, rec).Places
	if len(got) != 2 || got[0].Admin1 != "Донецкая область" || got[1].Country != "Россия" || got[1].Lon != 39.9423 {
		t.Fatalf("места: %+v", got)
	}
	if !strings.Contains(up.query(), "language=ru") {
		t.Fatalf("запрос геокодера: %s", up.query())
	}

	rec = c.do(http.MethodGet, "/api/weather/places?q=Нигде", nil)
	expect(t, rec, http.StatusOK, "ничего не найдено")
	if !strings.Contains(rec.Body.String(), `"places":[]`) {
		t.Fatalf("пустой список должен быть []: %s", rec.Body.String())
	}
	for _, q := range []string{"", "я", strings.Repeat("я", 61)} {
		expect(t, c.do(http.MethodGet, "/api/weather/places?q="+q, nil), http.StatusBadRequest, "q="+q)
	}
	up.setDown(true)
	expect(t, c.do(http.MethodGet, "/api/weather/places?q=Донецк", nil), http.StatusBadGateway, "геокодер недоступен")
}

func TestWeatherPlacesRateLimit(t *testing.T) {
	e, up, _ := newWeatherEnv(t, donetsk)
	a := stkSignIn(e, "alice")
	b := stkSignIn(e, "bob")
	for i := 0; i < placeSearchLimit; i++ {
		expect(t, a.do(http.MethodGet, "/api/weather/places?q=Донецк", nil), http.StatusOK, "поиск "+strconv.Itoa(i))
	}
	rec := a.do(http.MethodGet, "/api/weather/places?q=Донецк", nil)
	expect(t, rec, http.StatusTooManyRequests, "лимит поиска")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("нет Retry-After")
	}
	if _, s := up.counts(); s != placeSearchLimit {
		t.Fatalf("при 429 был запрос наружу: %d", s)
	}
	// Лимит на пользователя: второй не затронут; через минуту первый снова может.
	expect(t, b.do(http.MethodGet, "/api/weather/places?q=Донецк", nil), http.StatusOK, "поиск второго")
	e.clock.Add(61 * time.Second)
	expect(t, a.do(http.MethodGet, "/api/weather/places?q=Донецк", nil), http.StatusOK, "после минуты")
}

func TestWeatherRequiresSignInAndCSRF(t *testing.T) {
	e, _, _ := newWeatherEnv(t, donetsk)
	anon := e.client()
	for _, tc := range [][2]string{
		{http.MethodGet, "/api/weather"}, {http.MethodGet, "/api/weather/places?q=Донецк"},
		{http.MethodPut, "/api/weather/location"}, {http.MethodDelete, "/api/weather/location"},
	} {
		expect(t, anon.do(tc[0], tc[1], nil), http.StatusUnauthorized, "без входа "+tc[0]+" "+tc[1])
	}
	c := stkSignIn(e, "owner")
	c.noCSRF = true
	expect(t, c.do(http.MethodPut, "/api/weather/location", map[string]any{"name": "X", "lat": 1, "lon": 1}),
		http.StatusForbidden, "без CSRF-заголовка")
	expect(t, c.do(http.MethodPost, "/api/weather", nil), http.StatusMethodNotAllowed, "метод")
}

func TestWeatherLocationRateLimit(t *testing.T) {
	e, _, _ := newWeatherEnv(t, donetsk)
	a := stkSignIn(e, "alice")
	b := stkSignIn(e, "bob")
	body := map[string]any{"name": "Москва", "lat": 55.75, "lon": 37.62}
	for i := 0; i < locationSetLimit; i++ {
		expect(t, a.do(http.MethodPut, "/api/weather/location", body), http.StatusOK, "смена места "+strconv.Itoa(i))
	}
	rec := a.do(http.MethodPut, "/api/weather/location", body)
	expect(t, rec, http.StatusTooManyRequests, "лимит смены места")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("нет Retry-After")
	}
	// Сброс лимитом не ограничен; второй пользователь не затронут; через минуту первый снова может.
	expect(t, a.do(http.MethodDelete, "/api/weather/location", nil), http.StatusNoContent, "сброс")
	expect(t, b.do(http.MethodPut, "/api/weather/location", body), http.StatusOK, "смена места вторым")
	e.clock.Add(61 * time.Second)
	expect(t, a.do(http.MethodPut, "/api/weather/location", body), http.StatusOK, "после минуты")
}
