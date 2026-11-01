package web

import (
	"database/sql"
	"net/http"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
)

// ragRoutes — тряпки и губки с ресурсом (ФТ-19, docs/API.md).
func (s *Server) ragRoutes() {
	s.api("/api/products/{id}/rags", signedIn, methods{
		http.MethodGet:  s.handleListRags,
		http.MethodPost: s.handleStartRag,
	})
	s.api("/api/rags/wash", signedIn, methods{http.MethodPost: s.handleWashRags(true)})
	s.api("/api/rags/unwash", signedIn, methods{http.MethodPost: s.handleWashRags(false)})
	s.api("/api/rags/{id}", signedIn, methods{http.MethodPatch: s.handleRenameRag})
	s.api("/api/rags/{id}/unstart", signedIn, methods{http.MethodPost: s.handleUnstartRag})
	s.api("/api/rags/{id}/retire", signedIn, methods{http.MethodPost: s.handleRetireRag})
}

type ragsResponse struct {
	Rags []stock.Rag `json:"rags"`
}

func (s *Server) handleListRags(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Позиция не найдена")
	if !ok {
		return
	}
	list, err := s.stockSvc().Rags(r.Context(), currentFrom(r).user.ID, id, r.URL.Query().Get("all") == "1")
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ragsResponse{list})
}

func (s *Server) handleStartRag(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Позиция не найдена")
	var in stock.RagStartInput
	if !ok || !decodeJSON(w, r, &in) {
		return
	}
	uid := currentFrom(r).user.ID
	render := func(tx *sql.Tx, res stock.RagStartResult) ([]byte, error) {
		p, err := s.stockCatalog().WithTx(tx).Summary(r.Context(), uid, id)
		if err != nil {
			return nil, err
		}
		return stkEncode(struct {
			Rag         stock.Rag              `json:"rag"`
			Product     catalog.ProductSummary `json:"product"`
			MovementIDs []int64                `json:"movement_ids"`
		}{res.Rag, p, res.Consume.MovementIDs})
	}
	_, body, replay, err := s.stockSvc().StartRag(r.Context(), uid, id, in, render)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeOp(w, body, replay)
}

func (s *Server) handleWashRags(wash bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if !decodeJSON(w, r, &in) {
			return
		}
		uid := currentFrom(r).user.ID
		fn := s.stockSvc().Unwash
		if wash {
			fn = s.stockSvc().Wash
		}
		list, err := fn(r.Context(), uid, in.IDs)
		if err != nil {
			s.writeStockError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, ragsResponse{list})
	}
}

func (s *Server) handleRenameRag(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Тряпка не найдена")
	var in struct {
		Label string `json:"label"`
	}
	if !ok || !decodeJSON(w, r, &in) {
		return
	}
	rag, err := s.stockSvc().RenameRag(r.Context(), currentFrom(r).user.ID, id, in.Label)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rag)
}

func (s *Server) handleRetireRag(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Тряпка не найдена")
	var in struct {
		Reason string `json:"reason"`
	}
	if !ok || !decodeJSON(w, r, &in) {
		return
	}
	rag, err := s.stockSvc().RetireRag(r.Context(), currentFrom(r).user.ID, id, in.Reason)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rag)
}

// handleUnstartRag — отмена «В работу»: 200 {"product": ProductSummary}.
func (s *Server) handleUnstartRag(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Тряпка не найдена")
	if !ok {
		return
	}
	uid := currentFrom(r).user.ID
	productID, err := s.stockSvc().UnstartRag(r.Context(), uid, id)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	p, err := s.stockCatalog().Summary(r.Context(), uid, productID)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Product catalog.ProductSummary `json:"product"`
	}{p})
}
