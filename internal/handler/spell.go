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
}

// NewSpellHandler creates a SpellHandler.
func NewSpellHandler(spells *store.SpellStore, players *store.PlayerStore) *SpellHandler {
	return &SpellHandler{spells: spells, players: players}
}

// List handles GET /api/players/:playerId/spells.
func (h *SpellHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	spells, err := h.spells.ListForPlayer(r.Context(), playerID, userID, isAdmin)
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
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	player, err := h.players.Get(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}

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
		IsPrepared  bool     `json:"isPrepared"`
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
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

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

	updated, err := h.spells.Update(r.Context(), spellID, userID, isAdmin, bson.M(req))
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

// Delete handles DELETE /api/spells/:spellId (admin only — enforced by middleware).
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
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var slots map[string]model.SpellSlot
	if err := decodeJSON(r, &slots); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, isAdmin, bson.M{"spellSlots": slots})
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
