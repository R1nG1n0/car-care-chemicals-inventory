package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// catalogRoutes регистрирует адреса справочников, позиций и списка покупок
// (docs/API.md, этап 2). База — s.db, часы — s.now.
func (s *Server) catalogRoutes() {
	h := &catalogHandlers{s: s, cat: catalog.New(s.db)}

	s.api("/api/categories", signedIn, methods{
		http.MethodGet:  h.listCategories,
		http.MethodPost: h.createCategory,
	})
	s.api("/api/categories/reorder", signedIn, methods{http.MethodPost: h.reorder(h.cat.ReorderCategories)})
	s.api("/api/categories/{id}", signedIn, methods{http.MethodPatch: h.updateCategory})

	s.api("/api/locations", signedIn, methods{
		http.MethodGet:  h.listLocations,
		http.MethodPost: h.createLocation,
	})
	s.api("/api/locations/reorder", signedIn, methods{http.MethodPost: h.reorder(h.cat.ReorderLocations)})
	s.api("/api/locations/{id}", signedIn, methods{http.MethodPatch: h.updateLocation})

	s.api("/api/stores", signedIn, methods{
		http.MethodGet:  h.listStores,
		http.MethodPost: h.createStore,
	})
	s.api("/api/stores/{id}", signedIn, methods{http.MethodPatch: h.updateStore})

	s.api("/api/products", signedIn, methods{
		http.MethodGet:  h.listProducts,
		http.MethodPost: h.createProduct,
	})
	s.api("/api/products/{id}", signedIn, methods{
		http.MethodGet:   h.getProduct,
		http.MethodPatch: h.updateProduct,
	})
	s.api("/api/brands", signedIn, methods{http.MethodGet: h.listBrands})

	s.api("/api/shopping", signedIn, methods{
		http.MethodGet:  h.listShopping,
		http.MethodPost: h.addShopping,
	})
	s.api("/api/shopping/{id}", signedIn, methods{
		http.MethodPatch:  h.patchShopping,
		http.MethodDelete: h.deleteShopping,
	})
}

type catalogHandlers struct {
	s   *Server
	cat *catalog.Service
}

// fail переводит ошибки пакета catalog в ответ.
func (h *catalogHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ce *catalog.Error
	if !errors.As(err, &ce) {
		h.s.internalError(w, r, err)
		return
	}
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, catalog.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, catalog.ErrConflict):
		code = http.StatusConflict
	}
	writeFieldError(w, code, ce.Field, ce.Message)
}

// catPathID — {id} из адреса; не число — 404 с текстом notFound.
func catPathID(w http.ResponseWriter, r *http.Request, notFound string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, notFound)
		return 0, false
	}
	return id, true
}

// catQueryID — необязательный числовой параметр запроса; 0 — не задан.
func catQueryID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		writeFieldError(w, http.StatusBadRequest, name, "Неверный параметр")
		return 0, false
	}
	return id, true
}

func catArchived(r *http.Request) bool { return r.URL.Query().Get("archived") == "1" }

func (h *catalogHandlers) uid(r *http.Request) int64 { return currentFrom(r).user.ID }

// respond пишет v с кодом code или переводит ошибку.
func (h *catalogHandlers) respond(w http.ResponseWriter, r *http.Request, code int, v any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, code, v)
}

// ---------- Справочники ----------

type catRefBody struct {
	Name     *string `json:"name"`
	Color    *string `json:"color"`
	URL      *string `json:"url"`
	Archived *bool   `json:"archived"`
}

func (b catRefBody) patch() catalog.RefPatch {
	return catalog.RefPatch{Name: b.Name, Color: b.Color, URL: b.URL, Archived: b.Archived}
}

func catStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (h *catalogHandlers) listCategories(w http.ResponseWriter, r *http.Request) {
	list, err := h.cat.Categories(r.Context(), h.uid(r), catArchived(r))
	h.respond(w, r, http.StatusOK, map[string]any{"categories": list}, err)
}

func (h *catalogHandlers) createCategory(w http.ResponseWriter, r *http.Request) {
	var b catRefBody
	if !decodeJSON(w, r, &b) {
		return
	}
	c, err := h.cat.CreateCategory(r.Context(), h.uid(r), catStr(b.Name), catStr(b.Color))
	h.respond(w, r, http.StatusCreated, c, err)
}

func (h *catalogHandlers) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Категория не найдена")
	var b catRefBody
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	b.URL = nil
	c, err := h.cat.UpdateCategory(r.Context(), h.uid(r), id, b.patch())
	h.respond(w, r, http.StatusOK, c, err)
}

// reorder — POST …/reorder {ids}: 204.
func (h *catalogHandlers) reorder(fn func(ctx context.Context, userID int64, ids []int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			IDs []int64 `json:"ids"`
		}
		if !decodeJSON(w, r, &b) {
			return
		}
		if err := fn(r.Context(), h.uid(r), b.IDs); err != nil {
			h.fail(w, r, err)
			return
		}
		writeNoContent(w)
	}
}

func (h *catalogHandlers) listLocations(w http.ResponseWriter, r *http.Request) {
	list, err := h.cat.Locations(r.Context(), h.uid(r), catArchived(r))
	h.respond(w, r, http.StatusOK, map[string]any{"locations": list}, err)
}

func (h *catalogHandlers) createLocation(w http.ResponseWriter, r *http.Request) {
	var b catRefBody
	if !decodeJSON(w, r, &b) {
		return
	}
	l, err := h.cat.CreateLocation(r.Context(), h.uid(r), catStr(b.Name))
	h.respond(w, r, http.StatusCreated, l, err)
}

func (h *catalogHandlers) updateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Место не найдено")
	var b catRefBody
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	b.Color, b.URL = nil, nil
	l, err := h.cat.UpdateLocation(r.Context(), h.uid(r), id, b.patch())
	h.respond(w, r, http.StatusOK, l, err)
}

func (h *catalogHandlers) listStores(w http.ResponseWriter, r *http.Request) {
	list, err := h.cat.Stores(r.Context(), h.uid(r), catArchived(r))
	h.respond(w, r, http.StatusOK, map[string]any{"stores": list}, err)
}

func (h *catalogHandlers) createStore(w http.ResponseWriter, r *http.Request) {
	var b catRefBody
	if !decodeJSON(w, r, &b) {
		return
	}
	st, err := h.cat.CreateStore(r.Context(), h.uid(r), catStr(b.Name), catStr(b.URL))
	h.respond(w, r, http.StatusCreated, st, err)
}

func (h *catalogHandlers) updateStore(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Магазин не найден")
	var b catRefBody
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	b.Color = nil
	st, err := h.cat.UpdateStore(r.Context(), h.uid(r), id, b.patch())
	h.respond(w, r, http.StatusOK, st, err)
}

// ---------- Позиции ----------

type catProductBody struct {
	CategoryID  *int64          `json:"category_id"`
	Name        *string         `json:"name"`
	Brand       *string         `json:"brand"`
	Unit        *string         `json:"unit"`
	PackageSize *int64          `json:"package_size"`
	Barcode     *string         `json:"barcode"`
	MinStock    json.RawMessage `json:"min_stock"`
	Note        *string         `json:"note"`
	Archived    *bool           `json:"archived"`
}

// minStock разбирает min_stock: set — поле передано; nil — null.
func (b catProductBody) minStock() (set bool, v *int64, ok bool) {
	if len(b.MinStock) == 0 {
		return false, nil, true
	}
	if bytes.Equal(bytes.TrimSpace(b.MinStock), []byte("null")) {
		return true, nil, true
	}
	var n int64
	if err := json.Unmarshal(b.MinStock, &n); err != nil {
		return true, nil, false
	}
	return true, &n, true
}

func catBadMinStock(w http.ResponseWriter) {
	writeFieldError(w, http.StatusBadRequest, "min_stock", "Минимальный остаток — пусто или целое число от 0 до 1000000")
}

func (h *catalogHandlers) listProducts(w http.ResponseWriter, r *http.Request) {
	cat, ok := catQueryID(w, r, "category_id")
	if !ok {
		return
	}
	loc, ok := catQueryID(w, r, "location_id")
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := h.cat.Products(r.Context(), h.uid(r), catalog.ProductFilter{
		Query: q.Get("q"), CategoryID: cat, LocationID: loc, Filter: q.Get("filter"), Archived: catArchived(r),
	})
	h.respond(w, r, http.StatusOK, map[string]any{"products": list}, err)
}

func (h *catalogHandlers) createProduct(w http.ResponseWriter, r *http.Request) {
	var b catProductBody
	if !decodeJSON(w, r, &b) {
		return
	}
	_, minStock, ok := b.minStock()
	if !ok {
		catBadMinStock(w)
		return
	}
	if b.CategoryID == nil {
		writeFieldError(w, http.StatusBadRequest, "category_id", "Выберите категорию")
		return
	}
	in := catalog.ProductInput{CategoryID: *b.CategoryID, Name: catStr(b.Name), Brand: catStr(b.Brand), Unit: catStr(b.Unit),
		Barcode: catStr(b.Barcode), MinStock: minStock, Note: catStr(b.Note)}
	if b.PackageSize != nil {
		in.PackageSize = *b.PackageSize
	}
	p, err := h.cat.CreateProduct(r.Context(), h.uid(r), in, h.s.now().Unix())
	h.respond(w, r, http.StatusCreated, p, err)
}

func (h *catalogHandlers) getProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Позиция не найдена")
	if !ok {
		return
	}
	d, err := h.cat.Product(r.Context(), h.uid(r), id)
	h.respond(w, r, http.StatusOK, d, err)
}

func (h *catalogHandlers) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Позиция не найдена")
	var b catProductBody
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	set, minStock, ok := b.minStock()
	if !ok {
		catBadMinStock(w)
		return
	}
	p, err := h.cat.UpdateProduct(r.Context(), h.uid(r), id, catalog.ProductPatch{
		CategoryID: b.CategoryID, Name: b.Name, Brand: b.Brand, Unit: b.Unit, PackageSize: b.PackageSize,
		Barcode: b.Barcode, MinStockSet: set, MinStock: minStock, Note: b.Note, Archived: b.Archived,
	})
	h.respond(w, r, http.StatusOK, p, err)
}

func (h *catalogHandlers) listBrands(w http.ResponseWriter, r *http.Request) {
	list, err := h.cat.Brands(r.Context(), h.uid(r))
	h.respond(w, r, http.StatusOK, map[string]any{"brands": list}, err)
}

// ---------- Список покупок ----------

func (h *catalogHandlers) listShopping(w http.ResponseWriter, r *http.Request) {
	list, err := h.cat.Shopping(r.Context(), h.uid(r), h.s.now().Unix())
	h.respond(w, r, http.StatusOK, list, err)
}

func (h *catalogHandlers) addShopping(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ProductID *int64 `json:"product_id"`
		Title     string `json:"title"`
	}
	if !decodeJSON(w, r, &b) {
		return
	}
	it, err := h.cat.AddShopping(r.Context(), h.uid(r), b.ProductID, b.Title, h.s.now().Unix())
	h.respond(w, r, http.StatusCreated, it, err)
}

func (h *catalogHandlers) patchShopping(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Пункт списка не найден")
	var b struct {
		Done *bool `json:"done"`
	}
	if !ok || !decodeJSON(w, r, &b) {
		return
	}
	if b.Done == nil {
		writeFieldError(w, http.StatusBadRequest, "done", "Укажите, выполнен ли пункт")
		return
	}
	it, err := h.cat.SetShoppingDone(r.Context(), h.uid(r), id, *b.Done, h.s.now().Unix())
	h.respond(w, r, http.StatusOK, it, err)
}

func (h *catalogHandlers) deleteShopping(w http.ResponseWriter, r *http.Request) {
	id, ok := catPathID(w, r, "Пункт списка не найден")
	if !ok {
		return
	}
	if err := h.cat.DeleteShopping(r.Context(), h.uid(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	writeNoContent(w)
}
