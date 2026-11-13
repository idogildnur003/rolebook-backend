package model

import (
	"encoding/json"
	"go.mongodb.org/mongo-driver/v2/bson"
	"reflect"
	"testing"
)

func TestDefaultPlayer_SheetNoteDefaults(t *testing.T) {
	p := DefaultPlayer("p1", "camp", "u1", "Kaelen", 1, PlayerKindPC)
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"hitDie", "spellcastingAbility"} {
		v, ok := m[k]
		if !ok || v != nil {
			t.Errorf("%s: want present and nil, got %v (present=%v)", k, v, ok)
		}
	}
	for _, k := range []string{"hitDiceMax", "hitDiceSpent"} {
		if m[k] != float64(0) {
			t.Errorf("%s: want 0, got %v", k, m[k])
		}
	}
	if !reflect.DeepEqual(m["armorTraining"], []any{}) {
		t.Errorf("armorTraining: want empty array, got %#v", m["armorTraining"])
	}
	for _, k := range []string{"weaponProficiencies", "toolProficiencies", "appearance"} {
		if m[k] != "" {
			t.Errorf("%s: want empty string, got %v", k, m[k])
		}
	}
	if _, ok := m["languages"]; ok {
		t.Error("languages should be omitted for a PC")
	}
}

func TestPlayer_BSONRoundTripSheetNotes(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"hitDiceMax":    float64(5),
		"hitDiceSpent":  float64(2),
		"hitDie":        "d8",
		"armorTraining": []string{"light"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var p Player
	if err := bson.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.HitDiceMax != 5 || p.HitDiceSpent != 2 {
		t.Errorf("hit dice: got max=%d spent=%d, want 5/2", p.HitDiceMax, p.HitDiceSpent)
	}
	if p.HitDie == nil || *p.HitDie != "d8" {
		t.Errorf("hitDie: got %v, want d8", p.HitDie)
	}
	if !reflect.DeepEqual(p.ArmorTraining, []string{"light"}) {
		t.Errorf("armorTraining: got %v", p.ArmorTraining)
	}
}
