package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// SessionHandler handles session sub-resource operations on campaigns.
type SessionHandler struct {
	campaigns *store.CampaignStore
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(campaigns *store.CampaignStore) *SessionHandler {
	return &SessionHandler{campaigns: campaigns}
}

// Create handles POST /api/campaigns/:campaignId/sessions (DM only).
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	sess := model.Session{
		ID:          uuid.NewString(),
		Name:        req.Name,
		Description: req.Description,
	}
	created, err := h.campaigns.AddSession(r.Context(), campaignID, sess)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if created == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// Update handles PATCH /api/campaigns/:campaignId/sessions/:sessionId (DM only).
func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	allowed := map[string]bool{"name": true, "description": true}
	fields := bson.M{}
	for k, v := range req {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.campaigns.UpdateSession(r.Context(), campaignID, sessionID, fields)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/sessions/:sessionId (DM only).
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")

	found, err := h.campaigns.DeleteSession(r.Context(), campaignID, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
