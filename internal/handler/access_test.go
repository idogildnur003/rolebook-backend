package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestFindMember_DM(t *testing.T) {
	c := &model.Campaign{Members: []model.CampaignMember{
		{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
		{UserID: "u-1", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
	}}
	m := findMember(c, "u-dm")
	if m == nil || m.Role != model.RoleDM || m.PlayerID != "p-dm" {
		t.Fatalf("findMember(dm) = %+v", m)
	}
}

func TestFindMember_Player(t *testing.T) {
	c := &model.Campaign{Members: []model.CampaignMember{
		{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
		{UserID: "u-1", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
	}}
	m := findMember(c, "u-1")
	if m == nil || m.Role != model.RolePlayer || m.PlayerID != "p-1" {
		t.Fatalf("findMember(player) = %+v", m)
	}
}

func TestFindMember_NonMember(t *testing.T) {
	c := &model.Campaign{Members: []model.CampaignMember{
		{UserID: "u-dm", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
	}}
	if m := findMember(c, "u-stranger"); m != nil {
		t.Fatalf("findMember(stranger) = %+v, want nil", m)
	}
}

func TestFindMember_InactivePlayerStillFound(t *testing.T) {
	// Inactive players are still members (archived). Authorization decides what to do.
	c := &model.Campaign{Members: []model.CampaignMember{
		{UserID: "u-1", PlayerID: "p-1", Role: model.RolePlayer, IsActive: false},
	}}
	m := findMember(c, "u-1")
	if m == nil || m.IsActive {
		t.Fatalf("findMember(inactive) = %+v", m)
	}
}
