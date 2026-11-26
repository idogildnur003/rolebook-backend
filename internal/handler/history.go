package handler

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

const (
	historyDefaultLimit = 50
	historyMaxLimit     = 100
	// historyMaxQueryRunes keeps the escaped search inside MongoDB's regex limit.
	historyMaxQueryRunes = 100
)

// HistoryHandler serves a member's own change history.
type HistoryHandler struct {
	activity  *store.ActivityStore
	campaigns *store.CampaignStore
}

// NewHistoryHandler creates the handler.
func NewHistoryHandler(a *store.ActivityStore, c *store.CampaignStore) *HistoryHandler {
	return &HistoryHandler{activity: a, campaigns: c}
}

func encodeHistoryCursor(t time.Time, id string) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeHistoryCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	stamp, id, found := strings.Cut(string(raw), "|")
	if !found || id == "" {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}, "", err
	}
	return t, id, nil
}

// parseHistoryQuery reads filters. Campaign and subject are never taken from
// the query: the handler sets them from the route and the authenticated caller.
func parseHistoryQuery(q url.Values) (store.ActivityFilter, error) {
	f := store.ActivityFilter{
		Session:    q.Get("session"),
		Categories: q["category"],
		ByDM:       q.Get("byDm") == "true",
		Query:      cleanHistorySearch(q.Get("q")),
		Limit:      historyDefaultLimit,
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		f.Limit = min(n, historyMaxLimit)
	}
	if before := q.Get("before"); before != "" {
		t, id, err := decodeHistoryCursor(before)
		if err != nil {
			return f, err
		}
		f.BeforeTime, f.BeforeID = t, id
	}
	return f, nil
}

// cleanHistorySearch removes NUL bytes, which MongoDB rejects in a regex, and caps length.
func cleanHistorySearch(raw string) string {
	runes := []rune(strings.TrimSpace(strings.ReplaceAll(raw, "\x00", "")))
	if len(runes) > historyMaxQueryRunes {
		runes = runes[:historyMaxQueryRunes]
	}
	return string(runes)
}

// List handles GET /api/campaigns/:campaignId/history.
func (h *HistoryHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	f, err := parseHistoryQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor", "BAD_REQUEST")
		return
	}
	f.CampaignID, f.SubjectUserID = campaignID, membership.UserID
	pageSize := f.Limit
	f.Limit = pageSize + 1
	items, err := h.activity.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	var next *string
	if len(items) > pageSize {
		items = items[:pageSize]
		last := items[pageSize-1]
		cursor := encodeHistoryCursor(last.CreatedAt, last.ID)
		next = &cursor
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []model.ActivityEntry `json:"items"`
		NextCursor *string               `json:"nextCursor"`
	}{items, next})
}

// Sessions handles GET /api/campaigns/:campaignId/history/sessions.
func (h *HistoryHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	entries, err := h.activity.ListForSummary(r.Context(), campaignID, membership.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	pcPlayerID := membership.PlayerID
	if membership.IsDM {
		pcPlayerID = ""
	}
	writeJSON(w, http.StatusOK, struct {
		Sessions []activity.SessionBucket `json:"sessions"`
	}{activity.BuildSessionBuckets(entries, membership.Campaign, pcPlayerID)})
}
