package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
)

// inviteJSON — приглашение в ответе API: без токена.
type inviteJSON struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	Status    string `json:"status"`
}

func toInviteJSON(i auth.Invite) inviteJSON {
	return inviteJSON{ID: i.ID, CreatedAt: i.CreatedAt.Unix(), ExpiresAt: i.ExpiresAt.Unix(), Status: i.Status}
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	list, err := s.auth.ListInvites(r.Context(), currentFrom(r).user.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]inviteJSON, 0, len(list))
	for _, i := range list {
		out = append(out, toInviteJSON(i))
	}
	writeJSON(w, http.StatusOK, struct {
		Invites []inviteJSON `json:"invites"`
	}{out})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	inv, token, err := s.auth.CreateInvite(r.Context(), c.user.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.log.Info("создано приглашение", "login", c.user.Login, "invite", inv.ID)
	writeJSON(w, http.StatusCreated, struct {
		Invite inviteJSON `json:"invite"`
		URL    string     `json:"url"`
	}{toInviteJSON(inv), requestScheme(r) + "://" + r.Host + "/invite/" + token})
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	c := currentFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "Приглашение не найдено")
		return
	}
	if err := s.auth.RevokeInvite(r.Context(), c.user.ID, id); err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Приглашение не найдено")
			return
		}
		s.internalError(w, r, err)
		return
	}
	s.log.Info("приглашение отозвано", "login", c.user.Login, "invite", id)
	writeNoContent(w)
}

func (s *Server) handleCheckInvite(w http.ResponseWriter, r *http.Request) {
	exp, err := s.auth.CheckInvite(r.Context(), r.PathValue("token"))
	if err != nil {
		s.writeAuthError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ExpiresAt int64 `json:"expires_at"`
	}{exp.Unix()})
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := s.auth.AcceptInvite(r.Context(), r.PathValue("token"), req.Login, req.Password)
	if err != nil {
		s.writeAuthError(w, r, err)
		return
	}
	s.log.Info("приглашение принято", "login", u.Login, "ip", clientIP(r))
	s.startSession(w, r, u, http.StatusCreated)
}
