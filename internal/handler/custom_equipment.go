package handler

import (
	"errors"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// CustomEquipmentHandler exposes per-campaign homebrew equipment CRUD.
//
// Permissions:
//   - List / Create: any campaign member (DM or active player)
//   - Update: creator OR campaign DM
//   - Delete: campaign DM only — cascades to every player's inventory
type CustomEquipmentHandler struct {
	customEquipment *store.CustomEquipmentStore
	players         *store.PlayerStore
	campaigns       *store.CampaignStore
	avatars         *avatarstore.Store
}

func NewCustomEquipmentHandler(
	customEquipment *store.CustomEquipmentStore,
	players *store.PlayerStore,
	campaigns *store.CampaignStore,
	avatars *avatarstore.Store,
) *CustomEquipmentHandler {
	return &CustomEquipmentHandler{
		customEquipment: customEquipment,
		players:         players,
		campaigns:       campaigns,
		avatars:         avatars,
	}
}

// List handles GET /api/campaigns/:campaignId/custom-equipment.
func (h *CustomEquipmentHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	if resolveCampaignMembership(w, r, h.campaigns, campaignID) == nil {
		return
	}

	items, err := h.customEquipment.ListByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	for i := range items {
		items[i].ImageURI = h.avatars.ResolveImageURI(r.Context(), items[i].ImageURI)
	}
	writeJSON(w, http.StatusOK, items)
}

// Create handles POST /api/campaigns/:campaignId/custom-equipment.
// The request body mirrors CustomEquipment minus the server-owned fields
// (id, campaignId, createdBy, createdAt, updatedAt), which are stamped here.
func (h *CustomEquipmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	var body model.CustomEquipment
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if body.Category == "" {
		writeError(w, http.StatusBadRequest, "category is required", "BAD_REQUEST")
		return
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
		existing, err := h.customEquipment.GetByID(r.Context(), campaignID, candidate)
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
	if body.Tags == nil {
		body.Tags = []string{}
	}

	if err := h.customEquipment.Create(r.Context(), &body); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "custom equipment already exists", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	body.ImageURI = h.avatars.ResolveImageURI(r.Context(), body.ImageURI)
	writeJSON(w, http.StatusCreated, body)
}

// Update handles PATCH /api/campaigns/:campaignId/custom-equipment/:id.
// Permitted for the creator or the campaign DM.
func (h *CustomEquipmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	existing, err := h.customEquipment.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "custom equipment not found", "NOT_FOUND")
		return
	}

	// Creator or DM only.
	if !membership.IsDM && existing.CreatedBy != membership.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
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

	updated, err := h.customEquipment.Update(r.Context(), campaignID, id, bson.M(patch))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "custom equipment not found", "NOT_FOUND")
		return
	}
	updated.ImageURI = h.avatars.ResolveImageURI(r.Context(), updated.ImageURI)
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/custom-equipment/:id.
// DM only. Cascades to every player's inventory in the campaign.
func (h *CustomEquipmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can delete custom equipment", "FORBIDDEN")
		return
	}

	result, err := h.customEquipment.DeleteWithCascade(r.Context(), campaignID, id, h.players)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !result.CatalogDeleted {
		writeError(w, http.StatusNotFound, "custom equipment not found", "NOT_FOUND")
		return
	}
	if result.InventoryCleanupErr != nil {
		// Catalog entry is gone but the inventory cleanup failed. Log, surface a
		// warning header, and still return 204 — the inventory list handler
		// tolerates dangling references.
		log.Printf("[custom-equipment] cascade cleanup failed for %s/%s after delete: %v",
			campaignID, id, result.InventoryCleanupErr)
	}
	w.WriteHeader(http.StatusNoContent)
}

// Usage handles GET /api/campaigns/:campaignId/custom-equipment/usage.
// DM only. Returns each custom equipment entry with the list of players who
// currently hold it in their inventory.
func (h *CustomEquipmentHandler) Usage(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can view custom equipment usage", "FORBIDDEN")
		return
	}

	items, err := h.customEquipment.ListByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	summaries, err := h.players.ListInventorySummaries(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	usage := buildCustomEquipmentUsage(items, summaries)
	for i := range usage {
		usage[i].ImageURI = h.avatars.ResolveImageURI(r.Context(), usage[i].ImageURI)
	}
	writeJSON(w, http.StatusOK, usage)
}

// customEquipmentHolder is one player who currently holds a custom item.
type customEquipmentHolder struct {
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	Quantity   int    `json:"quantity"`
}

// customEquipmentUsage is a custom equipment entry plus the players holding it.
// The embedded CustomEquipment fields are promoted to the top level in JSON,
// so the shape is the normal custom-equipment object with extra "holders" and
// "createdByName". createdByName is the resolved display name of the item's
// creator (empty when the creator has no player record in the campaign — e.g.
// they have since left); the client renders a fallback in that case.
type customEquipmentUsage struct {
	model.CustomEquipment
	Holders       []customEquipmentHolder `json:"holders"`
	CreatedByName string                  `json:"createdByName"`
}

// buildCustomEquipmentUsage joins custom equipment with the players who hold
// each item and resolves each item's creator to a display name. Pure over its
// inputs (no Mongo) so it can be unit-tested. Holders are sorted by player name
// (then id) for deterministic output, and every item gets a non-nil holders
// slice so the JSON is always [] rather than null. Creator names are resolved
// by matching the item's CreatedBy (a user id) against players' LinkedUserID —
// this covers the DM too, whose stub player carries their user id and name.
func buildCustomEquipmentUsage(
	items []model.CustomEquipment,
	players []store.PlayerInventorySummary,
) []customEquipmentUsage {
	holdersByEquipment := make(map[string][]customEquipmentHolder)
	nameByUserID := make(map[string]string)
	for _, p := range players {
		if p.LinkedUserID != "" {
			nameByUserID[p.LinkedUserID] = p.Name
		}
		for _, inv := range p.Inventory {
			holdersByEquipment[inv.EquipmentID] = append(
				holdersByEquipment[inv.EquipmentID],
				customEquipmentHolder{
					PlayerID:   p.ID,
					PlayerName: p.Name,
					Quantity:   inv.Quantity,
				},
			)
		}
	}

	usage := make([]customEquipmentUsage, 0, len(items))
	for _, item := range items {
		holders := holdersByEquipment[item.ID]
		if holders == nil {
			holders = []customEquipmentHolder{}
		}
		sort.Slice(holders, func(i, j int) bool {
			if holders[i].PlayerName != holders[j].PlayerName {
				return holders[i].PlayerName < holders[j].PlayerName
			}
			return holders[i].PlayerID < holders[j].PlayerID
		})
		usage = append(usage, customEquipmentUsage{
			CustomEquipment: item,
			Holders:         holders,
			CreatedByName:   nameByUserID[item.CreatedBy],
		})
	}
	return usage
}
