package handler

import (
	"reflect"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
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
