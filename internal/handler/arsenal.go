package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/catalog"
)

type ArsenalHandler struct {
	catalog *catalog.ArsenalCatalog
}

func NewArsenalHandler(cat *catalog.ArsenalCatalog) *ArsenalHandler {
	return &ArsenalHandler{catalog: cat}
}

func parsePagination(r *http.Request) (page, limit int64) {
	page = 1
	limit = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.ParseInt(v, 10, 64); err == nil && p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.ParseInt(v, 10, 64); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	return
}

func (h *ArsenalHandler) ListSpells(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	data, total := h.catalog.ListSpells(page, limit)
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "page": page, "limit": limit, "total": total})
}

func (h *ArsenalHandler) GetSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "spellId")
	spell := h.catalog.GetSpell(id)
	if spell == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, spell)
}

func (h *ArsenalHandler) ListEquipment(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	data, total := h.catalog.ListEquipment(page, limit)
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "page": page, "limit": limit, "total": total})
}

func (h *ArsenalHandler) GetEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "equipmentId")
	item := h.catalog.GetEquipment(id)
	if item == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
