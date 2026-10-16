package web

import (
	"net/http"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
)

// Шаблоны расхода (ФТ-8). Маршруты — в stockRoutes.

type stkRecipeItemJSON struct {
	ProductID int64                  `json:"product_id"`
	Qty       int64                  `json:"qty"`
	Product   catalog.ProductSummary `json:"product"`
	Available int64                  `json:"available"`
	Short     bool                   `json:"short"`
}

type stkRecipeJSON struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Sort       int64               `json:"sort"`
	LastUsedAt *int64              `json:"last_used_at"`
	Cost       *int64              `json:"cost"`
	Items      []stkRecipeItemJSON `json:"items"`
}

// recipesJSON дополняет шаблоны сводками позиций.
func (s *Server) recipesJSON(r *http.Request, list []stock.Recipe) ([]stkRecipeJSON, error) {
	var ids []int64
	for _, rc := range list {
		for _, it := range rc.Items {
			ids = append(ids, it.ProductID)
		}
	}
	sums, err := s.stockCatalog().Summaries(r.Context(), currentFrom(r).user.ID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]catalog.ProductSummary, len(sums))
	for _, p := range sums {
		byID[p.ID] = p
	}
	out := make([]stkRecipeJSON, 0, len(list))
	for _, rc := range list {
		j := stkRecipeJSON{ID: rc.ID, Name: rc.Name, Sort: rc.Sort, LastUsedAt: rc.LastUsedAt, Cost: rc.Cost,
			Items: make([]stkRecipeItemJSON, 0, len(rc.Items))}
		for _, it := range rc.Items {
			j.Items = append(j.Items, stkRecipeItemJSON{ProductID: it.ProductID, Qty: it.Qty,
				Product: byID[it.ProductID], Available: it.Available, Short: it.Short})
		}
		out = append(out, j)
	}
	return out, nil
}

// writeRecipe отвечает одним шаблоном; wrap — в виде {"recipe": …}.
func (s *Server) writeRecipe(w http.ResponseWriter, r *http.Request, code int, id int64, wrap bool) {
	list, err := s.stockSvc().Recipes(r.Context(), currentFrom(r).user.ID, id)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	out, err := s.recipesJSON(r, list)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if wrap {
		writeJSON(w, code, struct {
			Recipe stkRecipeJSON `json:"recipe"`
		}{out[0]})
		return
	}
	writeJSON(w, code, out[0])
}

func (s *Server) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	list, err := s.stockSvc().Recipes(r.Context(), currentFrom(r).user.ID, 0)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	out, err := s.recipesJSON(r, list)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Recipes []stkRecipeJSON `json:"recipes"`
	}{out})
}

func (s *Server) handleGetRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Шаблон не найден")
	if !ok {
		return
	}
	s.writeRecipe(w, r, http.StatusOK, id, true)
}

func (s *Server) handleCreateRecipe(w http.ResponseWriter, r *http.Request) {
	var in stock.RecipeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	id, err := s.stockSvc().CreateRecipe(r.Context(), currentFrom(r).user.ID, in)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeRecipe(w, r, http.StatusCreated, id, false)
}

func (s *Server) handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Шаблон не найден")
	if !ok {
		return
	}
	var in stock.RecipeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.stockSvc().UpdateRecipe(r.Context(), currentFrom(r).user.ID, id, in); err != nil {
		s.writeStockError(w, r, err)
		return
	}
	s.writeRecipe(w, r, http.StatusOK, id, false)
}

func (s *Server) handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Шаблон не найден")
	if !ok {
		return
	}
	if err := s.stockSvc().DeleteRecipe(r.Context(), currentFrom(r).user.ID, id); err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeNoContent(w)
}

func (s *Server) handleReorderRecipes(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.stockSvc().ReorderRecipes(r.Context(), currentFrom(r).user.ID, in.IDs); err != nil {
		s.writeStockError(w, r, err)
		return
	}
	writeNoContent(w)
}

func (s *Server) handleApplyRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := stkPathID(w, r, "Шаблон не найден")
	if !ok {
		return
	}
	var in struct {
		Items []stock.ItemInput `json:"items"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	uid := currentFrom(r).user.ID
	res, err := s.stockSvc().ApplyRecipe(r.Context(), uid, id, in.Items)
	if err != nil {
		s.writeStockError(w, r, err)
		return
	}
	cat := s.stockCatalog()
	ms, err := cat.MovementsByIDs(r.Context(), uid, res.MovementIDs)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	products, err := cat.Summaries(r.Context(), uid, res.ProductIDs)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		RunID     int64                    `json:"run_id"`
		Movements []catalog.Movement       `json:"movements"`
		Products  []catalog.ProductSummary `json:"products"`
		Shortages []stock.Shortage         `json:"shortages"`
		Cost      *int64                   `json:"cost"`
	}{res.RunID, ms, products, res.Shortages, stkSumCost(ms)})
}
