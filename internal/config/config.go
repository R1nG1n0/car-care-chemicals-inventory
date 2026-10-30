// Package config загружает и проверяет конфигурацию garage.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Config — параметры запуска.
type Config struct {
	// Listen — адрес, на котором принимаются соединения. На relay это
	// только loopback: снаружи сайт доступен через nginx на адресе VPN (Р5.1).
	Listen string `json:"listen"`

	// DataDir — каталог базы данных и снимков.
	DataDir string `json:"data_dir"`

	// LogLevel — debug | info | warn | error.
	LogLevel string `json:"log_level"`

	// InsecureCookies убирает у cookie сессии флаг Secure. Только для
	// локальной разработки по http: на relay сайт работает через HTTPS.
	InsecureCookies bool `json:"insecure_cookies"`

	// Telegram — бот (ФТ-11). Пустой токен отключает бота.
	Telegram TelegramConfig `json:"telegram"`

	// Weather — погода для мойки и покрытий (ФТ-16).
	Weather WeatherConfig `json:"weather"`
}

// WeatherConfig — настройки погоды. Нет места по умолчанию — погода
// показывается только тем, кто выбрал своё место.
type WeatherConfig struct {
	// Default — место по умолчанию (на relay — Донецк).
	Default *WeatherPlace `json:"default"`

	// BaseURL и GeocodingURL — адреса Open-Meteo; пусто — стандартные.
	// Задаются только для тестов и стендов.
	BaseURL      string `json:"base_url"`
	GeocodingURL string `json:"geocoding_url"`

	// Timezone — часовой пояс прогноза для всех мест (IANA, например
	// "Europe/Moscow", или "auto" — по координатам). Пусто — Europe/Moscow.
	Timezone string `json:"timezone"`
}

// WeatherPlace — место с координатами.
type WeatherPlace struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// TelegramConfig — настройки Telegram-бота.
type TelegramConfig struct {
	// Token — токен бота. Хранится только в файле конфигурации и никогда не
	// попадает в журнал и в тексты ошибок (ФТ-11.1.3).
	Token string `json:"token"`

	// Interface — сетевой интерфейс, к которому привязываются соединения
	// бота (SO_BINDTODEVICE, на relay — туннель awg1). Пусто — без привязки.
	Interface string `json:"interface"`

	// SiteURL — необязательный адрес сайта: бот даёт на него ссылку, когда
	// штрихкод не найден (ФТ-11.4.3). Должен начинаться с https://.
	SiteURL string `json:"site_url"`
}

// Enabled — бот включён (токен задан).
func (t TelegramConfig) Enabled() bool { return t.Token != "" }

// String не выдаёт токен, даже если конфигурацию случайно напечатают (%v, %+v).
func (t TelegramConfig) String() string {
	if t.Token == "" {
		return "telegram{выключен}"
	}
	return "telegram{токен скрыт, interface=" + t.Interface + "}"
}

// GoString — то же для %#v.
func (t TelegramConfig) GoString() string { return t.String() }

// Default — значения для полей, которых нет в файле.
func Default() Config {
	return Config{
		Listen:   "127.0.0.1:8090",
		DataDir:  "/var/lib/garage",
		LogLevel: "info",
	}
}

// Load читает файл конфигурации и проверяет значения.
// Относительный data_dir считается от каталога файла конфигурации.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("чтение конфигурации: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("разбор конфигурации %s: %w", path, err)
	}
	if cfg.DataDir != "" && !filepath.IsAbs(cfg.DataDir) {
		cfg.DataDir = filepath.Join(filepath.Dir(path), cfg.DataDir)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("конфигурация %s: %w", path, err)
	}
	return cfg, nil
}

// Validate проверяет значения полей.
func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen %q: ожидается адрес:порт", c.Listen)
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("listen %q: нужен IP-адрес, а не имя", c.Listen)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("listen %q: неверный порт", c.Listen)
	}
	if c.DataDir == "" {
		return errors.New("data_dir не задан")
	}
	if _, err := c.SlogLevel(); err != nil {
		return err
	}
	if len(c.Telegram.Interface) > 15 || strings.ContainsAny(c.Telegram.Interface, " /\x00") {
		return fmt.Errorf("telegram.interface %q: неверное имя интерфейса", c.Telegram.Interface)
	}
	if u := c.Telegram.SiteURL; u != "" && !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("telegram.site_url %q: нужен адрес https://…", u)
	}
	return c.Weather.validate()
}

func (w WeatherConfig) validate() error {
	if d := w.Default; d != nil {
		if n := utf8.RuneCountInString(strings.TrimSpace(d.Name)); n < 1 || n > 80 {
			return errors.New("weather.default.name: от 1 до 80 символов")
		}
		if math.IsNaN(d.Lat) || d.Lat < -90 || d.Lat > 90 {
			return fmt.Errorf("weather.default.lat %v: широта от -90 до 90", d.Lat)
		}
		if math.IsNaN(d.Lon) || d.Lon < -180 || d.Lon > 180 {
			return fmt.Errorf("weather.default.lon %v: долгота от -180 до 180", d.Lon)
		}
	}
	if tz := w.Timezone; tz != "" && tz != "auto" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("weather.timezone %q: неизвестный часовой пояс (нужен вида Europe/Moscow или auto)", tz)
		}
	}
	for name, u := range map[string]string{"base_url": w.BaseURL, "geocoding_url": w.GeocodingURL} {
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return fmt.Errorf("weather.%s %q: нужен адрес http(s)://…", name, u)
		}
	}
	return nil
}

// SlogLevel переводит log_level в уровень slog.
func (c Config) SlogLevel() (slog.Level, error) {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("log_level %q: ожидается debug, info, warn или error", c.LogLevel)
}
