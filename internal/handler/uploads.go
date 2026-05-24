package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

const (
	UploadKindPlayerAvatar      = "player-avatar"
	UploadKindMap               = "map"
	UploadKindLocationThumbnail = "location-thumbnail"
	UploadKindNPCAvatar         = "npc-avatar"
	UploadKindEquipmentImage    = "equipment-image"
	UploadKindSpellImage        = "spell-image"
)

// UploadsHandler issues short-lived presigned URLs for direct-to-S3 uploads.
type UploadsHandler struct {
	avatars   *avatarstore.Store
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

// NewUploadsHandler wires the dependencies.
func NewUploadsHandler(avatars *avatarstore.Store, players *store.PlayerStore, campaigns *store.CampaignStore) *UploadsHandler {
	return &UploadsHandler{avatars: avatars, players: players, campaigns: campaigns}
}

type uploadURLRequest struct {
	Kind        string `json:"kind"`
	PlayerID    string `json:"playerId"`
	CampaignID  string `json:"campaignId"`
	ContentType string `json:"contentType"`
}

type uploadURLResponse struct {
	UploadURL string `json:"uploadUrl"`
	Key       string `json:"key"`
	ExpiresAt string `json:"expiresAt"`
}

// CreateURL handles POST /api/uploads/url. Routes by `kind`:
//   - player-avatar           → handlePlayerAvatar (DM or the player themselves)
//   - map                     → handleCampaignKind, DM only
//   - location-thumbnail      → handleCampaignKind, any member
//   - npc-avatar              → handleCampaignKind, any member
//   - equipment-image         → handleCampaignKind, any member
//   - spell-image             → handleCampaignKind, any member
func (h *UploadsHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	if !h.avatars.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, "upload storage not configured", "UPLOAD_NOT_CONFIGURED")
		return
	}

	var req uploadURLRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if _, ok := avatarstore.AllowedContentTypes[req.ContentType]; !ok {
		writeError(w, http.StatusBadRequest, "contentType must be image/jpeg, image/png, or image/webp", "BAD_REQUEST")
		return
	}

	switch req.Kind {
	case UploadKindPlayerAvatar:
		h.handlePlayerAvatar(w, r, &req)
	case UploadKindMap, UploadKindLocationThumbnail, UploadKindNPCAvatar, UploadKindEquipmentImage, UploadKindSpellImage:
		h.handleCampaignKind(w, r, &req)
	default:
		writeError(w, http.StatusBadRequest, "unsupported upload kind", "BAD_REQUEST")
	}
}

// handlePlayerAvatar preserves the existing player-avatar flow: only the player
// linked to the JWT subject (or DM via player.LinkedUserID check elsewhere) can
// presign their own avatar key.
func (h *UploadsHandler) handlePlayerAvatar(w http.ResponseWriter, r *http.Request, req *uploadURLRequest) {
	if req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "playerId is required", "BAD_REQUEST")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	player, err := h.players.Get(r.Context(), req.PlayerID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	if player.LinkedUserID != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	presigned, err := h.avatars.PresignPut(r.Context(), req.PlayerID, req.ContentType)
	if err != nil {
		writePresignError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, uploadURLResponse{
		UploadURL: presigned.URL,
		Key:       presigned.Key,
		ExpiresAt: presigned.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// handleCampaignKind handles map / location-thumbnail / npc-avatar uploads.
// Map uploads require DM; the others allow any campaign member.
func (h *UploadsHandler) handleCampaignKind(w http.ResponseWriter, r *http.Request, req *uploadURLRequest) {
	if req.CampaignID == "" {
		writeError(w, http.StatusBadRequest, "campaignId is required", "BAD_REQUEST")
		return
	}
	membership := resolveCampaignMembership(w, r, h.campaigns, req.CampaignID)
	if membership == nil {
		return
	}
	if req.Kind == UploadKindMap && !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can upload the campaign map", "FORBIDDEN")
		return
	}

	var prefix string
	switch req.Kind {
	case UploadKindMap:
		prefix = fmt.Sprintf("campaigns/%s/maps", req.CampaignID)
	case UploadKindLocationThumbnail:
		prefix = fmt.Sprintf("campaigns/%s/locations", req.CampaignID)
	case UploadKindNPCAvatar:
		prefix = fmt.Sprintf("campaigns/%s/npcs", req.CampaignID)
	case UploadKindEquipmentImage:
		prefix = fmt.Sprintf("campaigns/%s/equipment", req.CampaignID)
	case UploadKindSpellImage:
		prefix = fmt.Sprintf("campaigns/%s/spells", req.CampaignID)
	}

	presigned, err := h.avatars.PresignPutForKey(r.Context(), prefix, req.ContentType)
	if err != nil {
		writePresignError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, uploadURLResponse{
		UploadURL: presigned.URL,
		Key:       presigned.Key,
		ExpiresAt: presigned.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// writePresignError translates avatarstore errors to HTTP responses. Shared
// between handlePlayerAvatar and handleCampaignKind to avoid duplicating the
// switch.
func writePresignError(w http.ResponseWriter, err error) {
	if errors.Is(err, avatarstore.ErrNotConfigured) {
		writeError(w, http.StatusServiceUnavailable, "upload storage not configured", "UPLOAD_NOT_CONFIGURED")
		return
	}
	if errors.Is(err, avatarstore.ErrInvalidContentType) {
		writeError(w, http.StatusBadRequest, "invalid content type", "BAD_REQUEST")
		return
	}
	writeError(w, http.StatusInternalServerError, "could not create upload url", "INTERNAL_ERROR")
}
