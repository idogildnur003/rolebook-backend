package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type SpellHandler struct {
	players      *store.PlayerStore
	campaigns    *store.CampaignStore
	arsenal      *catalog.ArsenalCatalog
	customSpells *store.CustomSpellStore
}

func NewSpellHandler(
	players *store.PlayerStore,
	campaigns *store.CampaignStore,
	arsenal *catalog.ArsenalCatalog,
	customSpells *store.CustomSpellStore,
) *SpellHandler {
	return &SpellHandler{
		players:      players,
		campaigns:    campaigns,
		arsenal:      arsenal,
		customSpells: customSpells,
	}
}

// List handles GET /api/players/:playerId/spells.
func (h *SpellHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	// Get campaign for disabled spells filtering
	campaign, err := h.campaigns.GetByID(r.Context(), access.Player.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	disabledSet := make(map[string]bool, len(campaign.DisabledSpells))
	for _, id := range campaign.DisabledSpells {
		disabledSet[id] = true
	}

	spells := make([]model.PlayerSpell, 0)
	for _, s := range access.Player.Spells {
		if !disabledSet[s.SpellID] {
			spells = append(spells, s)
		}
	}
	writeJSON(w, http.StatusOK, spells)
}

// Create handles POST /api/players/:playerId/spells.
func (h *SpellHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req struct {
		SpellID    string `json:"spellId"`
		IsPrepared bool   `json:"isPrepared"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.SpellID == "" {
		writeError(w, http.StatusBadRequest, "spellId is required", "BAD_REQUEST")
		return
	}

	// Validate spell exists — SRD arsenal first, then campaign custom spells.
	var spellName string
	if spell := h.arsenal.GetSpell(req.SpellID); spell != nil {
		spellName = spell.Name
	} else {
		custom, err := h.customSpells.GetByID(r.Context(), access.Player.CampaignID, req.SpellID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		// The entry must exist AND be visible to the player whose spell list this
		// is. access.Player.ID is the owner; access.IsDM lets the DM add anywhere.
		if custom == nil || !model.CanPlayerSeeEntry(custom.VisibilityMode, custom.VisiblePlayerIDs, access.Player.ID, access.IsDM) {
			writeError(w, http.StatusNotFound, "spell not found in arsenal", "NOT_FOUND")
			return
		}
		spellName = custom.Name
	}

	playerSpell := model.PlayerSpell{
		SpellID:    req.SpellID,
		Name:       spellName,
		IsPrepared: req.IsPrepared,
	}

	if err := h.players.AddSpell(r.Context(), playerID, playerSpell); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "spell already added to player", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	activity.Describe(r, activity.Desc{Detail: playerSpell.Name})
	writeJSON(w, http.StatusCreated, playerSpell)
}

// Update handles PATCH /api/players/:playerId/spells/:spellId.
func (h *SpellHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	spellID := chi.URLParam(r, "spellId")

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
	delete(req, "spellId")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	found, err := h.players.UpdateSpell(r.Context(), playerID, spellID, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found on player", "NOT_FOUND")
		return
	}
	describeOrSkip(r, spellDesc(access.Player.Spells, spellID, req))
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/players/:playerId/spells/:spellId.
func (h *SpellHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	spellID := chi.URLParam(r, "spellId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	found, err := h.players.RemoveSpell(r.Context(), playerID, spellID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found on player", "NOT_FOUND")
		return
	}
	activity.Describe(r, spellDesc(access.Player.Spells, spellID, nil))
	w.WriteHeader(http.StatusNoContent)
}

// UpdateSpellSlots handles PUT /api/players/:playerId/spell-slots.
func (h *SpellHandler) UpdateSpellSlots(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	var slots map[string]model.SpellSlot
	if err := decodeJSON(r, &slots); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, access.IsDM, bson.M{"spellSlots": slots})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	activity.Describe(r, spellSlotDesc(access.Player.SpellSlots, updated.SpellSlots))
	writeJSON(w, http.StatusOK, updated)
}

// castLevels resolves which slot a cast spends. A cantrip spends none (0); a
// leveled spell spends its own level unless a higher slotLevel upcasts it.
func castLevels(spellLevel int, slotLevel *int) (int, error) {
	if slotLevel == nil {
		return spellLevel, nil
	}
	if spellLevel == 0 {
		return 0, errors.New("cantrips do not use spell slots")
	}
	if *slotLevel < spellLevel || *slotLevel > 9 {
		return 0, errors.New("slotLevel must be between the spell's level and 9")
	}
	return *slotLevel, nil
}

// Cast handles POST /api/players/:playerId/spells/:spellId/cast.
// Body (optional): {"slotLevel": n} to upcast. Spends one slot of that level
// atomically (none for a cantrip) and returns the player's spell slots.
func (h *SpellHandler) Cast(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	spellID := chi.URLParam(r, "spellId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req struct {
		SlotLevel *int `json:"slotLevel"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
			return
		}
	}

	var name string
	var spellLevel int
	if spell := h.arsenal.GetSpell(spellID); spell != nil {
		name, spellLevel = spell.Name, spell.Level
	} else {
		custom, err := h.customSpells.GetByID(r.Context(), access.Player.CampaignID, spellID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if custom == nil || !model.CanPlayerSeeEntry(custom.VisibilityMode, custom.VisiblePlayerIDs, access.Player.ID, access.IsDM) {
			writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
			return
		}
		name, spellLevel = custom.Name, custom.Level
	}

	slotLevel, err := castLevels(spellLevel, req.SlotLevel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	slots := access.Player.SpellSlots
	if slotLevel > 0 {
		slots, err = h.players.SpendSpellSlot(r.Context(), playerID, strconv.Itoa(slotLevel))
		if errors.Is(err, store.ErrNoSpellSlot) {
			writeError(w, http.StatusConflict, "no spell slot of that level available", "NO_SPELL_SLOT")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
	}

	activity.Describe(r, spellCastDesc(name, slotLevel, slots))
	writeJSON(w, http.StatusOK, slots)
}
