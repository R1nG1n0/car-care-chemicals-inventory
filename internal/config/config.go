// Package config загружает и проверяет конфигурацию garage.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
}

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
