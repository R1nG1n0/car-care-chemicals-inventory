package photo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{R: 200, A: 100})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webp собирает только заголовок нужного вида: Inspect не декодирует картинку.
func webp(kind string, w, h int) []byte {
	d := make([]byte, 40)
	copy(d, "RIFF")
	binary.LittleEndian.PutUint32(d[4:], 32)
	copy(d[8:], "WEBP")
	copy(d[12:], kind)
	switch kind {
	case "VP8 ":
		copy(d[23:], []byte{0x9d, 0x01, 0x2a})
		binary.LittleEndian.PutUint16(d[26:], uint16(w))
		binary.LittleEndian.PutUint16(d[28:], uint16(h))
	case "VP8L":
		d[20] = 0x2f
		binary.LittleEndian.PutUint32(d[21:], uint32(w-1)|uint32(h-1)<<14)
	case "VP8X":
		for i, v := range []int{w - 1, h - 1} {
			d[24+3*i], d[25+3*i], d[26+3*i] = byte(v), byte(v>>8), byte(v>>16)
		}
	}
	return d
}

func TestInspect(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Info
		err  error
	}{
		{"jpeg", jpegBytes(t, 40, 30), Info{JPEG, 40, 30}, nil},
		{"png с прозрачностью", pngBytes(t, 7, 9), Info{PNG, 7, 9}, nil},
		{"webp с потерями", webp("VP8 ", 1024, 768), Info{WebP, 1024, 768}, nil},
		{"webp без потерь", webp("VP8L", 640, 480), Info{WebP, 640, 480}, nil},
		{"webp расширенный", webp("VP8X", 4096, 1), Info{WebP, 4096, 1}, nil},
		{"пусто", nil, Info{}, ErrEmpty},
		{"gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), Info{}, ErrFormat},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), Info{}, ErrFormat},
		{"html под видом jpeg", []byte("<html><script>alert(1)</script></html>"), Info{}, ErrFormat},
		{"обрезанный jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, Info{}, ErrFormat},
		{"webp без блока", append([]byte("RIFF\x04\x00\x00\x00WEBP"), 0, 0), Info{}, ErrFormat},
		{"webp с чужим блоком", webp("ABCD", 10, 10), Info{}, ErrFormat},
		{"jpeg шире предела", jpegBytes(t, 4097, 10), Info{}, ErrDimensions},
		{"webp выше предела", webp("VP8L", 10, 4097), Info{}, ErrDimensions},
		{"webp нулевой", webp("VP8 ", 0, 5), Info{}, ErrDimensions},
		{"больше 400 КБ", append(jpegBytes(t, 8, 8), make([]byte, MaxBytes)...), Info{}, ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Inspect(tc.data)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ошибка %v, ожидалась %v", err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("получили %+v, ожидалось %+v", got, tc.want)
			}
		})
	}
}

func TestInspectLimitIsInclusive(t *testing.T) {
	d := jpegBytes(t, 8, 8)
	d = append(d, make([]byte, MaxBytes-len(d))...)
	if len(d) != MaxBytes {
		t.Fatalf("длина %d", len(d))
	}
	if _, err := Inspect(d); err != nil {
		t.Fatalf("ровно 400 КБ: %v", err)
	}
}
