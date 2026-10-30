package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL и DefaultGeocodingURL — адреса Open-Meteo.
	DefaultBaseURL      = "https://api.open-meteo.com"
	DefaultGeocodingURL = "https://geocoding-api.open-meteo.com"

	callTimeout     = 8 * time.Second
	maxResponseBody = 2 << 20 // прогноз на 7 дней — около 8 КБ; больше — ошибка
	searchLimit     = 8
	// fetchDays — дней в запросе: два лишних дня нужны, чтобы окно «после» у
	// последнего показанного дня не упиралось в край прогноза.
	fetchDays = 9
)

// ErrUnavailable — Open-Meteo не ответил или ответил не так, как ожидалось.
// Подробности (без адресов запросов) — в обёрнутой ошибке.
var ErrUnavailable = errors.New("прогноз недоступен")

// Options — настройки клиента.
type Options struct {
	// BaseURL и GeocodingURL — адреса API; пусто — стандартные Open-Meteo.
	BaseURL      string
	GeocodingURL string
	// HTTPClient — готовый клиент вместо собственного (тесты).
	HTTPClient *http.Client
	// Timezone — часовой пояс часов и дней прогноза (IANA или "auto");
	// пусто — DefaultTimezone.
	Timezone string
}

// Client — клиент Open-Meteo.
type Client struct {
	base string
	geo  string
	tz   string
	hc   *http.Client
}

// NewClient создаёт клиент с таймаутами.
func NewClient(opt Options) *Client {
	c := &Client{
		base: strings.TrimRight(opt.BaseURL, "/"),
		geo:  strings.TrimRight(opt.GeocodingURL, "/"),
		tz:   opt.Timezone,
		hc:   opt.HTTPClient,
	}
	if c.tz == "" {
		c.tz = DefaultTimezone
	}
	if c.base == "" {
		c.base = DefaultBaseURL
	}
	if c.geo == "" {
		c.geo = DefaultGeocodingURL
	}
	if c.hc == nil {
		c.hc = &http.Client{Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: callTimeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          2,
			ForceAttemptHTTP2:     true,
		}}
	}
	return c
}

// get запрашивает адрес и возвращает тело (не больше maxResponseBody).
// Ошибки очищены от адреса запроса: в нём координаты и поисковая фраза.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: запрос", ErrUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, fmt.Errorf("%w: чтение ответа: %v", ErrUnavailable, err)
	}
	if len(raw) > maxResponseBody {
		return nil, fmt.Errorf("%w: ответ слишком большой", ErrUnavailable)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	return raw, nil
}

func coord(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

// Forecast запрашивает почасовой прогноз на 7 дней.
func (c *Client) Forecast(ctx context.Context, lat, lon float64) (Forecast, error) {
	q := url.Values{}
	q.Set("latitude", coord(lat))
	q.Set("longitude", coord(lon))
	q.Set("hourly", "temperature_2m,precipitation_probability,precipitation,relative_humidity_2m,wind_speed_10m,weather_code")
	q.Set("daily", "temperature_2m_min,temperature_2m_max,precipitation_sum,precipitation_probability_max,weather_code")
	q.Set("timezone", c.tz)
	q.Set("forecast_days", strconv.Itoa(fetchDays))
	q.Set("wind_speed_unit", "ms")
	raw, err := c.get(ctx, c.base+"/v1/forecast?"+q.Encode())
	if err != nil {
		return Forecast{}, err
	}
	return ParseForecast(raw)
}

// Search ищет город по названию (геокодер Open-Meteo, русские названия).
func (c *Client) Search(ctx context.Context, name string) ([]Found, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("count", strconv.Itoa(searchLimit))
	q.Set("language", "ru")
	raw, err := c.get(ctx, c.geo+"/v1/search?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []struct {
			Name      string  `json:"name"`
			Admin1    string  `json:"admin1"`
			Country   string  `json:"country"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%w: неожиданный ответ геокодера", ErrUnavailable)
	}
	out := make([]Found, 0, len(r.Results))
	for _, p := range r.Results {
		if p.Name == "" || !validCoord(p.Latitude, p.Longitude) {
			continue
		}
		out = append(out, Found{Name: p.Name, Admin1: p.Admin1, Country: p.Country, Lat: p.Latitude, Lon: p.Longitude})
	}
	return out, nil
}

// ValidCoord — широта в пределах ±90, долгота ±180, числа конечные.
func ValidCoord(lat, lon float64) bool { return validCoord(lat, lon) }

func validCoord(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && math.Abs(lat) <= 90 && math.Abs(lon) <= 180
}

// ParseForecast разбирает ответ /v1/forecast. Часы с неизвестной температурой
// пропускаются, прочие пустые значения считаются нулём, а пустые осадки помечаются NoPrecip.
func ParseForecast(raw []byte) (Forecast, error) {
	var r struct {
		Timezone  string `json:"timezone"`
		UTCOffset int    `json:"utc_offset_seconds"`
		Hourly    struct {
			Time     []string   `json:"time"`
			Temp     []*float64 `json:"temperature_2m"`
			Prob     []*float64 `json:"precipitation_probability"`
			Precip   []*float64 `json:"precipitation"`
			Humidity []*float64 `json:"relative_humidity_2m"`
			Wind     []*float64 `json:"wind_speed_10m"`
			Code     []*float64 `json:"weather_code"`
		} `json:"hourly"`
		Daily struct {
			Time   []string   `json:"time"`
			TMin   []*float64 `json:"temperature_2m_min"`
			TMax   []*float64 `json:"temperature_2m_max"`
			Precip []*float64 `json:"precipitation_sum"`
			Prob   []*float64 `json:"precipitation_probability_max"`
			Code   []*float64 `json:"weather_code"`
		} `json:"daily"`
	}
	bad := fmt.Errorf("%w: неожиданный ответ прогноза", ErrUnavailable)
	if err := json.Unmarshal(raw, &r); err != nil {
		return Forecast{}, bad
	}
	h, d := r.Hourly, r.Daily
	n, m := len(h.Time), len(d.Time)
	if n == 0 || m == 0 || len(h.Temp) != n || len(h.Prob) != n || len(h.Precip) != n ||
		len(h.Humidity) != n || len(h.Wind) != n || len(h.Code) != n ||
		len(d.TMin) != m || len(d.TMax) != m || len(d.Precip) != m || len(d.Prob) != m || len(d.Code) != m {
		return Forecast{}, bad
	}
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil || r.Timezone == "" {
		loc = time.FixedZone("UTC"+strconv.Itoa(r.UTCOffset/3600), r.UTCOffset)
	}
	f := Forecast{Loc: loc, UTCOffset: r.UTCOffset}
	for i, ts := range h.Time {
		t, err := time.ParseInLocation("2006-01-02T15:04", ts, loc)
		if err != nil {
			return Forecast{}, bad
		}
		if h.Temp[i] == nil {
			continue
		}
		f.Hours = append(f.Hours, Hour{
			Time: t, Temp: *h.Temp[i], Precip: val(h.Precip[i]), Prob: int(math.Round(val(h.Prob[i]))),
			Wind: val(h.Wind[i]), Humidity: int(math.Round(val(h.Humidity[i]))), Code: int(val(h.Code[i])),
			NoPrecip: h.Precip[i] == nil || h.Prob[i] == nil,
		})
	}
	for i, date := range d.Time {
		if _, err := time.Parse("2006-01-02", date); err != nil || d.TMin[i] == nil || d.TMax[i] == nil {
			return Forecast{}, bad
		}
		f.Days = append(f.Days, Day{
			Date: date, TMin: *d.TMin[i], TMax: *d.TMax[i], Precip: val(d.Precip[i]),
			Prob: int(math.Round(val(d.Prob[i]))), Code: int(val(d.Code[i])),
		})
	}
	if len(f.Hours) == 0 {
		return Forecast{}, bad
	}
	return f, nil
}

func val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
