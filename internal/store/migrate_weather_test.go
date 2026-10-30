package store

import (
	"path/filepath"
	"testing"
)

// Миграция 009: место погоды пользователя — одна строка на пользователя,
// координаты проверяются, при удалении пользователя строка исчезает.
func TestMigrationWeatherPlaces(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (login, password_hash) VALUES ('a', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (1, 'Донецк', 48.023, 37.802)`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (1, 'Дубль', 1, 1)`,
		`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (2, 'Нет пользователя', 1, 1)`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("принято: %s", q)
		}
	}
	if _, err := db.Exec(`INSERT INTO users (login, password_hash) VALUES ('b', 'x')`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (2, 'X', 91, 0)`,
		`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (2, 'X', 0, 181)`,
		`INSERT INTO weather_places (user_id, name, lat, lon) VALUES (2, '', 0, 0)`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("принято: %s", q)
		}
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM weather_places`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("после удаления пользователя мест: %d, %v", n, err)
	}
}
