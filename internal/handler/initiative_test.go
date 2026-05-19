package handler

import (
	"reflect"
	"testing"

	"github.com/elad/rolebook-backend/internal/initiative"
	"github.com/elad/rolebook-backend/internal/model"
)

func mkParticipant(id, playerID string, init int) model.InitiativeParticipant {
	v := init
	return model.InitiativeParticipant{ID: id, PlayerID: playerID, Name: id, Initiative: &v}
}

// Rotating then re-syncing keeps every submitted participant in the cycle.
func TestEndTurnRotation_PreservesParticipants(t *testing.T) {
	c := &model.InitiativeCall{
		Status:              "open",
		HasTurnCycleStarted: true,
		Participants: []model.InitiativeParticipant{
			mkParticipant("a", "p1", 20),
			mkParticipant("b", "p2", 10),
			mkParticipant("c", "", 5),
		},
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
	}
	order := initiative.TurnOrderParticipantIDs(c)
	next := append(append([]string{}, order[1:]...), order[0])
	c.TurnOrderParticipantIDs = next
	c.CurrentTurnParticipantID = next[0]
	initiative.SyncTurnState(c)
	got := append([]string{}, c.TurnOrderParticipantIDs...)
	if len(got) != 3 {
		t.Fatalf("lost participants: %v", got)
	}
	if c.CurrentTurnParticipantID != got[0] {
		t.Fatalf("current %q != first %q", c.CurrentTurnParticipantID, got[0])
	}
}

// A fresh call with no rolls yields an empty order (end-turn must reject).
func TestTurnOrderEmpty_WhenNoRolls(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			{ID: "a", PlayerID: "p1", Name: "A", Initiative: nil},
		},
	}
	if got := initiative.TurnOrderParticipantIDs(c); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("expected empty order, got %v", got)
	}
}
