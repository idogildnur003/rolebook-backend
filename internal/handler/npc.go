package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/avatarstore"
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
	locations *store.LocationStore
	pins      *store.MapPinStore
	campaigns *store.CampaignStore
	avatars   *avatarstore.Store
}

// NewNPCHandler creates an NPCHandler.
func NewNPCHandler(npcs *store.NPCStore, locations *store.LocationStore, pins *store.MapPinStore, campaigns *store.CampaignStore, avatars *avatarstore.Store) *NPCHandler {
	return &NPCHandler{npcs: npcs, locations: locations, pins: pins, campaigns: campaigns, avatars: avatars}
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
	writeJSON(w, http.StatusOK, h.resolveAvatars(r.Context(), npcs))
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
	npc.AvatarURI = h.avatars.ResolveImageURI(r.Context(), npc.AvatarURI)
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

	if v, ok := patch["visibility"]; ok {
		if !isValidVisibilityPatch(v) {
			writeError(w, http.StatusBadRequest, "visibility must be { sharedWithAll: bool, sharedPlayerIds: string[] }", "BAD_REQUEST")
			return
		}
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
	updated.AvatarURI = h.avatars.ResolveImageURI(r.Context(), updated.AvatarURI)
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

// resolveAvatars rewrites each NPC's avatarUri to a signed GET URL when the
// value looks like an S3 key. Pass-through for fully-qualified URLs and empty
// values — see avatarstore.LooksLikeKey for the rule.
func (h *NPCHandler) resolveAvatars(ctx context.Context, npcs []model.NPC) []model.NPC {
	for i := range npcs {
		npcs[i].AvatarURI = h.avatars.ResolveImageURI(ctx, npcs[i].AvatarURI)
	}
	return npcs
}

type shareNPCRequest struct {
	RecipientPlayerIds []string `json:"recipientPlayerIds"`
	SharedWithAll      bool     `json:"sharedWithAll"`
	Note               string   `json:"note"`
	Cascade            struct {
		LocationIds []string `json:"locationIds"`
		MapPinIds   []string `json:"mapPinIds"`
	} `json:"cascade"`
}

// Share handles POST /api/campaigns/:campaignId/npcs/:id/share.
// Owner only. Creates clones (or reuses existing clones) for each recipient
// and optionally cascades linked locations and pins.
func (h *NPCHandler) Share(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	source, err := h.npcs.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if source == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}
	if source.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req shareNPCRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	recipients, err := resolveShareRecipients(r.Context(), h.campaigns, campaignID, source.OwnerPlayerID, req.RecipientPlayerIds, req.SharedWithAll)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	clones := make([]model.NPC, 0, len(recipients))
	for _, recip := range recipients {
		clone, err := h.cloneNpcForRecipientHandler(r.Context(), source, recip, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		clones = append(clones, *clone)
	}

	for i := range clones {
		clones[i].AvatarURI = h.avatars.ResolveImageURI(r.Context(), clones[i].AvatarURI)
	}
	writeJSON(w, http.StatusOK, clones)
}

// cloneNpcForRecipientHandler is the handler-level NPC clone with cascade.
// Mirrors LocationHandler.cloneLocationForRecipient. The package-level
// cloneNpcForRecipient in journal_share.go is the no-cascade fallback used by
// other share flows; this method adds the location and pin cascade.
func (h *NPCHandler) cloneNpcForRecipientHandler(ctx context.Context, source *model.NPC, recip recipient, req shareNPCRequest) (*model.NPC, error) {
	existing, err := h.npcs.FindCloneOf(ctx, source.CampaignID, source.ID, recip.PlayerID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	locMap, err := cascadeLinkedLocations(ctx, h.locations, source.CampaignID, source.LinkedLocationIds, recip, req.Cascade.LocationIds)
	if err != nil {
		return nil, err
	}
	rewrittenLinks := rewriteIds(source.LinkedLocationIds, locMap)

	now := time.Now().UTC()
	clone := *source
	clone.ID = uuid.NewString()
	clone.OwnerPlayerID = recip.PlayerID
	clone.OwnerUserID = recip.UserID
	clone.LinkedLocationIds = rewrittenLinks
	clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
	clone.ShareNote = req.Note
	clone.Clone = &model.CloneAudit{
		ClonedFromEntryId:       source.ID,
		ClonedFromOwnerPlayerId: source.OwnerPlayerID,
		ClonedAt:                now,
	}
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if err := h.npcs.Create(ctx, &clone); err != nil {
		return nil, err
	}

	if err := h.cascadePinsForNpcRecipient(ctx, source.CampaignID, source.ID, clone.ID, recip, req.Cascade.MapPinIds); err != nil {
		return nil, err
	}
	return &clone, nil
}

// cascadePinsForNpcRecipient mirrors LocationHandler.cascadePinsForRecipient
// for npc-typed pins.
func (h *NPCHandler) cascadePinsForNpcRecipient(ctx context.Context, campaignID, sourceNpcID, cloneNpcID string, recip recipient, cascadeIds []string) error {
	for _, pinID := range cascadeIds {
		pin, err := h.pins.GetByID(ctx, campaignID, pinID)
		if err != nil {
			return err
		}
		if pin == nil || pin.EntityID != sourceNpcID || pin.Type != model.MapPinNPC {
			continue
		}
		existing, err := h.pins.FindCloneOf(ctx, campaignID, pin.ID, recip.PlayerID)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		now := time.Now().UTC()
		clone := *pin
		clone.ID = uuid.NewString()
		clone.OwnerPlayerID = recip.PlayerID
		clone.OwnerUserID = recip.UserID
		clone.EntityID = cloneNpcID
		clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
		clone.Clone = &model.CloneAudit{
			ClonedFromEntryId:       pin.ID,
			ClonedFromOwnerPlayerId: pin.OwnerPlayerID,
			ClonedAt:                now,
		}
		clone.CreatedAt = now
		clone.UpdatedAt = now
		if err := h.pins.Create(ctx, &clone); err != nil {
			return err
		}
	}
	return nil
}
