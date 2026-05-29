package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/model"
)

// catalogImageRepo is the slice of CatalogImageStore the arsenal handlers use.
type catalogImageRepo interface {
	KeysByType(ctx context.Context, t model.CatalogImageType) (map[string]string, error)
	ImageKey(ctx context.Context, t model.CatalogImageType, itemID string) (string, error)
	SetImage(ctx context.Context, t model.CatalogImageType, itemID, key string) (string, error)
	DeleteImage(ctx context.Context, t model.CatalogImageType, itemID string) (string, error)
}

type ArsenalHandler struct {
	catalog *catalog.ArsenalCatalog
	images  catalogImageRepo
	avatars *avatarstore.Store
}

func NewArsenalHandler(cat *catalog.ArsenalCatalog, images catalogImageRepo, avatars *avatarstore.Store) *ArsenalHandler {
	return &ArsenalHandler{catalog: cat, images: images, avatars: avatars}
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
	keys, err := h.images.KeysByType(r.Context(), model.CatalogImageSpell)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	out := make([]model.Spell, len(data))
	for i, item := range data { // item is a copy
		if key, ok := keys[item.ID]; ok {
			item.ImageURI = h.avatars.ResolveImageURICached(r.Context(), key)
		}
		out[i] = item
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "page": page, "limit": limit, "total": total})
}

func (h *ArsenalHandler) GetSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "spellId")
	spell := h.catalog.GetSpell(id)
	if spell == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	out := *spell // copy; never mutate the shared catalog entry
	key, err := h.images.ImageKey(r.Context(), model.CatalogImageSpell, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if key != "" {
		out.ImageURI = h.avatars.ResolveImageURICached(r.Context(), key)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ArsenalHandler) ListEquipment(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	data, total := h.catalog.ListEquipment(page, limit)
	keys, err := h.images.KeysByType(r.Context(), model.CatalogImageEquipment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	out := make([]model.Equipment, len(data))
	for i, item := range data {
		if key, ok := keys[item.ID]; ok {
			item.ImageURI = h.avatars.ResolveImageURICached(r.Context(), key)
		}
		out[i] = item
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "page": page, "limit": limit, "total": total})
}

func (h *ArsenalHandler) GetEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "equipmentId")
	item := h.catalog.GetEquipment(id)
	if item == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	out := *item
	key, err := h.images.ImageKey(r.Context(), model.CatalogImageEquipment, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if key != "" {
		out.ImageURI = h.avatars.ResolveImageURICached(r.Context(), key)
	}
	writeJSON(w, http.StatusOK, out)
}

type setImageRequest struct {
	Key string `json:"key"`
}

// SetEquipmentImage handles PUT /api/admin/arsenal/equipment/{equipmentId}/image.
func (h *ArsenalHandler) SetEquipmentImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "equipmentId")
	if h.catalog.GetEquipment(id) == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	h.setImage(w, r, model.CatalogImageEquipment, id)
}

// DeleteEquipmentImage handles DELETE /api/admin/arsenal/equipment/{equipmentId}/image.
func (h *ArsenalHandler) DeleteEquipmentImage(w http.ResponseWriter, r *http.Request) {
	h.deleteImage(w, r, model.CatalogImageEquipment, chi.URLParam(r, "equipmentId"))
}

// SetSpellImage handles PUT /api/admin/arsenal/spells/{spellId}/image.
func (h *ArsenalHandler) SetSpellImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "spellId")
	if h.catalog.GetSpell(id) == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	h.setImage(w, r, model.CatalogImageSpell, id)
}

// DeleteSpellImage handles DELETE /api/admin/arsenal/spells/{spellId}/image.
func (h *ArsenalHandler) DeleteSpellImage(w http.ResponseWriter, r *http.Request) {
	h.deleteImage(w, r, model.CatalogImageSpell, chi.URLParam(r, "spellId"))
}

func (h *ArsenalHandler) setImage(w http.ResponseWriter, r *http.Request, t model.CatalogImageType, itemID string) {
	var req setImageRequest
	if err := decodeJSON(r, &req); err != nil || req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required", "BAD_REQUEST")
		return
	}
	// Confirm the uploaded object exists (no-op when storage is unconfigured).
	if err := h.avatars.Verify(r.Context(), req.Key); err != nil {
		if errors.Is(err, avatarstore.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "uploaded image not found", "BAD_REQUEST")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	prevKey, err := h.images.SetImage(r.Context(), t, itemID, req.Key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if prevKey != "" && prevKey != req.Key {
		_ = h.avatars.Delete(r.Context(), prevKey) // best-effort orphan cleanup
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imageUri": h.avatars.ResolveImageURICached(r.Context(), req.Key),
	})
}

func (h *ArsenalHandler) deleteImage(w http.ResponseWriter, r *http.Request, t model.CatalogImageType, itemID string) {
	deletedKey, err := h.images.DeleteImage(r.Context(), t, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if deletedKey != "" {
		_ = h.avatars.Delete(r.Context(), deletedKey) // best-effort
	}
	w.WriteHeader(http.StatusNoContent)
}
