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
}

// NewInventoryHandler creates an InventoryHandler.
func NewInventoryHandler(inventory *store.InventoryStore, players *store.PlayerStore) *InventoryHandler {
	return &InventoryHandler{inventory: inventory, players: players}
}

// List handles GET /api/players/:playerId/inventory.
func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	items, err := h.inventory.ListForPlayer(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// Create handles POST /api/players/:playerId/inventory.
func (h *InventoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	// Resolve the player to get linkedUserId for denormalization
	player, err := h.players.Get(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}

	var req struct {
		Name       string   `json:"name"`
		Quantity   int      `json:"quantity"`
		Category   string   `json:"category"`
		Tags       []string `json:"tags"`
		Notes      string   `json:"notes"`
		ImageURI   string   `json:"imageUri"`

		// Weapon fields
		Damage     string   `json:"damage"`
		DamageType string   `json:"damageType"`
		WeaponType string   `json:"weaponType"`
		Properties []string `json:"properties"`

		// Armor fields
		ArmorClass          *int    `json:"armorClass"`
		ArmorBonus          *int    `json:"armorBonus"`
		ShieldBonus         *int    `json:"shieldBonus"`
		ArmorType           string  `json:"armorType"`
		StrengthRequirement *int    `json:"strengthRequirement"`
		StealthDisadvantage *bool   `json:"stealthDisadvantage"`

		// Magic item fields
		CompatibleWith *string  `json:"compatibleWith"`
		EffectSummary  string   `json:"effectSummary"`
		Value          *float64 `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
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

	item := &model.InventoryItem{
		ID:           uuid.NewString(),
		PlayerID:     playerID,
		LinkedUserID: player.LinkedUserID,
		Name:         req.Name,
		Quantity:     req.Quantity,
		Category:     req.Category,
		Tags:         req.Tags,
		Notes:        req.Notes,
		ImageURI:     req.ImageURI,
		Damage:       req.Damage,
		DamageType:   req.DamageType,
		WeaponType:   req.WeaponType,
		Properties:   req.Properties,
		ArmorClass:          req.ArmorClass,
		ArmorBonus:          req.ArmorBonus,
		ShieldBonus:         req.ShieldBonus,
		ArmorType:           req.ArmorType,
		StrengthRequirement: req.StrengthRequirement,
		StealthDisadvantage: req.StealthDisadvantage,
		CompatibleWith:      req.CompatibleWith,
		EffectSummary:       req.EffectSummary,
		Value:               req.Value,
		UpdatedAt:    time.Now().UTC(),
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
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Strip protected fields
	delete(req, "_id")
	delete(req, "id")
	delete(req, "playerId")
	delete(req, "linkedUserId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.inventory.Update(r.Context(), itemID, userID, isAdmin, bson.M(req))
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

// Delete handles DELETE /api/inventory/:itemId (admin only — enforced by middleware).
func (h *InventoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemId")
	userID := middleware.UserIDFromContext(r.Context())
	found, err := h.inventory.Delete(r.Context(), itemID, userID, true)
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
