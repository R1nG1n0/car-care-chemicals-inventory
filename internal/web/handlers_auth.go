package web

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
)

type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type userResponse struct {
	User     auth.User     `json:"user"`
	Settings auth.Settings `json:"settings"`
}

// limitKeys — ключи ограничителя: логин (без учёта регистра) и адрес.
// Логин обрезается, чтобы мусорный ввод не раздувал память ограничителя.
func limitKeys(login, ip string) []string {
	login = strings.ToLower(login)
	if len(login) > 64 {
		login = login[:64]
	}
	return []string{"login:" + login, "ip:" + ip}
}

// writeTooMany — 429 с Retry-After (ТБ-4).
func writeTooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	mins := (secs + 59) / 60
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, errorResponse{
		Error:      fmt.Sprintf("Слишком много попыток. Повторите через %d %s", mins, pluralMinutes(mins)),
		RetryAfter: secs,
	})
}

// pluralMinutes — «минуту», «минуты» или «минут» после числа.
func pluralMinutes(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "минуту"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "минуты"
	}
	return "минут"
}

// startSession создаёт сессию, ставит cookie и отвечает { user, settings }.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u auth.User, code int) {
	sess, err := s.auth.CreateSession(r.Context(), u.ID, r.UserAgent())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	st, err := s.auth.GetSettings(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setSessionCookie(w, sess.Token)
	writeJSON(w, code, userResponse{User: u, Settings: st})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if !decodeJSON(w, r, &req) {
		return
	}
	ip := clientIP(r)
	keys := limitKeys(req.Login, ip)
	if blocked, wait := s.limiter.Blocked(keys...); blocked {
		s.log.Warn("вход: превышено число попыток", "login", req.Login, "ip", ip)
		writeTooMany(w, wait)
		return
	}
	u, err := s.auth.Authenticate(r.Context(), req.Login, req.Password)
	if errors.Is(err, auth.ErrBadCredentials) {
		s.limiter.Fail(keys...)
		s.log.Info("вход: неверный логин или пароль", "login", req.Login, "ip", ip)
		writeError(w, http.StatusUnauthorized, "Неверный логин или пароль")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.limiter.Reset(keys[0])
	s.log.Info("вход", "login", u.Login, "ip", ip)
	s.startSession(w, r, u, http.StatusOK)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	st, err := s.auth.GetSettings(r.Context(), c.user.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse{User: c.user, Settings: st})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		if err := s.auth.DeleteSession(r.Context(), cookie.Value); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearSessionCookie(w)
	writeNoContent(w)
}

func (s *Server) handleLogoutAll(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	if err := s.auth.DeleteUserSessions(r.Context(), c.user.ID, ""); err != nil {
		s.internalError(w, r, err)
		return
	}
	s.log.Info("выход со всех устройств", "login", c.user.Login)
	s.clearSessionCookie(w)
	writeNoContent(w)
}

type passwordRequest struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	var req passwordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Неудачи с текущим паролем считаются как неудачные входы: иначе
	// украденная сессия позволила бы перебирать пароль без ограничений.
	ip := clientIP(r)
	keys := limitKeys(c.user.Login, ip)
	if blocked, wait := s.limiter.Blocked(keys...); blocked {
		writeTooMany(w, wait)
		return
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		var ve *auth.ValidationError
		if errors.As(err, &ve) {
			writeFieldError(w, http.StatusBadRequest, "new", ve.Message)
			return
		}
		s.internalError(w, r, err)
		return
	}
	ok, err := s.auth.CheckUserPassword(r.Context(), c.user.ID, req.Current)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		s.limiter.Fail(keys...)
		s.log.Info("смена пароля: неверный текущий пароль", "login", c.user.Login, "ip", ip)
		writeFieldError(w, http.StatusForbidden, "current", "Текущий пароль неверный")
		return
	}
	if err := s.auth.SetPassword(r.Context(), c.user.ID, req.New, c.session.TokenHash); err != nil {
		s.writeAuthError(w, r, err)
		return
	}
	s.limiter.Reset(keys[0])
	s.log.Info("смена пароля", "login", c.user.Login)
	writeNoContent(w)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	var req auth.Settings
	if !decodeJSON(w, r, &req) {
		return
	}
	st, err := s.auth.UpdateSettings(r.Context(), c.user.ID, req)
	if err != nil {
		s.writeAuthError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
