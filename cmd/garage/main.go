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
	"syscall"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/config"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/web"
	webui "github.com/R1nG1n0/car-care-chemicals-inventory/web"
)

// version подставляется линковщиком: -X main.version=...
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "garage: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "/etc/garage/config.json", "путь к файлу конфигурации")
	showVersion := flag.Bool("version", false, "показать версию и выйти")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	level, _ := cfg.SlogLevel() // уровень уже проверен в config.Load
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	log.Info("запуск garage", "version", version, "config", *cfgPath)

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("создание каталога данных %s: %w", cfg.DataDir, err)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           web.New(log, version, webui.Dist()).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("сервер слушает", "addr", cfg.Listen)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("сервер: %w", err)
	case <-ctx.Done():
	}

	log.Info("остановка")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("остановка сервера: %w", err)
	}
	return nil
}
