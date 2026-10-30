package weather

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/store"
)

func newServiceDB(t *testing.T, def *Place) (*Service, func(login string) int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	user := func(login string) int64 {
		res, err := db.Exec(`INSERT INTO users (login, password_hash) VALUES (?, 'x')`, login)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	return NewService(ServiceOptions{DB: db.DB, Default: def}), user
}

func TestServicePlaces(t *testing.T) {
	ctx := context.Background()
	svc, user := newServiceDB(t, &Place{Name: "Донецк", Lat: 48.023, Lon: 37.802, Custom: true})
	a, b := user("a"), user("b")

	pl, ok, err := svc.PlaceFor(ctx, a)
	if err != nil || !ok || pl.Name != "Донецк" || pl.Custom {
		t.Fatalf("по умолчанию: %+v %v %v", pl, ok, err)
	}
	if _, err := svc.SetPlace(ctx, a, Place{Name: " Ростов ", Lat: 47.2, Lon: 39.7}); err != nil {
		t.Fatal(err)
	}
	if pl, _, _ := svc.PlaceFor(ctx, a); pl.Name != "Ростов" || !pl.Custom || pl.Lat != 47.2 {
		t.Fatalf("своё: %+v", pl)
	}
	if pl, _, _ := svc.PlaceFor(ctx, b); pl.Custom {
		t.Fatalf("чужое место: %+v", pl)
	}
	if err := svc.ResetPlace(ctx, a); err != nil {
		t.Fatal(err)
	}
	if pl, _, _ := svc.PlaceFor(ctx, a); pl.Name != "Донецк" {
		t.Fatalf("после сброса: %+v", pl)
	}
}

func TestServiceDisabledWithoutDefault(t *testing.T) {
	svc, user := newServiceDB(t, nil)
	if _, ok, err := svc.PlaceFor(context.Background(), user("a")); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestValidName(t *testing.T) {
	for name, ok := range map[string]bool{"Донецк": true, "": false, "   ": false, "a\x00b": false, "я": true} {
		if ValidName(name) != ok {
			t.Errorf("ValidName(%q) = %v", name, !ok)
		}
	}
	if ValidName(strings.Repeat("я", MaxNameRunes+1)) {
		t.Error("длинное имя принято")
	}
}
