package barcode

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
)

// render рисует штрихкод: чёрные штрихи на белом поле с тихой зоной, scale
// пикселей на модуль.
func render(t *testing.T, w gozxing.Writer, text string, format gozxing.BarcodeFormat, scale, height int) *image.Gray {
	t.Helper()
	m, err := w.Encode(text, format, widthHint(text), height/scale, nil)
	if err != nil {
		t.Fatal(err)
	}
	const quiet = 10
	img := image.NewGray(image.Rect(0, 0, (m.GetWidth()+2*quiet)*scale, (m.GetHeight()+2*quiet)*scale))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < m.GetHeight()*scale; y++ {
		for x := 0; x < m.GetWidth()*scale; x++ {
			if m.Get(x/scale, y/scale) {
				img.SetGray(x+quiet*scale, y+quiet*scale, color.Gray{0})
			}
		}
	}
	return img
}

// widthHint — ширина матрицы в модулях по длине текста (с запасом; писатель сам подгонит).
func widthHint(text string) int { return (len(text)*7 + 20) }

func TestDecodeEAN13(t *testing.T) {
	img := render(t, oned.NewEAN13Writer(), "4600051000057", gozxing.BarcodeFormat_EAN_13, 3, 120)
	code, err := DecodeImage(img)
	if err != nil || code != "4600051000057" {
		t.Fatalf("код %q, ошибка %v", code, err)
	}
}

func TestDecodeFormats(t *testing.T) {
	cases := []struct {
		name, text string
		w          gozxing.Writer
		f          gozxing.BarcodeFormat
	}{
		{"EAN-8", "96385074", oned.NewEAN8Writer(), gozxing.BarcodeFormat_EAN_8},
		{"UPC-A", "036000291452", oned.NewUPCAWriter(), gozxing.BarcodeFormat_UPC_A},
		{"Code 128", "12345678", oned.NewCode128Writer(), gozxing.BarcodeFormat_CODE_128},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, err := DecodeImage(render(t, c.w, c.text, c.f, 3, 120))
			if err != nil || code != c.text {
				t.Fatalf("код %q, ошибка %v", code, err)
			}
		})
	}
}

func TestDecodeRotatedAndLarge(t *testing.T) {
	img := render(t, oned.NewEAN13Writer(), "4600051000057", gozxing.BarcodeFormat_EAN_13, 3, 120)
	rot := rotate90(rotate90(rotate90(img))) // лежит на боку
	if code, err := DecodeImage(rot); err != nil || code != "4600051000057" {
		t.Fatalf("повёрнутый: %q, %v", code, err)
	}
	// Большое фото: код занимает малую часть кадра, масштаб 8 → ширина больше 1600.
	big := render(t, oned.NewEAN13Writer(), "4600051000057", gozxing.BarcodeFormat_EAN_13, 20, 400)
	if longSide(big) <= workSideLarge {
		t.Fatalf("тест не проверяет уменьшение: %d", longSide(big))
	}
	if code, err := DecodeImage(big); err != nil || code != "4600051000057" {
		t.Fatalf("большое: %q, %v", code, err)
	}
}

func TestDecodeBytesJPEGAndPNG(t *testing.T) {
	img := render(t, oned.NewEAN13Writer(), "4600051000057", gozxing.BarcodeFormat_EAN_13, 3, 120)
	var jb, pb bytes.Buffer
	if err := jpeg.Encode(&jb, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&pb, img); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"jpeg": jb.Bytes(), "png": pb.Bytes()} {
		if code, err := Decode(data); err != nil || code != "4600051000057" {
			t.Fatalf("%s: %q, %v", name, code, err)
		}
	}
}

func TestDecodeNothing(t *testing.T) {
	blank := image.NewGray(image.Rect(0, 0, 300, 200))
	for i := range blank.Pix {
		blank.Pix[i] = 255
	}
	if _, err := DecodeImage(blank); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидалось ErrNotFound: %v", err)
	}
	if _, err := Decode([]byte("не картинка")); err == nil {
		t.Fatal("мусор должен дать ошибку")
	}
}

func TestNormalizeAndSame(t *testing.T) {
	if c, ok := Normalize(" 4600 051-000057 "); !ok || c != "4600051000057" {
		t.Fatalf("Normalize: %q %v", c, ok)
	}
	for _, bad := range []string{"", "abc", "12a4", "https://x.ru"} {
		if _, ok := Normalize(bad); ok {
			t.Fatalf("%q не код", bad)
		}
	}
	if !Same("036000291452", "0036000291452") || !Same("0036000291452", "036000291452") {
		t.Fatal("UPC-A и EAN-13 с нулём — один код")
	}
	if Same("036000291452", "1036000291452") || Same("", "") || Same("123", "124") {
		t.Fatal("лишнее совпадение")
	}
	if SearchKey("0036000291452") != "036000291452" || SearchKey("4600051000057") != "4600051000057" {
		t.Fatal("SearchKey")
	}
}

func TestDecodeLimitsAndFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4001, 4001))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); !errors.Is(err, ErrTooBig) {
		t.Fatalf("8-битное 16+ Мп: %v", err)
	}
	buf.Reset()
	if err := png.Encode(&buf, image.NewRGBA64(image.Rect(0, 0, 3000, 3000))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); !errors.Is(err, ErrTooBig) {
		t.Fatalf("16-битное 9 Мп: %v", err)
	}
	if _, err := Decode([]byte("\x00\x00\x00\x18ftypheic....")); !errors.Is(err, ErrFormat) {
		t.Fatalf("HEIC: %v", err)
	}
}
