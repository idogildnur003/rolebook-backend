package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// ContentNoteHandler serves a member's PRIVATE per-entry notes. Owner-scoped:
// every read/write is bound to the authenticated caller's user id. No DM path.
type ContentNoteHandler struct {
	notes     *store.ContentNoteStore
	campaigns *store.CampaignStore
}

func NewContentNoteHandler(notes *store.ContentNoteStore, campaigns *store.CampaignStore) *ContentNoteHandler {
	return &ContentNoteHandler{notes: notes, campaigns: campaigns}
}

// List handles GET /api/campaigns/{campaignId}/content-notes — the caller's own notes.
func (h *ContentNoteHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	notes, err := h.notes.ListForUser(r.Context(), campaignID, membership.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

// Upsert handles PUT /api/campaigns/{campaignId}/content-notes/{targetType}/{entryId}.
// Body: {"body": "..."} — empty/whitespace body deletes the note. Owner-scoped.
func (h *ContentNoteHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	targetType := chi.URLParam(r, "targetType")
	entryID := chi.URLParam(r, "entryId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !model.IsValidNoteTarget(targetType) {
		writeError(w, http.StatusBadRequest, "targetType must be 'item' or 'spell'", "BAD_REQUEST")
		return
	}
	if entryID == "" {
		writeError(w, http.StatusBadRequest, "entryId is required", "BAD_REQUEST")
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	trimmed := strings.TrimSpace(body.Body)
	if trimmed == "" {
		// Empty body clears the note.
		if _, err := h.notes.Delete(r.Context(), campaignID, membership.UserID, targetType, entryID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	note, err := h.notes.Upsert(r.Context(), campaignID, membership.UserID, targetType, entryID, trimmed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, note)
}
