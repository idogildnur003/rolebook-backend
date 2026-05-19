package initiative

import (
	"sort"

	"github.com/elad/rolebook-backend/internal/model"
)

// SortParticipants orders by initiative descending, ties broken by name.
// Mirrors testApp sortInitiativeParticipants (nil initiative sorts last).
func SortParticipants(in []model.InitiativeParticipant) []model.InitiativeParticipant {
	out := make([]model.InitiativeParticipant, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		// -999 mirrors the testApp sortInitiativeParticipants sentinel:
		// participants with no roll (nil) sort to the end.
		li, ri := -999, -999
		if out[i].Initiative != nil {
			li = *out[i].Initiative
		}
		if out[j].Initiative != nil {
			ri = *out[j].Initiative
		}
		if li != ri {
			return li > ri
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func rotateIDsToFront(ids []string, activeID string) []string {
	if activeID == "" {
		return ids
	}
	idx := -1
	for i, id := range ids {
		if id == activeID {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ids
	}
	return append(append([]string{}, ids[idx:]...), ids[:idx]...)
}

// TurnOrderParticipantIDs returns the ordered participant ids for an open call.
// Mirrors testApp getInitiativeTurnOrderParticipantIds.
func TurnOrderParticipantIDs(c *model.InitiativeCall) []string {
	if c == nil || c.Status != "open" {
		return []string{}
	}
	submitted := make([]model.InitiativeParticipant, 0, len(c.Participants))
	for _, p := range c.Participants {
		if p.Initiative != nil {
			submitted = append(submitted, p)
		}
	}
	sortedIDs := make([]string, 0, len(submitted))
	for _, p := range SortParticipants(submitted) {
		sortedIDs = append(sortedIDs, p.ID)
	}
	if len(sortedIDs) == 0 {
		return []string{}
	}
	if !c.HasTurnCycleStarted {
		return sortedIDs
	}
	inSorted := make(map[string]bool, len(sortedIDs))
	for _, id := range sortedIDs {
		inSorted[id] = true
	}
	persisted := make([]string, 0, len(c.TurnOrderParticipantIDs))
	for _, id := range c.TurnOrderParticipantIDs {
		if inSorted[id] {
			persisted = append(persisted, id)
		}
	}
	inPersisted := make(map[string]bool, len(persisted))
	for _, id := range persisted {
		inPersisted[id] = true
	}
	missing := make([]string, 0)
	for _, id := range sortedIDs {
		if !inPersisted[id] {
			missing = append(missing, id)
		}
	}
	merged := sortedIDs
	if len(persisted) > 0 {
		merged = append(append([]string{}, persisted...), missing...)
	}
	return rotateIDsToFront(merged, c.CurrentTurnParticipantID)
}

// SyncTurnState recomputes turn order and current turn after a mutation.
// Mirrors testApp syncInitiativeCallTurnState.
func SyncTurnState(c *model.InitiativeCall) *model.InitiativeCall {
	if c == nil {
		return nil
	}
	order := TurnOrderParticipantIDs(c)
	c.TurnOrderParticipantIDs = order
	if len(order) > 0 {
		c.CurrentTurnParticipantID = order[0]
	} else {
		c.CurrentTurnParticipantID = ""
	}
	return c
}
