package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
)

// access — кому доступен адрес API.
type access int

const (
	public    access = iota // без входа
	signedIn                // нужна сессия
	ownerOnly               // нужна сессия владельца
)

// methods — обработчики адреса по методам.
type methods map[string]http.HandlerFunc

const (
	sessionCookie = "garage_session"
	// csrfHeader — заголовок, без которого изменяющие запросы отклоняются (ТБ-3).
	// Браузер не даёт чужой странице поставить его без CORS-разрешения.
	csrfHeader = "X-Requested-With"
	csrfValue  = "garage"
	// maxBody — предел тела запроса: формы API крошечные.
	maxBody = 64 << 10
)

// api регистрирует адрес. Метод выбирается здесь, а не шаблоном ServeMux
// («GET /api/…»): иначе запрос другим методом ушёл бы в общий обработчик
// /api/ и получил 404 вместо 405.
//
// Порядок проверок: метод (405) → CSRF-заголовок (403) → сессия (401) →
// владелец (403).
func (s *Server) api(pattern string, level access, m methods) {
	s.apiLimited(pattern, level, maxBody, m)
}

// apiLimited — api с другим пределом тела запроса (импорт файла, ФТ-10.3.3).
func (s *Server) apiLimited(pattern string, level access, bodyLimit int64, m methods) {
	allow := make([]string, 0, len(m)+1)
	for method := range m {
		allow = append(allow, method)
	}
	if _, ok := m[http.MethodGet]; ok {
		allow = append(allow, http.MethodHead)
	}
	sort.Strings(allow)
	allowHeader := strings.Join(allow, ", ")

	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		h, ok := m[method]
		if !ok {
			w.Header().Set("Allow", allowHeader)
			writeError(w, http.StatusMethodNotAllowed, "Метод не поддерживается")
			return
		}
		if method != http.MethodGet && r.Header.Get(csrfHeader) != csrfValue {
			writeError(w, http.StatusForbidden, "Запрос отклонён")
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		}
		if level == public {
			h(w, r)
			return
		}

		cookie, _ := r.Cookie(sessionCookie)
		var token string
		if cookie != nil {
			token = cookie.Value
		}
		sess, user, extended, err := s.auth.ResolveSession(r.Context(), token)
		if errors.Is(err, auth.ErrNoSession) {
			if cookie != nil {
				s.clearSessionCookie(w)
			}
			writeError(w, http.StatusUnauthorized, "Нужно войти")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if extended {
			s.setSessionCookie(w, token)
		}
		if level == ownerOnly && !user.IsOwner {
			writeError(w, http.StatusForbidden, "Доступно только владельцу")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), currentKey{}, current{user: user, session: sess})))
	})
}

type currentKey struct{}

// current — пользователь и сессия запроса. user_id для запросов к базе
// берётся только отсюда, а не из тела запроса (ТБ-5).
type current struct {
	user    auth.User
	session auth.Session
}

func currentFrom(r *http.Request) current {
	c, _ := r.Context().Value(currentKey{}).(current)
	return c
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(auth.SessionTTL / time.Second),
		HttpOnly: true,
		Secure:   !s.insecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !s.insecureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

// fromLoopback — соединение пришло с loopback, то есть от nginx на том же
// сервере (Р5.5). Только таким соединениям верим заголовкам прокси.
func fromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// clientIP — адрес клиента для ограничения попыток. X-Real-IP и
// X-Forwarded-For учитываются только за loopback-прокси; из
// X-Forwarded-For берётся правый адрес — его дописал наш nginx, левые
// мог подставить кто угодно.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !fromLoopback(r) {
		return host
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
			return ip.String()
		}
	}
	return host
}

// requestScheme — https, если запрос пришёл по TLS или через nginx
// (loopback с заголовками прокси: nginx garage слушает только 443).
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if fromLoopback(r) {
		switch proto := strings.ToLower(r.Header.Get("X-Forwarded-Proto")); {
		case proto == "https":
			return "https"
		case proto == "" && (r.Header.Get("X-Real-IP") != "" || r.Header.Get("X-Forwarded-For") != ""):
			return "https"
		}
	}
	return "http"
}

// decodeJSON читает тело запроса; при ошибке отвечает 400 и возвращает false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "Слишком большой запрос")
			return false
		}
		writeError(w, http.StatusBadRequest, "Неверный формат запроса")
		return false
	}
	return true
}

// writeAuthError переводит ошибки пакета auth в ответ.
func (s *Server) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		writeFieldError(w, http.StatusBadRequest, ve.Field, ve.Message)
	case errors.Is(err, auth.ErrLoginTaken):
		writeFieldError(w, http.StatusConflict, "login", "Этот логин уже занят")
	case errors.Is(err, auth.ErrInviteInvalid):
		writeError(w, http.StatusNotFound, "Приглашение не найдено или уже недействительно")
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "Не найдено")
	default:
		s.internalError(w, r, err)
	}
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("ошибка обработки запроса", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
}
