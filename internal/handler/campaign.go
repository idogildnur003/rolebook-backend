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

// campaignListItem is the slim shape returned by List.
// myRole and myPlayerId reflect the caller. members is included only for DM callers.
type campaignListItem struct {
	ID         string                  `json:"id"`
	MyRole     model.Role              `json:"myRole"`
	MyPlayerID string                  `json:"myPlayerId"`
	Name       string                  `json:"name"`
	ThemeImage string                  `json:"themeImage"`
	Sessions   []campaignListSession   `json:"sessions"`
	Members    []campaignMemberSummary `json:"members,omitempty"`
}

type campaignListSession struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// campaignMemberSummary is the userId-free per-member shape on the wire.
type campaignMemberSummary struct {
	PlayerID string     `json:"playerId"`
	Role     model.Role `json:"role"`
	IsActive bool       `json:"isActive"`
}

// campaignDetail is the full per-campaign shape returned by Get/Create/Update.
// Mirrors the Campaign model (sans userId leaks) and adds the caller-specific
// myRole + myPlayerId fields.
type campaignDetail struct {
	ID                string                  `json:"id"`
	MyRole            model.Role              `json:"myRole"`
	MyPlayerID        string                  `json:"myPlayerId"`
	Name              string                  `json:"name"`
	ThemeImage        string                  `json:"themeImage"`
	MapImageURI       *string                 `json:"mapImageUri"`
	MapPins           []model.MapPin          `json:"mapPins"`
	Sessions          []model.Session         `json:"sessions"`
	Members           []campaignMemberSummary `json:"members"`
	DisabledSpells    []string                `json:"disabledSpells"`
	DisabledEquipment []string                `json:"disabledEquipment"`
	UpdatedAt         time.Time               `json:"updatedAt"`
}

func toMemberSummaries(members []model.CampaignMember) []campaignMemberSummary {
	out := make([]campaignMemberSummary, len(members))
	for i, m := range members {
		out[i] = campaignMemberSummary{PlayerID: m.PlayerID, Role: m.Role, IsActive: m.IsActive}
	}
	return out
}

func toCampaignDetail(c *model.Campaign, callerUserID string) campaignDetail {
	myRole := model.Role("")
	myPlayerID := ""
	for _, m := range c.Members {
		if m.UserID == callerUserID {
			myRole = m.Role
			myPlayerID = m.PlayerID
			break
		}
	}
	return campaignDetail{
		ID:                c.ID,
		MyRole:            myRole,
		MyPlayerID:        myPlayerID,
		Name:              c.Name,
		ThemeImage:        c.ThemeImage,
		MapImageURI:       c.MapImageURI,
		MapPins:           c.MapPins,
		Sessions:          c.Sessions,
		Members:           toMemberSummaries(c.Members),
		DisabledSpells:    c.DisabledSpells,
		DisabledEquipment: c.DisabledEquipment,
		UpdatedAt:         c.UpdatedAt,
	}
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

	items := make([]campaignListItem, len(campaigns))
	for i, c := range campaigns {
		myRole := model.Role("")
		myPlayerID := ""
		for _, m := range c.Members {
			if m.UserID == userID {
				myRole = m.Role
				myPlayerID = m.PlayerID
				break
			}
		}
		sessions := make([]campaignListSession, len(c.Sessions))
		for j, s := range c.Sessions {
			sessions[j] = campaignListSession{ID: s.ID, Name: s.Name}
		}
		item := campaignListItem{
			ID:         c.ID,
			MyRole:     myRole,
			MyPlayerID: myPlayerID,
			Name:       c.Name,
			ThemeImage: c.ThemeImage,
			Sessions:   sessions,
		}
		if myRole == model.RoleDM {
			item.Members = toMemberSummaries(c.Members)
		}
		items[i] = item
	}
	writeJSON(w, http.StatusOK, items)
}

// Get handles GET /api/campaigns/:id.
func (h *CampaignHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, id)
	if membership == nil {
		return
	}
	writeJSON(w, http.StatusOK, toCampaignDetail(membership.Campaign, membership.UserID))
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
		ID:                uuid.NewString(),
		DM:                userID,
		Name:              req.Name,
		ThemeImage:        req.ThemeImage,
		MapImageURI:       req.MapImageURI,
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

// SetPlayerActive handles PATCH /api/campaigns/:id/players/:playerId (DM only).
// Body: { "isActive": bool }
// Used to archive (isActive=false) or restore (isActive=true) a campaign player.
func (h *CampaignHandler) SetPlayerActive(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "id")
	playerID := chi.URLParam(r, "playerId")
	ctx := r.Context()

	campaign, err := h.campaigns.GetByID(ctx, campaignID)
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

	var req struct {
		IsActive bool `json:"isActive"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	found, err := h.campaigns.SetPlayerActive(ctx, campaignID, playerID, req.IsActive)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "player not found in campaign", "NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"playerId": playerID,
		"isActive": req.IsActive,
	})
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
