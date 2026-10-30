package web

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/weather"
)

// placeSearchLimit, locationSetLimit и previewLimit — поисков города, смен места
// и просмотров прогноза другого города в минуту на пользователя.
const (
	placeSearchLimit = 30
	locationSetLimit = 10
	previewLimit     = 30
)

const (
	minQueryRunes = 2
	maxQueryRunes = 60
)

// weatherRoutes регистрирует адреса погоды (ФТ-16, docs/API.md).
func (s *Server) weatherRoutes() {
	s.api("/api/weather", signedIn, methods{http.MethodGet: s.handleWeather})
	s.api("/api/weather/places", signedIn, methods{http.MethodGet: s.handleWeatherPlaces})
	s.api("/api/weather/location", signedIn, methods{
		http.MethodPut:    s.handleWeatherSetLocation,
		http.MethodDelete: s.handleWeatherResetLocation,
	})
}

type weatherResponse struct {
	Enabled   bool           `json:"enabled"`
	Location  *weather.Place `json:"location"`
	FetchedAt *int64         `json:"fetched_at"`
	Stale     bool           `json:"stale"`
	UTCOffset int            `json:"utc_offset"`
	// Preview — прогноз другого города по запросу (?lat&lon&name), а не места по умолчанию.
	Preview    bool               `json:"preview"`
	Days       []weather.DayView  `json:"days"`
	TodayHours []weather.HourView `json:"today_hours"`
	Best       weather.Best       `json:"best"`
}

// GET /api/weather — прогноз и оценка дней для места пользователя.
// С ?lat=&lon=&name= — для любого города (просмотр на экране погоды, место по
// умолчанию не меняется).
func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	place, preview, ok := s.previewPlace(w, r)
	if !ok {
		return
	}
	var err error
	if !preview {
		place, ok, err = s.weather.PlaceFor(r.Context(), currentFrom(r).user.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	if !ok {
		writeJSON(w, http.StatusOK, weatherResponse{Days: []weather.DayView{}, TodayHours: []weather.HourView{}})
		return
	}
	res, err := s.weather.Forecast(r.Context(), place)
	if err != nil {
		if r.Context().Err() == nil {
			s.log.Warn("погода: прогноз недоступен", "err", err)
		}
		writeError(w, http.StatusBadGateway, "Прогноз сейчас недоступен")
		return
	}
	view := weather.Build(res.Forecast, s.weather.Now())
	fetched := res.FetchedAt.Unix()
	writeJSON(w, http.StatusOK, weatherResponse{
		Enabled: true, Location: &place, FetchedAt: &fetched, Stale: res.Stale, UTCOffset: res.Forecast.UTCOffset,
		Preview: preview, Days: view.Days, TodayHours: view.TodayHours, Best: view.Best,
	})
}

// previewPlace разбирает ?lat=&lon=&name= — город для просмотра. Нет lat и lon —
// preview=false (место по умолчанию). ok=false — ответ с ошибкой уже записан.
func (s *Server) previewPlace(w http.ResponseWriter, r *http.Request) (place weather.Place, preview, ok bool) {
	q := r.URL.Query()
	if !q.Has("lat") && !q.Has("lon") {
		return weather.Place{}, false, true
	}
	lat, errLat := strconv.ParseFloat(q.Get("lat"), 64)
	lon, errLon := strconv.ParseFloat(q.Get("lon"), 64)
	name := strings.TrimSpace(q.Get("name"))
	switch {
	case errLat != nil || errLon != nil || !weather.ValidCoord(lat, lon):
		field := "lat"
		if errLat == nil && math.Abs(lat) <= 90 {
			field = "lon"
		}
		writeFieldError(w, http.StatusBadRequest, field, "Координаты места указаны неверно")
		return weather.Place{}, false, false
	case !weather.ValidName(name):
		writeFieldError(w, http.StatusBadRequest, "name",
			fmt.Sprintf("Название места — от 1 до %d символов", weather.MaxNameRunes))
		return weather.Place{}, false, false
	}
	// Каждый новый город — запрос к Open-Meteo: ограничиваем, как и поиск.
	if !s.allowPerUser(w, r, s.previewLimiter, "preview:") {
		return weather.Place{}, false, false
	}
	return weather.Place{Name: name, Lat: lat, Lon: lon}, true, true
}

// GET /api/weather/places?q= — поиск города для выбора своего места.
func (s *Server) handleWeatherPlaces(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < minQueryRunes || n > maxQueryRunes {
		writeFieldError(w, http.StatusBadRequest, "q",
			fmt.Sprintf("Название города — от %d до %d символов", minQueryRunes, maxQueryRunes))
		return
	}
	if !s.allowPerUser(w, r, s.placeLimiter, "places:") {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	found, err := s.weather.Search(ctx, q)
	if err != nil {
		if errors.Is(err, weather.ErrUnavailable) {
			s.log.Warn("погода: геокодер недоступен", "err", err)
		}
		writeError(w, http.StatusBadGateway, "Поиск города сейчас недоступен")
		return
	}
	if found == nil {
		found = []weather.Found{}
	}
	writeJSON(w, http.StatusOK, struct {
		Places []weather.Found `json:"places"`
	}{found})
}

// allowPerUser считает обращение к адресу в окне lim и отвечает 429 с
// Retry-After, когда лимит пользователя исчерпан.
func (s *Server) allowPerUser(w http.ResponseWriter, r *http.Request, lim *auth.Limiter, prefix string) bool {
	key := prefix + strconv.FormatInt(currentFrom(r).user.ID, 10)
	if blocked, wait := lim.Blocked(key); blocked {
		secs := max(1, int(math.Ceil(wait.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, errorResponse{
			Error: "Слишком много запросов. Повторите через минуту", RetryAfter: secs})
		return false
	}
	lim.Fail(key)
	return true
}

// PUT /api/weather/location {name, lat, lon} — своё место пользователя.
func (s *Server) handleWeatherSetLocation(w http.ResponseWriter, r *http.Request) {
	if !s.allowPerUser(w, r, s.locationLimiter, "location:") {
		return
	}
	var req struct {
		Name string   `json:"name"`
		Lat  *float64 `json:"lat"`
		Lon  *float64 `json:"lon"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	switch {
	case !weather.ValidName(req.Name):
		writeFieldError(w, http.StatusBadRequest, "name",
			fmt.Sprintf("Название места — от 1 до %d символов", weather.MaxNameRunes))
		return
	case req.Lat == nil || req.Lon == nil || !weather.ValidCoord(*req.Lat, *req.Lon):
		field := "lat"
		if req.Lat != nil && math.Abs(*req.Lat) <= 90 {
			field = "lon"
		}
		writeFieldError(w, http.StatusBadRequest, field, "Координаты места указаны неверно")
		return
	}
	place, err := s.weather.SetPlace(r.Context(), currentFrom(r).user.ID,
		weather.Place{Name: req.Name, Lat: *req.Lat, Lon: *req.Lon})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

// DELETE /api/weather/location — вернуться к месту по умолчанию.
func (s *Server) handleWeatherResetLocation(w http.ResponseWriter, r *http.Request) {
	if err := s.weather.ResetPlace(r.Context(), currentFrom(r).user.ID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeNoContent(w)
}
