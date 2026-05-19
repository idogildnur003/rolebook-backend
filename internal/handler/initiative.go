package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/elad/rolebook-backend/internal/initiative"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// InitiativeHandler exposes the per-campaign initiative tracker.
//
// Permissions:
//   - Get: any campaign member.
//   - Start, Enemy, Resolve: DM only.
//   - Submit: the player themselves, or the DM.
//   - EndTurn: DM, or the player whose turn it currently is.
type InitiativeHandler struct {
	calls     *store.InitiativeStore
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

func NewInitiativeHandler(calls *store.InitiativeStore, players *store.PlayerStore, campaigns *store.CampaignStore) *InitiativeHandler {
	return &InitiativeHandler{calls: calls, players: players, campaigns: campaigns}
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// Get handles GET /api/campaigns/{campaignId}/initiative.
// Returns 204 when there is no call (including after the TTL sweeps a
// resolved one). A still-resolved call is returned so the client can render
// the terminal state for one poll cycle before its hook stops.
func (h *InitiativeHandler) Get(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	if resolveCampaignMembership(w, r, h.campaigns, campaignID) == nil {
		return
	}
	call, err := h.calls.Get(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if call == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, call)
}

// Start handles POST /api/campaigns/{campaignId}/initiative (DM only).
// Builds player participants from the campaign's PC players and overwrites
// any prior call.
func (h *InitiativeHandler) Start(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	if !m.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	// isDM=true: DM access already asserted above; fetch all PC players regardless of caller.
	players, err := h.players.ListForCampaign(r.Context(), campaignID, m.UserID, true, model.PlayerKindPC)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	participants := make([]model.InitiativeParticipant, 0, len(players))
	for _, p := range players {
		participants = append(participants, model.InitiativeParticipant{
			ID:              "player:" + p.ID,
			ParticipantType: "player",
			PlayerID:        p.ID,
			Name:            p.Name,
			Initiative:      nil,
			AvatarURI:       p.AvatarURI,
		})
	}
	now := nowMillis()
	call := &model.InitiativeCall{
		CampaignID:              campaignID,
		ID:                      uuid.NewString(),
		Status:                  "open",
		StartedAt:               now,
		UpdatedAt:               now,
		StartedByPlayerID:       m.PlayerID,
		Participants:            participants,
		TurnOrderParticipantIDs: []string{},
		HasTurnCycleStarted:     false,
	}
	if err := h.calls.StartReplace(r.Context(), call); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, call)
}

// mutateWithRetry loads the call, applies mutate, recomputes turn state, and
// writes with optimistic versioning, retrying on conflict (max 3). mutate
// returns an (httpStatus, code, message) on rejection, or 0 to proceed.
func (h *InitiativeHandler) mutateWithRetry(
	w http.ResponseWriter, r *http.Request, campaignID string,
	mutate func(c *model.InitiativeCall) (int, string, string),
) {
	for attempt := 0; attempt < 3; attempt++ {
		call, err := h.calls.Get(r.Context(), campaignID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if call == nil || call.Status != "open" {
			writeError(w, http.StatusNotFound, "no open initiative call", "NOT_FOUND")
			return
		}
		expected := call.Version
		if status, code, msg := mutate(call); status != 0 {
			writeError(w, status, msg, code)
			return
		}
		call.UpdatedAt = nowMillis()
		initiative.SyncTurnState(call)
		updated, err := h.calls.UpdateWithVersion(r.Context(), call, expected)
		if err == store.ErrInitiativeVersionConflict {
			continue
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	writeError(w, http.StatusConflict, "initiative is being updated, retry", "CONFLICT")
}

type submitInitiativeRequest struct {
	PlayerID   string `json:"playerId"`
	Initiative int    `json:"initiative"`
}

// Submit handles POST /api/campaigns/{campaignId}/initiative/submit.
// The DM may submit for anyone; a player may submit only for their own player.
func (h *InitiativeHandler) Submit(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	var body submitInitiativeRequest
	if err := decodeJSON(r, &body); err != nil || body.PlayerID == "" {
		writeError(w, http.StatusBadRequest, "playerId required", "BAD_REQUEST")
		return
	}
	if !m.IsDM && body.PlayerID != m.PlayerID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	h.mutateWithRetry(w, r, campaignID, func(c *model.InitiativeCall) (int, string, string) {
		found := false
		for i := range c.Participants {
			if c.Participants[i].PlayerID == body.PlayerID {
				v := body.Initiative
				at := nowMillis()
				c.Participants[i].Initiative = &v
				c.Participants[i].SubmittedAt = &at
				c.Participants[i].SubmittedByPlayerID = m.PlayerID
				found = true
			}
		}
		if !found {
			return http.StatusNotFound, "NOT_FOUND", "participant not in call"
		}
		return 0, "", ""
	})
}

type enemyInitiativeRequest struct {
	ParticipantID string `json:"participantId"`
	Name          string `json:"name"`
	Initiative    int    `json:"initiative"`
}

// Enemy handles POST /api/campaigns/{campaignId}/initiative/enemies (DM only).
func (h *InitiativeHandler) Enemy(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	if !m.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	var body enemyInitiativeRequest
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required", "BAD_REQUEST")
		return
	}
	h.mutateWithRetry(w, r, campaignID, func(c *model.InitiativeCall) (int, string, string) {
		at := nowMillis()
		v := body.Initiative
		if body.ParticipantID != "" {
			for i := range c.Participants {
				if c.Participants[i].ID == body.ParticipantID {
					c.Participants[i].Name = body.Name
					c.Participants[i].Initiative = &v
					c.Participants[i].SubmittedAt = &at
					c.Participants[i].SubmittedByPlayerID = m.PlayerID
					return 0, "", ""
				}
			}
		}
		c.Participants = append(c.Participants, model.InitiativeParticipant{
			ID:                  uuid.NewString(),
			ParticipantType:     "enemy",
			Name:                body.Name,
			Initiative:          &v,
			SubmittedAt:         &at,
			SubmittedByPlayerID: m.PlayerID,
		})
		return 0, "", ""
	})
}

// EndTurn handles POST /api/campaigns/{campaignId}/initiative/end-turn.
// Allowed for the DM, or the player whose turn it currently is.
func (h *InitiativeHandler) EndTurn(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	h.mutateWithRetry(w, r, campaignID, func(c *model.InitiativeCall) (int, string, string) {
		order := initiative.TurnOrderParticipantIDs(c)
		if len(order) == 0 {
			return http.StatusBadRequest, "BAD_REQUEST", "no turn order yet"
		}
		if !m.IsDM {
			byID := make(map[string]model.InitiativeParticipant, len(c.Participants))
			for _, p := range c.Participants {
				byID[p.ID] = p
			}
			cur := byID[order[0]]
			if cur.PlayerID == "" || cur.PlayerID != m.PlayerID {
				return http.StatusForbidden, "FORBIDDEN", "not your turn"
			}
		}
		next := order
		if len(order) > 1 {
			next = append(append([]string{}, order[1:]...), order[0])
		}
		c.TurnOrderParticipantIDs = next
		c.CurrentTurnParticipantID = next[0]
		c.HasTurnCycleStarted = true
		return 0, "", ""
	})
}

// Resolve handles POST /api/campaigns/{campaignId}/initiative/resolve (DM only).
func (h *InitiativeHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	m := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if m == nil {
		return
	}
	if !m.IsDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}
	call, err := h.calls.Resolve(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if call == nil {
		writeError(w, http.StatusNotFound, "no initiative call", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, call)
}
