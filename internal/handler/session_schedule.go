package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// SessionScheduleHandler handles the schedule sub-resources under a session.
type SessionScheduleHandler struct {
	campaigns *store.CampaignStore
}

// NewSessionScheduleHandler creates a SessionScheduleHandler.
func NewSessionScheduleHandler(campaigns *store.CampaignStore) *SessionScheduleHandler {
	return &SessionScheduleHandler{campaigns: campaigns}
}

var allowedDayParts = map[string]bool{"morning": true, "noon": true, "evening": true}

func parseDateKey(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// validateTimezoneHeader returns the X-Timezone value if non-empty and valid,
// "" if absent, or writes a 400 INVALID_TIMEZONE and returns "" with errored=true.
func validateTimezoneHeader(w http.ResponseWriter, r *http.Request) (tz string, errored bool) {
	tz = r.Header.Get("X-Timezone")
	if tz == "" {
		return "", false
	}
	if _, err := time.LoadLocation(tz); err != nil {
		writeError(w, http.StatusBadRequest, "invalid X-Timezone header", "INVALID_TIMEZONE")
		return "", true
	}
	return tz, false
}

// findSessionSchedule returns the existing schedule for sessionID within the
// campaign, or nil if either the session is not found or it has no schedule.
func findSessionSchedule(c *model.Campaign, sessionID string) *model.SessionSchedule {
	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			return c.Sessions[i].Schedule
		}
	}
	return nil
}

// PutAvailability handles PUT /api/campaigns/:campaignId/sessions/:sessionId/availability.
// Caller must be an active campaign member; only their own entry is replaced.
func (h *SessionScheduleHandler) PutAvailability(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")
	ctx := r.Context()

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	// Read access via membership is OK even when archived, but writes are not.
	member := findMember(m.Campaign, m.UserID)
	if member == nil || !member.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req struct {
		AvailabilityByDate map[string]model.SessionDayParts `json:"availabilityByDate"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.AvailabilityByDate == nil {
		req.AvailabilityByDate = map[string]model.SessionDayParts{}
	}
	for dateKey := range req.AvailabilityByDate {
		if !parseDateKey(dateKey) {
			writeError(w, http.StatusBadRequest, "invalid date key: "+dateKey, "INVALID_DATE_KEY")
			return
		}
	}

	tz, errored := validateTimezoneHeader(w, r)
	if errored {
		return
	}

	// Find the current session to detect bootstrap path.
	existingSchedule := findSessionSchedule(m.Campaign, sessionID)
	// Non-DM callers writing before bootstrap are rejected below by the store
	// (ErrScheduleNotInitialized → 409), so the timezone-required check only
	// applies to the DM's bootstrap path.
	if m.IsDM && existingSchedule == nil && tz == "" {
		writeError(w, http.StatusBadRequest, "X-Timezone header required on first availability write", "INVALID_TIMEZONE")
		return
	}

	sess, err := h.campaigns.SetSessionAvailability(ctx, campaignID, sessionID, m.PlayerID, req.AvailabilityByDate, tz, m.IsDM)
	if errors.Is(err, store.ErrScheduleNotInitialized) {
		writeError(w, http.StatusConflict, "the DM has not initialized this session's schedule yet", "SCHEDULE_NOT_INITIALIZED")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// DeleteAvailability handles DELETE /.../availability — removes the caller's entry.
func (h *SessionScheduleHandler) DeleteAvailability(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")
	ctx := r.Context()

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	member := findMember(m.Campaign, m.UserID)
	if member == nil || !member.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	sess, err := h.campaigns.DeleteSessionAvailability(ctx, campaignID, sessionID, m.PlayerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PutConfirmedSlot handles PUT /.../confirmed-slot (DM only).
func (h *SessionScheduleHandler) PutConfirmedSlot(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")
	ctx := r.Context()

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	if !m.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req struct {
		Date            string  `json:"date"`
		DayPart         string  `json:"dayPart"`
		StartAt         *string `json:"startAt,omitempty"`
		DurationMinutes *int    `json:"durationMinutes,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if !parseDateKey(req.Date) {
		writeError(w, http.StatusBadRequest, "invalid date", "INVALID_DATE_KEY")
		return
	}
	if !allowedDayParts[req.DayPart] {
		writeError(w, http.StatusBadRequest, "invalid dayPart", "INVALID_DAY_PART")
		return
	}
	slot := model.SessionConfirmedSlot{Date: req.Date, DayPart: req.DayPart}
	if req.StartAt != nil {
		t, err := time.Parse(time.RFC3339, *req.StartAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid startAt", "BAD_REQUEST")
			return
		}
		slot.StartAt = &t
	}
	if req.DurationMinutes != nil {
		if *req.DurationMinutes <= 0 {
			writeError(w, http.StatusBadRequest, "durationMinutes must be > 0", "INVALID_DURATION")
			return
		}
		slot.DurationMinutes = req.DurationMinutes
	}

	tz, errored := validateTimezoneHeader(w, r)
	if errored {
		return
	}
	// Bootstrap-path requires a timezone.
	existingSchedule := findSessionSchedule(m.Campaign, sessionID)
	if existingSchedule == nil && tz == "" {
		writeError(w, http.StatusBadRequest, "X-Timezone header required when initializing the schedule", "INVALID_TIMEZONE")
		return
	}

	sess, err := h.campaigns.SetSessionConfirmedSlot(ctx, campaignID, sessionID, slot, tz)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// DeleteConfirmedSlot handles DELETE /.../confirmed-slot (DM only).
func (h *SessionScheduleHandler) DeleteConfirmedSlot(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")
	ctx := r.Context()

	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	if !m.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	sess, err := h.campaigns.DeleteSessionConfirmedSlot(ctx, campaignID, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
