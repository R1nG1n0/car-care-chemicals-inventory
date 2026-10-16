// Package webui встраивает собранный интерфейс в бинарник.
//
// Сборка — `make web` (Vite, результат в web/dist). Без неё в каталоге
// лежит только файл .keep: Go-сборка и тесты работают, а сервер вместо
// интерфейса показывает подсказку, что его нужно собрать.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist возвращает файлы интерфейса: index.html, assets/….
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Путь задан константой и встроен при компиляции — ошибки быть не может.
		panic(err)
	}
	return sub
}
