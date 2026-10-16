package store

import (
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTest(t)
	applied, err := db.Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("повторный запуск применил %v", applied)
	}
}

func TestFileMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "garage.db")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("права на файл БД %v, ожидались 0600", st.Mode().Perm())
	}
}

func TestForeignKeysOn(t *testing.T) {
	db := openTest(t)
	var on int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatal("внешние ключи выключены")
	}
}

// Сослаться на справочник другого пользователя нельзя на уровне базы (ТБ-5).
func TestCrossUserReferenceRejected(t *testing.T) {
	db := openTest(t)
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, login, password_hash) VALUES (1, 'owner', 'h'), (2, 'friend', 'h')`)
	mustExec(`INSERT INTO categories (id, user_id, name, color) VALUES (10, 1, 'Шампуни', 'blue')`)
	mustExec(`INSERT INTO locations (id, user_id, name) VALUES (20, 1, 'Коробка 3')`)

	// Своя категория — можно.
	mustExec(`INSERT INTO products (user_id, category_id, name, unit, package_size) VALUES (1, 10, 'Koch Chemie Gsf', 'ml', 1000)`)

	// Чужая категория — нельзя.
	if _, err := db.Exec(`INSERT INTO products (user_id, category_id, name, unit, package_size)
		VALUES (2, 10, 'Чужая', 'ml', 500)`); err == nil {
		t.Fatal("позиция второго пользователя сослалась на категорию первого")
	}

	// Чужое место хранения у экземпляра — нельзя.
	mustExec(`INSERT INTO categories (id, user_id, name, color) VALUES (11, 2, 'Шампуни', 'blue')`)
	mustExec(`INSERT INTO products (id, user_id, category_id, name, unit, package_size) VALUES (31, 2, 11, 'Своя', 'ml', 500)`)
	if _, err := db.Exec(`INSERT INTO units (user_id, product_id, location_id, initial_qty, remaining_qty)
		VALUES (2, 31, 20, 500, 500)`); err == nil {
		t.Fatal("экземпляр второго пользователя положен в место первого")
	}
}

func TestConstraints(t *testing.T) {
	db := openTest(t)
	if _, err := db.Exec(`INSERT INTO users (id, login, password_hash) VALUES (1, 'owner', 'h')`); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"цвет вне палитры":         `INSERT INTO categories (user_id, name, color) VALUES (1, 'X', 'pink')`,
		"неизвестная единица":      `INSERT INTO categories (id, user_id, name, color) VALUES (5, 1, 'Y', 'blue'); INSERT INTO products (user_id, category_id, name, unit, package_size) VALUES (1, 5, 'P', 'l', 1)`,
		"логин без учёта регистра": `INSERT INTO users (login, password_hash) VALUES ('OWNER', 'h')`,
		"дата не в формате":        `INSERT INTO purchases (user_id, date) VALUES (1, '15.10.2026')`,
	}
	for name, q := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := db.Exec(q); err == nil {
				t.Fatal("ожидалась ошибка ограничения")
			}
		})
	}
}
