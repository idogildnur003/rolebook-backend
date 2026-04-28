// Package migrate holds the pure logic of the one-shot members migration.
// I/O (Mongo, env, logging) lives in cmd/migrate-members.
package migrate

import (
	"github.com/elad/rolebook-backend/internal/model"
)

// LegacyCampaign is the on-disk shape we read before the migration.
// Mirrors the old model.Campaign with DM scalar + Players array.
type LegacyCampaign struct {
	ID      string                 `bson:"_id"`
	DM      string                 `bson:"dm,omitempty"`
	Members []model.CampaignMember `bson:"members,omitempty"`
	Players []legacyPlayerEntry    `bson:"players,omitempty"`
}

type legacyPlayerEntry struct {
	UserID   string `bson:"userId"`
	PlayerID string `bson:"playerId"`
	IsActive bool   `bson:"isActive"`
}

// Result describes what to do for one campaign.
type Result struct {
	Status         Status
	NewMembers     []model.CampaignMember // present when Status==Migrate
	DMPlayer       *model.Player          // present when Status==Migrate
	OrphanDMPlayer *string                // playerId of an existing orphan DM stub to reuse (Status==MigrateReuseOrphan)
}

type Status int

const (
	StatusAlreadyMigrated Status = iota
	StatusMigrate
	StatusMigrateReuseOrphan // a DM Player was created on a prior interrupted run; reuse it
)

// PlanCampaign decides what to do with a single legacy campaign.
//
// dmDisplayName is looked up by the caller (User.Email → displayNameFromEmail).
// existingDMPlayerID is the ID of an orphan DM-kind Player in this campaign,
// if one was found by the caller; otherwise empty.
// newPlayerID is a fresh UUID to assign to a new DM stub Player.
func PlanCampaign(c LegacyCampaign, dmDisplayName, existingDMPlayerID, newPlayerID string) Result {
	if isAlreadyMigrated(c) {
		return Result{Status: StatusAlreadyMigrated}
	}

	dmPlayerID := newPlayerID
	if existingDMPlayerID != "" {
		dmPlayerID = existingDMPlayerID
	}

	dmPlayer := model.DefaultPlayer(dmPlayerID, c.ID, c.DM, dmDisplayName, 0, model.PlayerKindDM)

	members := make([]model.CampaignMember, 0, len(c.Players)+1)
	members = append(members, model.CampaignMember{
		UserID:   c.DM,
		PlayerID: dmPlayerID,
		Role:     model.RoleDM,
		IsActive: true,
	})
	for _, p := range c.Players {
		members = append(members, model.CampaignMember{
			UserID:   p.UserID,
			PlayerID: p.PlayerID,
			Role:     model.RolePlayer,
			IsActive: p.IsActive,
		})
	}

	status := StatusMigrate
	if existingDMPlayerID != "" {
		status = StatusMigrateReuseOrphan
	}
	return Result{Status: status, NewMembers: members, DMPlayer: dmPlayer}
}

// isAlreadyMigrated returns true when the campaign document is in the new shape:
// no top-level dm, no legacy players[], and a non-empty members[] with a Role on every entry.
func isAlreadyMigrated(c LegacyCampaign) bool {
	if c.DM != "" {
		return false
	}
	if len(c.Players) > 0 {
		return false
	}
	if len(c.Members) == 0 {
		return false
	}
	for _, m := range c.Members {
		if m.Role == "" {
			return false
		}
	}
	return true
}
