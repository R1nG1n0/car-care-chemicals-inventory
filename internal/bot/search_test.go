package bot

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
	"image"
	"image/color"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

func TestSearchCard(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	shelf := e.exec(`INSERT INTO locations (user_id, name) VALUES (?, 'Полка')`, a.id)
	shop := e.exec(`INSERT INTO stores (user_id, name) VALUES (?, 'Автохим')`, a.id)
	p := e.product(a, "Шампунь Gsf", "Gyeon", "ml", 500, ptr(int64(300)), "")
	e.buy(a, p, 2, 119000, &shop, &shelf)
	e.product(a, "Полироль", "", "pcs", 1, nil, "")

	e.say(a, "шампунь")
	m := e.fake.last()
	contains(t, m.Text, "Шампунь Gsf (Gyeon)", "1 000 мл", "запечатан ×2: 500 мл из 500 мл · Полка",
		"Минимум: 300 мл", "Последняя цена: 1 190 ₽", "Автохим")
	for _, step := range []string{"−10", "−50", "−100"} {
		if _, ok := m.button(step); !ok {
			t.Fatalf("нет кнопки %s: %+v", step, m.Kb)
		}
	}

	// Регистр и бренд.
	e.say(a, "GYEON")
	contains(t, e.fake.last().Text, "Шампунь Gsf")

	// Позиция без остатка в штуках — без кнопок расхода.
	e.say(a, "полироль")
	contains(t, e.fake.last().Text, "нет в наличии")
	if len(e.fake.last().Kb) != 0 {
		t.Fatal("у позиции без остатка не должно быть кнопок расхода")
	}

	e.say(a, "нет такого")
	contains(t, e.fake.last().Text, "Ничего не нашлось")
}

func TestSearchListAndLimit(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	for i := 1; i <= 3; i++ {
		p := e.product(a, fmt.Sprintf("Шампунь %d", i), "", "ml", 500, nil, "")
		e.buy(a, p, 1, 1000, nil, nil)
	}
	e.say(a, "шампунь")
	m := e.fake.last()
	contains(t, m.Text, "Найдено: 3 позиции", "Шампунь 1", "Шампунь 3")
	if len(m.Kb) != 3 {
		t.Fatalf("кнопок: %d", len(m.Kb))
	}
	// Кнопка открывает карточку.
	e.click(a, m, "Шампунь 2")
	contains(t, e.fake.last().Text, "Остаток: 500 мл")

	for i := 4; i <= 12; i++ {
		e.product(a, fmt.Sprintf("Шампунь %d", i), "", "ml", 500, nil, "")
	}
	e.say(a, "шампунь")
	m = e.fake.last()
	contains(t, m.Text, "Найдено: 12 позиций", "уточните запрос")
	if len(m.Kb) != maxResults {
		t.Fatalf("показано кнопок %d, ожидалось %d", len(m.Kb), maxResults)
	}
}

func TestSearchBarcode(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	upc := e.product(a, "Шампунь UPC", "", "ml", 500, nil, "036000291452")
	e.buy(a, upc, 1, 1000, nil, nil)
	e.product(a, "Воск EAN", "", "ml", 300, nil, "4600051000057")

	e.say(a, "4600051000057")
	contains(t, e.fake.last().Text, "Воск EAN")

	// UPC-A и тот же код в EAN-13 с нулём — один товар (в обе стороны).
	e.say(a, "0036000291452")
	contains(t, e.fake.last().Text, "Шампунь UPC")
	e.say(a, "036000291452")
	contains(t, e.fake.last().Text, "Шампунь UPC")

	// Пробелы между цифрами допустимы.
	e.say(a, "4600 0510 00057")
	contains(t, e.fake.last().Text, "Воск EAN")

	// Не найден: сообщение и ссылка на сайт (ФТ-11.4.3).
	e.say(a, "4607777777777")
	m := e.fake.last()
	contains(t, m.Text, "не найден", "https://garage.example")
	if b, ok := m.button("сайт"); !ok || b.URL != "https://garage.example" {
		t.Fatalf("кнопка-ссылка: %+v", m.Kb)
	}

	// Меньше 8 цифр — не штрихкод, а обычный поиск.
	e.say(a, "1234567")
	contains(t, e.fake.last().Text, "Ничего не нашлось")
}

func barcodeJPEG(t *testing.T, code string) []byte {
	t.Helper()
	m, err := oned.NewEAN13Writer().Encode(code, gozxing.BarcodeFormat_EAN_13, 120, 40, nil)
	if err != nil {
		t.Fatal(err)
	}
	const scale, quiet = 3, 10
	img := image.NewGray(image.Rect(0, 0, (m.GetWidth()+2*quiet)*scale, (m.GetHeight()+2*quiet)*scale))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < m.GetHeight()*scale; y++ {
		for x := 0; x < m.GetWidth()*scale; x++ {
			if m.Get(x/scale, y/scale) {
				img.SetGray(x+quiet*scale, y+quiet*scale, color.Gray{})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (e *env) photo(u user, sizes ...telegram.PhotoSize) {
	e.upd++
	e.bot.HandleUpdate(e.ctx, telegram.Update{UpdateID: e.upd, Message: &telegram.Message{
		MessageID: e.upd, Photo: sizes,
		From: &telegram.User{ID: u.tg}, Chat: telegram.Chat{ID: u.tg, Type: "private"}}})
}

func TestSearchBarcodeFromPhoto(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.product(a, "Воск EAN", "", "ml", 300, nil, "4600051000057")
	e.fake.files["small"] = []byte("мусор")
	e.fake.files["big"] = barcodeJPEG(t, "4600051000057")

	// Берётся самый крупный размер, а не первый.
	e.photo(a, telegram.PhotoSize{FileID: "small", Width: 90, Height: 60, FileSize: 5},
		telegram.PhotoSize{FileID: "big", Width: 1280, Height: 400, FileSize: 3000})
	contains(t, e.fake.last().Text, "Воск EAN")

	// Нет штрихкода — просьба переснять.
	var blank bytes.Buffer
	_ = jpeg.Encode(&blank, image.NewGray(image.Rect(0, 0, 200, 100)), nil)
	e.fake.files["blank"] = blank.Bytes()
	e.photo(a, telegram.PhotoSize{FileID: "blank", Width: 200, Height: 100, FileSize: int64(blank.Len())})
	contains(t, e.fake.last().Text, "Не вижу штрихкод", "цифры")

	// Слишком большое фото.
	e.photo(a, telegram.PhotoSize{FileID: "big", Width: 5000, Height: 4000, FileSize: 11 << 20})
	contains(t, e.fake.last().Text, "слишком большое")

	// Фото нигде не сохраняется.
	for _, table := range []string{"product_photos"} {
		if n := e.count(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Fatalf("в %s появились строки", table)
		}
	}
}

func TestPhotoTooBigAndUnsupportedFormat(t *testing.T) {
	e := newEnv(t)
	a := e.linked("owner", 1001)
	e.fake.files["heic"] = []byte("\x00\x00\x00\x18ftypheic....")
	e.photo(a, telegram.PhotoSize{FileID: "heic", Width: 100, Height: 100, FileSize: 20})
	contains(t, e.fake.last().Text, "обычное фото")
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4001, 4001))); err != nil {
		t.Fatal(err)
	}
	e.fake.files["huge"] = buf.Bytes()
	e.photo(a, telegram.PhotoSize{FileID: "huge", Width: 4001, Height: 4001, FileSize: int64(buf.Len())})
	contains(t, e.fake.last().Text, "слишком большое")
}
