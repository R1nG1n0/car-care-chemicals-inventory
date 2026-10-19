// Package barcode — распознавание штрихкода на фото и правила сравнения кодов
// (ФТ-9, ФТ-11.4). Распознаёт EAN-13, EAN-8, UPC-A, UPC-E и Code 128 на чистом
// Go (gozxing), без cgo.
package barcode

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // форматы фото из Telegram и камеры
	_ "image/png"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
)

const (
	// MaxCodeLen — длина кода позиции (как у поля barcode, ФТ-9).
	MaxCodeLen = 32
	// maxPixels — предел размера фото (4000×4000): защита памяти от «бомб»
	// при распаковке; для 16-битных моделей цвета (8 байт на пиксель) — вдвое меньше.
	maxPixels   = 16_000_000
	maxPixels16 = 8_000_000
	// workSides — длинные стороны, до которых уменьшаем фото перед поиском:
	// сначала крупнее, затем мельче (размытие помогает на шумных снимках).
	workSideLarge = 1600
	workSideSmall = 800
)

// ErrNotFound — на фото нет читаемого штрихкода.
var ErrNotFound = errors.New("штрихкод не найден")

// ErrTooBig — фото больше допустимого размера в пикселях.
var ErrTooBig = errors.New("фото слишком большое")

// ErrFormat — формат фото не поддерживается (не JPEG и не PNG, например HEIC).
var ErrFormat = errors.New("формат фото не поддерживается")

// Normalize убирает пробелы и дефисы и проверяет, что остались только цифры
// (не длиннее MaxCodeLen). ok=false — это не код товара.
func Normalize(raw string) (code string, ok bool) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == ' ' || (r >= '‐' && r <= '―') || r == '−':
		default:
			return "", false
		}
	}
	code = b.String()
	if code == "" || len(code) > MaxCodeLen {
		return "", false
	}
	return code, true
}

// Same — один и тот же товар: коды совпадают или это UPC-A (12 цифр) и тот
// же код в виде EAN-13 с ведущим нулём (ФТ-11.4.1).
func Same(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	short, long := a, b
	if len(a) > len(b) {
		short, long = b, a
	}
	return len(short) == 12 && len(long) == 13 && long == "0"+short
}

// SearchKey — что искать подстрокой в названии/бренде/коде: для EAN-13 с
// ведущим нулём — без нуля, найдутся обе записи кода.
func SearchKey(code string) string {
	if len(code) == 13 && code[0] == '0' {
		return code[1:]
	}
	return code
}

// Decode находит штрихкод на фото (JPEG или PNG) и возвращает его цифры.
// Большие фото уменьшаются; если не нашлось, пробуется поворот на 90°.
func Decode(data []byte) (string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if errors.Is(err, image.ErrFormat) {
		return "", ErrFormat
	}
	if err != nil {
		return "", fmt.Errorf("фото не открывается: %w", err)
	}
	limit := maxPixels
	switch cfg.ColorModel {
	case color.RGBA64Model, color.NRGBA64Model, color.Gray16Model:
		limit = maxPixels16
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > limit/cfg.Height {
		return "", ErrTooBig
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("фото не открывается: %w", err)
	}
	return DecodeImage(img)
}

// DecodeImage — Decode для уже открытого изображения.
func DecodeImage(img image.Image) (string, error) {
	base := toGray(img)
	for _, side := range []int{workSideLarge, workSideSmall} {
		g := base
		if longSide(g) > side {
			g = shrink(g, side)
		} else if side == workSideSmall && longSide(base) <= workSideSmall {
			continue // уже мельче — повторять нечего
		}
		for _, rotated := range []bool{false, true} {
			cur := g
			if rotated {
				cur = rotate90(g)
			}
			if code, ok := readGray(cur); ok {
				return code, nil
			}
		}
	}
	return "", ErrNotFound
}

// readGray пробует прочитать код одним проходом по всем форматам.
func readGray(img image.Image) (string, bool) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", false
	}
	hints := map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true}
	readers := []gozxing.Reader{oned.NewMultiFormatUPCEANReader(hints), oned.NewCode128Reader()}
	for _, r := range readers {
		res, err := r.Decode(bmp, hints)
		if err != nil {
			continue
		}
		if code, ok := Normalize(res.GetText()); ok {
			// UPC-A библиотека отдаёт как EAN-13 с нулём; на сайте он читается
			// как 12 цифр — приводим к тому же виду.
			if len(code) == 13 && code[0] == '0' && res.GetBarcodeFormat() == gozxing.BarcodeFormat_EAN_13 {
				code = code[1:]
			}
			return code, true
		}
	}
	return "", false
}
