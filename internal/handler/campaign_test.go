package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

// The wire-shape contract for this refactor: NO userIds appear on the wire.
// Identity is by playerId. Caller-specific fields (myRole, myPlayerId) are
// computed per request from the JWT. These tests lock the contract by
// JSON-marshaling the wire types and asserting the result contains no
// userId-shaped tokens. They would have caught a regression where someone
// adds a UserID field with a default json tag, or removes the json:"-" tag
// from a model type that ends up being marshaled.

func TestCampaignMemberSummary_OmitsUserIDOnWire(t *testing.T) {
	s := campaignMemberSummary{
		PlayerID: "p-1",
		Role:     model.RolePlayer,
		IsActive: true,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(strings.ToLower(got), "userid") {
		t.Errorf("campaignMemberSummary leaked userId: %s", got)
	}
}

func TestCampaignDetail_OmitsCallerUserIDOnWire(t *testing.T) {
	c := &model.Campaign{
		ID:   "c-1",
		Name: "Test",
		Members: []model.CampaignMember{
			{UserID: "u-dm-secret", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
			{UserID: "u-1-secret", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
		},
	}
	detail := toCampaignDetail(c, "u-dm-secret")
	b, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	// Neither the caller's userId nor any other member's userId should appear.
	if strings.Contains(got, "u-dm-secret") || strings.Contains(got, "u-1-secret") {
		t.Errorf("campaignDetail leaked a userId on the wire: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "userid") {
		t.Errorf("campaignDetail emitted a userId-shaped key: %s", got)
	}
	// Sanity: the playerId-keyed identity *is* present.
	if !strings.Contains(got, "p-dm") || !strings.Contains(got, "p-1") {
		t.Errorf("campaignDetail missing expected playerIds: %s", got)
	}
}

func TestToMemberSummaries_OmitsUserIDOnWire(t *testing.T) {
	in := []model.CampaignMember{
		{UserID: "u-secret", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
	}
	out := toMemberSummaries(in)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "u-secret") {
		t.Errorf("toMemberSummaries leaked userId: %s", got)
	}
}
