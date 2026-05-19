package initiative

import (
	"reflect"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func ip(id string, init *int, name string) model.InitiativeParticipant {
	return model.InitiativeParticipant{ID: id, Name: name, Initiative: init}
}
func intp(v int) *int { return &v }

func TestSortParticipants_ByInitiativeDescThenName(t *testing.T) {
	got := SortParticipants([]model.InitiativeParticipant{
		ip("a", intp(10), "Zed"),
		ip("b", intp(20), "Amy"),
		ip("c", intp(10), "Ada"),
	})
	want := []string{"b", "c", "a"} // 20, then 10/"Ada", then 10/"Zed"
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("got %v want %v", ids, want)
	}
}

func TestTurnOrderIDs_NotStarted_ReturnsSortedSubmitted(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(5), "A"), ip("b", nil, "B"), ip("c", intp(15), "C"),
		},
	}
	got := TurnOrderParticipantIDs(c)
	want := []string{"c", "a"} // "b" excluded (no roll)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTurnOrderIDs_Started_RotatesToCurrent(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"c", "a", "d"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(5), "A"), ip("c", intp(15), "C"), ip("d", intp(1), "D"),
		},
	}
	got := TurnOrderParticipantIDs(c)
	want := []string{"a", "d", "c"} // rotated so current ("a") is first
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSyncTurnState_SetsCurrentToFirst(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(5), "A"), ip("c", intp(15), "C"),
		},
	}
	out := SyncTurnState(c)
	if out.CurrentTurnParticipantID != "c" {
		t.Fatalf("current = %q want c", out.CurrentTurnParticipantID)
	}
	if !reflect.DeepEqual(out.TurnOrderParticipantIDs, []string{"c", "a"}) {
		t.Fatalf("order = %v", out.TurnOrderParticipantIDs)
	}
}

func TestTurnOrderIDs_NilCall_ReturnsEmpty(t *testing.T) {
	if got := TurnOrderParticipantIDs(nil); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("got %v want []", got)
	}
}

func TestTurnOrderIDs_ResolvedCall_ReturnsEmpty(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "resolved",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(5), "A"),
		},
	}
	if got := TurnOrderParticipantIDs(c); !reflect.DeepEqual(got, []string{}) {
		t.Fatalf("got %v want []", got)
	}
}

func TestSyncTurnState_NilCall_ReturnsNil(t *testing.T) {
	if SyncTurnState(nil) != nil {
		t.Fatal("SyncTurnState(nil) must return nil")
	}
}
