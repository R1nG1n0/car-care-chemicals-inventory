// Package web — HTTP-сервер garage: API и файлы интерфейса.
package web

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
)

// Server обслуживает /api/ и одностраничный интерфейс.
type Server struct {
	log     *slog.Logger
	version string
	ui      fs.FS
	mux     *http.ServeMux
}

// New собирает сервер. ui — файлы интерфейса (обычно webui.Dist()),
// nil — интерфейс не подключён.
func New(log *slog.Logger, version string, ui fs.FS) *Server {
	s := &Server{log: log, version: version, ui: ui, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", only(http.MethodGet, s.handleHealth))
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Нет такого метода API")
	})
	s.mux.HandleFunc("/", s.handleSPA)
}

// only пропускает запросы с одним методом, на остальные отвечает 405.
// Шаблон «GET /api/…» в ServeMux не годится: запрос другим методом ушёл бы
// в общий обработчик /api/ и получил 404 вместо 405.
func only(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method && !(method == http.MethodGet && r.Method == http.MethodHead) {
			w.Header().Set("Allow", method)
			writeError(w, http.StatusMethodNotAllowed, "Метод не поддерживается")
			return
		}
		h(w, r)
	}
}

// Handler возвращает обработчик со служебными заголовками (ТБ-6).
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		s.mux.ServeHTTP(w, r)
	})
}

// contentSecurityPolicy — строгая политика без встроенных скриптов и стилей
// и без внешних адресов: шрифты и иконки поставляются вместе с приложением.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
