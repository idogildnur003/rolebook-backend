package handler

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type LimitedUsesHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

func NewLimitedUsesHandler(players *store.PlayerStore, campaigns *store.CampaignStore) *LimitedUsesHandler {
	return &LimitedUsesHandler{players: players, campaigns: campaigns}
}

func parseLimitedUseName(raw any) (string, error) {
	s, ok := raw.(string)
	if !ok {
		return "", errors.New("name must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > model.LimitedUseNameMax {
		return "", fmt.Errorf("name must be 1-%d characters", model.LimitedUseNameMax)
	}
	return s, nil
}

func parseLimitedUseNote(raw any) (string, error) {
	s, ok := raw.(string)
	if !ok {
		return "", errors.New("note must be a string")
	}
	s = strings.TrimSpace(s)
	if len([]rune(s)) > model.LimitedUseNoteMax {
		return "", fmt.Errorf("note must be at most %d characters", model.LimitedUseNoteMax)
	}
	return s, nil
}

func parseLimitedUseMax(raw any) (int, error) {
	f, ok := raw.(float64)
	if !ok || f != math.Trunc(f) || f < 1 || f > model.LimitedUseMaxUses {
		return 0, fmt.Errorf("max must be an integer from 1 to %d", model.LimitedUseMaxUses)
	}
	return int(f), nil
}

func parseLimitedUseResets(raw any) (bool, error) {
	b, ok := raw.(bool)
	if !ok {
		return false, errors.New("resets must be a boolean")
	}
	return b, nil
}

// parseLimitedUseCreate validates a create body. name, max and resets are
// required; note is optional. ID is left empty and Remaining is set to Max.
func parseLimitedUseCreate(body map[string]any) (model.LimitedUse, error) {
	var lu model.LimitedUse
	var err error

	raw, ok := body["name"]
	if !ok {
		return lu, errors.New("name is required")
	}
	if lu.Name, err = parseLimitedUseName(raw); err != nil {
		return lu, err
	}
	if raw, ok = body["note"]; ok {
		if lu.Note, err = parseLimitedUseNote(raw); err != nil {
			return lu, err
		}
	}
	if raw, ok = body["max"]; !ok {
		return lu, errors.New("max is required")
	}
	if lu.Max, err = parseLimitedUseMax(raw); err != nil {
		return lu, err
	}
	if raw, ok = body["resets"]; !ok {
		return lu, errors.New("resets is required")
	}
	if lu.Resets, err = parseLimitedUseResets(raw); err != nil {
		return lu, err
	}
	lu.Remaining = lu.Max
	return lu, nil
}

// parseLimitedUsePatch validates an update body. Unknown keys are ignored;
// at least one known key is required.
func parseLimitedUsePatch(body map[string]any) (store.LimitedUsePatch, error) {
	var p store.LimitedUsePatch
	if raw, ok := body["name"]; ok {
		s, err := parseLimitedUseName(raw)
		if err != nil {
			return p, err
		}
		p.Name = &s
	}
	if raw, ok := body["note"]; ok {
		s, err := parseLimitedUseNote(raw)
		if err != nil {
			return p, err
		}
		p.Note = &s
	}
	if raw, ok := body["max"]; ok {
		n, err := parseLimitedUseMax(raw)
		if err != nil {
			return p, err
		}
		p.Max = &n
	}
	if raw, ok := body["resets"]; ok {
		b, err := parseLimitedUseResets(raw)
		if err != nil {
			return p, err
		}
		p.Resets = &b
	}
	if p.Name == nil && p.Note == nil && p.Max == nil && p.Resets == nil {
		return p, errors.New("no valid fields to update")
	}
	return p, nil
}

// writeLimitedUseStoreError maps a store error from a limited-use mutation to a response.
func writeLimitedUseStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrLimitedUseNotFound):
		writeError(w, http.StatusNotFound, "limited use not found", "LIMITED_USE_NOT_FOUND")
	case errors.Is(err, store.ErrNoUsesLeft):
		writeError(w, http.StatusConflict, "no uses left", "NO_USES_LEFT")
	case errors.Is(err, store.ErrAlreadyFull):
		writeError(w, http.StatusConflict, "already at max uses", "ALREADY_FULL")
	case errors.Is(err, store.ErrLimitedUseChanged):
		writeError(w, http.StatusConflict, "limited use changed, please retry", "LIMITED_USE_CHANGED")
	case errors.Is(err, store.ErrNotResettable):
		writeError(w, http.StatusBadRequest, "this ability does not reset", "NOT_RESETTABLE")
	case errors.Is(err, store.ErrLimitReached):
		writeError(w, http.StatusBadRequest, fmt.Sprintf("at most %d limited uses per character", model.LimitedUseMaxCount), "LIMIT_REACHED")
	case errors.Is(err, mongo.ErrNoDocuments):
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
	}
}

// decodeBody reads a JSON object body, writing a 400 on failure.
func (h *LimitedUsesHandler) decodeBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil || body == nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return nil, false
	}
	return body, true
}

// List handles GET /api/players/:playerId/limited-uses.
func (h *LimitedUsesHandler) List(w http.ResponseWriter, r *http.Request) {
	access := resolvePlayerAccess(w, r, h.players, h.campaigns, chi.URLParam(r, "playerId"))
	if access == nil {
		return
	}
	list := access.Player.LimitedUses
	if list == nil {
		list = model.LimitedUseList{}
	}
	writeJSON(w, http.StatusOK, list)
}

// Create handles POST /api/players/:playerId/limited-uses.
func (h *LimitedUsesHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	body, ok := h.decodeBody(w, r)
	if !ok {
		return
	}
	lu, err := parseLimitedUseCreate(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	lu.ID = uuid.NewString()
	item, err := h.players.AddLimitedUse(r.Context(), playerID, lu)
	if err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// Update handles PATCH /api/players/:playerId/limited-uses/:limitedUseId.
func (h *LimitedUsesHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	body, ok := h.decodeBody(w, r)
	if !ok {
		return
	}
	patch, err := parseLimitedUsePatch(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	item, err := h.players.UpdateLimitedUse(r.Context(), playerID, chi.URLParam(r, "limitedUseId"), patch)
	if err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// Delete handles DELETE /api/players/:playerId/limited-uses/:limitedUseId.
func (h *LimitedUsesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	if err := h.players.RemoveLimitedUse(r.Context(), playerID, chi.URLParam(r, "limitedUseId")); err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Use handles POST /api/players/:playerId/limited-uses/:limitedUseId/use.
func (h *LimitedUsesHandler) Use(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	item, removed, err := h.players.SpendLimitedUse(r.Context(), playerID, chi.URLParam(r, "limitedUseId"))
	if err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item, "removed": removed})
}

// Restore handles POST /api/players/:playerId/limited-uses/:limitedUseId/restore.
func (h *LimitedUsesHandler) Restore(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	item, err := h.players.RestoreLimitedUse(r.Context(), playerID, chi.URLParam(r, "limitedUseId"))
	if err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// Reset handles POST /api/players/:playerId/limited-uses/:limitedUseId/reset.
func (h *LimitedUsesHandler) Reset(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	if resolvePlayerAccess(w, r, h.players, h.campaigns, playerID) == nil {
		return
	}
	item, err := h.players.ResetLimitedUse(r.Context(), playerID, chi.URLParam(r, "limitedUseId"))
	if err != nil {
		writeLimitedUseStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
