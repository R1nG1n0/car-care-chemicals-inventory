package catalog

import (
	"errors"
	"testing"
)

// Имена справочников уникальны без учёта регистра, в том числе кириллицы
// (решение владельца): «Коробка Новая» и «коробка новая» — дубль, 409 name.
// Регистр своего же имени менять можно; у другого пользователя — можно.
func TestRefNamesCaseInsensitive(t *testing.T) {
	f := newFixture(t)
	wantTaken := func(what string, err error) {
		t.Helper()
		var e *Error
		if !errors.As(err, &e) || !errors.Is(err, ErrConflict) || e.Field != "name" {
			t.Fatalf("%s: %v, ожидался 409 с полем name", what, err)
		}
	}

	// Категории: «Шампуни» есть по умолчанию.
	_, err := f.svc.CreateCategory(f.ctx, f.u1, "шампуни", "blue")
	wantTaken("категория строчными", err)
	_, err = f.svc.CreateCategory(f.ctx, f.u1, "ШАМПУНИ", "blue")
	wantTaken("категория прописными", err)
	c, err := f.svc.CreateCategory(f.ctx, f.u1, "Глины", "plum")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.UpdateCategory(f.ctx, f.u1, c.ID, RefPatch{Name: ptr("шампуНИ")})
	wantTaken("переименование категории", err)
	if c, err = f.svc.UpdateCategory(f.ctx, f.u1, c.ID, RefPatch{Name: ptr("ГЛИНЫ")}); err != nil || c.Name != "ГЛИНЫ" {
		t.Fatalf("смена регистра своего имени: %v %+v", err, c)
	}

	// Места — в том числе архивные: имя занято, пока запись есть.
	l, err := f.svc.CreateLocation(f.ctx, f.u1, "Коробка Новая")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateLocation(f.ctx, f.u1, "коробка новая")
	wantTaken("место строчными", err)
	if _, err := f.svc.UpdateLocation(f.ctx, f.u1, l.ID, RefPatch{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateLocation(f.ctx, f.u1, "КОРОБКА НОВАЯ")
	wantTaken("место как архивное", err)
	l2, err := f.svc.CreateLocation(f.ctx, f.u1, "Полка")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.UpdateLocation(f.ctx, f.u1, l2.ID, RefPatch{Name: ptr("коробка НОВАЯ")})
	wantTaken("переименование места", err)

	// Магазины.
	s, err := f.svc.CreateStore(f.ctx, f.u1, "Автохим", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.CreateStore(f.ctx, f.u1, "АВТОХИМ", "")
	wantTaken("магазин прописными", err)
	s2, err := f.svc.CreateStore(f.ctx, f.u1, "Озон", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.UpdateStore(f.ctx, f.u1, s2.ID, RefPatch{Name: ptr("автохим")})
	wantTaken("переименование магазина", err)
	if s, err = f.svc.UpdateStore(f.ctx, f.u1, s.ID, RefPatch{Name: ptr("автохим")}); err != nil || s.Name != "автохим" {
		t.Fatalf("смена регистра своего магазина: %v %+v", err, s)
	}

	// У второго пользователя те же имена свободны.
	if _, err := f.svc.CreateLocation(f.ctx, f.u2, "коробка новая"); err != nil {
		t.Fatalf("место второго: %v", err)
	}
	if _, err := f.svc.CreateStore(f.ctx, f.u2, "АВТОХИМ", ""); err != nil {
		t.Fatalf("магазин второго: %v", err)
	}
}
