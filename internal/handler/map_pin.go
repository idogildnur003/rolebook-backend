package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// MapPinHandler exposes per-campaign map-pin CRUD plus sharing.
//
// Permissions:
//   - List, Create: any campaign member.
//   - Get, Update, Delete, Share: owner only.
//
// Type-dependent body rules on Create:
//   - location/npc pins: entityId is required, title/notes are not allowed
//     (the underlying entity carries those). The entity must be visible to
//     the caller.
//   - item/majorFinding/travelMarker/custom pins: title is required, entityId
//     is not allowed.
type MapPinHandler struct {
	pins      *store.MapPinStore
	locations *store.LocationStore
	npcs      *store.NPCStore
	campaigns *store.CampaignStore
}

// NewMapPinHandler creates a MapPinHandler.
func NewMapPinHandler(pins *store.MapPinStore, locations *store.LocationStore, npcs *store.NPCStore, campaigns *store.CampaignStore) *MapPinHandler {
	return &MapPinHandler{pins: pins, locations: locations, npcs: npcs, campaigns: campaigns}
}

// List handles GET /api/campaigns/:campaignId/map-pins.
func (h *MapPinHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	pins, err := h.pins.ListVisibleToCaller(r.Context(), campaignID, membership.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, pins)
}

type createMapPinRequest struct {
	Type      model.MapPinType `json:"type"`
	EntityID  string           `json:"entityId"`
	Title     string           `json:"title"`
	Notes     string           `json:"notes"`
	X         float64          `json:"x"`
	Y         float64          `json:"y"`
	SessionID string           `json:"sessionId"`
}

// Create handles POST /api/campaigns/:campaignId/map-pins.
func (h *MapPinHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	var req createMapPinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if !model.IsValidMapPinType(req.Type) {
		writeError(w, http.StatusBadRequest, "invalid pin type", "BAD_REQUEST")
		return
	}
	if model.MapPinTypeReferencesEntity(req.Type) {
		if req.EntityID == "" {
			writeError(w, http.StatusBadRequest, "entityId is required for location/npc pins", "BAD_REQUEST")
			return
		}
		if req.Title != "" || req.Notes != "" {
			writeError(w, http.StatusBadRequest, "title and notes are derived from the linked entity for location/npc pins", "BAD_REQUEST")
			return
		}
		if err := h.assertEntityVisible(r, campaignID, req.Type, req.EntityID, membership.PlayerID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
			return
		}
	} else {
		if req.EntityID != "" {
			writeError(w, http.StatusBadRequest, "entityId is only valid for location/npc pins", "BAD_REQUEST")
			return
		}
		if req.Title == "" {
			writeError(w, http.StatusBadRequest, "title is required for non-entity pins", "BAD_REQUEST")
			return
		}
	}

	now := time.Now().UTC()
	pin := &model.MapPin{
		ID:            uuid.NewString(),
		CampaignID:    campaignID,
		OwnerPlayerID: membership.PlayerID,
		OwnerUserID:   membership.UserID,
		Type:          req.Type,
		EntityID:      req.EntityID,
		Title:         req.Title,
		Notes:         req.Notes,
		X:             req.X,
		Y:             req.Y,
		SessionID:     req.SessionID,
		Visibility:    model.Visibility{SharedPlayerIds: []string{}},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.pins.Create(r.Context(), pin); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, pin)
}

// Update handles PATCH /api/campaigns/:campaignId/map-pins/:id.
// Owner only. Strips immutable + type-defining fields (type/entityId stay fixed
// once a pin is created — moving a pin between types isn't supported).
func (h *MapPinHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.pins.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "pin not found", "NOT_FOUND")
		return
	}
	if existing.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	for _, k := range []string{"id", "_id", "campaignId", "ownerPlayerId", "ownerUserId", "type", "entityId", "createdAt", "updatedAt", "clone"} {
		delete(patch, k)
	}
	if len(patch) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.pins.Update(r.Context(), campaignID, id, bson.M(patch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "pin not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/map-pins/:id. Owner only.
// No cascade — pins are leaves.
func (h *MapPinHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.pins.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "pin not found", "NOT_FOUND")
		return
	}
	if existing.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	if _, err := h.pins.Delete(r.Context(), campaignID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// assertEntityVisible looks up the entity referenced by a location/npc pin and
// confirms the caller can see it (owner or shared). Returns an error message
// safe for the wire if the entity is missing or not visible.
func (h *MapPinHandler) assertEntityVisible(r *http.Request, campaignID string, pinType model.MapPinType, entityID, playerID string) error {
	switch pinType {
	case model.MapPinLocation:
		loc, err := h.locations.GetByID(r.Context(), campaignID, entityID)
		if err != nil {
			return errors.New("could not validate entity")
		}
		if loc == nil || !visibilityAllowsRead(loc.OwnerPlayerID, loc.Visibility, playerID) {
			return errors.New("location not found or not visible")
		}
	case model.MapPinNPC:
		npc, err := h.npcs.GetByID(r.Context(), campaignID, entityID)
		if err != nil {
			return errors.New("could not validate entity")
		}
		if npc == nil || !visibilityAllowsRead(npc.OwnerPlayerID, npc.Visibility, playerID) {
			return errors.New("npc not found or not visible")
		}
	}
	return nil
}

// visibilityAllowsRead encodes the read filter:
//   - owner can always read,
//   - sharedWithAll opens it to every member,
//   - sharedPlayerIds opens it to specific members.
//
// Lives here for now; Task 14 will move it to internal/handler/journal_share.go
// alongside the clone-share helpers.
func visibilityAllowsRead(ownerPlayerID string, v model.Visibility, callerPlayerID string) bool {
	if ownerPlayerID == callerPlayerID {
		return true
	}
	if v.SharedWithAll {
		return true
	}
	for _, id := range v.SharedPlayerIds {
		if id == callerPlayerID {
			return true
		}
	}
	return false
}
