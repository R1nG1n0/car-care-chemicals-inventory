package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

// importMaxBody — предел файла импорта (ФТ-10.3.3, docs/API.md). Действует
// только на POST /api/import; остальные адреса принимают до maxBody.
const importMaxBody = 25 << 20

// transferRoutes регистрирует экспорт и импорт (ФТ-10.3).
func (s *Server) transferRoutes() {
	svc := transfer.New(s.db, s.now)
	s.api("/api/export.json", signedIn, methods{http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
		u := currentFrom(r).user
		f, err := svc.Export(r.Context(), u.ID)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		body, err := json.Marshal(f)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		s.sendFile(w, r, "garage-"+u.Login+"-"+s.fileDate(r)+".json", "application/json; charset=utf-8", body)
	}})
	csvRoute := func(path, name string, build func(r *http.Request, uid int64, tz int) ([]byte, error)) {
		s.api(path, signedIn, methods{http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
			tz, ok := s.clientTZ(w, r)
			if !ok {
				return
			}
			u := currentFrom(r).user
			body, err := build(r, u.ID, tz)
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			s.sendFile(w, r, "garage-"+u.Login+"-"+name+"-"+s.fileDate(r)+".csv", "text/csv; charset=utf-8", body)
		}})
	}
	csvRoute("/api/export/products.csv", "products", func(r *http.Request, uid int64, _ int) ([]byte, error) {
		return svc.ProductsCSV(r.Context(), uid)
	})
	csvRoute("/api/export/purchases.csv", "purchases", func(r *http.Request, uid int64, _ int) ([]byte, error) {
		return svc.PurchasesCSV(r.Context(), uid)
	})
	csvRoute("/api/export/movements.csv", "movements", func(r *http.Request, uid int64, tz int) ([]byte, error) {
		return svc.MovementsCSV(r.Context(), uid, tz)
	})

	s.apiLimited("/api/import", signedIn, importMaxBody, methods{http.MethodPost: func(w http.ResponseWriter, r *http.Request) {
		s.handleImport(w, r, svc)
	}})
}

// clientTZ разбирает необязательный tz — смещение часового пояса клиента, минуты от UTC.
func (s *Server) clientTZ(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("tz")
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < -14*60 || n > 14*60 {
		writeFieldError(w, http.StatusBadRequest, "tz", "Неверный часовой пояс")
		return 0, false
	}
	return n, true
}

// fileDate — сегодняшняя дата для имени файла (в поясе клиента, если он передан).
func (s *Server) fileDate(r *http.Request) string {
	tz, _ := strconv.Atoi(r.URL.Query().Get("tz"))
	if tz < -14*60 || tz > 14*60 {
		tz = 0
	}
	return s.now().UTC().Add(time.Duration(tz) * time.Minute).Format("2006-01-02")
}

func (s *Server) sendFile(w http.ResponseWriter, r *http.Request, name, contentType string, body []byte) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

type importResponse struct {
	Imported transfer.Counts `json:"imported"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request, svc *transfer.Service) {
	// Файл до 25 МБ с мобильного интернета не уложится в общий срок чтения запроса.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(3 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(4 * time.Minute))

	var f transfer.File
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&f)
	if err == nil {
		if _, tail := dec.Token(); !errors.Is(tail, io.EOF) {
			err = errors.New("лишние данные после JSON")
		}
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &tooBig):
			writeError(w, http.StatusRequestEntityTooLarge, "Файл больше 25 МБ")
		case errors.As(err, &typeErr):
			writeFieldError(w, http.StatusBadRequest, typeErr.Field, typeErr.Field+": неверный тип значения")
		default:
			writeError(w, http.StatusBadRequest, "Файл повреждён или это не JSON экспорта garage")
		}
		return
	}
	counts, err := svc.Import(r.Context(), currentFrom(r).user.ID, &f)
	var ie *transfer.ImportError
	var ne *transfer.NotEmptyError
	switch {
	case errors.As(err, &ie):
		writeFieldError(w, http.StatusBadRequest, ie.Path, ie.Error())
	case errors.As(err, &ne):
		writeError(w, http.StatusConflict, ne.Message)
	case err != nil:
		s.internalError(w, r, err)
	default:
		s.log.Info("импорт данных", "user", currentFrom(r).user.ID, "products", counts.Products, "movements", counts.Movements)
		writeJSON(w, http.StatusOK, importResponse{Imported: counts})
	}
}
