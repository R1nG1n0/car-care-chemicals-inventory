// Package photo — проверка фото позиции (ФТ-10.4.3): тип определяется по
// содержимому, а не по заголовку запроса, размеры берутся из заголовка
// файла. Картинка не декодируется целиком, поэтому проверка дешёвая.
package photo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg" // регистрирует формат для image.DecodeConfig
	_ "image/png"  // то же для PNG
)

// Пределы фото. Браузер сжимает до 1024 px и 300 КБ, сервер принимает с запасом.
const (
	MaxBytes = 400 << 10
	MaxSide  = 4096
)

// Типы, которые принимает сервер.
const (
	JPEG = "image/jpeg"
	PNG  = "image/png"
	WebP = "image/webp"
)

// Ошибки проверки.
var (
	ErrEmpty      = errors.New("пустое тело")
	ErrTooLarge   = errors.New("фото больше 400 КБ")
	ErrFormat     = errors.New("не JPEG, PNG или WebP")
	ErrDimensions = errors.New("размеры фото вне 1…4096 px")
)

// Info — что узнали о фото.
type Info struct {
	Mime          string
	Width, Height int
}

// Inspect проверяет байты фото: размер, тип по сигнатуре, размеры по заголовку.
func Inspect(data []byte) (Info, error) {
	if len(data) == 0 {
		return Info{}, ErrEmpty
	}
	if len(data) > MaxBytes {
		return Info{}, ErrTooLarge
	}
	var info Info
	var err error
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		info.Mime = JPEG
		info.Width, info.Height, err = decodeConfig(data)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		info.Mime = PNG
		info.Width, info.Height, err = decodeConfig(data)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		info.Mime = WebP
		info.Width, info.Height, err = webpConfig(data)
	default:
		return Info{}, ErrFormat
	}
	if err != nil {
		return Info{}, ErrFormat
	}
	if info.Width < 1 || info.Height < 1 || info.Width > MaxSide || info.Height > MaxSide {
		return Info{}, ErrDimensions
	}
	return info, nil
}

func decodeConfig(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg.Width, cfg.Height, err
}

// webpConfig читает размеры из первого блока WebP: VP8 (с потерями),
// VP8L (без потерь) или VP8X (расширенный).
func webpConfig(d []byte) (w, h int, err error) {
	bad := errors.New("повреждённый WebP")
	if len(d) < 30 {
		return 0, 0, bad
	}
	switch string(d[12:16]) {
	case "VP8 ":
		// Кадр: 3 байта тега, код старта 9d 01 2a, затем 14 бит ширины и высоты.
		if d[23] != 0x9d || d[24] != 0x01 || d[25] != 0x2a {
			return 0, 0, bad
		}
		return int(binary.LittleEndian.Uint16(d[26:]) & 0x3fff), int(binary.LittleEndian.Uint16(d[28:]) & 0x3fff), nil
	case "VP8L":
		if d[20] != 0x2f {
			return 0, 0, bad
		}
		bits := binary.LittleEndian.Uint32(d[21:])
		return int(bits&0x3fff) + 1, int(bits>>14&0x3fff) + 1, nil
	case "VP8X":
		return int(uint32(d[24])|uint32(d[25])<<8|uint32(d[26])<<16) + 1,
			int(uint32(d[27])|uint32(d[28])<<8|uint32(d[29])<<16) + 1, nil
	}
	return 0, 0, bad
}
