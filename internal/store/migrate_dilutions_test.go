package store

import (
	"path/filepath"
	"testing"
)

// Миграция 010: пропорции позиции с проверкой диапазонов и каскадом,
// раствор в движениях и строках шаблонов.
func TestMigrationDilutions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (login, password_hash) VALUES ('a', 'x')`,
		`INSERT INTO users (login, password_hash) VALUES ('b', 'x')`,
		`INSERT INTO categories (user_id, name, color) VALUES (1, 'К', 'blue')`,
		`INSERT INTO products (user_id, category_id, name, unit, package_size) VALUES (1, 1, 'П', 'ml', 100)`,
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio, sort) VALUES (1, 1, 'Салон', 10, 0)`,
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio, sort) VALUES (1, 1, 'Край', 1000, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio) VALUES (1, 1, '', 5)`,
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio) VALUES (1, 1, 'x', 0)`,
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio) VALUES (1, 1, 'x', 1001)`,
		`INSERT INTO product_dilutions (user_id, product_id, name, ratio) VALUES (2, 1, 'чужая позиция', 5)`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("принято: %s", q)
		}
	}
	// Колонки раствора есть и проверяются.
	if _, err := db.Exec(`INSERT INTO recipes (user_id, name) VALUES (1, 'Р')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO recipe_items (user_id, recipe_id, product_id, qty, solution_ml, solution_ratio)
		VALUES (1, 1, 1, 91, 1000, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO recipe_items (user_id, recipe_id, product_id, qty, solution_ml, solution_ratio)
		VALUES (1, 1, 1, 91, 0, 10)`); err == nil {
		t.Fatal("принят объём раствора 0")
	}
	if _, err := db.Exec(`DELETE FROM recipe_items`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM products WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM product_dilutions`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("пропорций после удаления позиции: %d, %v", n, err)
	}
}
