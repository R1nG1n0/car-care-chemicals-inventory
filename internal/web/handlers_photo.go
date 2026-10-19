package web

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/photo"
)

// photoMaxBody — предел тела PUT /api/products/{id}/photo (ФТ-10.4.3):
// только на этом адресе, остальные принимают до maxBody.
const photoMaxBody = photo.MaxBytes

// photoRoutes регистрирует фото позиции (ФТ-10.4).
func (h *catalogHandlers) photoRoutes() {
	h.s.apiLimited("/api/products/{id}/photo", signedIn, photoMaxBody, methods{
		http.MethodGet:    h.getPhoto,
		http.MethodPut:    h.putPhoto,
		http.MethodDelete: h.deletePhoto,
	})
}

// putPhoto принимает сырые байты картинки. Заголовок Content-Type не
// проверяется: тип определяется по содержимому.
func (h *catalogHandlers) putPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Позиция не найдена")
	if !ok {
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeFieldError(w, http.StatusRequestEntityTooLarge, "photo", "Фото больше 400 КБ")
			return
		}
		writeError(w, http.StatusBadRequest, "Не удалось прочитать фото")
		return
	}
	p, err := h.cat.SetPhoto(r.Context(), h.uid(r), id, data, h.s.now().Unix())
	h.respond(w, r, http.StatusOK, p, err)
}

// getPhoto отдаёт байты. Адрес с ?v=<photo_updated_at> неизменен, поэтому
// кэш браузера живёт год (с Vary: Cookie — отдельно для каждой сессии); чужая позиция и позиция без фото — одинаковый 404.
func (h *catalogHandlers) getPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Позиция не найдена")
	if !ok {
		return
	}
	p, err := h.cat.Photo(r.Context(), h.uid(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", p.Mime)
	hd.Set("Content-Length", strconv.Itoa(len(p.Data)))
	hd.Set("Cache-Control", "private, max-age=31536000, immutable")
	// Один браузер — несколько пользователей: кэш по адресу не должен отдать
	// фото владельца тому, кто вошёл после него (кэш учитывает cookie сессии).
	hd.Set("Vary", "Cookie")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(p.Data)
	}
}

func (h *catalogHandlers) deletePhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Позиция не найдена")
	if !ok {
		return
	}
	p, err := h.cat.DeletePhoto(r.Context(), h.uid(r), id)
	h.respond(w, r, http.StatusOK, p, err)
}
