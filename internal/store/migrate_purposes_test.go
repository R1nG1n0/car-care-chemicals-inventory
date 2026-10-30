package store

import (
	"path/filepath"
	"testing"
)

// 008 применяется к рабочей базе с данными: каждому существующему
// пользователю засевается стартовый набор назначений, позиции получают
// пустые новые поля, ограничения pH и оценки работают.
func TestMigrationPurposesSeedsExistingUsers(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"001_init.sql", "002_movement_batch.sql", "003_location_checked.sql",
		"004_purchase_lines_purchase_idx.sql", "005_product_photos.sql", "006_telegram.sql", "007_client_ops.sql"} {
		if err := db.applyMigration(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO users (id, login, password_hash) VALUES (1, 'owner', 'h'), (2, 'friend', 'h')`,
		`INSERT INTO categories (id, user_id, name, color) VALUES (10, 1, 'Шампуни', 'blue')`,
		`INSERT INTO products (id, user_id, category_id, name, unit, package_size) VALUES (30, 1, 10, 'Gsf', 'ml', 1000)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	applied, err := db.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(applied) == 0 || applied[0] != "008_purposes.sql" {
		t.Fatalf("применены %v, ожидалась первой 008_purposes.sql", applied)
	}
	for _, user := range []int64{1, 2} {
		var n int
		var names string
		if err := db.QueryRow(`SELECT COUNT(*), group_concat(name, ',') FROM
			(SELECT name FROM purposes WHERE user_id = ? ORDER BY sort)`, user).Scan(&n, &names); err != nil {
			t.Fatal(err)
		}
		if n != 7 || names != "ЛКП,стекло,пластик,кожа,диски,шины,текстиль" {
			t.Fatalf("у пользователя %d назначения: %d %q", user, n, names)
		}
	}
	var ph, rating *int64
	var inc, ins string
	var dnb bool
	if err := db.QueryRow(`SELECT ph_x10, incompatible, instruction, rating, do_not_buy FROM products WHERE id = 30`).
		Scan(&ph, &inc, &ins, &rating, &dnb); err != nil || ph != nil || rating != nil || inc != "" || ins != "" || dnb {
		t.Fatalf("новые поля существующей позиции: %v %v %q %q %v %v", ph, rating, inc, ins, dnb, err)
	}
	for _, q := range []string{
		`UPDATE products SET ph_x10 = 141 WHERE id = 30`,
		`UPDATE products SET rating = 6 WHERE id = 30`,
		`UPDATE products SET rating = 0 WHERE id = 30`,
		// Связь с назначением другого пользователя запрещена составным ключом.
		`INSERT INTO product_purposes (user_id, product_id, purpose_id)
			SELECT 1, 30, id FROM purposes WHERE user_id = 2 LIMIT 1`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("база приняла %s", q)
		}
	}
}
