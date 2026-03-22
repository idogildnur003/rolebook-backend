package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// SpellHandler handles spell and spell-slot endpoints.
type SpellHandler struct {
	spells  *store.SpellStore
	players *store.PlayerStore
	arsenal *store.ArsenalStore
}

// NewSpellHandler creates a SpellHandler.
func NewSpellHandler(spells *store.SpellStore, players *store.PlayerStore, arsenal *store.ArsenalStore) *SpellHandler {
	return &SpellHandler{spells: spells, players: players, arsenal: arsenal}
}

// List handles GET /api/players/:playerId/spells.
func (h *SpellHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isDM := middleware.RoleFromContext(r.Context()) == model.RoleDM

	spells, err := h.spells.ListForPlayer(r.Context(), playerID, userID, isDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, spells)
}

// Create handles POST /api/players/:playerId/spells.
func (h *SpellHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isDM := middleware.RoleFromContext(r.Context()) == model.RoleDM

	player, err := h.players.Get(r.Context(), playerID, userID, isDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}

	var req struct {
		ArsenalSpellID string   `json:"arsenalSpellId"`
		Name           string   `json:"name"`
		Level          int      `json:"level"`
		School         string   `json:"school"`
		CastingTime    string   `json:"castingTime"`
		Range          string   `json:"range"`
		Components     []string `json:"components"`
		Material       string   `json:"material"`
		Duration       string   `json:"duration"`
		Description    string   `json:"description"`
		IsPrepared     bool     `json:"isPrepared"`
		IsRitual       bool     `json:"isRitual"`
		Source         string   `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	// If an arsenal spell ID is provided, copy fields from the catalog.
	if req.ArsenalSpellID != "" {
		src, err := h.arsenal.GetSpell(r.Context(), req.ArsenalSpellID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if src == nil {
			writeError(w, http.StatusNotFound, "arsenal spell not found", "NOT_FOUND")
			return
		}
		// Populate from catalog; client may still override isPrepared.
		req.Name = src.Name
		req.Level = src.Level
		req.School = src.School
		req.CastingTime = src.CastingTime
		req.Range = src.Range
		req.Components = src.Components
		req.Material = src.Material
		req.Duration = src.Duration
		req.Description = src.Description
		req.IsRitual = src.IsRitual
		req.Source = src.Source
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	spell := &model.Spell{
		ID:           uuid.NewString(),
		PlayerID:     playerID,
		LinkedUserID: player.LinkedUserID,
		Name:         req.Name,
		Level:        req.Level,
		School:       req.School,
		CastingTime:  req.CastingTime,
		Range:        req.Range,
		Components:   req.Components,
		Material:     req.Material,
		Duration:     req.Duration,
		Description:  req.Description,
		IsPrepared:   req.IsPrepared,
		IsRitual:     req.IsRitual,
		Source:       req.Source,
		UpdatedAt:    time.Now().UTC(),
	}
	if err := h.spells.Create(r.Context(), spell); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, spell)
}

// Update handles PATCH /api/spells/:spellId.
func (h *SpellHandler) Update(w http.ResponseWriter, r *http.Request) {
	spellID := chi.URLParam(r, "spellId")
	userID := middleware.UserIDFromContext(r.Context())
	isDM := middleware.RoleFromContext(r.Context()) == model.RoleDM

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "playerId")
	delete(req, "linkedUserId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.spells.Update(r.Context(), spellID, userID, isDM, bson.M(req))
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

// Delete handles DELETE /api/spells/:spellId (DM only — enforced by middleware).
func (h *SpellHandler) Delete(w http.ResponseWriter, r *http.Request) {
	spellID := chi.URLParam(r, "spellId")
	userID := middleware.UserIDFromContext(r.Context())
	found, err := h.spells.Delete(r.Context(), spellID, userID, true)
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

// UpdateSpellSlots handles PUT /api/players/:playerId/spell-slots.
// Replaces the entire spell slot map atomically. Returns the full updated player object.
func (h *SpellHandler) UpdateSpellSlots(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isDM := middleware.RoleFromContext(r.Context()) == model.RoleDM

	var slots map[string]model.SpellSlot
	if err := decodeJSON(r, &slots); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, isDM, bson.M{"spellSlots": slots})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
