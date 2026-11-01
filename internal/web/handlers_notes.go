package web

import (
	"net/http"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// notesRoutes — заметки (ФТ-18, docs/API.md).
func (h *catalogHandlers) notesRoutes() {
	h.s.api("/api/notes", signedIn, methods{
		http.MethodGet:  h.listNotes,
		http.MethodPost: h.createNote,
	})
	h.s.api("/api/notes/{id}", signedIn, methods{
		http.MethodGet:    h.getNote,
		http.MethodPatch:  h.updateNote,
		http.MethodDelete: h.deleteNote,
	})
}

func (h *catalogHandlers) listNotes(w http.ResponseWriter, r *http.Request) {
	product, ok := catQueryID(w, r, "product_id")
	if !ok {
		return
	}
	recipe, ok := catQueryID(w, r, "recipe_id")
	if !ok {
		return
	}
	list, err := h.cat.Notes(r.Context(), h.uid(r), catalog.NoteFilter{
		Query: r.URL.Query().Get("q"), ProductID: product, RecipeID: recipe,
	})
	h.respond(w, r, http.StatusOK, map[string]any{"notes": list}, err)
}

func (h *catalogHandlers) getNote(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Заметка не найдена")
	if !ok {
		return
	}
	n, err := h.cat.Note(r.Context(), h.uid(r), id)
	h.respond(w, r, http.StatusOK, n, err)
}

func (h *catalogHandlers) createNote(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Title      string  `json:"title"`
		Body       string  `json:"body"`
		ProductIDs []int64 `json:"product_ids"`
		RecipeIDs  []int64 `json:"recipe_ids"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	n, err := h.cat.CreateNote(r.Context(), h.uid(r), catalog.NoteInput{
		Title: b.Title, Body: b.Body, ProductIDs: b.ProductIDs, RecipeIDs: b.RecipeIDs,
	}, h.s.now().Unix())
	h.respond(w, r, http.StatusCreated, n, err)
}

func (h *catalogHandlers) updateNote(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Заметка не найдена")
	var b struct {
		Title      *string  `json:"title"`
		Body       *string  `json:"body"`
		ProductIDs *[]int64 `json:"product_ids"`
		RecipeIDs  *[]int64 `json:"recipe_ids"`
	}
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	n, err := h.cat.UpdateNote(r.Context(), h.uid(r), id, catalog.NotePatch{
		Title: b.Title, Body: b.Body, ProductIDs: b.ProductIDs, RecipeIDs: b.RecipeIDs,
	}, h.s.now().Unix())
	h.respond(w, r, http.StatusOK, n, err)
}

func (h *catalogHandlers) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Заметка не найдена")
	if !ok {
		return
	}
	if err := h.cat.DeleteNote(r.Context(), h.uid(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeNoContent(w)
}
