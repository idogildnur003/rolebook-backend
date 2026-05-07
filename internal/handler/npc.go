package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// NPCHandler exposes per-campaign NPC CRUD plus sharing.
//
// Permissions:
//   - List, Create: any campaign member.
//   - Get, Update, Delete, Share: owner only.
//   - DM has no override.
type NPCHandler struct {
	npcs      *store.NPCStore
	pins      *store.MapPinStore
	campaigns *store.CampaignStore
}

// NewNPCHandler creates an NPCHandler.
func NewNPCHandler(npcs *store.NPCStore, pins *store.MapPinStore, campaigns *store.CampaignStore) *NPCHandler {
	return &NPCHandler{npcs: npcs, pins: pins, campaigns: campaigns}
}

// List handles GET /api/campaigns/:campaignId/npcs.
func (h *NPCHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	npcs, err := h.npcs.ListVisibleToCaller(r.Context(), campaignID, membership.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	// TODO(image-rewrite): Task 13 expands this to call avatarstore.ResolveImageURI on avatarUri.
	writeJSON(w, http.StatusOK, npcs)
}

type createNPCRequest struct {
	Name              string   `json:"name"`
	ShortNotes        string   `json:"shortNotes"`
	FullDescription   string   `json:"fullDescription"`
	AvatarURI         string   `json:"avatarUri"`
	SessionID         string   `json:"sessionId"`
	LinkedLocationIds []string `json:"linkedLocationIds"`
}

// Create handles POST /api/campaigns/:campaignId/npcs.
func (h *NPCHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	var req createNPCRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	now := time.Now().UTC()
	npc := &model.NPC{
		ID:                uuid.NewString(),
		CampaignID:        campaignID,
		OwnerPlayerID:     membership.PlayerID,
		OwnerUserID:       membership.UserID,
		Name:              req.Name,
		ShortNotes:        req.ShortNotes,
		FullDescription:   req.FullDescription,
		AvatarURI:         req.AvatarURI,
		SessionID:         req.SessionID,
		LinkedLocationIds: normalizeStringSlice(req.LinkedLocationIds),
		Visibility:        model.Visibility{SharedPlayerIds: []string{}},
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := h.npcs.Create(r.Context(), npc); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, npc)
}

// Update handles PATCH /api/campaigns/:campaignId/npcs/:id.
// Owner only. Accepts a partial of the createNPCRequest fields plus
// optional `visibility` and `shareNote`. Server-owned fields are stripped.
func (h *NPCHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.npcs.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
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
	for _, k := range []string{"id", "_id", "campaignId", "ownerPlayerId", "ownerUserId", "createdAt", "updatedAt", "clone"} {
		delete(patch, k)
	}
	if len(patch) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	if v, ok := patch["linkedLocationIds"].([]any); ok {
		patch["linkedLocationIds"] = normalizeAnySlice(v)
	}

	updated, err := h.npcs.Update(r.Context(), campaignID, id, bson.M(patch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/npcs/:id.
// Owner only. Cascades to pins where entityId == this NPC's id.
func (h *NPCHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.npcs.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}
	if existing.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	if _, err := h.npcs.Delete(r.Context(), campaignID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if _, err := h.pins.DeleteByEntity(r.Context(), campaignID, id); err != nil {
		log.Printf("[npc] pin cascade failed for %s/%s: %v", campaignID, id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
