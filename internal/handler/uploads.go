package handler

import (
	"errors"
	"net/http"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

// UploadKindPlayerAvatar is the only kind currently supported by /uploads/url.
const UploadKindPlayerAvatar = "player-avatar"

// UploadsHandler issues short-lived presigned URLs for direct-to-S3 uploads.
type UploadsHandler struct {
	avatars *avatarstore.Store
	players *store.PlayerStore
}

// NewUploadsHandler wires the dependencies.
func NewUploadsHandler(avatars *avatarstore.Store, players *store.PlayerStore) *UploadsHandler {
	return &UploadsHandler{avatars: avatars, players: players}
}

type uploadURLRequest struct {
	Kind        string `json:"kind"`
	PlayerID    string `json:"playerId"`
	ContentType string `json:"contentType"`
}

type uploadURLResponse struct {
	UploadURL string `json:"uploadUrl"`
	Key       string `json:"key"`
	ExpiresAt string `json:"expiresAt"`
}

// CreateURL handles POST /api/uploads/url.
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
	if req.Kind != UploadKindPlayerAvatar {
		writeError(w, http.StatusBadRequest, "unsupported upload kind", "BAD_REQUEST")
		return
	}
	if req.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "playerId is required", "BAD_REQUEST")
		return
	}
	if _, ok := avatarstore.AllowedContentTypes[req.ContentType]; !ok {
		writeError(w, http.StatusBadRequest, "contentType must be image/jpeg, image/png, or image/webp", "BAD_REQUEST")
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
		if errors.Is(err, avatarstore.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "upload storage not configured", "UPLOAD_NOT_CONFIGURED")
			return
		}
		if errors.Is(err, avatarstore.ErrInvalidContentType) {
			writeError(w, http.StatusBadRequest, "invalid content type", "BAD_REQUEST")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create upload url", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, uploadURLResponse{
		UploadURL: presigned.URL,
		Key:       presigned.Key,
		ExpiresAt: presigned.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
