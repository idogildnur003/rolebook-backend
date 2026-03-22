package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// PlayerHandler handles all player CRUD endpoints.
type PlayerHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

// NewPlayerHandler creates a PlayerHandler.
func NewPlayerHandler(players *store.PlayerStore, campaigns *store.CampaignStore) *PlayerHandler {
	return &PlayerHandler{players: players, campaigns: campaigns}
}

// ListForCampaign handles GET /api/campaigns/:campaignId/players.
// DM sees all players; a regular user sees only their own.
func (h *PlayerHandler) ListForCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())

	campaign, err := h.campaigns.GetByID(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	isDM := campaign.DM == userID
	players, err := h.players.ListForCampaign(r.Context(), campaignID, userID, isDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

// Get handles GET /api/players/:playerId.
// DM of the player's campaign or the player's linked user can access.
func (h *PlayerHandler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	writeJSON(w, http.StatusOK, access.Player)
}

// Create handles POST /api/players (campaign DM only).
func (h *PlayerHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	var req struct {
		CampaignID   string `json:"campaignId"`
		Name         string `json:"name"`
		ClassName    string `json:"className"`
		Level        int    `json:"level"`
		Race         string `json:"race"`
		LinkedUserID string `json:"linkedUserId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.CampaignID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "campaignId and name are required", "BAD_REQUEST")
		return
	}

	campaign, err := h.campaigns.GetByID(r.Context(), req.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	if campaign.DM != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	if req.Level <= 0 {
		req.Level = 1
	}

	player := model.DefaultPlayer(uuid.NewString(), req.CampaignID, req.LinkedUserID, req.Name, req.Level)
	if req.ClassName != "" {
		player.ClassName = &req.ClassName
	}
	if req.Race != "" {
		player.Race = req.Race
	}

	if err := h.players.Create(r.Context(), player); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, player)
}

// Update handles PATCH /api/players/:playerId.
// DM of the player's campaign or the player's linked user can update.
// Protected fields (campaignId, linkedUserId) are stripped before applying.
func (h *PlayerHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	userID := middleware.UserIDFromContext(r.Context())

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	// Strip protected fields
	delete(req, "campaignId")
	delete(req, "linkedUserId")
	delete(req, "_id")
	delete(req, "id")

	// Validate death save bounds
	if v, ok := req["deathSaveSuccesses"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveSuccesses must be 0-3", "BAD_REQUEST")
			return
		}
	}
	if v, ok := req["deathSaveFailures"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveFailures must be 0-3", "BAD_REQUEST")
			return
		}
	}

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, access.IsDM, bson.M(req))
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

// Delete handles DELETE /api/players/:playerId (campaign DM only).
func (h *PlayerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	if !access.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	found, err := h.players.Delete(r.Context(), playerID, userID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
