package handler

import (
	"net/http"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// playerAccess holds the resolved player and whether the caller is the campaign DM.
type playerAccess struct {
	Player *model.Player
	IsDM   bool
}

// resolvePlayerAccess fetches a player (unfiltered) and determines whether the
// requesting user is the DM of the player's campaign or the player's linked user.
// Returns nil with an HTTP error written if the player/campaign is not found or
// the user has no access.
func resolvePlayerAccess(w http.ResponseWriter, r *http.Request, players *store.PlayerStore, campaigns *store.CampaignStore, playerID string) *playerAccess {
	ctx := r.Context()
	userID := middleware.UserIDFromContext(ctx)

	player, err := players.Get(ctx, playerID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return nil
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return nil
	}

	campaign, err := campaigns.GetByID(ctx, player.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return nil
	}
	isDM := false
	if campaign != nil {
		if m := findMember(campaign, userID); m != nil && m.Role == model.RoleDM {
			isDM = true
		}
	}

	if !isDM && player.LinkedUserID != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return nil
	}

	return &playerAccess{Player: player, IsDM: isDM}
}

// campaignMembership holds the resolved campaign and the caller's role within it.
type campaignMembership struct {
	Campaign *model.Campaign
	IsDM     bool
	IsMember bool
	UserID   string
	PlayerID string // caller's playerId in this campaign (empty when not a member)
}

// resolveCampaignMembership loads a campaign and resolves the caller's membership.
// Writes an HTTP error and returns nil on DB error, missing campaign, or non-member.
// IsMember is true when the caller has any entry in members[] (DM included);
// archived (isActive=false) players are still considered members for read access.
// Per-feature gating on isActive must be applied by the handler if needed.
func resolveCampaignMembership(w http.ResponseWriter, r *http.Request, campaigns *store.CampaignStore, campaignID string) *campaignMembership {
	ctx := r.Context()
	userID := middleware.UserIDFromContext(ctx)

	campaign, err := campaigns.GetByID(ctx, campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return nil
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return nil
	}

	m := findMember(campaign, userID)
	if m == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return nil
	}

	return &campaignMembership{
		Campaign: campaign,
		IsDM:     m.Role == model.RoleDM,
		IsMember: true,
		UserID:   userID,
		PlayerID: m.PlayerID,
	}
}

// findMember returns the member entry for userID, or nil if none.
// Pure function over a Campaign value; safe to test without Mongo.
func findMember(c *model.Campaign, userID string) *model.CampaignMember {
	for i := range c.Members {
		if c.Members[i].UserID == userID {
			return &c.Members[i]
		}
	}
	return nil
}
