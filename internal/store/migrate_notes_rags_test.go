package store

import (
	"path/filepath"
	"testing"
)

// Миграция 011: заметки со связями, ресурс позиции и тряпки — диапазоны,
// изоляция по пользователю и каскады.
func TestMigrationNotesRags(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, ok bool) {
		t.Helper()
		_, err := db.Exec(q)
		if ok && err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if !ok && err == nil {
			t.Fatalf("принято: %s", q)
		}
	}
	for _, q := range []string{
		`INSERT INTO users (login, password_hash) VALUES ('a', 'x')`,
		`INSERT INTO users (login, password_hash) VALUES ('b', 'x')`,
		`INSERT INTO categories (user_id, name, color) VALUES (1, 'К', 'blue')`,
		`INSERT INTO products (user_id, category_id, name, unit, package_size, wear_limit) VALUES (1, 1, 'Тряпка', 'pcs', 1, 100)`,
		`INSERT INTO recipes (user_id, name) VALUES (1, 'Р')`,
		`INSERT INTO notes (user_id, title, body, created_at, updated_at) VALUES (1, 'З', '', 1, 1)`,
		`INSERT INTO note_products (user_id, note_id, product_id) VALUES (1, 1, 1)`,
		`INSERT INTO note_recipes (user_id, note_id, recipe_id) VALUES (1, 1, 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, created_at) VALUES (1, 1, '№ 1', 1, 1)`,
	} {
		exec(q, true)
	}
	for _, q := range []string{
		`INSERT INTO notes (user_id, title, created_at, updated_at) VALUES (1, '', 1, 1)`,
		`INSERT INTO products (user_id, category_id, name, unit, package_size, wear_limit) VALUES (1, 1, 'Н', 'pcs', 1, 0)`,
		`INSERT INTO products (user_id, category_id, name, unit, package_size, wear_limit) VALUES (1, 1, 'Н', 'pcs', 1, 1001)`,
		`INSERT INTO note_products (user_id, note_id, product_id) VALUES (2, 1, 1)`, // чужие заметка и позиция
		`INSERT INTO rags (user_id, product_id, label, number, created_at) VALUES (2, 1, 'x', 1, 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, created_at) VALUES (1, 1, '', 2, 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, created_at) VALUES (1, 1, 'яяяяяяяяяяяяяяяяяяяяя', 2, 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, washes, created_at) VALUES (1, 1, 'x', 2, -1, 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, status, created_at) VALUES (1, 1, 'x', 2, 'lost', 1)`,
		`INSERT INTO rags (user_id, product_id, label, number, retire_reason, created_at) VALUES (1, 1, 'x', 2, 'sold', 1)`,
	} {
		exec(q, false)
	}
	exec(`INSERT INTO products (user_id, category_id, name, unit, package_size, wear_limit) VALUES (1, 1, 'Г', 'pcs', 1, 1000)`, true)

	// Удаление позиции и шаблона убирает связи и тряпки, заметка остаётся.
	exec(`DELETE FROM recipes WHERE id = 1`, true)
	exec(`DELETE FROM products WHERE id = 1`, true)
	var notes, links, rags int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM notes), (SELECT COUNT(*) FROM note_products)
		+ (SELECT COUNT(*) FROM note_recipes), (SELECT COUNT(*) FROM rags)`).Scan(&notes, &links, &rags); err != nil {
		t.Fatal(err)
	}
	if notes != 1 || links != 0 || rags != 0 {
		t.Fatalf("после каскада: заметок %d, связей %d, тряпок %d", notes, links, rags)
	}
	// Удаление заметки убирает связи.
	exec(`INSERT INTO products (user_id, category_id, name, unit, package_size) VALUES (1, 1, 'П', 'ml', 1)`, true)
	exec(`INSERT INTO note_products (user_id, note_id, product_id) VALUES (1, 1, 3)`, true)
	exec(`DELETE FROM notes WHERE id = 1`, true)
	if err := db.QueryRow(`SELECT COUNT(*) FROM note_products`).Scan(&links); err != nil || links != 0 {
		t.Fatalf("связи после удаления заметки: %d, %v", links, err)
	}
}
