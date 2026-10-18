// Package web — HTTP-сервер garage: API и файлы интерфейса.
package web

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
)

// Options — зависимости сервера.
type Options struct {
	Log     *slog.Logger
	Version string
	// UI — файлы интерфейса (обычно webui.Dist()), nil — интерфейс не подключён.
	UI fs.FS
	// Auth — учётные записи, сессии и приглашения.
	Auth *auth.Service
	// Limiter — ограничение попыток входа; nil — 5 неудач за 15 минут (ТБ-4).
	Limiter *auth.Limiter
	// InsecureCookies убирает Secure у cookie сессии (локальная разработка по http).
	InsecureCookies bool
	// DB — база для разделов этапа 2 (справочники, склад); nil — разделы не подключаются.
	DB *sql.DB
	// Now — часы для отметок времени; nil — time.Now. В тестах подменяется.
	Now func() time.Time
}

// Server обслуживает /api/ и одностраничный интерфейс.
type Server struct {
	log             *slog.Logger
	version         string
	ui              fs.FS
	auth            *auth.Service
	limiter         *auth.Limiter
	insecureCookies bool
	db              *sql.DB
	now             func() time.Time
	mux             *http.ServeMux
}

// New собирает сервер.
func New(opt Options) *Server {
	s := &Server{
		log:             opt.Log,
		version:         opt.Version,
		ui:              opt.UI,
		auth:            opt.Auth,
		limiter:         opt.Limiter,
		insecureCookies: opt.InsecureCookies,
		db:              opt.DB,
		now:             opt.Now,
		mux:             http.NewServeMux(),
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.limiter == nil {
		s.limiter = auth.NewLimiter(auth.LimitAttempts, auth.LimitWindow, nil)
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.api("/api/health", public, methods{http.MethodGet: s.handleHealth})

	s.api("/api/auth/login", public, methods{http.MethodPost: s.handleLogin})
	s.api("/api/auth/me", signedIn, methods{http.MethodGet: s.handleMe})
	s.api("/api/auth/logout", public, methods{http.MethodPost: s.handleLogout})
	s.api("/api/auth/logout-all", signedIn, methods{http.MethodPost: s.handleLogoutAll})
	s.api("/api/auth/password", signedIn, methods{http.MethodPost: s.handlePassword})
	s.api("/api/auth/settings", signedIn, methods{http.MethodPut: s.handleSettings})

	s.api("/api/invites", ownerOnly, methods{
		http.MethodGet:  s.handleListInvites,
		http.MethodPost: s.handleCreateInvite,
	})
	s.api("/api/invites/{id}", ownerOnly, methods{http.MethodDelete: s.handleRevokeInvite})
	s.api("/api/invites/by-token/{token}", public, methods{http.MethodGet: s.handleCheckInvite})
	s.api("/api/invites/by-token/{token}/accept", public, methods{http.MethodPost: s.handleAcceptInvite})

	// Этап 2: адреса регистрируются в своих файлах (handlers_catalog.go,
	// handlers_stock.go), чтобы разделы развивались независимо.
	if s.db != nil {
		s.catalogRoutes()
		s.stockRoutes()
	}

	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "Нет такого метода API")
	})
	s.mux.HandleFunc("/", s.handleSPA)
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
// 'wasm-unsafe-eval' разрешает только компиляцию WebAssembly (сканер штрихкодов,
// ФТ-9.6) — eval и new Function для JavaScript остаются запрещены.
const contentSecurityPolicy = "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; img-src 'self'; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: s.version})
}

type errorResponse struct {
	Error      string `json:"error"`
	Field      string `json:"field,omitempty"`
	RetryAfter int    `json:"retry_after,omitempty"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorResponse{Error: msg})
}

func writeFieldError(w http.ResponseWriter, code int, field, msg string) {
	writeJSON(w, code, errorResponse{Error: msg, Field: field})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeNoContent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
