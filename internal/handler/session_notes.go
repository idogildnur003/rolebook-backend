package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/store"
)

// sessionNoteMaxLength caps the size of a single session note (in characters).
const sessionNoteMaxLength = 10_000

// errSessionNoteTooLong signals that the input exceeds sessionNoteMaxLength.
var errSessionNoteTooLong = errors.New("session note text too long")

// normalizeSessionNoteText trims leading/trailing whitespace and decides
// whether the input represents a clear (empty/whitespace) or an upsert.
// It rejects text longer than sessionNoteMaxLength.
//
// Returns:
//   - text:    the trimmed text (empty when cleared)
//   - cleared: true when the trimmed input is empty
//   - err:     non-nil when the input is too long
func normalizeSessionNoteText(s string) (text string, cleared bool, err error) {
	if len(s) > sessionNoteMaxLength {
		return "", false, errSessionNoteTooLong
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", true, nil
	}
	return trimmed, false, nil
}

// normalizeSessionNoteDirection coerces arbitrary input to a valid text
// direction. Only the exact string "rtl" maps to RTL; everything else
// (including "", "RTL", unknown values) defaults to "ltr".
func normalizeSessionNoteDirection(s string) string {
	if s == "rtl" {
		return "rtl"
	}
	return "ltr"
}

// SessionNotesHandler serves per-user, per-session private notes.
// Notes live on each CampaignMember entry and are never serialized in
// any other endpoint's response.
type SessionNotesHandler struct {
	campaigns *store.CampaignStore
}

// NewSessionNotesHandler constructs a SessionNotesHandler.
func NewSessionNotesHandler(campaigns *store.CampaignStore) *SessionNotesHandler {
	return &SessionNotesHandler{campaigns: campaigns}
}

// sessionNoteDTO is the wire shape for a single note (text + direction).
type sessionNoteDTO struct {
	Text      string `json:"text"`
	Direction string `json:"direction"`
}

// sessionNotesGetResponse is the wire shape returned by GetMine.
type sessionNotesGetResponse struct {
	Notes map[string]sessionNoteDTO `json:"notes"`
}

// sessionNotesPutRequest is the wire shape accepted by PutMine.
type sessionNotesPutRequest struct {
	Text      string `json:"text"`
	Direction string `json:"direction"`
}

// sessionNotesPutResponse is the wire shape returned by PutMine.
type sessionNotesPutResponse struct {
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
	Direction string `json:"direction"`
}

// GetMine handles GET /api/campaigns/:campaignId/my-session-notes.
// Returns the caller's notes for every session in the campaign as a map.
// Allowed for any campaign member — including inactive members (read access
// is preserved on archive, matching the codebase convention).
func (h *SessionNotesHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return // error already written
	}

	notes, err := h.campaigns.GetMemberSessionNotes(r.Context(), campaignID, membership.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	dto := make(map[string]sessionNoteDTO, len(notes))
	for id, n := range notes {
		dto[id] = sessionNoteDTO{Text: n.Text, Direction: normalizeSessionNoteDirection(n.Direction)}
	}
	writeJSON(w, http.StatusOK, sessionNotesGetResponse{Notes: dto})
}

// PutMine handles PUT /api/campaigns/:campaignId/sessions/:sessionId/my-notes.
// Upserts the caller's note for the named session. Empty/whitespace-only text
// removes the note. Caller must be an *active* member of the campaign.
func (h *SessionNotesHandler) PutMine(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")

	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	// Active-only gate for writes (read remained open in GetMine above).
	if member := findMember(membership.Campaign, membership.UserID); member == nil || !member.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	// sessionId must exist on the campaign.
	sessionExists := false
	for _, s := range membership.Campaign.Sessions {
		if s.ID == sessionID {
			sessionExists = true
			break
		}
	}
	if !sessionExists {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}

	var req sessionNotesPutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	text, cleared, err := normalizeSessionNoteText(req.Text)
	if err != nil {
		if errors.Is(err, errSessionNoteTooLong) {
			writeError(w, http.StatusBadRequest, "text exceeds 10000 characters", "BAD_REQUEST")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid text", "BAD_REQUEST")
		return
	}
	direction := normalizeSessionNoteDirection(req.Direction)

	if cleared {
		ok, err := h.campaigns.DeleteMemberSessionNote(r.Context(), campaignID, membership.UserID, sessionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
			return
		}
		writeJSON(w, http.StatusOK, sessionNotesPutResponse{SessionID: sessionID, Text: "", Direction: direction})
		return
	}

	ok, err := h.campaigns.UpsertMemberSessionNote(r.Context(), campaignID, membership.UserID, sessionID, text, direction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, sessionNotesPutResponse{SessionID: sessionID, Text: text, Direction: direction})
}
