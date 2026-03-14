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

// PlayerHandler handles all player CRUD endpoints.
// campaigns store is used only for campaign-existence validation on player Create.
type PlayerHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

// NewPlayerHandler creates a PlayerHandler.
func NewPlayerHandler(players *store.PlayerStore, campaigns *store.CampaignStore) *PlayerHandler {
	return &PlayerHandler{players: players, campaigns: campaigns}
}

// ListForCampaign handles GET /api/campaigns/:campaignId/players.
func (h *PlayerHandler) ListForCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	players, err := h.players.ListForCampaign(r.Context(), campaignID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

// Get handles GET /api/players/:playerId.
func (h *PlayerHandler) Get(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, player)
}

// Create handles POST /api/players (admin only — enforced by middleware).
func (h *PlayerHandler) Create(w http.ResponseWriter, r *http.Request) {
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
	// Verify campaign exists before creating the player
	campaign, err := h.campaigns.GetByID(r.Context(), req.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
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
	player.UpdatedAt = time.Now().UTC()

	if err := h.players.Create(r.Context(), player); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, player)
}

// Update handles PATCH /api/players/:playerId.
// Players can only update their own character; admins can update any.
// Protected fields (campaignId, linkedUserId) are stripped before applying.
func (h *PlayerHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

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

	updated, err := h.players.Update(r.Context(), playerID, userID, isAdmin, bson.M(req))
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

// Delete handles DELETE /api/players/:playerId (admin only — enforced by middleware).
func (h *PlayerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	found, err := h.players.Delete(r.Context(), playerID, "", true) // admin: isAdmin=true
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
