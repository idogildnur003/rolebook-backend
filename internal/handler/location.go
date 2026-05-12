package handler

import (
	"context"
	"errors"
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

// LocationHandler exposes per-campaign location CRUD plus sharing.
//
// Permissions:
//   - List, Create: any campaign member.
//   - Get, Update, Delete, Share: owner only.
//   - DM has no override (matches the spec's "private journal" model).
type LocationHandler struct {
	locations *store.LocationStore
	npcs      *store.NPCStore
	pins      *store.MapPinStore
	campaigns *store.CampaignStore
	avatars   *avatarstore.Store
}

// NewLocationHandler creates a LocationHandler.
func NewLocationHandler(locations *store.LocationStore, npcs *store.NPCStore, pins *store.MapPinStore, campaigns *store.CampaignStore, avatars *avatarstore.Store) *LocationHandler {
	return &LocationHandler{locations: locations, npcs: npcs, pins: pins, campaigns: campaigns, avatars: avatars}
}

// List handles GET /api/campaigns/:campaignId/locations.
func (h *LocationHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	locs, err := h.locations.ListVisibleToCaller(r.Context(), campaignID, membership.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, h.resolveThumbnails(r.Context(), locs))
}

type createLocationRequest struct {
	Name             string   `json:"name"`
	ShortNotes       string   `json:"shortNotes"`
	FullDescription  string   `json:"fullDescription"`
	ThumbnailURI     string   `json:"thumbnailUri"`
	SessionID        string   `json:"sessionId"`
	ParentLocationID string   `json:"parentLocationId"`
	LinkedNpcIds     []string `json:"linkedNpcIds"`
}

// Create handles POST /api/campaigns/:campaignId/locations.
func (h *LocationHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	var req createLocationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if req.ParentLocationID != "" {
		if err := h.checkSubLocationDepth(r, campaignID, req.ParentLocationID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
			return
		}
	}

	now := time.Now().UTC()
	loc := &model.Location{
		ID:               uuid.NewString(),
		CampaignID:       campaignID,
		OwnerPlayerID:    membership.PlayerID,
		OwnerUserID:      membership.UserID,
		Name:             req.Name,
		ShortNotes:       req.ShortNotes,
		FullDescription:  req.FullDescription,
		ThumbnailURI:     req.ThumbnailURI,
		SessionID:        req.SessionID,
		ParentLocationID: req.ParentLocationID,
		LinkedNpcIds:     normalizeStringSlice(req.LinkedNpcIds),
		Visibility:       model.Visibility{SharedPlayerIds: []string{}},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := h.locations.Create(r.Context(), loc); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	loc.ThumbnailURI = h.avatars.ResolveImageURI(r.Context(), loc.ThumbnailURI)
	writeJSON(w, http.StatusCreated, loc)
}

// checkSubLocationDepth returns nil iff the parent (if it exists) has no parent
// itself — i.e. the proposed entry would be at most one level deep.
func (h *LocationHandler) checkSubLocationDepth(r *http.Request, campaignID, parentID string) error {
	parent, err := h.locations.GetByID(r.Context(), campaignID, parentID)
	if err != nil {
		return errors.New("could not validate parent location")
	}
	if parent == nil {
		return errors.New("parent location not found")
	}
	if parent.ParentLocationID != "" {
		return errors.New("sub-locations cannot have their own sub-locations")
	}
	return nil
}

// normalizeStringSlice de-dupes and drops empties; never returns nil.
func normalizeStringSlice(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Update handles PATCH /api/campaigns/:campaignId/locations/:id.
// Owner only. Accepts a partial of the createLocationRequest fields plus
// optional `visibility` and `shareNote`. Server-owned fields are stripped.
func (h *LocationHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.locations.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "location not found", "NOT_FOUND")
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

	var oldThumbKey, newThumbKey string
	thumbKeyChanged := false
	if v, ok := patch["thumbnailUri"]; ok {
		newStr, _ := v.(string)
		newThumbKey = newStr
		oldThumbKey = existing.ThumbnailURI
		if newThumbKey != oldThumbKey {
			thumbKeyChanged = true
			if newThumbKey != "" {
				if err := h.avatars.Verify(r.Context(), newThumbKey); err != nil {
					if errors.Is(err, avatarstore.ErrNotFound) {
						writeError(w, http.StatusBadRequest, "uploaded image not found", "UPLOAD_NOT_FOUND")
						return
					}
					writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
					return
				}
			}
		}
	}

	if v, ok := patch["visibility"]; ok {
		if !isValidVisibilityPatch(v) {
			writeError(w, http.StatusBadRequest, "visibility must be { sharedWithAll: bool, sharedPlayerIds: string[] }", "BAD_REQUEST")
			return
		}
	}

	if v, ok := patch["parentLocationId"].(string); ok && v != "" {
		if err := h.checkSubLocationDepth(r, campaignID, v); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
			return
		}
	}
	if v, ok := patch["linkedNpcIds"].([]any); ok {
		patch["linkedNpcIds"] = normalizeAnySlice(v)
	}

	updated, err := h.locations.Update(r.Context(), campaignID, id, bson.M(patch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "location not found", "NOT_FOUND")
		return
	}
	if thumbKeyChanged && oldThumbKey != "" {
		if err := h.avatars.Delete(r.Context(), oldThumbKey); err != nil {
			log.Printf("location %s/%s: delete old thumbnail %q: %v", campaignID, id, oldThumbKey, err)
		}
	}
	updated.ThumbnailURI = h.avatars.ResolveImageURI(r.Context(), updated.ThumbnailURI)
	writeJSON(w, http.StatusOK, updated)
}

// normalizeAnySlice converts a JSON-decoded []any of strings to []string,
// dropping empties and duplicates.
func normalizeAnySlice(in []any) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, v := range in {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Delete handles DELETE /api/campaigns/:campaignId/locations/:id.
// Owner only. Cascades to pins where entityId == this location's id.
func (h *LocationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.locations.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "location not found", "NOT_FOUND")
		return
	}
	if existing.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	if _, err := h.locations.Delete(r.Context(), campaignID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if _, err := h.pins.DeleteByEntity(r.Context(), campaignID, id); err != nil {
		// Pin cascade failed but the location is gone. Log and continue.
		// Dangling pins are tolerated — UI degrades gracefully.
		log.Printf("[location] pin cascade failed for %s/%s: %v", campaignID, id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveThumbnails rewrites each location's thumbnailUri to a signed GET URL
// when the value looks like an S3 key. Pass-through for local file URIs and
// existing https:// URLs — see avatarstore.LooksLikeKey for the rule.
func (h *LocationHandler) resolveThumbnails(ctx context.Context, locs []model.Location) []model.Location {
	for i := range locs {
		locs[i].ThumbnailURI = h.avatars.ResolveImageURI(ctx, locs[i].ThumbnailURI)
	}
	return locs
}

type shareLocationRequest struct {
	RecipientPlayerIds []string `json:"recipientPlayerIds"`
	SharedWithAll      bool     `json:"sharedWithAll"`
	Note               string   `json:"note"`
	Cascade            struct {
		NpcIds    []string `json:"npcIds"`
		MapPinIds []string `json:"mapPinIds"`
	} `json:"cascade"`
}

// Share handles POST /api/campaigns/:campaignId/locations/:id/share.
// Owner only. Creates clones (or reuses existing clones) for each recipient
// and optionally cascades linked NPCs and pins.
func (h *LocationHandler) Share(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	source, err := h.locations.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if source == nil {
		writeError(w, http.StatusNotFound, "location not found", "NOT_FOUND")
		return
	}
	if source.OwnerPlayerID != membership.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req shareLocationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	recipients, err := resolveShareRecipients(r.Context(), h.campaigns, campaignID, source.OwnerPlayerID, req.RecipientPlayerIds, req.SharedWithAll)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	clones := make([]model.Location, 0, len(recipients))
	for _, recip := range recipients {
		clone, err := h.cloneLocationForRecipient(r.Context(), source, recip, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		clones = append(clones, *clone)
	}

	for i := range clones {
		clones[i].ThumbnailURI = h.avatars.ResolveImageURI(r.Context(), clones[i].ThumbnailURI)
	}
	writeJSON(w, http.StatusOK, clones)
}

// cloneLocationForRecipient produces (and persists) a clone of source for the
// given recipient. If a clone already exists, it's returned as-is (idempotent).
// Cascades NPCs and pins per the request.
func (h *LocationHandler) cloneLocationForRecipient(ctx context.Context, source *model.Location, recip recipient, req shareLocationRequest) (*model.Location, error) {
	existing, err := h.locations.FindCloneOf(ctx, source.CampaignID, source.ID, recip.PlayerID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	npcMap, err := cascadeLinkedNpcs(ctx, h.npcs, source.CampaignID, source.LinkedNpcIds, recip, req.Cascade.NpcIds)
	if err != nil {
		return nil, err
	}
	rewrittenLinks := rewriteIds(source.LinkedNpcIds, npcMap)

	now := time.Now().UTC()
	clone := *source
	clone.ID = uuid.NewString()
	clone.OwnerPlayerID = recip.PlayerID
	clone.OwnerUserID = recip.UserID
	clone.LinkedNpcIds = rewrittenLinks
	clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
	clone.ShareNote = req.Note
	clone.Clone = &model.CloneAudit{
		ClonedFromEntryId:       source.ID,
		ClonedFromOwnerPlayerId: source.OwnerPlayerID,
		ClonedAt:                now,
	}
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if err := h.locations.Create(ctx, &clone); err != nil {
		return nil, err
	}

	if err := h.cascadePinsForRecipient(ctx, source.CampaignID, source.ID, clone.ID, recip, req.Cascade.MapPinIds); err != nil {
		return nil, err
	}
	return &clone, nil
}

// cascadePinsForRecipient clones each pin in cascadeIds for the recipient,
// repointing entityId from the source location to the cloned location.
// Pins that don't reference the source location (or don't exist) are skipped.
func (h *LocationHandler) cascadePinsForRecipient(ctx context.Context, campaignID, sourceLocationID, cloneLocationID string, recip recipient, cascadeIds []string) error {
	for _, pinID := range cascadeIds {
		pin, err := h.pins.GetByID(ctx, campaignID, pinID)
		if err != nil {
			return err
		}
		if pin == nil || pin.EntityID != sourceLocationID || pin.Type != model.MapPinLocation {
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
		clone.EntityID = cloneLocationID
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
