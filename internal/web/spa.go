package web

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// Интерфейс — одностраничное приложение. Сервер отдаёт его файлы, а на любой
// неизвестный путь вне /api/ — index.html: экранами управляет роутер в браузере.

// Манифест PWA: в системной таблице типов его может не быть (на macOS
// mime.TypeByExtension возвращает пустую строку), а с nosniff браузер
// не станет угадывать тип сам.
func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// assetsPrefix — каталог файлов сборки с хешем в имени: их можно кэшировать
// навсегда, при новой сборке меняется имя.
const assetsPrefix = "assets/"

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

	if s.ui != nil && name != "" && name != "index.html" {
		if f, err := s.ui.Open(name); err == nil {
			st, err := f.Stat()
			f.Close()
			if err == nil && !st.IsDir() {
				if strings.HasPrefix(name, assetsPrefix) {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFileFS(w, r, s.ui, name)
				return
			}
		}
		// Отсутствующий файл сборки — 404, а не index.html: иначе браузер
		// получил бы HTML вместо скрипта и молча сломался.
		if strings.HasPrefix(name, assetsPrefix) {
			http.NotFound(w, r)
			return
		}
	}
	s.serveIndex(w, r)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if s.ui != nil {
		if f, err := s.ui.Open("index.html"); err == nil {
			defer f.Close()
			if rs, ok := f.(io.ReadSeeker); ok {
				http.ServeContent(w, r, "index.html", time.Time{}, rs)
				return
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			s.log.Error("чтение index.html интерфейса", "err", err)
		}
	}
	// Интерфейс не собран: Go-сборка без `make web`.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, uiMissingPage)
}

const uiMissingPage = `<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>garage</title></head>
<body><h1>Интерфейс не собран</h1>
<p>Бинарник собран без интерфейса. Выполните <code>make web</code> и соберите garage заново
(<code>make build-linux</code> делает это сам). API работает: <code>/api/health</code>.</p>
</body></html>
`
