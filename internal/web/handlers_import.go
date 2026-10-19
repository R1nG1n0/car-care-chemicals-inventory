package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/transfer"
)

type importResponse struct {
	Imported transfer.Counts `json:"imported"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request, svc *transfer.Service) {
	// Файл до 100 МБ с мобильного интернета не уложится в общий срок чтения запроса.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(transferReadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(transferWriteTimeout))

	// Один импорт на сервер: большой файл занимает память и единственное
	// соединение с базой.
	release, ok := svc.BeginImport()
	if !ok {
		writeError(w, http.StatusConflict, "Импорт уже идёт. Подождите, пока он закончится, и повторите")
		return
	}
	defer release()

	// Дешёвая проверка до чтения тела: непустой склад — 409 сразу. Решающая
	// проверка всё равно внутри транзакции импорта.
	uid := currentFrom(r).user.ID
	if err := svc.CheckEmpty(r.Context(), uid); err != nil {
		s.importFailed(w, r, err)
		return
	}

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
			writeError(w, http.StatusRequestEntityTooLarge, "Файл больше 100 МБ")
		case errors.As(err, &typeErr):
			writeFieldError(w, http.StatusBadRequest, typeErr.Field, typeErr.Field+": неверный тип значения")
		default:
			writeError(w, http.StatusBadRequest, "Файл повреждён или это не JSON экспорта garage")
		}
		return
	}
	counts, err := svc.Import(r.Context(), uid, &f)
	if err != nil {
		s.importFailed(w, r, err)
		return
	}
	s.log.Info("импорт данных", "user", uid, "products", counts.Products, "movements", counts.Movements)
	writeJSON(w, http.StatusOK, importResponse{Imported: counts})
}

// importFailed переводит ошибку проверки или импорта в ответ.
func (s *Server) importFailed(w http.ResponseWriter, r *http.Request, err error) {
	var ie *transfer.ImportError
	var ne *transfer.NotEmptyError
	switch {
	case errors.As(err, &ie):
		writeFieldError(w, http.StatusBadRequest, ie.Path, ie.Error())
	case errors.As(err, &ne):
		writeError(w, http.StatusConflict, ne.Message)
	default:
		s.internalError(w, r, err)
	}
}
