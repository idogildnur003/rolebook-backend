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

// CampaignHandler handles all campaign CRUD endpoints.
type CampaignHandler struct {
	campaigns *store.CampaignStore
	players   *store.PlayerStore // needed for player-role campaign visibility
}

// NewCampaignHandler creates a CampaignHandler.
func NewCampaignHandler(campaigns *store.CampaignStore, players *store.PlayerStore) *CampaignHandler {
	return &CampaignHandler{campaigns: campaigns, players: players}
}

// List handles GET /api/campaigns.
func (h *CampaignHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.UserIDFromContext(ctx)

	campaigns, err := h.campaigns.ListByUser(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, campaigns)
}

// Get handles GET /api/campaigns/:id.
func (h *CampaignHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	campaign, err := h.campaigns.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	hasAccess := campaign.DM == userID
	if !hasAccess {
		for _, p := range campaign.Players {
			if p.UserID == userID && p.IsActive {
				hasAccess = true
				break
			}
		}
	}

	if !hasAccess {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, campaign)
}

// Create handles POST /api/campaigns (DM only — enforced by middleware).
func (h *CampaignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		ThemeImage  string  `json:"themeImage"`
		MapImageURI *string `json:"mapImageUri"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())

	campaign := &model.Campaign{
		ID:          uuid.NewString(),
		DM:          userID,
		Name:        req.Name,
		ThemeImage:  req.ThemeImage,
		MapImageURI: req.MapImageURI,
		MapPins:           []model.MapPin{},
		Sessions:          []model.Session{},
		Players:           []model.CampaignPlayer{},
		DisabledSpells:    []string{},
		DisabledEquipment: []string{},
		UpdatedAt:         time.Now().UTC(),
	}
	if err := h.campaigns.Create(r.Context(), campaign); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, campaign)
}

// Update handles PATCH /api/campaigns/:id (DM only — enforced by middleware).
func (h *CampaignHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	campaign, err := h.campaigns.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if campaign.DM != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Only allow mutable fields
	allowed := map[string]bool{"name": true, "themeImage": true, "mapImageUri": true, "mapPins": true, "disabledSpells": true, "disabledEquipment": true}
	fields := bson.M{}
	for k, v := range req {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.campaigns.Update(r.Context(), id, fields)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:id (DM only — enforced by middleware).
// Cascade: deletes all players in the campaign (spells and inventory are embedded).
func (h *CampaignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	campaign, err := h.campaigns.GetByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	userID := middleware.UserIDFromContext(ctx)
	if campaign.DM != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	// Get all player IDs in this campaign for cascade
	playerIDs, err := h.players.IDsForCampaign(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Cascade via PlayerStore (deletes players + their inventory + spells)
	if err := h.players.DeleteByIDs(ctx, playerIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	found, err := h.campaigns.Delete(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
