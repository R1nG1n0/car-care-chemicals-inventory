package web

import (
	"net/http"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/bot"
)

// TelegramBot — что сайту нужно знать о боте (ФТ-11.2): включён ли он и как
// его зовут. Имя становится известно после первого getMe.
type TelegramBot interface {
	Enabled() bool
	BotUsername() string
}

// telegramRoutes регистрирует адреса привязки Telegram (docs/API.md).
func (s *Server) telegramRoutes() {
	links := bot.NewLinks(s.db, s.now)
	s.api("/api/telegram", signedIn, methods{
		http.MethodGet:    s.handleTelegramStatus(links),
		http.MethodDelete: s.handleTelegramUnlink(links),
	})
	s.api("/api/telegram/link", signedIn, methods{http.MethodPost: s.handleTelegramLink(links)})
}

// botUsername — имя бота или пусто, если бот выключен или ещё не назвался.
func (s *Server) botUsername() string {
	if s.telegram == nil || !s.telegram.Enabled() {
		return ""
	}
	return s.telegram.BotUsername()
}

type telegramStatus struct {
	Enabled      bool    `json:"enabled"`
	BotUsername  *string `json:"bot_username"`
	Linked       bool    `json:"linked"`
	TelegramName *string `json:"telegram_name"`
	LinkedAt     *int64  `json:"linked_at"`
}

// GET /api/telegram — состояние бота и привязки пользователя.
func (s *Server) handleTelegramStatus(links *bot.Links) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := telegramStatus{Enabled: s.telegram != nil && s.telegram.Enabled()}
		if name := s.botUsername(); name != "" {
			out.BotUsername = &name
		}
		link, ok, err := links.ByUser(r.Context(), currentFrom(r).user.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if ok {
			out.Linked = true
			out.TelegramName = &link.TGName
			out.LinkedAt = &link.LinkedAt
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// POST /api/telegram/link — одноразовая ссылка t.me/<бот>?start=<код>, 10 минут.
func (s *Server) handleTelegramLink(links *bot.Links) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := s.botUsername()
		if name == "" {
			writeError(w, http.StatusServiceUnavailable, "Бот не настроен")
			return
		}
		code, exp, err := links.NewCode(r.Context(), currentFrom(r).user.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		// Код нигде не журналируется: он одноразовый, но это секрет привязки.
		s.log.Info("создана ссылка привязки Telegram", "login", currentFrom(r).user.Login)
		writeJSON(w, http.StatusOK, struct {
			URL       string `json:"url"`
			ExpiresAt int64  `json:"expires_at"`
		}{"https://t.me/" + name + "?start=" + code, exp})
	}
}

// DELETE /api/telegram — отключить Telegram: привязка и состояние уведомлений.
func (s *Server) handleTelegramUnlink(links *bot.Links) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := links.Unlink(r.Context(), currentFrom(r).user.ID); err != nil {
			s.internalError(w, r, err)
			return
		}
		s.log.Info("Telegram отключён", "login", currentFrom(r).user.Login)
		writeNoContent(w)
	}
}
