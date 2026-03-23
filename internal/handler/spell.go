package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type SpellHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
	arsenal   *store.ArsenalStore
}

func NewSpellHandler(players *store.PlayerStore, campaigns *store.CampaignStore, arsenal *store.ArsenalStore) *SpellHandler {
	return &SpellHandler{players: players, campaigns: campaigns, arsenal: arsenal}
}

// List handles GET /api/players/:playerId/spells.
func (h *SpellHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	// Get campaign for disabled spells filtering
	campaign, err := h.campaigns.GetByID(r.Context(), access.Player.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	disabledSet := make(map[string]bool, len(campaign.DisabledSpells))
	for _, id := range campaign.DisabledSpells {
		disabledSet[id] = true
	}

	spells := make([]model.PlayerSpell, 0)
	for _, s := range access.Player.Spells {
		if !disabledSet[s.SpellID] {
			spells = append(spells, s)
		}
	}
	writeJSON(w, http.StatusOK, spells)
}

// Create handles POST /api/players/:playerId/spells.
func (h *SpellHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req struct {
		SpellID    string `json:"spellId"`
		IsPrepared bool   `json:"isPrepared"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.SpellID == "" {
		writeError(w, http.StatusBadRequest, "spellId is required", "BAD_REQUEST")
		return
	}

	// Validate spell exists in arsenal
	spell, err := h.arsenal.GetSpell(r.Context(), req.SpellID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if spell == nil {
		writeError(w, http.StatusNotFound, "spell not found in arsenal", "NOT_FOUND")
		return
	}

	playerSpell := model.PlayerSpell{
		SpellID:    req.SpellID,
		Name:       spell.Name,
		IsPrepared: req.IsPrepared,
	}

	if err := h.players.AddSpell(r.Context(), playerID, playerSpell); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "spell already added to player", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, playerSpell)
}

// Update handles PATCH /api/players/:playerId/spells/:spellId.
func (h *SpellHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	spellID := chi.URLParam(r, "spellId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Strip immutable fields
	delete(req, "spellId")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	found, err := h.players.UpdateSpell(r.Context(), playerID, spellID, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found on player", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/players/:playerId/spells/:spellId.
func (h *SpellHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	spellID := chi.URLParam(r, "spellId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	found, err := h.players.RemoveSpell(r.Context(), playerID, spellID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found on player", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateSpellSlots handles PUT /api/players/:playerId/spell-slots.
func (h *SpellHandler) UpdateSpellSlots(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	var slots map[string]model.SpellSlot
	if err := decodeJSON(r, &slots); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, access.IsDM, bson.M{"spellSlots": slots})
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
