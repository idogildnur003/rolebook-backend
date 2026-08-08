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

func TestTurnOrderIDs_SkippedParticipantsExcluded(t *testing.T) {
	pSkipped := ip("b", intp(20), "Bob")
	pSkipped.IsSkipped = true
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(15), "Alice"),
			pSkipped, // would otherwise be first (20)
			ip("c", intp(10), "Cara"),
		},
	}
	got := TurnOrderParticipantIDs(c)
	want := []string{"a", "c"} // "b" excluded by IsSkipped
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// When the current participant is skipped, SyncTurnState auto-advances.
func TestSyncTurnState_AdvancesPastSkippedCurrent(t *testing.T) {
	pCurrent := ip("a", intp(20), "Alice")
	pCurrent.IsSkipped = true
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			pCurrent,
			ip("b", intp(10), "Bob"),
		},
	}
	SyncTurnState(c)
	if c.CurrentTurnParticipantID != "b" {
		t.Fatalf("expected auto-advance to b, got %q", c.CurrentTurnParticipantID)
	}
}

// orderOf runs the resort mutation the handler performs and returns the
// resulting turn order.
func resortedOrder(c *model.InitiativeCall) []string {
	Resort(c)
	SyncTurnState(c)
	return c.TurnOrderParticipantIDs
}

// The frozen-order bug Resort exists to fix: once the cycle has started, a
// combatant who rolls late (or an enemy added mid-fight) is appended to the
// end of the order regardless of how high they rolled.
func TestTurnOrderIDs_Started_AppendsLateRollerLast(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("b", intp(14), "Bram"),
			ip("c", intp(9), "Cleo"),
			ip("goblin", intp(22), "Goblin"), // added late, highest roll
		},
	}
	got := TurnOrderParticipantIDs(c)
	want := []string{"a", "b", "c", "goblin"} // goblin last despite rolling 22
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// Resort re-derives the order from initiative values while leaving the
// combatant who is currently acting on their turn.
func TestResort_ReordersByInitiative_KeepingCurrentTurn(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("b", intp(14), "Bram"),
			ip("c", intp(9), "Cleo"),
			ip("goblin", intp(22), "Goblin"),
		},
	}
	got := resortedOrder(c)
	// Sorted is [goblin 22, a 18, b 14, c 9], rotated so the acting
	// combatant ("a") stays current: the round finishes b -> c -> goblin,
	// and goblin leads the next round.
	want := []string{"a", "b", "c", "goblin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if c.CurrentTurnParticipantID != "a" {
		t.Fatalf("current turn moved to %q, want a", c.CurrentTurnParticipantID)
	}
}

// A mid-order combatant who rolled late slots into their true position
// rather than staying pinned to the end of the round.
func TestResort_MovesLateRollerIntoMidOrder(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("b", intp(14), "Bram"),
			ip("c", intp(9), "Cleo"),
			ip("goblin", intp(12), "Goblin"), // between Bram and Cleo
		},
	}
	got := resortedOrder(c)
	want := []string{"a", "b", "goblin", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// Resort keeps the cycle "started", so the freshly sorted order is the new
// frozen baseline rather than re-sorting on every later mutation.
func TestResort_KeepsCycleStarted(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("b", intp(14), "Bram"),
		},
	}
	resortedOrder(c)
	if !c.HasTurnCycleStarted {
		t.Fatal("Resort must not clear HasTurnCycleStarted")
	}
	// A later addition is appended again, not auto-sorted.
	c.Participants = append(c.Participants, ip("goblin", intp(22), "Goblin"))
	got := TurnOrderParticipantIDs(c)
	want := []string{"a", "b", "goblin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// When the acting combatant is no longer in the cycle (skipped or removed),
// there is nothing to rotate to, so the top roller leads.
func TestResort_CurrentGone_FallsBackToTopRoller(t *testing.T) {
	pSkipped := ip("a", intp(18), "Aria")
	pSkipped.IsSkipped = true
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a", "b", "c"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			pSkipped,
			ip("b", intp(14), "Bram"),
			ip("c", intp(9), "Cleo"),
			ip("goblin", intp(22), "Goblin"),
		},
	}
	got := resortedOrder(c)
	want := []string{"goblin", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if c.CurrentTurnParticipantID != "goblin" {
		t.Fatalf("current turn = %q, want goblin", c.CurrentTurnParticipantID)
	}
}

// Before the first end-turn the order is already derived straight from
// initiative, so resorting changes nothing.
func TestResort_BeforeCycleStarted_IsNoOp(t *testing.T) {
	c := &model.InitiativeCall{
		Status: "open",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("goblin", intp(22), "Goblin"),
		},
	}
	before := TurnOrderParticipantIDs(c)
	got := resortedOrder(c)
	if !reflect.DeepEqual(got, before) {
		t.Fatalf("got %v want unchanged %v", got, before)
	}
}

// Unrolled participants stay out of the cycle across a resort.
func TestResort_ExcludesUnrolledParticipants(t *testing.T) {
	c := &model.InitiativeCall{
		Status:                   "open",
		HasTurnCycleStarted:      true,
		TurnOrderParticipantIDs:  []string{"a"},
		CurrentTurnParticipantID: "a",
		Participants: []model.InitiativeParticipant{
			ip("a", intp(18), "Aria"),
			ip("b", nil, "Bram"), // never rolled
		},
	}
	got := resortedOrder(c)
	want := []string{"a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestResort_NilCall_DoesNotPanic(t *testing.T) {
	Resort(nil) // must be safe; handler guards are elsewhere
}
