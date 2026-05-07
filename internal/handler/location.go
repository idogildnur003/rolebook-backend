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
	pins      *store.MapPinStore
	campaigns *store.CampaignStore
	avatars   *avatarstore.Store
}

// NewLocationHandler creates a LocationHandler.
func NewLocationHandler(locations *store.LocationStore, pins *store.MapPinStore, campaigns *store.CampaignStore, avatars *avatarstore.Store) *LocationHandler {
	return &LocationHandler{locations: locations, pins: pins, campaigns: campaigns, avatars: avatars}
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
