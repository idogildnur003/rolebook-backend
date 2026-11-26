package handler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/testutil"
)

func TestPlayerUpdateDesc_HPCategory(t *testing.T) {
	d := playerUpdateDesc(&model.Player{CurrentHP: 38}, &model.Player{CurrentHP: 21})
	if d.Category != model.ActivityCategoryHP || d.Title != "HP changed" || len(d.Changes) != 1 {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerUpdateDesc_Mixed(t *testing.T) {
	d := playerUpdateDesc(&model.Player{CurrentHP: 38, AC: 14}, &model.Player{CurrentHP: 21, AC: 15})
	if d.Category != model.ActivityCategoryCharacter || d.Title != "" {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerDeleteDesc_CapturesIdentity(t *testing.T) {
	d := playerDeleteDesc(&model.Player{ID: "p1", CampaignID: "c1", LinkedUserID: "u1", Name: "Thalia"})
	if d.SubjectUserID != "u1" || d.PlayerName != "Thalia" || d.PlayerID != "p1" || d.CampaignID != "c1" || d.Detail != "Thalia" {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerDeleteDesc_CompanionOwner(t *testing.T) {
	d := playerDeleteDesc(&model.Player{ID: "pet", CampaignID: "c1", LinkedUserID: "owner", OwnerPlayerID: "p1", Kind: "companion", Name: "Biscuit"})
	if d.SubjectUserID != "owner" || d.PlayerName != "Biscuit" {
		t.Fatalf("got %+v", d)
	}
}

func TestSpellSlotDesc(t *testing.T) {
	d := spellSlotDesc(
		map[string]model.SpellSlot{"3": {Max: 3, Used: 1}, "1": {Max: 4, Used: 0}},
		map[string]model.SpellSlot{"3": {Max: 3, Used: 2}, "1": {Max: 4, Used: 0}},
	)
	want := []model.ActivityChange{{Field: "L3 slots", Before: 2, After: 1}}
	if !reflect.DeepEqual(d.Changes, want) || d.Title != "Used spell slot" {
		t.Fatalf("got %+v", d)
	}
}

func TestSpellSlotDesc_Restored(t *testing.T) {
	d := spellSlotDesc(map[string]model.SpellSlot{"2": {Max: 3, Used: 2}}, map[string]model.SpellSlot{"2": {Max: 3, Used: 0}})
	if d.Title != "Restored spell slots" {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerUpdateDesc_SlotsOnlyIsSpellSlotEntry(t *testing.T) {
	d := playerUpdateDesc(
		&model.Player{SpellSlots: map[string]model.SpellSlot{"1": {Max: 4, Used: 1}}},
		&model.Player{SpellSlots: map[string]model.SpellSlot{"1": {Max: 4, Used: 2}}},
	)
	want := []model.ActivityChange{{Field: "L1 slots", Before: 3, After: 2}}
	if d.Type != "spellslots.update" || d.Category != model.ActivityCategorySpells || d.Title != "Used spell slot" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func TestPlayerUpdateDesc_NoChangeHasNoChanges(t *testing.T) {
	p := model.DefaultPlayer("p1", "c1", "u1", "Thalia", 1, model.PlayerKindPC)
	q := *p
	if d := playerUpdateDesc(p, &q); len(d.Changes) != 0 {
		t.Fatalf("got %+v", d)
	}
}

// Every field a PATCH can set must produce a change, so no edit is logged as a
// bare "Character updated". Fields the PATCH strips are exempt.
func TestPlayerUpdateDesc_EveryPatchableFieldIsDescribed(t *testing.T) {
	exempt := map[string]bool{"updatedat": true}
	for k := range protectedPlayerKeys {
		exempt[k] = true
	}
	typ := reflect.TypeOf(model.Player{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := strings.ToLower(strings.Split(f.Tag.Get("bson"), ",")[0])
		if exempt[tag] {
			continue
		}
		before := model.DefaultPlayer("p1", "c1", "u1", "Thalia", 1, model.PlayerKindPC)
		after := *before
		v := reflect.ValueOf(&after).Elem().Field(i)
		if !testutil.Mutate(v) {
			t.Fatalf("test cannot mutate field %s (%s); extend mutate", f.Name, f.Type)
		}
		if d := playerUpdateDesc(before, &after); len(d.Changes) == 0 {
			t.Errorf("changing %s (%q) logs no change", f.Name, tag)
		}
	}
}

func TestSpellCastDesc(t *testing.T) {
	d := spellCastDesc("Fireball", 3, map[string]model.SpellSlot{"3": {Max: 3, Used: 2}})
	want := []model.ActivityChange{{Field: "L3 slots", Before: 2, After: 1}}
	if d.Detail != "Fireball" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
	if d := spellCastDesc("Fire Bolt", 0, nil); d.Detail != "Fire Bolt" || len(d.Changes) != 0 {
		t.Fatalf("cantrip: %+v", d)
	}
}

func TestCastLevels(t *testing.T) {
	n := func(v int) *int { return &v }
	cases := []struct {
		spell   int
		slot    *int
		want    int
		wantErr bool
	}{
		{3, nil, 3, false},
		{0, nil, 0, false},
		{3, n(5), 5, false},
		{3, n(2), 0, true},
		{3, n(10), 0, true},
		{0, n(1), 0, true},
	}
	for _, c := range cases {
		got, err := castLevels(c.spell, c.slot)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("castLevels(%d, %v) = %d, %v", c.spell, c.slot, got, err)
		}
	}
}
