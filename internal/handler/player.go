package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// displayNameFromEmail derives a human-readable display name from an email address.
// "john.doe@example.com" → "John Doe"
// "jdoe@example.com"    → "Jdoe"
func displayNameFromEmail(email string) string {
	local := email
	if at := strings.IndexByte(email, '@'); at >= 0 {
		local = email[:at]
	}
	// Replace common separators with spaces
	local = strings.Map(func(r rune) rune {
		if r == '.' || r == '_' || r == '-' || r == '+' {
			return ' '
		}
		return r
	}, local)
	// Title-case each word
	words := strings.Fields(local)
	for i, w := range words {
		if w == "" {
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	result := strings.Join(words, " ")
	if result == "" {
		return "Player"
	}
	return result
}

// DisplayNameFromEmailExported is the exported alias used by the migration tool
// (cmd/migrate-members) to derive the same display name the API uses when
// linking a user to a campaign. Keeping the implementation private avoids
// accidental misuse from non-DM flows.
func DisplayNameFromEmailExported(email string) string { return displayNameFromEmail(email) }

// PlayerHandler handles all player CRUD endpoints.
type PlayerHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
	users     *store.UserStore
	avatars   *avatarstore.Store
}

// NewPlayerHandler creates a PlayerHandler.
func NewPlayerHandler(players *store.PlayerStore, campaigns *store.CampaignStore, users *store.UserStore, avatars *avatarstore.Store) *PlayerHandler {
	return &PlayerHandler{players: players, campaigns: campaigns, users: users, avatars: avatars}
}

// resolveAvatarURI rewrites a stored avatarUri (typically an S3 key) into a
// short-lived presigned GET URL suitable for the wire. Pass-through when the
// store is unconfigured or the value already looks like a URL.
func (h *PlayerHandler) resolveAvatarURI(ctx context.Context, p *model.Player) {
	if p == nil || p.AvatarURI == "" {
		return
	}
	p.AvatarURI = h.avatars.ResolveAvatarURI(ctx, p.AvatarURI)
}

func (h *PlayerHandler) resolveAvatarURIs(ctx context.Context, ps []model.Player) {
	for i := range ps {
		if ps[i].AvatarURI == "" {
			continue
		}
		ps[i].AvatarURI = h.avatars.ResolveAvatarURI(ctx, ps[i].AvatarURI)
	}
}

// listKindForRole returns the kind filter for the campaign roster listing.
// The DM sees every kind (PCs, NPCs, enemies); any non-DM path stays PC-only.
func listKindForRole(isDM bool) model.PlayerKind {
	if isDM {
		return ""
	}
	return model.PlayerKindPC
}

// ListForCampaign handles GET /api/campaigns/:campaignId/players (campaign DM only).
func (h *PlayerHandler) ListForCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	userID := membership.UserID

	players, err := h.players.ListForCampaign(r.Context(), campaignID, userID, true, listKindForRole(membership.IsDM))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	h.resolveAvatarURIs(r.Context(), players)
	writeJSON(w, http.StatusOK, players)
}

// GetMyPlayer handles GET /api/campaigns/:campaignId/player.
// Returns the caller's own player character in the given campaign.
func (h *PlayerHandler) GetMyPlayer(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())

	players, err := h.players.ListForCampaign(r.Context(), campaignID, userID, false, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if len(players) == 0 {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	h.resolveAvatarURI(r.Context(), &players[0])
	writeJSON(w, http.StatusOK, players[0])
}

// Get handles GET /api/players/:playerId.
// DM of the player's campaign or the player's linked user can access.
func (h *PlayerHandler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	h.resolveAvatarURI(r.Context(), access.Player)
	writeJSON(w, http.StatusOK, access.Player)
}

// Create handles POST /api/players (campaign DM only).
// Only campaignId and userEmail are required. The player fills in their own details later.
func (h *PlayerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CampaignID string `json:"campaignId"`
		UserEmail  string `json:"userEmail"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.CampaignID == "" || req.UserEmail == "" {
		writeError(w, http.StatusBadRequest, "campaignId and userEmail are required", "BAD_REQUEST")
		return
	}

	membership := resolveCampaignMembership(w, r, h.campaigns, req.CampaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	linkedUser, err := h.users.FindByEmail(r.Context(), req.UserEmail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if linkedUser == nil {
		writeError(w, http.StatusNotFound, "user not found", "NOT_FOUND")
		return
	}

	// Seed the player name from their email so the DM never sees a blank/ghost entry.
	// The player can rename their character at any time from their own profile screen.
	initialName := displayNameFromEmail(linkedUser.Email)
	player := model.DefaultPlayer(uuid.NewString(), req.CampaignID, linkedUser.ID, initialName, 1, model.PlayerKindPC)

	if err := h.players.Create(r.Context(), player); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	cm := model.CampaignMember{UserID: linkedUser.ID, PlayerID: player.ID, Role: model.RolePlayer, IsActive: true}
	if err := h.campaigns.AddMember(r.Context(), req.CampaignID, cm); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, player)
}

// CreateNPC handles POST /api/campaigns/:campaignId/npcs (campaign DM only).
// Creates an unlinked, DM-owned character (kind "npc" or "enemy"). The DM
// fills in the rest of the sheet via PATCH /api/players/:playerId afterward.
func (h *PlayerHandler) CreateNPC(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	var req struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if !model.IsDMCreatableKind(req.Kind) {
		writeError(w, http.StatusBadRequest, "kind must be npc or enemy", "BAD_REQUEST")
		return
	}

	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	player := model.DefaultPlayer(uuid.NewString(), campaignID, "", req.Name, 1, model.PlayerKind(req.Kind))
	if err := h.players.Create(r.Context(), player); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, player)
}

// Update handles PATCH /api/players/:playerId.
// DM of the player's campaign or the player's linked user can update.
// Protected fields (campaignId, linkedUserId) are stripped before applying.
func (h *PlayerHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	userID := middleware.UserIDFromContext(r.Context())

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	// Strip protected fields
	delete(req, "campaignId")
	delete(req, "linkedUserId")
	delete(req, "_id")
	delete(req, "id")

	// Validate death save bounds
	if v, ok := req["deathSaveSuccesses"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveSuccesses must be 0-3", "BAD_REQUEST")
			return
		}
	}
	if v, ok := req["deathSaveFailures"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveFailures must be 0-3", "BAD_REQUEST")
			return
		}
	}

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	var oldAvatarKey string
	avatarKeyChanged := false
	if v, ok := req["avatarUri"]; ok {
		newAvatarKey, _ := v.(string)
		oldAvatarKey = access.Player.AvatarURI
		if newAvatarKey != oldAvatarKey {
			avatarKeyChanged = true
			if newAvatarKey != "" {
				if err := h.avatars.Verify(r.Context(), newAvatarKey); err != nil {
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

	updated, err := h.players.Update(r.Context(), playerID, userID, access.IsDM, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	if avatarKeyChanged && oldAvatarKey != "" {
		if err := h.avatars.Delete(r.Context(), oldAvatarKey); err != nil {
			log.Printf("player %s: delete old avatar %q: %v", playerID, oldAvatarKey, err)
		}
	}
	h.resolveAvatarURI(r.Context(), updated)
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/players/:playerId (campaign DM only).
func (h *PlayerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}
	if !access.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	found, err := h.players.Delete(r.Context(), playerID, userID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
