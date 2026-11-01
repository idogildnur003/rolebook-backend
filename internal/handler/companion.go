package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/elad/rolebook-backend/internal/model"
)

// resolveCompanionOwner decides which PC owns a new companion. Non-DMs always
// own their own pet (the request field is ignored). A DM must name an owner PC.
// status is 0 on success; otherwise status/code describe the error to return.
func resolveCompanionOwner(m *campaignMembership, reqOwnerPlayerID string, ownerIsPC bool) (ownerPlayerID string, status int, code string) {
	if !m.IsDM {
		if m.PlayerID == "" {
			return "", http.StatusForbidden, "FORBIDDEN"
		}
		return m.PlayerID, 0, ""
	}
	if reqOwnerPlayerID == "" || !ownerIsPC {
		return "", http.StatusBadRequest, "BAD_REQUEST"
	}
	return reqOwnerPlayerID, 0, ""
}

// isDuplicateKey reports whether err is a MongoDB duplicate-key error.
func isDuplicateKey(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}

// CreateCompanion handles POST /api/campaigns/:campaignId/companions.
// A player creates their own pet; the DM creates one for a named PC.
func (h *PlayerHandler) CreateCompanion(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	var req struct {
		Name          string `json:"name"`
		CreatureType  string `json:"creatureType"`
		OwnerPlayerID string `json:"ownerPlayerId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}

	ctx := r.Context()
	var owner *model.Player
	ownerIsPC := false
	if m.IsDM && req.OwnerPlayerID != "" {
		p, err := h.players.Get(ctx, req.OwnerPlayerID, m.UserID, true)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if p != nil && p.Kind == string(model.PlayerKindPC) && p.CampaignID == campaignID {
			owner = p
			ownerIsPC = true
		}
	}

	ownerPlayerID, status, code := resolveCompanionOwner(m, req.OwnerPlayerID, ownerIsPC)
	if status != 0 {
		msg := "forbidden"
		if status == http.StatusBadRequest {
			msg = "ownerPlayerId must be a player character in this campaign"
		}
		writeError(w, status, msg, code)
		return
	}

	if !m.IsDM {
		p, err := h.players.Get(ctx, ownerPlayerID, m.UserID, false)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if p == nil || p.Kind != string(model.PlayerKindPC) || p.CampaignID != campaignID {
			writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
			return
		}
		owner = p
	}

	existing, err := h.players.FindCompanionByOwner(ctx, campaignID, ownerPlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "companion already exists", "COMPANION_EXISTS")
		return
	}

	pet := model.DefaultCompanion(uuid.NewString(), campaignID, owner.LinkedUserID, ownerPlayerID, name, strings.TrimSpace(req.CreatureType))
	if err := h.players.Create(ctx, pet); err != nil {
		if isDuplicateKey(err) {
			writeError(w, http.StatusConflict, "companion already exists", "COMPANION_EXISTS")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	h.resolveAvatarURI(ctx, pet)
	writeJSON(w, http.StatusCreated, pet)
}

// ListCompanions handles GET /api/campaigns/:campaignId/companions.
// The DM sees every pet in the campaign; a player sees only their own.
func (h *PlayerHandler) ListCompanions(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}

	pets, err := h.players.ListCompanions(r.Context(), campaignID, m.UserID, m.IsDM)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	h.resolveAvatarURIs(r.Context(), pets)
	writeJSON(w, http.StatusOK, pets)
}
