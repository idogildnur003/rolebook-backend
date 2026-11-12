package model

import (
	"encoding/json"
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
