package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
}

// NewLocationHandler creates a LocationHandler.
func NewLocationHandler(locations *store.LocationStore, pins *store.MapPinStore, campaigns *store.CampaignStore) *LocationHandler {
	return &LocationHandler{locations: locations, pins: pins, campaigns: campaigns}
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
	// TODO(image-rewrite): Task 13 expands this to call avatarstore.ResolveImageURI on thumbnailUri.
	writeJSON(w, http.StatusOK, locs)
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
