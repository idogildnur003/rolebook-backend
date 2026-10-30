package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultCompanion_SetsKindOwnerAndType(t *testing.T) {
	p := DefaultCompanion("c1", "camp", "u1", "p1", "Ash", "Wolf")
	if p.Kind != string(PlayerKindCompanion) || p.OwnerPlayerID != "p1" || p.LinkedUserID != "u1" || p.CreatureType != "Wolf" || p.Name != "Ash" {
		t.Fatalf("got %+v", p)
	}
	if p.Traits == nil || p.Actions == nil {
		t.Fatal("traits/actions must be non-nil")
	}
}

func TestPCJSON_OmitsCompanionFields(t *testing.T) {
	b, _ := json.Marshal(DefaultPlayer("p1", "camp", "u1", "Kaelen", 1, PlayerKindPC))
	for _, k := range []string{"ownerPlayerId", "creatureType", "otherSpeeds", "senses", "languages", "traits", "actions"} {
		if strings.Contains(string(b), `"`+k+`"`) {
			t.Errorf("PC JSON contains %q", k)
		}
	}
}

func TestIsDMCreatableKind_RejectsCompanion(t *testing.T) {
	if IsDMCreatableKind("companion") {
		t.Fatal("companion must not be creatable via the NPC endpoint")
	}
}

func TestCompanionAction_OmitsNilAttackBonus(t *testing.T) {
	b, _ := json.Marshal(CompanionAction{Name: "Howl"})
	if strings.Contains(string(b), "attackBonus") {
		t.Fatalf("got %s", b)
	}
}
