package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// ArsenalHandler handles the global spell and equipment catalog endpoints.
type ArsenalHandler struct {
	arsenal *store.ArsenalStore
}

// NewArsenalHandler creates an ArsenalHandler.
func NewArsenalHandler(arsenal *store.ArsenalStore) *ArsenalHandler {
	return &ArsenalHandler{arsenal: arsenal}
}

// ListSpells handles GET /api/arsenal/spells.
func (h *ArsenalHandler) ListSpells(w http.ResponseWriter, r *http.Request) {
	spells, err := h.arsenal.ListSpells(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, spells)
}

// CreateSpell handles POST /api/arsenal/spells (DM only).
func (h *ArsenalHandler) CreateSpell(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Level       int      `json:"level"`
		School      string   `json:"school"`
		CastingTime string   `json:"castingTime"`
		Range       string   `json:"range"`
		Components  []string `json:"components"`
		Material    string   `json:"material"`
		Duration    string   `json:"duration"`
		Description string   `json:"description"`
		IsRitual    bool     `json:"isRitual"`
		Source      string   `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	spell := &model.ArsenalSpell{
		ID: uuid.NewString(), Name: req.Name, Level: req.Level,
		School: req.School, CastingTime: req.CastingTime, Range: req.Range,
		Components: req.Components, Material: req.Material, Duration: req.Duration,
		Description: req.Description, IsRitual: req.IsRitual, Source: req.Source,
		UpdatedAt: time.Now().UTC(),
	}
	if err := h.arsenal.CreateSpell(r.Context(), spell); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, spell)
}

// UpdateSpell handles PATCH /api/arsenal/spells/:id (DM only).
func (h *ArsenalHandler) UpdateSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "updatedAt")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.arsenal.UpdateSpell(r.Context(), id, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteSpell handles DELETE /api/arsenal/spells/:id (DM only).
func (h *ArsenalHandler) DeleteSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	found, err := h.arsenal.DeleteSpell(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListEquipment handles GET /api/arsenal/equipment.
func (h *ArsenalHandler) ListEquipment(w http.ResponseWriter, r *http.Request) {
	items, err := h.arsenal.ListEquipment(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateEquipment handles POST /api/arsenal/equipment (DM only).
func (h *ArsenalHandler) CreateEquipment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
		Notes    string   `json:"notes"`
		ImageURI string   `json:"imageUri"`

		// Weapon fields
		Damage     string   `json:"damage"`
		DamageType string   `json:"damageType"`
		WeaponType string   `json:"weaponType"`
		Properties []string `json:"properties"`

		// Armor fields
		ArmorClass          *int    `json:"armorClass"`
		ArmorBonus          *int    `json:"armorBonus"`
		ShieldBonus         *int    `json:"shieldBonus"`
		ArmorType           string  `json:"armorType"`
		StrengthRequirement *int    `json:"strengthRequirement"`
		StealthDisadvantage *bool   `json:"stealthDisadvantage"`

		// Magic item fields
		CompatibleWith *string  `json:"compatibleWith"`
		EffectSummary  string   `json:"effectSummary"`
		Value          *float64 `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.Properties == nil {
		req.Properties = []string{}
	}
	item := &model.ArsenalEquipment{
		ID:                  uuid.NewString(),
		Name:                req.Name,
		Category:            req.Category,
		Tags:                req.Tags,
		Notes:               req.Notes,
		ImageURI:            req.ImageURI,
		Damage:              req.Damage,
		DamageType:          req.DamageType,
		WeaponType:          req.WeaponType,
		Properties:          req.Properties,
		ArmorClass:          req.ArmorClass,
		ArmorBonus:          req.ArmorBonus,
		ShieldBonus:         req.ShieldBonus,
		ArmorType:           req.ArmorType,
		StrengthRequirement: req.StrengthRequirement,
		StealthDisadvantage: req.StealthDisadvantage,
		CompatibleWith:      req.CompatibleWith,
		EffectSummary:       req.EffectSummary,
		Value:               req.Value,
		UpdatedAt:           time.Now().UTC(),
	}
	if err := h.arsenal.CreateEquipment(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateEquipment handles PATCH /api/arsenal/equipment/:id (DM only).
func (h *ArsenalHandler) UpdateEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "updatedAt")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.arsenal.UpdateEquipment(r.Context(), id, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteEquipment handles DELETE /api/arsenal/equipment/:id (DM only).
func (h *ArsenalHandler) DeleteEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	found, err := h.arsenal.DeleteEquipment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
