package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Fatalf("ожидались значения по умолчанию, получено %+v", cfg)
	}
}

func TestLoadRelativeDataDir(t *testing.T) {
	p := writeConfig(t, `{"data_dir": ".dev"}`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(filepath.Dir(p), ".dev"); cfg.DataDir != want {
		t.Fatalf("data_dir = %q, ожидался %q", cfg.DataDir, want)
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]string{
		"не JSON":          `{`,
		"без порта":        `{"listen": "127.0.0.1"}`,
		"имя вместо IP":    `{"listen": "localhost:8090"}`,
		"порт вне границ":  `{"listen": "127.0.0.1:70000"}`,
		"пустой data_dir":  `{"data_dir": ""}`,
		"неверный уровень": `{"log_level": "trace"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body)); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "нет.json")); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestInsecureCookies(t *testing.T) {
	if Default().InsecureCookies {
		t.Fatal("по умолчанию cookie должны быть Secure")
	}
	cfg, err := Load(writeConfig(t, `{"insecure_cookies": true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureCookies {
		t.Fatal("insecure_cookies не прочитан")
	}
	if _, err := Load(writeConfig(t, `{"insecure_cookies": "yes"}`)); err == nil {
		t.Fatal("строка вместо bool принята")
	}
}

func TestLoadTelegram(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{"telegram": {"token": "123:secret", "interface": "awg1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Telegram.Enabled() || cfg.Telegram.Token != "123:secret" || cfg.Telegram.Interface != "awg1" {
		t.Fatalf("telegram = %+v", cfg.Telegram)
	}
	// Токен не выводится при печати конфигурации.
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(f, cfg); strings.Contains(out, "secret") {
			t.Fatalf("%s печатает токен: %s", f, out)
		}
	}
}

func TestLoadTelegramDisabledByDefault(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.Enabled() {
		t.Fatal("без токена бот должен быть выключен")
	}
}

func TestLoadTelegramBadInterface(t *testing.T) {
	for _, body := range []string{
		`{"telegram": {"interface": "очень-длинное-имя-интерфейса"}}`,
		`{"telegram": {"interface": "a b"}}`,
	} {
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Fatalf("ожидалась ошибка для %s", body)
		}
	}
}
