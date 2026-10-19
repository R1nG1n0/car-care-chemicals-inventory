package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/report"
)

// reportRoutes регистрирует отчёты (ФТ-10.2, docs/API.md).
func (s *Server) reportRoutes() {
	svc := report.New(s.db)
	s.api("/api/reports/spending", signedIn, methods{http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.reportPeriod(w, r)
		if !ok {
			return
		}
		out, err := svc.Spending(r.Context(), currentFrom(r).user.ID, p)
		s.reportRespond(w, r, out, err)
	}})
	s.api("/api/reports/consumption", signedIn, methods{http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.reportPeriod(w, r)
		if !ok {
			return
		}
		out, err := svc.Consumption(r.Context(), currentFrom(r).user.ID, p)
		s.reportRespond(w, r, out, err)
	}})
}

// reportPeriod разбирает from, to и tz; неверный параметр — 400 с field.
func (s *Server) reportPeriod(w http.ResponseWriter, r *http.Request) (report.Period, bool) {
	q := r.URL.Query()
	tz := 0
	if v := q.Get("tz"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeFieldError(w, http.StatusBadRequest, "tz", "Неверный часовой пояс")
			return report.Period{}, false
		}
		tz = n
	}
	p, err := report.ParsePeriod(q.Get("from"), q.Get("to"), tz)
	if err != nil {
		s.reportRespond(w, r, nil, err)
		return report.Period{}, false
	}
	return p, true
}

func (s *Server) reportRespond(w http.ResponseWriter, r *http.Request, v any, err error) {
	var ve *report.ValidationError
	switch {
	case errors.As(err, &ve):
		writeFieldError(w, http.StatusBadRequest, ve.Field, ve.Message)
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, v)
	}
}
