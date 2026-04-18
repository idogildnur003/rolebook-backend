package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type InventoryHandler struct {
	players         *store.PlayerStore
	campaigns       *store.CampaignStore
	arsenal         *catalog.ArsenalCatalog
	customEquipment *store.CustomEquipmentStore
}

func NewInventoryHandler(
	players *store.PlayerStore,
	campaigns *store.CampaignStore,
	arsenal *catalog.ArsenalCatalog,
	customEquipment *store.CustomEquipmentStore,
) *InventoryHandler {
	return &InventoryHandler{
		players:         players,
		campaigns:       campaigns,
		arsenal:         arsenal,
		customEquipment: customEquipment,
	}
}

// List handles GET /api/players/:playerId/inventory.
func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	campaign, err := h.campaigns.GetByID(r.Context(), access.Player.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	disabledSet := make(map[string]bool, len(campaign.DisabledEquipment))
	for _, id := range campaign.DisabledEquipment {
		disabledSet[id] = true
	}

	items := make([]model.PlayerInventoryItem, 0)
	for _, item := range access.Player.Inventory {
		if !disabledSet[item.EquipmentID] {
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

// Create handles POST /api/players/:playerId/inventory.
func (h *InventoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req struct {
		EquipmentID string `json:"equipmentId"`
		Quantity    int    `json:"quantity"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.EquipmentID == "" {
		writeError(w, http.StatusBadRequest, "equipmentId is required", "BAD_REQUEST")
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	var itemName string
	if equipment := h.arsenal.GetEquipment(req.EquipmentID); equipment != nil {
		itemName = equipment.Name
	} else {
		custom, err := h.customEquipment.GetByID(r.Context(), access.Player.CampaignID, req.EquipmentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if custom == nil {
			writeError(w, http.StatusNotFound, "equipment not found in arsenal", "NOT_FOUND")
			return
		}
		itemName = custom.Name
	}

	playerItem := model.PlayerInventoryItem{
		EquipmentID: req.EquipmentID,
		Name:        itemName,
		Quantity:    req.Quantity,
	}

	if err := h.players.AddInventoryItem(r.Context(), playerID, playerItem); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "equipment already in player inventory", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, playerItem)
}

// Update handles PATCH /api/players/:playerId/inventory/:equipmentId.
func (h *InventoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	equipmentID := chi.URLParam(r, "equipmentId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Strip immutable fields
	delete(req, "equipmentId")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	found, err := h.players.UpdateInventoryItem(r.Context(), playerID, equipmentID, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "item not found in player inventory", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/players/:playerId/inventory/:equipmentId.
func (h *InventoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	equipmentID := chi.URLParam(r, "equipmentId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	found, err := h.players.RemoveInventoryItem(r.Context(), playerID, equipmentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "item not found in player inventory", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
