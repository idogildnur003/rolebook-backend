package handler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/testutil"
)

// Every field a user can edit on these entries must show up in the history, so
// no "Updated …" entry is logged without saying what changed. Each case lists
// the fields its description leaves out on purpose.
func TestUpdateDescs_EveryEditableFieldIsDescribed(t *testing.T) {
	cases := []struct {
		name string
		zero any
		desc func(before, after any) activity.Desc
		skip map[string]bool
	}{
		{"campaign", model.Campaign{}, func(b, a any) activity.Desc {
			return campaignUpdateDesc(b.(*model.Campaign), a.(*model.Campaign))
		}, map[string]bool{"sessions": true, "members": true, "mapImageWidth": true, "mapImageHeight": true}},
		{"session", model.Session{}, func(b, a any) activity.Desc {
			return sessionUpdateDesc(&model.Campaign{Sessions: []model.Session{*b.(*model.Session)}}, a.(*model.Session))
		}, map[string]bool{"id": true, "schedule": true}},
		{"location", model.Location{}, func(b, a any) activity.Desc { return updateDesc("x", b, a) }, nil},
		{"npc", model.NPC{}, func(b, a any) activity.Desc { return updateDesc("x", b, a) }, nil},
		{"map pin", model.MapPin{}, func(b, a any) activity.Desc { return updateDesc("x", b, a) }, nil},
		{"custom spell", model.CustomSpell{}, func(b, a any) activity.Desc { return updateDesc("x", b, a) }, nil},
		{"custom equipment", model.CustomEquipment{}, func(b, a any) activity.Desc { return updateDesc("x", b, a) }, nil},
		{"content request", model.ContentRequest{}, func(b, a any) activity.Desc {
			return requestEditDesc(b.(*model.ContentRequest), a.(*model.ContentRequest))
		}, map[string]bool{"status": true, "resolvedAt": true, "resolvedByUserId": true, "resultId": true,
			"proposedByUserId": true, "proposedByName": true, "kind": true, "targetType": true}},
		{"limited use", model.LimitedUse{}, func(b, a any) activity.Desc {
			before := *b.(*model.LimitedUse)
			return limitedUseUpdateDesc(model.LimitedUseList{before}, before.ID, a.(*model.LimitedUse))
		}, map[string]bool{"id": true}},
	}
	server := map[string]bool{"id": true, "_id": true, "campaignId": true, "createdAt": true, "updatedAt": true,
		"createdBy": true, "ownerPlayerId": true, "ownerUserId": true, "clone": true}
	for _, c := range cases {
		typ := reflect.TypeOf(c.zero)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" || name == "" || server[name] || c.skip[name] {
				continue
			}
			before := reflect.New(typ)
			after := reflect.New(typ)
			if !testutil.Mutate(after.Elem().Field(i)) {
				t.Fatalf("%s: cannot mutate %s; extend testutil.Mutate", c.name, f.Name)
			}
			if d := c.desc(before.Interface(), after.Interface()); len(d.Changes) == 0 {
				t.Errorf("%s: changing %q logs no change", c.name, name)
			}
		}
	}
}

func TestLimitedUseUpdateDesc_NoteChangedButNotLogged(t *testing.T) {
	list := model.LimitedUseList{{ID: "a", Name: "Rage", Note: "private", Remaining: 2, Max: 2}}
	d := limitedUseUpdateDesc(list, "a", &model.LimitedUse{ID: "a", Name: "Rage", Note: "secret plan", Remaining: 2, Max: 3})
	want := []model.ActivityChange{{Field: "max", Before: 2, After: 3}, {Field: "note"}}
	if d.Detail != "Rage" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func TestSpellDesc_NoPreparedChangeHasNoChanges(t *testing.T) {
	spells := []model.PlayerSpell{{SpellID: "fb", Name: "Fireball", IsPrepared: true}}
	if d := spellDesc(spells, "fb", map[string]any{"isPrepared": true}); len(d.Changes) != 0 {
		t.Fatalf("got %+v", d)
	}
}

func TestInventoryUpdateDesc_Rename(t *testing.T) {
	items := []model.PlayerInventoryItem{{EquipmentID: "r", Name: "Rope", Quantity: 1}}
	d := inventoryUpdateDesc(items, "r", map[string]any{"name": "Silk Rope"})
	want := []model.ActivityChange{{Field: "name", Before: "Rope", After: "Silk Rope"}}
	if !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func TestSessionUpdateDesc(t *testing.T) {
	c := &model.Campaign{Sessions: []model.Session{{ID: "s1", Name: "Session 1"}}}
	d := sessionUpdateDesc(c, &model.Session{ID: "s1", Name: "The Heist"})
	want := []model.ActivityChange{{Field: "name", Before: "Session 1", After: "The Heist"}}
	if d.Detail != "The Heist" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func initiativeFixture() *model.InitiativeCall {
	return &model.InitiativeCall{Participants: []model.InitiativeParticipant{
		{ID: "player-p1", ParticipantType: "player", PlayerID: "p1", Name: "Thalia"},
		{ID: "e1", ParticipantType: "enemy", Name: "Goblin"},
	}}
}

func TestEndTurnDesc(t *testing.T) {
	c := initiativeFixture()
	d := endTurnDesc(c, "player-p1", "e1")
	want := []model.ActivityChange{{Field: "turn", Before: "Thalia", After: "Goblin"}}
	if d.Detail != "Thalia" || d.PlayerID != "p1" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
	if d := endTurnDesc(c, "e1", "player-p1"); d.PlayerID != "" || d.Detail != "Goblin" {
		t.Fatalf("enemy turn: %+v", d)
	}
}

func TestResortDesc(t *testing.T) {
	c := initiativeFixture()
	d := resortDesc(c, []string{"player-p1", "e1"}, []string{"e1", "player-p1"})
	want := []model.ActivityChange{{Field: "turn order", Before: "Thalia, Goblin", After: "Goblin, Thalia"}}
	if !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
	if d := resortDesc(c, []string{"e1"}, []string{"e1"}); len(d.Changes) != 0 {
		t.Fatalf("same order: %+v", d)
	}
}

func TestEnemyInitiativeDesc(t *testing.T) {
	d := enemyInitiativeDesc("Goblin", nil, 14)
	if d.Detail != "Goblin" || d.Title != "" || d.Changes[0].Before != nil || d.Changes[0].After != 14 {
		t.Fatalf("new: %+v", d)
	}
	prev := 9
	if d := enemyInitiativeDesc("Goblin", &prev, 14); d.Title != "Re-rolled enemy initiative" || d.Changes[0].Before != 9 {
		t.Fatalf("re-roll: %+v", d)
	}
}

func TestParticipantNamesCaps(t *testing.T) {
	c := &model.InitiativeCall{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		c.Participants = append(c.Participants, model.InitiativeParticipant{ID: n, Name: n})
	}
	if got := participantNames(c, nil); got != "a, b, c, d, e, f and 2 more" {
		t.Fatalf("got %q", got)
	}
}
