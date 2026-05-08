package handler

import (
	"context"
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

	if v, ok := patch["visibility"]; ok {
		if !isValidVisibilityPatch(v) {
			writeError(w, http.StatusBadRequest, "visibility must be { sharedWithAll: bool, sharedPlayerIds: string[] }", "BAD_REQUEST")
			return
		}
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

type shareMapPinRequest struct {
	RecipientPlayerIds []string `json:"recipientPlayerIds"`
	SharedWithAll      bool     `json:"sharedWithAll"`
	Note               string   `json:"note"`
	CascadeEntity      bool     `json:"cascadeEntity"`
}

// Share handles POST /api/campaigns/:campaignId/map-pins/:id/share.
// Owner only. Pin cloning is the simplest of the three share flows: just clone
// the pin per recipient. If cascadeEntity is true AND the pin is a location/npc
// pin, also clone the underlying entity (and repoint the pin clone's entityId
// to the entity clone).
func (h *MapPinHandler) Share(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	source, err := h.pins.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if source == nil {
		writeError(w, http.StatusNotFound, "pin not found", "NOT_FOUND")
		return
	}
	if source.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req shareMapPinRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	recipients, err := resolveShareRecipients(r.Context(), h.campaigns, campaignID, source.OwnerPlayerID, req.RecipientPlayerIds, req.SharedWithAll)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	clones := make([]model.MapPin, 0, len(recipients))
	for _, recip := range recipients {
		clone, err := h.cloneMapPinForRecipient(r.Context(), source, recip, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		clones = append(clones, *clone)
	}
	writeJSON(w, http.StatusOK, clones)
}

// cloneMapPinForRecipient produces (and persists) a clone of the source pin
// for the given recipient, idempotently. If req.CascadeEntity is true and
// the pin references a location/npc, also clone the entity and repoint the
// pin's entityId.
func (h *MapPinHandler) cloneMapPinForRecipient(ctx context.Context, source *model.MapPin, recip recipient, req shareMapPinRequest) (*model.MapPin, error) {
	existing, err := h.pins.FindCloneOf(ctx, source.CampaignID, source.ID, recip.PlayerID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	now := time.Now().UTC()
	clone := *source
	clone.ID = uuid.NewString()
	clone.OwnerPlayerID = recip.PlayerID
	clone.OwnerUserID = recip.UserID
	clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
	clone.ShareNote = req.Note
	clone.Clone = &model.CloneAudit{
		ClonedFromEntryId:       source.ID,
		ClonedFromOwnerPlayerId: source.OwnerPlayerID,
		ClonedAt:                now,
	}
	clone.CreatedAt = now
	clone.UpdatedAt = now

	if req.CascadeEntity && source.EntityID != "" {
		switch source.Type {
		case model.MapPinLocation:
			cloneEntityID, err := cloneLocationForRecipientShallow(ctx, h.locations, source.CampaignID, source.EntityID, recip)
			if err != nil {
				return nil, err
			}
			if cloneEntityID != "" {
				clone.EntityID = cloneEntityID
			}
		case model.MapPinNPC:
			cloneEntityID, err := cloneNpcForRecipient(ctx, h.npcs, source.CampaignID, source.EntityID, recip)
			if err != nil {
				return nil, err
			}
			if cloneEntityID != "" {
				clone.EntityID = cloneEntityID
			}
		}
	}

	if err := h.pins.Create(ctx, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}
