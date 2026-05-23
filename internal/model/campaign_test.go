package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// CampaignMember must never serialize its SessionNotes map onto the wire.
// This guards the per-user notes privacy contract: notes live inside the
// storage struct only and are surfaced through a dedicated endpoint that
// filters to the caller. A regression here (e.g. someone changes the json
// tag) would leak every member's notes to anyone fetching the campaign.
func TestCampaignMember_OmitsSessionNotesOnWire(t *testing.T) {
	m := CampaignMember{
		UserID:   "u-1",
		PlayerID: "p-1",
		Role:     RolePlayer,
		IsActive: true,
		SessionNotes: map[string]MemberSessionNote{
			"s-1": {Text: "secret"},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(strings.ToLower(got), "sessionnotes") {
		t.Errorf("CampaignMember leaked sessionNotes key: %s", got)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("CampaignMember leaked sessionNotes value: %s", got)
	}
}
