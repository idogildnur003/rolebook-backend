package activity

import (
	"strings"

	"github.com/elad/rolebook-backend/internal/model"
)

// copperPerCoin is each denomination's value in copper.
var copperPerCoin = map[string]int{"cp": 1, "sp": 10, "ep": 50, "gp": 100, "pp": 1000}

// SessionSummary holds a session's net numbers for the caller's main character.
// A nil field means the session had no change of that kind.
type SessionSummary struct {
	CoinsGP   *float64 `json:"coinsGp"`
	HPFirst   *int     `json:"hpFirst"`
	HPLast    *int     `json:"hpLast"`
	ItemsNet  *int     `json:"itemsNet"`
	SlotsUsed *int     `json:"slotsUsed"`
}

// SessionBucket is one group in the History session list. A nil SessionID is
// the "Between sessions" group.
type SessionBucket struct {
	SessionID *string        `json:"sessionId"`
	Name      string         `json:"name"`
	Date      string         `json:"date"`
	Count     int            `json:"count"`
	DMCount   int            `json:"dmCount"`
	Summary   SessionSummary `json:"summary"`
}

type summaryTotals struct {
	copper, items, slots                int
	hasCoins, hasHP, hasItems, hasSlots bool
	hpFirst, hpLast                     int
}

// BuildSessionBuckets groups newest-first entries by session, keeping that
// order. Summary numbers count only pcPlayerID; "" leaves them empty.
func BuildSessionBuckets(entries []model.ActivityEntry, c *model.Campaign, pcPlayerID string) []SessionBucket {
	buckets := []SessionBucket{}
	index := map[string]int{}
	totals := map[string]*summaryTotals{}
	for i := range entries {
		e := &entries[i]
		at, ok := index[e.SessionID]
		if !ok {
			at = len(buckets)
			index[e.SessionID] = at
			totals[e.SessionID] = &summaryTotals{}
			buckets = append(buckets, newBucket(e, c))
		}
		buckets[at].Count++
		if e.ActorRole == string(model.RoleDM) {
			buckets[at].DMCount++
		}
		if pcPlayerID != "" && e.PlayerID == pcPlayerID {
			totals[e.SessionID].add(e)
		}
	}
	for key, at := range index {
		buckets[at].Summary = totals[key].summary()
	}
	return buckets
}

func newBucket(e *model.ActivityEntry, c *model.Campaign) SessionBucket {
	if e.SessionID == "" {
		return SessionBucket{Name: "Between sessions"}
	}
	id := e.SessionID
	b := SessionBucket{SessionID: &id, Name: e.SessionName, Date: e.SessionDate}
	if c == nil {
		return b
	}
	for i := range c.Sessions {
		s := &c.Sessions[i]
		if s.ID != id {
			continue
		}
		b.Name = s.Name
		if s.Schedule != nil && s.Schedule.ConfirmedSlot != nil {
			b.Date = s.Schedule.ConfirmedSlot.Date
		}
		break
	}
	return b
}

// add folds in one entry. Entries arrive newest first, so the latest HP is the
// first seen and the earliest HP keeps being overwritten.
func (t *summaryTotals) add(e *model.ActivityEntry) {
	for _, c := range e.Changes {
		before, hasBefore := toInt(c.Before)
		after, hasAfter := toInt(c.After)
		switch {
		case strings.HasPrefix(e.Type, "coins."):
			if worth, ok := copperPerCoin[c.Field]; ok && hasBefore && hasAfter {
				t.copper += (after - before) * worth
				t.hasCoins = true
			}
		case c.Field == "HP" && hasBefore && hasAfter:
			if !t.hasHP {
				t.hpLast = after
				t.hasHP = true
			}
			t.hpFirst = before
		case strings.HasPrefix(e.Type, "inventory.") && c.Field == "qty":
			t.items += after - before
			t.hasItems = true
		case (e.Type == "spellslots.update" || e.Type == "spell.cast") && !strings.HasSuffix(c.Field, " max slots") && hasBefore && hasAfter:
			t.hasSlots = true
			if before > after {
				t.slots += before - after
			}
		}
	}
}

func (t *summaryTotals) summary() SessionSummary {
	var s SessionSummary
	if t.hasCoins {
		gp := float64(t.copper) / 100
		s.CoinsGP = &gp
	}
	if t.hasHP {
		first, last := t.hpFirst, t.hpLast
		s.HPFirst, s.HPLast = &first, &last
	}
	if t.hasItems {
		items := t.items
		s.ItemsNet = &items
	}
	if t.hasSlots {
		slots := t.slots
		s.SlotsUsed = &slots
	}
	return s
}

// toInt normalizes a stored number; Mongo returns int32 or int64 for what was
// written as int. A missing value counts as 0 with ok false.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}
