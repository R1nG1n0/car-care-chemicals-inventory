package web

import "net/http"

// locationUnits — GET /api/locations/{id}/units: экземпляры места для инвентаризации.
func (h *catalogHandlers) locationUnits(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Место не найдено")
	if !ok {
		return
	}
	list, err := h.cat.LocationUnits(r.Context(), h.uid(r), id)
	h.respond(w, r, http.StatusOK, map[string]any{"units": list}, err)
}

// locationChecked — POST /api/locations/{id}/checked: отметка «проверено» (сейчас).
func (h *catalogHandlers) locationChecked(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Место не найдено")
	if !ok {
		return
	}
	l, err := h.cat.MarkLocationChecked(r.Context(), h.uid(r), id, h.s.now().Unix())
	h.respond(w, r, http.StatusOK, l, err)
}
