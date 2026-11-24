package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
)

func TestWorldDesc_SubjectIsActor(t *testing.T) {
	d := worldDesc("Moonhaven")
	if d.Detail != "Moonhaven" || d.SubjectUserID != "" || d.PlayerID != "" {
		t.Fatalf("got %+v", d)
	}
}

func TestInitiativeSubmitDesc(t *testing.T) {
	prior := 12
	d := initiativeSubmitDesc("p1", &prior, 17)
	if d.PlayerID != "p1" || len(d.Changes) != 1 || d.Changes[0].Field != "initiative" || d.Changes[0].Before != 12 || d.Changes[0].After != 17 {
		t.Fatalf("got %+v", d)
	}
	if d := initiativeSubmitDesc("p1", nil, 17); d.Changes[0].Before != nil {
		t.Fatalf("first roll: %+v", d)
	}
}

func TestInitiativeSkipDesc_PlayerTarget(t *testing.T) {
	d := initiativeSkipDesc(&model.InitiativeParticipant{ParticipantType: "player", PlayerID: "p1", Name: "Thalia"}, true)
	if d.PlayerID != "p1" || d.Title != "Skipped in initiative" || d.Detail != "Thalia" {
		t.Fatalf("got %+v", d)
	}
}

func TestInitiativeSkipDesc_Resume(t *testing.T) {
	d := initiativeSkipDesc(&model.InitiativeParticipant{ParticipantType: "player", PlayerID: "p1", Name: "Thalia"}, false)
	if d.Title != "Resumed in initiative" {
		t.Fatalf("got %+v", d)
	}
}

func TestInitiativeSkipDesc_EnemyTarget(t *testing.T) {
	d := initiativeSkipDesc(&model.InitiativeParticipant{ParticipantType: "enemy", Name: "Goblin"}, true)
	if d.PlayerID != "" || d.SubjectUserID != "" || d.Detail != "Goblin" {
		t.Fatalf("got %+v", d)
	}
}

func TestAccountDesc_NoCampaignNoData(t *testing.T) {
	for _, key := range []string{
		"POST /api/auth/change-password", "POST /api/auth/change-email",
		"POST /api/auth/verify-email-change", "POST /api/auth/resend-email-change",
	} {
		if info, ok := activity.Registry[key]; !ok || !info.AccountLevel {
			t.Fatalf("%s must be account level: %+v", key, info)
		}
	}
}
