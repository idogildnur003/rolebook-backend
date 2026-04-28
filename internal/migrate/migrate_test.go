package migrate

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestPlanCampaign_FreshMigration(t *testing.T) {
	c := LegacyCampaign{
		ID: "c1",
		DM: "u-dm",
		Players: []legacyPlayerEntry{
			{UserID: "u-1", PlayerID: "p-1", IsActive: true},
			{UserID: "u-2", PlayerID: "p-2", IsActive: false},
		},
	}
	r := PlanCampaign(c, "Alice", "", "p-dm-new")

	if r.Status != StatusMigrate {
		t.Fatalf("status = %v, want StatusMigrate", r.Status)
	}
	if r.DMPlayer == nil || r.DMPlayer.ID != "p-dm-new" || r.DMPlayer.Kind != string(model.PlayerKindDM) {
		t.Fatalf("DMPlayer = %+v", r.DMPlayer)
	}
	if r.DMPlayer.LinkedUserID != "u-dm" || r.DMPlayer.CampaignID != "c1" || r.DMPlayer.Name != "Alice" {
		t.Fatalf("DMPlayer fields = %+v", r.DMPlayer)
	}
	if len(r.NewMembers) != 3 {
		t.Fatalf("NewMembers length = %d, want 3", len(r.NewMembers))
	}
	if r.NewMembers[0].Role != model.RoleDM || r.NewMembers[0].PlayerID != "p-dm-new" || r.NewMembers[0].UserID != "u-dm" || !r.NewMembers[0].IsActive {
		t.Fatalf("DM member = %+v", r.NewMembers[0])
	}
	if r.NewMembers[1].Role != model.RolePlayer || r.NewMembers[1].UserID != "u-1" || !r.NewMembers[1].IsActive {
		t.Fatalf("player1 member = %+v", r.NewMembers[1])
	}
	if r.NewMembers[2].IsActive {
		t.Fatalf("player2 IsActive should be carried over as false: %+v", r.NewMembers[2])
	}
}

func TestPlanCampaign_AlreadyMigrated(t *testing.T) {
	c := LegacyCampaign{
		ID: "c1",
		Members: []model.CampaignMember{
			{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
			{UserID: "u-1", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
		},
	}
	r := PlanCampaign(c, "Alice", "", "ignored")
	if r.Status != StatusAlreadyMigrated {
		t.Fatalf("status = %v, want StatusAlreadyMigrated", r.Status)
	}
}

func TestPlanCampaign_AlreadyMigrated_MissingRoleIsNotMigrated(t *testing.T) {
	// Defensive: a members[] without Role on every entry is treated as "not yet migrated"
	// so a second pass repairs it.
	c := LegacyCampaign{
		ID: "c1",
		DM: "u-dm",
		Members: []model.CampaignMember{
			{UserID: "u-dm", PlayerID: "p-dm" /* no Role */},
		},
	}
	r := PlanCampaign(c, "Alice", "", "p-dm-new")
	if r.Status == StatusAlreadyMigrated {
		t.Fatalf("expected migration to run; got AlreadyMigrated")
	}
}

func TestPlanCampaign_ReuseOrphanDMPlayer(t *testing.T) {
	c := LegacyCampaign{
		ID:      "c1",
		DM:      "u-dm",
		Players: []legacyPlayerEntry{},
	}
	r := PlanCampaign(c, "Alice", "p-orphan", "p-dm-new")
	if r.Status != StatusMigrateReuseOrphan {
		t.Fatalf("status = %v, want StatusMigrateReuseOrphan", r.Status)
	}
	if r.DMPlayer.ID != "p-orphan" {
		t.Fatalf("DMPlayer.ID = %q, want p-orphan (reuse)", r.DMPlayer.ID)
	}
	if r.NewMembers[0].PlayerID != "p-orphan" {
		t.Fatalf("DM member playerId = %q, want p-orphan", r.NewMembers[0].PlayerID)
	}
}

func TestPlanCampaign_NoPlayers(t *testing.T) {
	c := LegacyCampaign{ID: "c1", DM: "u-dm"}
	r := PlanCampaign(c, "Alice", "", "p-dm-new")
	if r.Status != StatusMigrate {
		t.Fatalf("status = %v", r.Status)
	}
	if len(r.NewMembers) != 1 || r.NewMembers[0].Role != model.RoleDM {
		t.Fatalf("members = %+v", r.NewMembers)
	}
}

func TestPlanCampaign_HalfMigrated_LegacyDMFieldStillSet(t *testing.T) {
	// Defensive: a campaign with both the new members[] AND a leftover legacy
	// `dm` field (e.g. hand-constructed, or a future regression where unset failed)
	// must NOT be treated as already-migrated. A second pass should re-run and
	// drop the legacy field via the CLI's $unset.
	c := LegacyCampaign{
		ID: "c1",
		DM: "u-dm",
		Members: []model.CampaignMember{
			{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
			{UserID: "u-1", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
		},
	}
	r := PlanCampaign(c, "Alice", "", "p-dm-new")
	if r.Status == StatusAlreadyMigrated {
		t.Fatalf("expected migration to re-run; got AlreadyMigrated")
	}
}

func TestPlanCampaign_HalfMigrated_LegacyPlayersFieldStillSet(t *testing.T) {
	// Symmetric to the above: leftover legacy players[] must also force a re-run.
	c := LegacyCampaign{
		ID: "c1",
		Members: []model.CampaignMember{
			{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
		},
		Players: []legacyPlayerEntry{
			{UserID: "u-1", PlayerID: "p-1", IsActive: true},
		},
	}
	r := PlanCampaign(c, "Alice", "", "p-dm-new")
	if r.Status == StatusAlreadyMigrated {
		t.Fatalf("expected migration to re-run; got AlreadyMigrated")
	}
}
