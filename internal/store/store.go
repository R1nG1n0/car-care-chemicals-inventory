// Package store отвечает за базу данных garage и её схему.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite" // драйвер на чистом Go: без cgo, кросс-компиляция без тулчейнов
)

// DB — обёртка над *sql.DB с методами обслуживания схемы.
type DB struct {
	*sql.DB
}

// Open открывает базу данных, создавая файл при необходимости.
func Open(path string) (*DB, error) {
	dsn := "file:" + url.PathEscape(path) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=synchronous(NORMAL)"

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("открытие БД %s: %w", path, err)
	}

	// Одно соединение снимает вопрос «database is locked»: нагрузка —
	// единицы запросов в минуту, сериализация ничего не стоит.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("проверка соединения с БД %s: %w", path, err)
	}

	// В базе хеши паролей и токенов. SQLite создаёт файл с учётом umask,
	// поэтому режим задаём явно.
	if err := os.Chmod(path, 0o600); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("установка прав на файл БД %s: %w", path, err)
	}

	return &DB{DB: sqlDB}, nil
}
