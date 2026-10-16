package store

import (
	"path/filepath"
	"testing"
)

// 002 применяется к рабочей базе, где уже есть 001 с данными: у старых
// движений batch_id = id, новые столбец и индекс на месте.
func TestMigrationBatchOnExistingData(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// База «до обновления»: только 001.
	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	if err := db.applyMigration("001_init.sql"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, login, password_hash) VALUES (1, 'owner', 'h')`,
		`INSERT INTO categories (id, user_id, name, color) VALUES (10, 1, 'Шампуни', 'blue')`,
		`INSERT INTO products (id, user_id, category_id, name, unit, package_size) VALUES (30, 1, 10, 'Gsf', 'ml', 1000)`,
		`INSERT INTO units (id, user_id, product_id, initial_qty, remaining_qty) VALUES (40, 1, 30, 1000, 900)`,
		`INSERT INTO movements (id, user_id, unit_id, type, qty_delta, created_at) VALUES
			(7, 1, 40, 'adjust', 1000, 100), (8, 1, 40, 'consume', -60, 200), (9, 1, 40, 'consume', -40, 200)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	applied, err := db.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(applied) == 0 || applied[0] != "002_movement_batch.sql" {
		t.Fatalf("применены %v, ожидалась 002_movement_batch.sql первой", applied)
	}
	var bad int
	if err := db.QueryRow(`SELECT COUNT(*) FROM movements WHERE batch_id IS NULL OR batch_id <> id`).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Fatalf("старых движений без своей группы: %d", bad)
	}
	var idx int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'movements_batch'`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatal("нет индекса movements_batch")
	}
	// Данные целы, новые движения пишутся с группой.
	if _, err := db.Exec(`INSERT INTO movements (user_id, unit_id, type, qty_delta, batch_id) VALUES (1, 40, 'consume', -1, 8)`); err != nil {
		t.Fatal(err)
	}
}
