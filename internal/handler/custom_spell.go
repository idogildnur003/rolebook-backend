package handler

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// CustomSpellHandler exposes per-campaign homebrew spell CRUD.
//
// Permissions:
//   - List: any campaign member (results filtered by visibility)
//   - Create: campaign DM only (players submit a content-request to propose)
//   - Update: campaign DM only (visibility changes cascade to spell lists)
//   - Delete: campaign DM only — cascades to every player's spell list
type CustomSpellHandler struct {
	customSpells *store.CustomSpellStore
	players      *store.PlayerStore
	campaigns    *store.CampaignStore
	avatars      *avatarstore.Store
}

func NewCustomSpellHandler(
	customSpells *store.CustomSpellStore,
	players *store.PlayerStore,
	campaigns *store.CampaignStore,
	avatars *avatarstore.Store,
) *CustomSpellHandler {
	return &CustomSpellHandler{
		customSpells: customSpells,
		players:      players,
		campaigns:    campaigns,
		avatars:      avatars,
	}
}

// List handles GET /api/campaigns/:campaignId/custom-spells.
func (h *CustomSpellHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	spells, err := h.customSpells.ListVisibleToPlayer(r.Context(), campaignID, membership.PlayerID, membership.IsDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	for i := range spells {
		spells[i].ImageURI = h.avatars.ResolveImageURI(r.Context(), spells[i].ImageURI)
	}
	writeJSON(w, http.StatusOK, spells)
}

// Create handles POST /api/campaigns/:campaignId/custom-spells.
// The request body mirrors CustomSpell minus the server-owned fields
// (id, campaignId, createdBy, createdAt, updatedAt), which are stamped here.
func (h *CustomSpellHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can create custom spells directly; players submit a request", "FORBIDDEN")
		return
	}

	var body model.CustomSpell
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if body.Level < 0 || body.Level > 9 {
		writeError(w, http.StatusBadRequest, "level must be between 0 and 9", "BAD_REQUEST")
		return
	}
	body.VisibilityMode = model.NormalizeVisibilityMode(body.VisibilityMode)
	if !model.IsValidVisibilityMode(body.VisibilityMode) {
		writeError(w, http.StatusBadRequest, "visibilityMode must be 'campaign' or 'players'", "BAD_REQUEST")
		return
	}
	if body.VisibilityMode != model.VisibilityPlayers {
		body.VisiblePlayerIDs = nil
	}

	// Generate a unique id; retry up to 3x on slug collisions.
	const maxIDAttempts = 3
	var id string
	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		candidate, err := store.GenerateID(body.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		existing, err := h.customSpells.GetByID(r.Context(), campaignID, candidate)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if existing == nil {
			id = candidate
			break
		}
	}
	if id == "" {
		writeError(w, http.StatusInternalServerError, "failed to allocate id", "INTERNAL_ERROR")
		return
	}

	now := time.Now().UTC()
	body.ID = id
	body.CampaignID = campaignID
	body.CreatedBy = membership.UserID
	body.CreatedAt = now
	body.UpdatedAt = now
	if body.Components == nil {
		body.Components = []string{}
	}

	if err := h.customSpells.Create(r.Context(), &body); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "custom spell already exists", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	body.ImageURI = h.avatars.ResolveImageURI(r.Context(), body.ImageURI)
	writeJSON(w, http.StatusCreated, body)
}

// Update handles PATCH /api/campaigns/:campaignId/custom-spells/:id.
// DM only. When the visibility narrows, the change cascades to remove the spell
// from the spell lists of players who can no longer see it.
func (h *CustomSpellHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.customSpells.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "custom spell not found", "NOT_FOUND")
		return
	}

	// DM only.
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can edit custom spells", "FORBIDDEN")
		return
	}

	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Strip server-owned fields.
	for _, k := range []string{"id", "_id", "campaignId", "createdBy", "createdAt", "updatedAt"} {
		delete(patch, k)
	}
	if len(patch) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	if raw, ok := patch["visibilityMode"]; ok {
		mode, isString := raw.(string)
		if !isString {
			writeError(w, http.StatusBadRequest, "visibilityMode must be a string", "BAD_REQUEST")
			return
		}
		mode = model.NormalizeVisibilityMode(mode)
		if !model.IsValidVisibilityMode(mode) {
			writeError(w, http.StatusBadRequest, "visibilityMode must be 'campaign' or 'players'", "BAD_REQUEST")
			return
		}
		patch["visibilityMode"] = mode
		if mode != model.VisibilityPlayers {
			patch["visiblePlayerIds"] = []string{}
		}
	}

	updated, err := h.customSpells.Update(r.Context(), campaignID, id, bson.M(patch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "custom spell not found", "NOT_FOUND")
		return
	}

	// Cascade when EITHER the mode OR the player list changed — narrowing the
	// players list alone (mode unchanged) must still pull the spell from players
	// who lost access. AllowedPlayerIDsForCascade no-ops for campaign mode.
	_, modeTouched := patch["visibilityMode"]
	_, idsTouched := patch["visiblePlayerIds"]
	if modeTouched || idsTouched {
		allowed, runCascade := model.AllowedPlayerIDsForCascade(updated.VisibilityMode, updated.VisiblePlayerIDs)
		if runCascade {
			if _, err := h.customSpells.RemoveExceptForPlayers(r.Context(), campaignID, id, allowed, h.players); err != nil {
				// Best-effort: the visibility change persisted; log and continue.
				log.Printf("[custom-spell] visibility cascade failed for %s/%s: %v", campaignID, id, err)
			}
		}
	}

	updated.ImageURI = h.avatars.ResolveImageURI(r.Context(), updated.ImageURI)
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/custom-spells/:id.
// DM only. Cascades to every player's spell list in the campaign.
func (h *CustomSpellHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can delete custom spells", "FORBIDDEN")
		return
	}

	result, err := h.customSpells.DeleteWithCascade(r.Context(), campaignID, id, h.players)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !result.CatalogDeleted {
		writeError(w, http.StatusNotFound, "custom spell not found", "NOT_FOUND")
		return
	}
	if result.CleanupErr != nil {
		// Catalog entry is gone but the spell cleanup failed. Log and still
		// return 204 — the spell list handler tolerates dangling references.
		log.Printf("[custom-spell] cascade cleanup failed for %s/%s after delete: %v",
			campaignID, id, result.CleanupErr)
	}
	w.WriteHeader(http.StatusNoContent)
}
