package web

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

func countPhotos(t *testing.T, e *catEnv, login string) int {
	t.Helper()
	var n int
	err := e.db.QueryRow(`SELECT COUNT(*) FROM product_photos p JOIN users u ON u.id = p.user_id WHERE u.login = ?`, login).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Правки по ревью экспорта и импорта: экспорт без фото, строгая сверка остатков,
// один импорт за раз, Clear-Site-Data при выходе, метка фото после удаления.

func TestExportWithAndWithoutPhotos(t *testing.T) {
	ce := catNewEnv(t)
	e := ce.env
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	id := trExport(t, a).Products[0].ID
	phPut(t, a, id, phJPEG(t, 40, 30))

	with := a.do(http.MethodGet, "/api/export.json", nil)
	expect(t, with, http.StatusOK, "экспорт с фото")
	if with.Header().Get("Content-Length") != "" {
		t.Fatal("экспорт потоком не должен знать длину заранее")
	}
	without := a.do(http.MethodGet, "/api/export.json?photos=0", nil)
	expect(t, without, http.StatusOK, "экспорт без фото")
	if !strings.Contains(without.Header().Get("Content-Disposition"), "-nophoto.json") {
		t.Fatalf("имя файла без фото: %q", without.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(with.Body.String(), `"photo":{`) || strings.Contains(without.Body.String(), `"photo"`) {
		t.Fatal("фото есть только в полном экспорте")
	}

	f1 := decode[transfer.File](t, with)
	f2 := decode[transfer.File](t, without)
	expect(t, stkSignIn(e, "friend").do(http.MethodPost, "/api/import", f1), http.StatusOK, "импорт файла с фото")
	expect(t, stkSignIn(e, "friend2").do(http.MethodPost, "/api/import", f2), http.StatusOK, "импорт файла без фото")
	if n := countPhotos(t, ce, "friend"); n != 1 {
		t.Fatalf("у friend фото: %d, ожидалось 1", n)
	}
	if n := countPhotos(t, ce, "friend2"); n != 0 {
		t.Fatalf("у friend2 фото: %d, ожидалось 0", n)
	}
}

func TestImportRejectsInconsistentStock(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	good := trExport(t, a)
	if len(good.Units) < 3 {
		t.Fatal("в сидах мало экземпляров")
	}
	sealed, open, finished := -1, -1, -1
	for i, u := range good.Units {
		switch {
		case u.Status == "sealed" && sealed < 0:
			sealed = i
		case u.Status == "open" && open < 0:
			open = i
		case u.Status != "sealed" && u.Status != "open" && finished < 0:
			finished = i
		}
	}
	if sealed < 0 || open < 0 || finished < 0 {
		t.Fatalf("нет экземпляров всех статусов: %d %d %d", sealed, open, finished)
	}
	cases := []struct {
		name  string
		field string
		idx   int
		patch func(u *transfer.Unit)
	}{
		{"остаток не сходится с движениями", "remaining_qty", sealed, func(u *transfer.Unit) { u.RemainingQty-- }},
		{"запечатанный с отметкой вскрытия", "opened_at", sealed, func(u *transfer.Unit) { v := int64(1); u.OpenedAt = &v }},
		{"запечатанный с неполным остатком", "remaining_qty", sealed, func(u *transfer.Unit) { u.RemainingQty = 1 }},
		{"вскрытый с нулевым остатком", "remaining_qty", open, func(u *transfer.Unit) { u.RemainingQty = 0 }},
		{"законченный с остатком", "remaining_qty", finished, func(u *transfer.Unit) { u.RemainingQty = 1 }},
		{"законченный без даты", "finished_at", finished, func(u *transfer.Unit) { u.FinishedAt = nil }},
	}
	for i, tc := range cases {
		f := trExport(t, a)
		tc.patch(&f.Units[tc.idx])
		rec := stkSignIn(e, "bad"+strconv.Itoa(i)+"user").do(http.MethodPost, "/api/import", f)
		expect(t, rec, http.StatusBadRequest, tc.name)
		want := "units[" + strconv.Itoa(tc.idx) + "]." + tc.field
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("%s: тело %s, ожидался путь %s", tc.name, rec.Body.String(), want)
		}
	}
	// Движение без экземпляра в сумме: лишнее движение тоже ломает сверку.
	f := trExport(t, a)
	f.Movements[0].QtyDelta += 7
	expect(t, stkSignIn(e, "badmove").do(http.MethodPost, "/api/import", f), http.StatusBadRequest, "лишний приход в движении")
}

func TestImportParallelOneWinner(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	file := trExport(t, a)
	b := stkSignIn(e, "friend")

	const n = 4
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = b.do(http.MethodPost, "/api/import", file).Code
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Fatalf("коды параллельных импортов %v: нужен ровно один 200, остальные 409", codes)
	}
	if got := trExport(t, b); len(got.Products) != len(file.Products) {
		t.Fatalf("позиций после гонки %d, в файле %d: дубль импорта", len(got.Products), len(file.Products))
	}
}

func TestLogoutClearsBrowserCache(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	rec := c.do(http.MethodPost, "/api/auth/logout", nil)
	expect(t, rec, http.StatusNoContent, "logout")
	if rec.Header().Get("Clear-Site-Data") != `"cache"` {
		t.Fatalf("Clear-Site-Data: %q", rec.Header().Get("Clear-Site-Data"))
	}
	c2 := stkSignIn(e, "owner2")
	rec = c2.do(http.MethodPost, "/api/auth/logout-all", nil)
	expect(t, rec, http.StatusNoContent, "logout-all")
	if rec.Header().Get("Clear-Site-Data") != `"cache"` {
		t.Fatalf("logout-all, Clear-Site-Data: %q", rec.Header().Get("Clear-Site-Data"))
	}
}

func TestPhotoVersionGrowsAfterDelete(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	id := phProduct(t, c, "Шампунь")
	first := phPut(t, c, id, phJPEG(t, 20, 20))
	expect(t, c.do(http.MethodDelete, phPath(id), nil), http.StatusOK, "удаление фото")
	second := phPut(t, c, id, phJPEG(t, 21, 21))
	if *second.PhotoUpdatedAt <= *first.PhotoUpdatedAt {
		t.Fatalf("метка после DELETE+PUT не выросла: %d после %d", *second.PhotoUpdatedAt, *first.PhotoUpdatedAt)
	}
}
