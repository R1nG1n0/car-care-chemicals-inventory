package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

// Пределы импорта и экспорта (ФТ-10.3, docs/API.md). Согласованы с nginx
// (deploy/garage.nginx.conf, location = /api/import) и клиентом (web/src/api/transfer.ts).
const (
	// importMaxBody действует только на POST /api/import; остальные адреса принимают до maxBody.
	importMaxBody = 100 << 20
	// Сроки чтения тела, записи ответа импорта и записи файла экспорта (общий
	// WriteTimeout сервера — 30 с, для больших файлов их продлевают).
	transferReadTimeout  = 10 * time.Minute
	transferWriteTimeout = 12 * time.Minute
)

// transferRoutes регистрирует экспорт и импорт (ФТ-10.3).
func (s *Server) transferRoutes() {
	svc := transfer.New(s.db, s.now)
	s.api("/api/export.json", signedIn, methods{http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
		s.handleExportJSON(w, r, svc)
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

// handleExportJSON отдаёт файл потоком. ?photos=0 — без фото позиций.
func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request, svc *transfer.Service) {
	u := currentFrom(r).user
	withPhotos := r.URL.Query().Get("photos") != "0"
	f, err := svc.Export(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(transferWriteTimeout))
	name := "garage-" + u.Login + "-" + s.fileDate(r)
	if !withPhotos {
		name += "-nophoto"
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	// Заголовок уже отправлен: ошибка посередине оставит оборванный файл
	// (он не пройдёт импорт), поэтому её нужно хотя бы записать.
	if err := svc.WriteJSON(r.Context(), u.ID, w, f, withPhotos); err != nil {
		s.log.Error("экспорт оборван", "user", u.ID, "error", err)
	}
}
