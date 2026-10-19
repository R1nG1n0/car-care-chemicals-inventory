package web

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/photo"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

// Фото позиции (ФТ-10.4): адреса, проверка содержимого, кэш, изоляция.

// phSend отправляет сырые байты от имени клиента; ctype — заголовок Content-Type.
func phSend(c *client, method, target string, body []byte, ctype string) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.RemoteAddr = c.remoteAddr
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("X-Requested-With", "garage")
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if c.token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.token})
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	return rec
}

func phJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 255, A: 255})
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func phPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(1, 1, color.NRGBA{G: 200, A: 80})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// phWebP — заголовок VP8L нужного размера (сервер читает только заголовок).
func phWebP(w, h int) []byte {
	d := make([]byte, 40)
	copy(d, "RIFF")
	copy(d[8:], "WEBPVP8L")
	d[20] = 0x2f
	bits := uint32(w-1) | uint32(h-1)<<14
	d[21], d[22], d[23], d[24] = byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24)
	return d
}

func phProduct(t *testing.T, c *client, name string) int64 {
	t.Helper()
	return catProduct(t, c, map[string]any{"category_id": catFirstCategory(t, c), "name": name,
		"unit": "ml", "package_size": 500}).ID
}

func phPath(id int64) string { return "/api/products/" + strconv.FormatInt(id, 10) + "/photo" }

func phPut(t *testing.T, c *client, id int64, data []byte) catalog.ProductSummary {
	t.Helper()
	rec := phSend(c, http.MethodPut, phPath(id), data, "application/octet-stream")
	expect(t, rec, http.StatusOK, "PUT фото")
	return decode[catalog.ProductSummary](t, rec)
}

func TestPhotoLifecycle(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	id := phProduct(t, c, "Шампунь")

	if p := catalogOne(t, c, id); p.PhotoUpdatedAt != nil {
		t.Fatalf("до загрузки photo_updated_at = %v", *p.PhotoUpdatedAt)
	}
	expect(t, c.do(http.MethodGet, phPath(id), nil), http.StatusNotFound, "фото ещё нет")

	data := phJPEG(t, 120, 80)
	// Content-Type запроса игнорируется: тип берётся из содержимого.
	rec := phSend(c, http.MethodPut, phPath(id), data, "text/plain")
	expect(t, rec, http.StatusOK, "PUT jpeg")
	put := decode[catalog.ProductSummary](t, rec)
	if put.PhotoUpdatedAt == nil || *put.PhotoUpdatedAt != e.clock.Now().Unix() || put.ID != id {
		t.Fatalf("ответ PUT: %+v", put)
	}
	if p := catalogOne(t, c, id); p.PhotoUpdatedAt == nil {
		t.Fatal("в списке нет photo_updated_at")
	}

	rec = phSend(c, http.MethodGet, phPath(id)+"?v=1", nil, "")
	expect(t, rec, http.StatusOK, "GET фото")
	h := rec.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("Cache-Control") != "private, max-age=31536000, immutable" ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Vary") != "Cookie" || h.Get("Content-Length") != strconv.Itoa(len(data)) {
		t.Fatalf("заголовки: %v", h)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatal("байты фото изменились")
	}
	if head := phSend(c, http.MethodHead, phPath(id), nil, ""); head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD: код %d, тело %d байт", head.Code, head.Body.Len())
	}

	// Замена в ту же секунду всё равно меняет метку: иначе кэш ?v= отдал бы старое фото.
	png2 := phPNG(t, 10, 10)
	again := phPut(t, c, id, png2)
	if *again.PhotoUpdatedAt <= *put.PhotoUpdatedAt {
		t.Fatalf("метка не выросла: %d после %d", *again.PhotoUpdatedAt, *put.PhotoUpdatedAt)
	}
	rec = c.do(http.MethodGet, phPath(id), nil)
	if rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), png2) {
		t.Fatalf("после замены: %s, %d байт", rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	webp := phWebP(300, 200)
	phPut(t, c, id, webp)
	if rec = c.do(http.MethodGet, phPath(id), nil); rec.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("webp: %s", rec.Header().Get("Content-Type"))
	}

	del := phSend(c, http.MethodDelete, phPath(id), nil, "")
	expect(t, del, http.StatusOK, "DELETE фото")
	if decode[catalog.ProductSummary](t, del).PhotoUpdatedAt != nil {
		t.Fatal("после удаления метка осталась")
	}
	expect(t, c.do(http.MethodGet, phPath(id), nil), http.StatusNotFound, "после удаления")
	expect(t, phSend(c, http.MethodDelete, phPath(id), nil, ""), http.StatusOK, "повторное удаление без фото")
	if p := catalogOne(t, c, id); p.PhotoUpdatedAt != nil {
		t.Fatal("в списке осталась метка")
	}
}

func catalogOne(t *testing.T, c *client, id int64) catalog.ProductSummary {
	t.Helper()
	d := decode[struct {
		Product catalog.ProductSummary `json:"product"`
	}](t, c.do(http.MethodGet, "/api/products/"+strconv.FormatInt(id, 10), nil))
	return d.Product
}

func TestPhotoRejectsBadInput(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	id := phProduct(t, c, "Шампунь")
	big := append(phJPEG(t, 8, 8), make([]byte, photo.MaxBytes)...)
	cases := []struct {
		name  string
		data  []byte
		ctype string
		code  int
		text  string
	}{
		{"пустое тело", nil, "image/jpeg", 400, "не передано"},
		{"gif под видом jpeg", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), "image/jpeg", 400, "JPEG, PNG или WebP"},
		{"html под видом png", []byte("<script>alert(1)</script>"), "image/png", 400, "JPEG, PNG или WebP"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml", 400, "JPEG, PNG или WebP"},
		{"jpeg шире 4096", phJPEG(t, 4097, 4), "image/jpeg", 400, "4096"},
		{"webp выше 4096", phWebP(8, 5000), "image/webp", 400, "4096"},
		{"больше 400 КБ", big, "image/jpeg", 413, "400 КБ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := phSend(c, http.MethodPut, phPath(id), tc.data, tc.ctype)
			expect(t, rec, tc.code, tc.name)
			got := decode[errBody](t, rec)
			if got.Field != "photo" || !strings.Contains(got.Error, tc.text) {
				t.Fatalf("ошибка %+v", got)
			}
		})
	}
	if p := catalogOne(t, c, id); p.PhotoUpdatedAt != nil {
		t.Fatal("отказы что-то записали")
	}
	// Ровно 400 КБ допустимы.
	exact := phJPEG(t, 8, 8)
	exact = append(exact, make([]byte, photo.MaxBytes-len(exact))...)
	phPut(t, c, id, exact)

	// Предел тела только на этом адресе: остальные принимают лишь крошечные тела.
	if rec := phSend(c, http.MethodPost, "/api/categories", []byte(`{"name":"`+strings.Repeat("a", 100<<10)+`","color":"blue"}`), "application/json"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("общий предел: код %d", rec.Code)
	}
}

func TestPhotoAccessAndMethods(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	b := stkSignIn(e, "friend")
	id := phProduct(t, a, "Секретный шампунь")
	phPut(t, a, id, phJPEG(t, 20, 20))
	data := phJPEG(t, 5, 5)

	// Чужая позиция, несуществующая и мусорный id — одинаковый 404; фото владельца цело.
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, phPath(id)}, {http.MethodPut, phPath(id)}, {http.MethodDelete, phPath(id)},
		{http.MethodPut, phPath(99999)}, {http.MethodGet, phPath(99999)}, {http.MethodDelete, phPath(99999)},
		{http.MethodGet, "/api/products/abc/photo"}, {http.MethodPut, "/api/products/0/photo"},
	} {
		rec := phSend(b, tc.method, tc.target, data, "image/jpeg")
		expect(t, rec, http.StatusNotFound, tc.method+" "+tc.target)
	}
	if rec := a.do(http.MethodGet, phPath(id), nil); rec.Code != http.StatusOK || bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatal("фото владельца изменилось")
	}
	// Чужая позиция не раскрывается и через сводки.
	if strings.Contains(b.do(http.MethodGet, "/api/export.json", nil).Body.String(), "photo") {
		t.Fatal("в экспорте второго есть фото")
	}

	anon := e.client()
	expect(t, phSend(anon, http.MethodGet, phPath(id), nil, ""), http.StatusUnauthorized, "GET без входа")
	expect(t, phSend(anon, http.MethodPut, phPath(id), data, ""), http.StatusUnauthorized, "PUT без входа")
	// Без CSRF-заголовка изменение отклоняется.
	nc := e.client()
	nc.token, nc.noCSRF = a.token, true
	req := httptest.NewRequest(http.MethodPut, phPath(id), bytes.NewReader(data))
	req.RemoteAddr = nc.remoteAddr
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: a.token})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	expect(t, rec, http.StatusForbidden, "PUT без X-Requested-With")

	rec = a.do(http.MethodPost, phPath(id), nil)
	expect(t, rec, http.StatusMethodNotAllowed, "POST")
	if allow := rec.Header().Get("Allow"); allow != "DELETE, GET, HEAD, PUT" {
		t.Fatalf("Allow: %q", allow)
	}
}

// Фото уходит вместе с позицией при удалении пользователя (каскад).
func TestPhotoCascade(t *testing.T) {
	e := catNewEnv(t)
	c := e.catLogin("owner")
	id := phProduct(t, c, "Шампунь")
	phPut(t, c, id, phJPEG(t, 6, 6))
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM product_photos`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("фото в базе: %d, %v", n, err)
	}
	if _, err := e.db.Exec(`DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM product_photos`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("после удаления пользователя фото: %d, %v", n, err)
	}
}

func TestExportImportPhotoRoundTrip(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	list := decode[catProducts](t, a.do(http.MethodGet, "/api/products", nil)).Products
	jpg, pg := phJPEG(t, 90, 60), phPNG(t, 12, 12)
	for i, d := range [][]byte{jpg, pg} {
		phPut(t, a, list[i].ID, d)
	}
	file := trExport(t, a)
	withPhoto := 0
	for _, p := range file.Products {
		if p.Photo != nil {
			withPhoto++
			if p.Photo.Mime != "image/jpeg" && p.Photo.Mime != "image/png" || p.Photo.Data == "" {
				t.Fatalf("фото в файле: %+v", p.Photo)
			}
		}
	}
	if withPhoto != 2 || len(file.Products) < 2 {
		t.Fatalf("фото в экспорте: %d из %d", withPhoto, len(file.Products))
	}

	// Сырое тело файла: в JSON поле — объект {mime, data}.
	raw := a.do(http.MethodGet, "/api/export.json", nil).Body.String()
	if !strings.Contains(raw, `"photo":{"mime":"image/`) {
		t.Fatalf("формат поля photo: %.300s", raw)
	}

	b := stkSignIn(e, "friend")
	expect(t, b.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт с фото")
	got := decode[catProducts](t, b.do(http.MethodGet, "/api/products", nil)).Products
	byName := map[string]catalog.ProductSummary{}
	for _, p := range got {
		byName[p.Name] = p
	}
	for i, want := range [][]byte{jpg, pg} {
		p := byName[list[i].Name]
		if p.PhotoUpdatedAt == nil {
			t.Fatalf("у %q после импорта нет фото", p.Name)
		}
		rec := b.do(http.MethodGet, phPath(p.ID), nil)
		expect(t, rec, http.StatusOK, "фото после импорта")
		if !bytes.Equal(rec.Body.Bytes(), want) {
			t.Fatalf("байты фото %q изменились", p.Name)
		}
	}
	// Повторный экспорт второго пользователя совпадает по фото.
	again := trExport(t, b)
	n := 0
	for _, p := range again.Products {
		if p.Photo != nil {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("фото во втором экспорте: %d", n)
	}
}

func TestImportPhotoValidation(t *testing.T) {
	e := newEnv(t, false)
	a := stkSignIn(e, "owner")
	trSeed(t, a)
	good := trExport(t, a)
	b64 := func(d []byte) string { return transferB64(d) }
	big := append(phJPEG(t, 8, 8), make([]byte, photo.MaxBytes)...)
	cases := []struct {
		name  string
		photo *transfer.Photo
		field string
		text  string
	}{
		{"не base64", &transfer.Photo{Mime: "image/jpeg", Data: "!!!не base64!!!"}, "products[0].photo.data", "base64"},
		{"gif с заявленным jpeg", &transfer.Photo{Mime: "image/jpeg", Data: b64([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"))}, "products[0].photo", "JPEG, PNG или WebP"},
		{"пустое", &transfer.Photo{Mime: "image/jpeg", Data: ""}, "products[0].photo", "не передано"},
		{"больше 400 КБ", &transfer.Photo{Mime: "image/jpeg", Data: b64(big)}, "products[0].photo", "400 КБ"},
		{"шире 4096", &transfer.Photo{Mime: "image/jpeg", Data: b64(phJPEG(t, 4100, 3))}, "products[0].photo", "4096"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := good
			f.Products = append([]transfer.Product(nil), good.Products...)
			f.Products[0].Photo = tc.photo
			rec := stkSignIn(e, "user"+strconv.Itoa(i)).do(http.MethodPost, "/api/import", f)
			expect(t, rec, http.StatusBadRequest, tc.name)
			got := decode[errBody](t, rec)
			if got.Field != tc.field || !strings.Contains(got.Error, tc.text) {
				t.Fatalf("ошибка %+v", got)
			}
		})
	}
	// Тип берётся из содержимого, заявленный mime ничего не решает.
	f := good
	f.Products = append([]transfer.Product(nil), good.Products...)
	f.Products[0].Photo = &transfer.Photo{Mime: "image/png", Data: b64(phJPEG(t, 10, 10))}
	c := stkSignIn(e, "typed")
	expect(t, c.do(http.MethodPost, "/api/import", f), http.StatusOK, "mime в файле неверный")
	for _, p := range decode[catProducts](t, c.do(http.MethodGet, "/api/products", nil)).Products {
		if p.PhotoUpdatedAt != nil {
			if rec := c.do(http.MethodGet, phPath(p.ID), nil); rec.Header().Get("Content-Type") != "image/jpeg" {
				t.Fatalf("тип после импорта: %s", rec.Header().Get("Content-Type"))
			}
		}
	}
}

func transferB64(d []byte) string { return base64.StdEncoding.EncodeToString(d) }
