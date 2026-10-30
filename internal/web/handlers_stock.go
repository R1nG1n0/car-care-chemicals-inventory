package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
)

// stockRoutes регистрирует адреса покупок, экземпляров, расхода, движений
// и шаблонов (docs/API.md, этап 2). База — s.db, часы — s.now.
//
// Пишет пакет stock, ответы (сводки позиций, экземпляры, движения) читает
// пакет catalog — уже после фиксации транзакции записи: у базы одно
// соединение, чтение мимо открытой транзакции ждало бы её вечно.
func (s *Server) stockRoutes() {
	s.api("/api/purchases", signedIn, methods{
		http.MethodGet:  s.handleListPurchases,
		http.MethodPost: s.handleCreatePurchase,
	})
	s.api("/api/purchases/{id}", signedIn, methods{http.MethodGet: s.handleGetPurchase})

	s.api("/api/units", signedIn, methods{http.MethodPost: s.handleAddUnit})
	s.api("/api/units/{id}/move", signedIn, methods{http.MethodPost: s.handleMoveUnit})
	s.api("/api/units/{id}/adjust", signedIn, methods{http.MethodPost: s.handleAdjustUnit})
	s.api("/api/units/{id}/writeoff", signedIn, methods{http.MethodPost: s.handleWriteoffUnit})

	s.api("/api/consume", signedIn, methods{http.MethodPost: s.handleConsume})
	s.api("/api/movements", signedIn, methods{http.MethodGet: s.handleListMovements})
	s.api("/api/movements/revert", signedIn, methods{http.MethodPost: s.handleRevert})

	s.api("/api/recipes", signedIn, methods{
		http.MethodGet:  s.handleListRecipes,
		http.MethodPost: s.handleCreateRecipe,
	})
	s.api("/api/recipes/reorder", signedIn, methods{http.MethodPost: s.handleReorderRecipes})
	s.api("/api/recipes/{id}", signedIn, methods{
		http.MethodGet:    s.handleGetRecipe,
		http.MethodPut:    s.handleUpdateRecipe,
		http.MethodDelete: s.handleDeleteRecipe,
	})
	s.api("/api/recipes/{id}/apply", signedIn, methods{http.MethodPost: s.handleApplyRecipe})
}

func (s *Server) stockSvc() *stock.Service       { return stock.New(s.db, s.now) }
func (s *Server) stockCatalog() *catalog.Service { return catalog.New(s.db) }

type stkShortageResponse struct {
	Error     string `json:"error"`
	Available int64  `json:"available"`
}

// writeStockError переводит ошибки пакетов stock и catalog в ответ.
func (s *Server) writeStockError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		ve *stock.ValidationError
		nf *stock.NotFoundError
		ce *stock.ConflictError
		se *stock.ShortageError
		ke *catalog.Error
	)
	switch {
	case errors.As(err, &ve):
		writeFieldError(w, http.StatusBadRequest, ve.Field, ve.Message)
	case errors.As(err, &nf):
		writeError(w, http.StatusNotFound, nf.Message)
	case errors.As(err, &ce):
		writeFieldError(w, http.StatusConflict, ce.Field, ce.Message)
	case errors.As(err, &se):
		writeJSON(w, http.StatusConflict, stkShortageResponse{Error: se.Error(), Available: se.Available})
	case errors.As(err, &ke) && errors.Is(err, catalog.ErrNotFound):
		writeError(w, http.StatusNotFound, ke.Message)
	default:
		s.internalError(w, r, err)
	}
}

// stkPathID — id из адреса; неверный — 404 с текстом notFound.
func stkPathID(w http.ResponseWriter, r *http.Request, notFound string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, notFound)
		return 0, false
	}
	return id, true
}

// stkQueryInt — целый параметр запроса; пусто — 0, неверный — 400.
func stkQueryInt(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		writeFieldError(w, http.StatusBadRequest, name, "Неверное значение параметра "+name)
		return 0, false
	}
	return n, true
}

// ---------- Покупки ----------

func (s *Server) handleCreatePurchase(w http.ResponseWriter, r *http.Request) {
	var in stock.PurchaseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	uid := currentFrom(r).user.ID
	st := s.stockSvc()
	id, err := st.CreatePurchase(r.Context(), uid, in)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	p, err := st.Purchase(r.Context(), uid, id)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Purchase stock.Purchase `json:"purchase"`
	}{p})
}

func (s *Server) handleListPurchases(w http.ResponseWriter, r *http.Request) {
	before, ok := stkQueryInt(w, r, "before")
	if !ok {
		return
	}
	limit, ok := stkQueryInt(w, r, "limit")
	if !ok {
		return
	}
	list, err := s.stockSvc().Purchases(r.Context(), currentFrom(r).user.ID, before, int(min(limit, 1000)))
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Purchases []stock.PurchaseShort `json:"purchases"`
	}{list})
}

func (s *Server) handleGetPurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Покупка не найдена")
	if !ok {
		return
	}
	p, err := s.stockSvc().Purchase(r.Context(), currentFrom(r).user.ID, id)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Purchase stock.Purchase `json:"purchase"`
	}{p})
}

// ---------- Экземпляры ----------

// writeUnit отвечает экземпляром в формате API.
func (s *Server) writeUnit(w http.ResponseWriter, r *http.Request, code int, unitID int64) {
	u, err := s.stockCatalog().UnitByID(r.Context(), currentFrom(r).user.ID, unitID)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, code, u)
}

// writeUnitMovement отвечает экземпляром и полем movement — движением
// корректировки или списания (для «Отменить»); null — ничего не изменилось.
func (s *Server) writeUnitMovement(w http.ResponseWriter, r *http.Request, unitID, movementID int64) {
	user := currentFrom(r).user.ID
	u, err := s.stockCatalog().UnitByID(r.Context(), user, unitID)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	var mv *catalog.Movement
	if movementID != 0 {
		ms, err := s.stockCatalog().MovementsByIDs(r.Context(), user, []int64{movementID})
		if err != nil {
			s.writeStockError(w, r, err)
			return
		}
		if len(ms) == 1 {
			mv = &ms[0]
		}
	}
	writeJSON(w, http.StatusOK, struct {
		catalog.Unit
		Movement *catalog.Movement `json:"movement"`
	}{u, mv})
}

func (s *Server) handleAddUnit(w http.ResponseWriter, r *http.Request) {
	var in stock.AddUnitInput
	if !decodeJSON(w, r, &in) {
		return
	}
	id, err := s.stockSvc().AddUnit(r.Context(), currentFrom(r).user.ID, in)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeUnit(w, r, http.StatusCreated, id)
}

func (s *Server) handleMoveUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Экземпляр не найден")
	if !ok {
		return
	}
	var in struct {
		LocationID *int64 `json:"location_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.LocationID == nil {
		writeFieldError(w, http.StatusBadRequest, "location_id", "Выберите место")
		return
	}
	if err := s.stockSvc().Move(r.Context(), currentFrom(r).user.ID, id, *in.LocationID); err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeUnit(w, r, http.StatusOK, id)
}

func (s *Server) handleAdjustUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Экземпляр не найден")
	if !ok {
		return
	}
	var in struct {
		RemainingQty *int64 `json:"remaining_qty"`
		Reason       string `json:"reason"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.RemainingQty == nil {
		writeFieldError(w, http.StatusBadRequest, "remaining_qty", "Укажите остаток")
		return
	}
	moved, err := s.stockSvc().Adjust(r.Context(), currentFrom(r).user.ID, id, *in.RemainingQty, in.Reason)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeUnitMovement(w, r, id, moved)
}

func (s *Server) handleWriteoffUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Экземпляр не найден")
	if !ok {
		return
	}
	var in struct {
		Qty    *int64 `json:"qty"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	moved, err := s.stockSvc().Writeoff(r.Context(), currentFrom(r).user.ID, id, in.Qty, in.Reason)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeUnitMovement(w, r, id, moved)
}

// ---------- Расход и журнал ----------

// stkSumCost складывает себестоимость движений; nil — ни у одного нет цены.
func stkSumCost(ms []catalog.Movement) *int64 {
	var sum *int64
	for _, m := range ms {
		if m.Cost == nil {
			continue
		}
		if sum == nil {
			sum = new(int64)
		}
		*sum += *m.Cost
	}
	return sum
}

// writeOp отвечает на расход или применение шаблона: впервые — 201 с
// собранным ответом, повтор по номеру операции — 200 с сохранённым ответом
// и заголовком X-Garage-Replayed (ФТ-12.4.1).
func writeOp(w http.ResponseWriter, body []byte, replay bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	code := http.StatusCreated
	if replay {
		w.Header().Set("X-Garage-Replayed", "1")
		code = http.StatusOK
	}
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

// stkEncode — JSON ответа с переводом строки, как у writeJSON.
func stkEncode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	return append(b, '\n'), err
}

func (s *Server) handleConsume(w http.ResponseWriter, r *http.Request) {
	var in stock.ConsumeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	uid := currentFrom(r).user.ID
	// Ответ собирается внутри транзакции списания (чтения идут через tx) и
	// при client_op_id сохраняется вместе с ним.
	render := func(tx *sql.Tx, res stock.ConsumeResult) ([]byte, error) {
		cat := s.stockCatalog().WithTx(tx)
		ms, err := cat.MovementsByIDs(r.Context(), uid, res.MovementIDs)
		if err != nil {
			return nil, err
		}
		p, err := cat.Summary(r.Context(), uid, res.ProductID)
		if err != nil {
			return nil, err
		}
		return stkEncode(struct {
			Movements       []catalog.Movement     `json:"movements"`
			Product         catalog.ProductSummary `json:"product"`
			OpenedUnitIDs   []int64                `json:"opened_unit_ids"`
			FinishedUnitIDs []int64                `json:"finished_unit_ids"`
			Cost            *int64                 `json:"cost"`
			Solution        *stock.SolutionInfo    `json:"solution"`
		}{ms, p, res.OpenedUnitIDs, res.FinishedUnitIDs, stkSumCost(ms), res.Solution})
	}
	_, body, replay, err := s.stockSvc().ConsumeOp(r.Context(), uid, in, render)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeOp(w, body, replay)
}

func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	uid := currentFrom(r).user.ID
	ids, err := s.stockSvc().Revert(r.Context(), uid, in.IDs)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	products, err := s.stockCatalog().Summaries(r.Context(), uid, ids)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Products []catalog.ProductSummary `json:"products"`
	}{products})
}

func (s *Server) handleListMovements(w http.ResponseWriter, r *http.Request) {
	var args [3]int64
	for i, name := range []string{"product_id", "before", "limit"} {
		v, ok := stkQueryInt(w, r, name)
		if !ok {
			return
		}
		args[i] = v
	}
	uid := currentFrom(r).user.ID
	ids, err := s.stockSvc().MovementIDs(r.Context(), uid, args[0], args[1], int(min(args[2], 1000)))
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	ms, err := s.stockCatalog().MovementsByIDs(r.Context(), uid, ids)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Movements []catalog.Movement `json:"movements"`
	}{ms})
}
