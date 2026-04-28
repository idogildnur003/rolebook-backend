package handler

import (
	"errors"
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
	players   *store.PlayerStore
	users     *store.UserStore
	db        *store.DB
}

// NewCampaignHandler creates a CampaignHandler.
func NewCampaignHandler(campaigns *store.CampaignStore, players *store.PlayerStore, users *store.UserStore, db *store.DB) *CampaignHandler {
	return &CampaignHandler{campaigns: campaigns, players: players, users: users, db: db}
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

// Create handles POST /api/campaigns. Any authenticated user may create a
// campaign and becomes its DM (a first-class member with a backing Player).
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

	// Look up the creator's email so the DM stub Player has a sensible display name.
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "user not found", "UNAUTHORIZED")
		return
	}

	dmName := displayNameFromEmail(user.Email)
	dmPlayer := model.DefaultPlayer(uuid.NewString(), "", userID, dmName, 0, model.PlayerKindDM)

	campaign := &model.Campaign{
		ID:                uuid.NewString(),
		Name:              req.Name,
		ThemeImage:        req.ThemeImage,
		MapImageURI:       req.MapImageURI,
		MapPins:           []model.MapPin{},
		Sessions:          []model.Session{},
		Members:           []model.CampaignMember{},
		DisabledSpells:    []string{},
		DisabledEquipment: []string{},
		UpdatedAt:         time.Now().UTC(),
	}
	dmPlayer.CampaignID = campaign.ID
	campaign.Members = []model.CampaignMember{
		{UserID: userID, PlayerID: dmPlayer.ID, Role: model.RoleDM, IsActive: true},
	}

	if err := h.campaigns.CreateWithDMMember(r.Context(), h.db.Client(), campaign, dmPlayer, h.players); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, toCampaignDetail(campaign, userID))
}

// Update handles PATCH /api/campaigns/:id. DM only — enforced in handler via
// resolveCampaignMembership.
func (h *CampaignHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, id)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
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
	writeJSON(w, http.StatusOK, toCampaignDetail(updated, membership.UserID))
}

// SetPlayerActive handles PATCH /api/campaigns/:id/players/:playerId. DM only —
// enforced in handler via resolveCampaignMembership. The DM cannot be archived;
// returns 400 BAD_REQUEST when targeting the DM's own member entry.
// Body: { "isActive": bool }
// Used to archive (isActive=false) or restore (isActive=true) a campaign player.
func (h *CampaignHandler) SetPlayerActive(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "id")
	playerID := chi.URLParam(r, "playerId")
	ctx := r.Context()

	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
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

	// Pre-check: distinguish "member doesn't exist" from "member is the DM".
	// resolveCampaignMembership already loaded the campaign, but we re-scan
	// from membership.Campaign to avoid a second fetch.
	var targetRole model.Role
	found := false
	for _, m := range membership.Campaign.Members {
		if m.PlayerID == playerID {
			targetRole = m.Role
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "player not found in campaign", "NOT_FOUND")
		return
	}
	if targetRole == model.RoleDM {
		writeError(w, http.StatusBadRequest, "the DM cannot be archived", "BAD_REQUEST")
		return
	}

	ok, err := h.campaigns.SetPlayerActive(ctx, campaignID, playerID, req.IsActive)
	if errors.Is(err, store.ErrCannotArchiveDM) {
		// Backstop for the pre-check above (or for any future caller that
		// reaches the store without doing one). Same response shape.
		writeError(w, http.StatusBadRequest, "the DM cannot be archived", "BAD_REQUEST")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !ok {
		// Race: member existed at pre-check but no longer does. Treat as 404.
		writeError(w, http.StatusNotFound, "player not found in campaign", "NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"playerId": playerID,
		"isActive": req.IsActive,
	})
}

// Delete handles DELETE /api/campaigns/:id. DM only — enforced in handler via
// resolveCampaignMembership.
// Cascade: deletes all players in the campaign (spells and inventory are embedded).
func (h *CampaignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	membership := resolveCampaignMembership(w, r, h.campaigns, id)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	playerIDs, err := h.players.IDsForCampaign(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
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
