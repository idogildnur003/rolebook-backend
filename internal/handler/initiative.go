package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
