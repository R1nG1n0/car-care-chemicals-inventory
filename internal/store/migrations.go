package store

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate применяет непрокатанные миграции по возрастанию имени файла
// и возвращает список применённых. Каждая миграция выполняется в своей
// транзакции: упавшая не оставляет схему в половинчатом состоянии.
func (db *DB) Migrate() ([]string, error) {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return nil, fmt.Errorf("создание таблицы schema_migrations: %w", err)
	}

	done, err := db.appliedMigrations()
	if err != nil {
		return nil, err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("чтение встроенных миграций: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var applied []string
	for _, name := range names {
		if done[name] {
			continue
		}
		if err := db.applyMigration(name); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}

	return applied, nil
}

func (db *DB) appliedMigrations() (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("чтение применённых миграций: %w", err)
	}
	defer rows.Close()

	done := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("чтение применённых миграций: %w", err)
		}
		done[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение применённых миграций: %w", err)
	}

	return done, nil
}

func (db *DB) applyMigration(name string) error {
	body, err := migrationFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("чтение миграции %s: %w", name, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("миграция %s: начало транзакции: %w", name, err)
	}
	defer tx.Rollback() //nolint:errcheck // после успешного Commit это no-op

	if _, err := tx.Exec(string(body)); err != nil {
		return fmt.Errorf("миграция %s: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("миграция %s: отметка о применении: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("миграция %s: фиксация: %w", name, err)
	}

	return nil
}
