package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// InventoryHandler handles inventory item endpoints.
type InventoryHandler struct {
	inventory *store.InventoryStore
	players   *store.PlayerStore
	campaigns *store.CampaignStore
	arsenal   *store.ArsenalStore
}

// NewInventoryHandler creates an InventoryHandler.
func NewInventoryHandler(inventory *store.InventoryStore, players *store.PlayerStore, campaigns *store.CampaignStore, arsenal *store.ArsenalStore) *InventoryHandler {
	return &InventoryHandler{inventory: inventory, players: players, campaigns: campaigns, arsenal: arsenal}
}

// List handles GET /api/players/:playerId/inventory.
func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	items, err := h.inventory.ListForPlayer(r.Context(), playerID, userID, access.IsDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
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
		ArsenalEquipmentID string   `json:"arsenalEquipmentId"`
		Name               string   `json:"name"`
		Quantity           int      `json:"quantity"`
		Category           string   `json:"category"`
		Tags               []string `json:"tags"`
		Notes              string   `json:"notes"`
		ImageURI           string   `json:"imageUri"`

		// Weapon fields
		Damage     string   `json:"damage"`
		DamageType string   `json:"damageType"`
		WeaponType string   `json:"weaponType"`
		Properties []string `json:"properties"`

		// Armor fields
		ArmorClass          *int   `json:"armorClass"`
		ArmorBonus          *int   `json:"armorBonus"`
		ShieldBonus         *int   `json:"shieldBonus"`
		ArmorType           string `json:"armorType"`
		StrengthRequirement *int   `json:"strengthRequirement"`
		StealthDisadvantage *bool  `json:"stealthDisadvantage"`

		// Magic item fields
		CompatibleWith *string  `json:"compatibleWith"`
		EffectSummary  string   `json:"effectSummary"`
		Value          *float64 `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	// If an arsenal equipment ID is provided, copy fields from the catalog.
	if req.ArsenalEquipmentID != "" {
		src, err := h.arsenal.GetEquipment(r.Context(), req.ArsenalEquipmentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if src == nil {
			writeError(w, http.StatusNotFound, "arsenal equipment not found", "NOT_FOUND")
			return
		}
		req.Name = src.Name
		req.Category = src.Category
		req.Tags = src.Tags
		req.ImageURI = src.ImageURI
		req.Damage = src.Damage
		req.DamageType = src.DamageType
		req.WeaponType = src.WeaponType
		req.Properties = src.Properties
		req.ArmorClass = src.ArmorClass
		req.ArmorBonus = src.ArmorBonus
		req.ShieldBonus = src.ShieldBonus
		req.ArmorType = src.ArmorType
		req.StrengthRequirement = src.StrengthRequirement
		req.StealthDisadvantage = src.StealthDisadvantage
		req.CompatibleWith = src.CompatibleWith
		req.EffectSummary = src.EffectSummary
		req.Value = src.Value
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	if req.Properties == nil {
		req.Properties = []string{}
	}

	item := &model.InventoryItem{
		ID:                  uuid.NewString(),
		PlayerID:            playerID,
		LinkedUserID:        access.Player.LinkedUserID,
		Name:                req.Name,
		Quantity:            req.Quantity,
		Category:            req.Category,
		Tags:                req.Tags,
		Notes:               req.Notes,
		ImageURI:            req.ImageURI,
		Damage:              req.Damage,
		DamageType:          req.DamageType,
		WeaponType:          req.WeaponType,
		Properties:          req.Properties,
		ArmorClass:          req.ArmorClass,
		ArmorBonus:          req.ArmorBonus,
		ShieldBonus:         req.ShieldBonus,
		ArmorType:           req.ArmorType,
		StrengthRequirement: req.StrengthRequirement,
		StealthDisadvantage: req.StealthDisadvantage,
		CompatibleWith:      req.CompatibleWith,
		EffectSummary:       req.EffectSummary,
		Value:               req.Value,
		UpdatedAt:           time.Now().UTC(),
	}
	if err := h.inventory.Create(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// Update handles PATCH /api/inventory/:itemId.
func (h *InventoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemId")
	userID := middleware.UserIDFromContext(r.Context())

	// Fetch the item to find which player it belongs to.
	item, err := h.inventory.GetByID(r.Context(), itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}

	// Resolve access via the item's player.
	access := resolvePlayerAccess(w, r, h.players, h.campaigns, item.PlayerID)
	if access == nil {
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "playerId")
	delete(req, "linkedUserId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.inventory.Update(r.Context(), itemID, userID, access.IsDM, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/inventory/:itemId (campaign DM only).
func (h *InventoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemId")

	// Fetch the item to find which player it belongs to.
	item, err := h.inventory.GetByID(r.Context(), itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}

	// Resolve access via the item's player — must be campaign DM.
	access := resolvePlayerAccess(w, r, h.players, h.campaigns, item.PlayerID)
	if access == nil {
		return
	}
	if !access.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	found, err := h.inventory.Delete(r.Context(), itemID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
