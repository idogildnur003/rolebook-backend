package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/store"
)

type ArsenalHandler struct {
	arsenal *store.ArsenalStore
}

func NewArsenalHandler(arsenal *store.ArsenalStore) *ArsenalHandler {
	return &ArsenalHandler{arsenal: arsenal}
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
	result, err := h.arsenal.ListSpells(r.Context(), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ArsenalHandler) GetSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "spellId")
	spell, err := h.arsenal.GetSpell(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if spell == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, spell)
}

func (h *ArsenalHandler) ListEquipment(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	result, err := h.arsenal.ListEquipment(r.Context(), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ArsenalHandler) GetEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "equipmentId")
	item, err := h.arsenal.GetEquipment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
