// Команда garage — личный учёт автохимии и инструментов детейлинга.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/bot"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/config"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/web"
	webui "github.com/R1nG1n0/car-care-chemicals-inventory/web"
)

// version подставляется линковщиком: -X main.version=...
var version = "dev"

// dbFile — имя файла базы в data_dir.
const dbFile = "garage.db"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "garage: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "/etc/garage/config.json", "путь к файлу конфигурации")
	showVersion := flag.Bool("version", false, "показать версию и выйти")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	args := flag.Args()
	if len(args) > 0 && args[0] != "user" {
		usage()
		return fmt.Errorf("неизвестная команда %q", args[0])
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	level, _ := cfg.SlogLevel() // уровень уже проверен в config.Load
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("создание каталога данных %s: %w", cfg.DataDir, err)
	}
	db, err := openDB(log, filepath.Join(cfg.DataDir, dbFile))
	if err != nil {
		return err
	}
	defer db.Close()
	svc := auth.New(db.DB, auth.Options{})

	if len(args) > 0 {
		return runUser(context.Background(), svc, args[1:])
	}
	return serve(log, cfg, *cfgPath, svc, db)
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintln(out, `Использование:
  garage [-config путь]                     запустить сервер
  garage [-config путь] user add <логин> [-owner]
                                            создать пользователя (первый — владелец)
  garage [-config путь] user passwd <логин> задать новый пароль, все сессии сбрасываются
  garage [-config путь] user list           список пользователей
  garage -version                           показать версию

Флаги:`)
	flag.PrintDefaults()
}

// openDB открывает базу и применяет миграции с записью в лог.
func openDB(log *slog.Logger, path string) (*store.DB, error) {
	db, err := store.Open(path)
	if err != nil {
		return nil, err
	}
	applied, err := db.Migrate()
	for _, name := range applied {
		log.Info("применена миграция", "name", name)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func serve(log *slog.Logger, cfg config.Config, cfgPath string, svc *auth.Service, db *store.DB) error {
	log.Info("запуск garage", "version", version, "config", cfgPath)
	if cfg.InsecureCookies {
		log.Warn("insecure_cookies: cookie сессии без Secure — только для локальной разработки")
	}

	// Бот (ФТ-11) живёт рядом с сервером: пустой токен — бота нет. Недоступный
	// Telegram сайт не затрагивает: бот сам повторяет попытки.
	var tgBot *bot.Bot
	if cfg.Telegram.Enabled() {
		tgBot = bot.New(bot.Options{
			DB:      db.DB,
			API:     telegram.New(cfg.Telegram.Token, telegram.Options{Interface: cfg.Telegram.Interface}),
			Log:     log,
			SiteURL: cfg.Telegram.SiteURL,
		})
		log.Info("Telegram-бот включён", "interface", cfg.Telegram.Interface)
	}
	opts := web.Options{
		Log:             log,
		Version:         version,
		UI:              webui.Dist(),
		Auth:            svc,
		InsecureCookies: cfg.InsecureCookies,
		DB:              db.DB,
	}
	if tgBot != nil {
		opts.Telegram = tgBot // не присваивать nil-указатель: интерфейс стал бы непустым
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           web.New(opts).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	botDone := make(chan struct{})
	if tgBot != nil {
		go func() {
			defer close(botDone)
			tgBot.Run(ctx)
		}()
	} else {
		close(botDone)
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("сервер слушает", "addr", cfg.Listen)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		stop() // бот останавливается вместе с сервером
		<-botDone
		return fmt.Errorf("сервер: %w", err)
	case <-ctx.Done():
	}

	log.Info("остановка")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("остановка сервера: %w", err)
	}
	// Бот получил ту же отмену ctx; ждём недолго, getUpdates прерывается сразу.
	select {
	case <-botDone:
	case <-shutdownCtx.Done():
	}
	return nil
}
