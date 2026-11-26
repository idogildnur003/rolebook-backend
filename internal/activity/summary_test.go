package activity

import (
	"testing"
	"time"

	"github.com/elad/rolebook-backend/internal/model"
)

var t0 = time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)

func entry(session string, minutes int, playerID, typ string, changes ...model.ActivityChange) model.ActivityEntry {
	return model.ActivityEntry{SessionID: session, PlayerID: playerID, Type: typ, Changes: changes, CreatedAt: t0.Add(time.Duration(minutes) * time.Minute)}
}

func chg(field string, before, after any) model.ActivityChange {
	return model.ActivityChange{Field: field, Before: before, After: after}
}

func TestBuckets_CurrentNameWins(t *testing.T) {
	e := entry("s1", 0, "p", "x")
	e.SessionName, e.SessionDate = "Old name", "2026-11-08"
	c := &model.Campaign{Sessions: []model.Session{{ID: "s1", Name: "New name"}}}
	got := BuildSessionBuckets([]model.ActivityEntry{e}, c, "p")
	if got[0].Name != "New name" || got[0].Date != "2026-11-08" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestBuckets_DeletedUsesSnapshot(t *testing.T) {
	e := entry("gone", 0, "p", "x")
	e.SessionName, e.SessionDate = "Session 9", "2026-10-01"
	got := BuildSessionBuckets([]model.ActivityEntry{e}, &model.Campaign{}, "p")
	if got[0].Name != "Session 9" || got[0].Date != "2026-10-01" || got[0].SessionID == nil {
		t.Fatalf("got %+v", got[0])
	}
}

func TestBuckets_BetweenHasNilID(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{entry("", 0, "p", "x")}, &model.Campaign{}, "p")
	if got[0].SessionID != nil || got[0].Name != "Between sessions" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestBuckets_NewestFirstAndCounts(t *testing.T) {
	dm := entry("s2", 9, "p", "x")
	dm.ActorRole = "dm"
	got := BuildSessionBuckets([]model.ActivityEntry{dm, entry("s2", 8, "p", "x"), entry("s1", 1, "p", "x")}, &model.Campaign{}, "p")
	if len(got) != 2 || *got[0].SessionID != "s2" || got[0].Count != 2 || got[0].DMCount != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestSummary_CoinsInGP(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 2, "p", "coins.adjust", chg("cp", 0, 100)),
		entry("s", 1, "p", "coins.adjust", chg("gp", 5, 6)),
	}, &model.Campaign{}, "p")
	if got[0].Summary.CoinsGP == nil || *got[0].Summary.CoinsGP != 2.0 {
		t.Fatalf("got %+v", got[0].Summary)
	}
}

func TestSummary_PCOnly(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{entry("s", 0, "pet", "player.update", chg("HP", 10, 4))}, &model.Campaign{}, "p")
	if got[0].Count != 1 || got[0].Summary.HPFirst != nil {
		t.Fatalf("got %+v", got[0])
	}
}

func TestSummary_HPFirstLast(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 2, "p", "player.update", chg("HP", 30, 21)),
		entry("s", 1, "p", "player.update", chg("HP", 38, 30)),
	}, &model.Campaign{}, "p")
	s := got[0].Summary
	if s.HPFirst == nil || *s.HPFirst != 38 || *s.HPLast != 21 {
		t.Fatalf("got %+v", s)
	}
}

func TestSummary_ItemsNet(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 2, "p", "inventory.delete", chg("qty", 1, nil)),
		entry("s", 1, "p", "inventory.create", chg("qty", nil, 2)),
	}, &model.Campaign{}, "p")
	if n := got[0].Summary.ItemsNet; n == nil || *n != 1 {
		t.Fatalf("got %v", n)
	}
}

func TestSummary_SlotsOnlyCountsSpend(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 2, "p", "spellslots.update", chg("L3 slots", 1, 2)),
		entry("s", 1, "p", "spellslots.update", chg("L3 slots", 2, 1)),
	}, &model.Campaign{}, "p")
	if n := got[0].Summary.SlotsUsed; n == nil || *n != 1 {
		t.Fatalf("got %v", n)
	}
}

func TestSummary_CastsCountAndMaxChangesDoNot(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 3, "p", "spell.cast", chg("L1 slots", 2, 1)),
		entry("s", 2, "p", "spellslots.update", chg("L2 max slots", 3, 1)),
		entry("s", 1, "p", "player.update", chg("L2 slots", 3, 1)),
	}, &model.Campaign{}, "p")
	if n := got[0].Summary.SlotsUsed; n == nil || *n != 1 {
		t.Fatalf("got %v", n)
	}
}

func TestSummary_MongoNumberTypes(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{
		entry("s", 3, "p", "player.update", chg("HP", int32(30), int64(20))),
		entry("s", 2, "p", "coins.adjust", chg("gp", float64(1), int32(3))),
	}, &model.Campaign{}, "p")
	s := got[0].Summary
	if s.HPLast == nil || *s.HPLast != 20 || s.CoinsGP == nil || *s.CoinsGP != 2 {
		t.Fatalf("got %+v", s)
	}
}

func TestSummary_DMHasCountsOnly(t *testing.T) {
	got := BuildSessionBuckets([]model.ActivityEntry{entry("s", 0, "npc", "player.update", chg("HP", 9, 1))}, &model.Campaign{}, "")
	s := got[0].Summary
	if got[0].Count != 1 || s.CoinsGP != nil || s.HPFirst != nil || s.ItemsNet != nil || s.SlotsUsed != nil {
		t.Fatalf("got %+v", got[0])
	}
}
