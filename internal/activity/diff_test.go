package activity

import (
	"reflect"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestDiffCoins(t *testing.T) {
	got := DiffCoins(model.Coins{GP: 120, SP: 5}, model.Coins{GP: 225, SP: 5})
	want := []model.ActivityChange{{Field: "gp", Before: 120, After: 225}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_LabelsHP(t *testing.T) {
	got := DiffPlayerPatch(&model.Player{CurrentHP: 38, MaxHP: 38}, &model.Player{CurrentHP: 21, MaxHP: 42})
	want := []model.ActivityChange{
		{Field: "HP", Before: 38, After: 21},
		{Field: "max HP", Before: 38, After: 42},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_AbilityScore(t *testing.T) {
	got := DiffPlayerPatch(
		&model.Player{AbilityScores: map[string]int{"STR": 14, "DEX": 12}},
		&model.Player{AbilityScores: map[string]int{"STR": 16, "DEX": 12}},
	)
	want := []model.ActivityChange{{Field: "STR", Before: 14, After: 16}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_NoChange(t *testing.T) {
	p := &model.Player{CurrentHP: 10, Name: "Thalia"}
	if got := DiffPlayerPatch(p, p); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_IgnoresFreeText(t *testing.T) {
	got := DiffPlayerPatch(&model.Player{Appearance: "secret old"}, &model.Player{Appearance: "secret new"})
	want := []model.ActivityChange{{Field: "appearance"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_Conditions(t *testing.T) {
	got := DiffPlayerPatch(
		&model.Player{Conditions: map[string]bool{"prone": true}},
		&model.Player{Conditions: map[string]bool{"poisoned": true}},
	)
	want := []model.ActivityChange{
		{Field: "poisoned", Before: false, After: true},
		{Field: "prone", Before: true, After: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_XPAndBonuses(t *testing.T) {
	got := DiffPlayerPatch(
		&model.Player{ExperiencePoints: 100, HitDiceMax: 4, ProficiencyBonus: 2, InitiativeBonus: 1},
		&model.Player{ExperiencePoints: 400, HitDiceMax: 5, ProficiencyBonus: 3, InitiativeBonus: 2},
	)
	want := []model.ActivityChange{
		{Field: "XP", Before: 100, After: 400},
		{Field: "hit dice max", Before: 4, After: 5},
		{Field: "proficiency bonus", Before: 2, After: 3},
		{Field: "initiative bonus", Before: 1, After: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_ShortTextFields(t *testing.T) {
	wizard := "Wizard"
	got := DiffPlayerPatch(&model.Player{Race: "Elf"}, &model.Player{Race: "Human", ClassName: &wizard})
	want := []model.ActivityChange{
		{Field: "class", Before: nil, After: "Wizard"},
		{Field: "race", Before: "Elf", After: "Human"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffPlayerPatch_ListsRecordChangedOnly(t *testing.T) {
	got := DiffPlayerPatch(
		&model.Player{ProficientSkills: []string{"Arcana"}, FeaturesAndFeats: []string{"Alert"}},
		&model.Player{ProficientSkills: []string{"Arcana", "History"}, FeaturesAndFeats: []string{"Alert"}},
	)
	want := []model.ActivityChange{{Field: "skill proficiencies"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
