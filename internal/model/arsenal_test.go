package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSpell_EnrichedFieldsSerialize(t *testing.T) {
	s := Spell{
		ID:                 "spell-fireball",
		Name:               "Fireball",
		Level:              3,
		Classes:            []string{"Sorcerer", "Wizard"},
		Concentration:      false,
		Damage:             "8d6",
		DamageType:         "fire",
		SavingThrowAbility: "DEX",
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"classes":["Sorcerer","Wizard"]`, `"damage":"8d6"`, `"damageType":"fire"`, `"savingThrowAbility":"DEX"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestSpell_EmptyEnrichedFieldsOmitted(t *testing.T) {
	b, err := json.Marshal(Spell{ID: "spell-x", Name: "X", Level: 0})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, absent := range []string{"classes", "damage", "damageType", "savingThrowAbility", "concentration"} {
		if strings.Contains(got, absent) {
			t.Errorf("expected %q omitted, got %s", absent, got)
		}
	}
}
