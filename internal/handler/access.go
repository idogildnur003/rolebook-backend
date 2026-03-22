package handler

import (
	"context"
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

	// Fetch the player without ownership filter so we can check campaign DM.
	player, err := players.Get(ctx, playerID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return nil
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return nil
	}

	isDM, ok := isCampaignDM(ctx, campaigns, player.CampaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return nil
	}

	// NPCs are DM-only. Regular players cannot access them even if they have the ID.
	if player.IsNPC && !isDM {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return nil
	}

	// Allow if the user is the campaign DM or the player's linked user.
	if !isDM && player.LinkedUserID != userID {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return nil
	}

	return &playerAccess{Player: player, IsDM: isDM}
}

// isCampaignDM checks whether userID is the DM of the given campaign.
// Returns (isDM, ok). ok is false if there was a DB error.
func isCampaignDM(ctx context.Context, campaigns *store.CampaignStore, campaignID, userID string) (bool, bool) {
	campaign, err := campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return false, false
	}
	if campaign == nil {
		return false, true
	}
	return campaign.DM == userID, true
}
