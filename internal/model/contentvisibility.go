package model

import "strings"

// Visibility modes for an approved custom catalog entry (CustomEquipment,
// CustomSpell). An empty/missing mode is treated as VisibilityCampaign so
// pre-existing documents (created before this feature) stay campaign-wide.
const (
	// VisibilityCampaign — every campaign member can see and use the entry.
	VisibilityCampaign = "campaign"
	// VisibilityPlayers — only the players listed in VisiblePlayerIDs (plus the DM).
	VisibilityPlayers = "players"
)

// NormalizeVisibilityMode lower-cases/trims and maps "" to VisibilityCampaign.
// Unrecognised values pass through unchanged so IsValidVisibilityMode can reject them.
func NormalizeVisibilityMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "", VisibilityCampaign:
		return VisibilityCampaign
	case VisibilityPlayers:
		return VisibilityPlayers
	default:
		return m
	}
}

// IsValidVisibilityMode reports whether m is exactly one supported mode.
func IsValidVisibilityMode(m string) bool {
	return m == VisibilityCampaign || m == VisibilityPlayers
}

// CanPlayerSeeEntry reports whether the player (or DM) may see/use an entry.
// Campaign (and empty/legacy) → everyone; players → listed players only, or DM.
func CanPlayerSeeEntry(mode string, visiblePlayerIDs []string, playerID string, isDM bool) bool {
	if isDM {
		return true
	}
	if NormalizeVisibilityMode(mode) != VisibilityPlayers {
		return true
	}
	for _, id := range visiblePlayerIDs {
		if id == playerID {
			return true
		}
	}
	return false
}

// AllowedPlayerIDsForCascade returns the set of players who keep access after a
// visibility value is applied, and whether a removal cascade should run.
// Campaign visibility never removes anyone (runCascade=false). Players visibility
// keeps exactly the listed ids; the caller pulls the entry from everyone else.
func AllowedPlayerIDsForCascade(mode string, visiblePlayerIDs []string) (allowed []string, runCascade bool) {
	if NormalizeVisibilityMode(mode) != VisibilityPlayers {
		return nil, false
	}
	return visiblePlayerIDs, true
}
