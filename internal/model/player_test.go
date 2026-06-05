package model

import (
	"encoding/json"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestExpertiseBonusRoundTrips guards against the regression where expertiseBonus
// was written to Mongo via the generic PATCH $set but silently dropped on read
// because the Player struct had no matching field. The PATCH handler turns the
// request body into a bson map and $sets it; reads decode bson back into Player;
// the API then serialises Player to JSON for the frontend. All three hops must
// preserve expertiseBonus under that exact key.
func TestExpertiseBonusRoundTrips(t *testing.T) {
	// PATCH path: arbitrary update map -> bson -> stored doc -> decoded Player.
	raw, err := bson.Marshal(bson.M{"expertiseBonus": 7})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var fromMongo Player
	if err := bson.Unmarshal(raw, &fromMongo); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}
	if fromMongo.ExpertiseBonus != 7 {
		t.Fatalf("ExpertiseBonus after bson round-trip = %d, want 7", fromMongo.ExpertiseBonus)
	}

	// Read path: Player -> JSON sent to the frontend, keyed "expertiseBonus".
	out, err := json.Marshal(Player{ExpertiseBonus: 7})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(out, &wire); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got, ok := wire["expertiseBonus"]; !ok || got.(float64) != 7 {
		t.Fatalf("JSON expertiseBonus = %v (present=%v), want 7", got, ok)
	}
}

// TestEquipmentAssignmentsRoundTrips mirrors the expertiseBonus guard for the
// nested equipmentAssignments field: a PATCH body must survive bson storage and
// decode back, and the read path must serialise it under the exact JSON keys
// the frontend equipment sections use.
func TestEquipmentAssignmentsRoundTrips(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"equipmentAssignments": bson.M{
			"armor":                   []string{"leather-armor"},
			"weaponsArrowsThrowables": []string{"dagger"},
			"accessoriesConsumables":  []string{},
			"preparedSpells":          []string{},
		},
	})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var fromMongo Player
	if err := bson.Unmarshal(raw, &fromMongo); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}
	if len(fromMongo.EquipmentAssignments.Armor) != 1 || fromMongo.EquipmentAssignments.Armor[0] != "leather-armor" {
		t.Fatalf("Armor after bson round-trip = %v, want [leather-armor]", fromMongo.EquipmentAssignments.Armor)
	}
	if len(fromMongo.EquipmentAssignments.WeaponsArrowsThrowables) != 1 {
		t.Fatalf("WeaponsArrowsThrowables = %v, want [dagger]", fromMongo.EquipmentAssignments.WeaponsArrowsThrowables)
	}

	out, err := json.Marshal(DefaultPlayer("p", "c", "u", "A", 1, PlayerKindPC))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(out, &wire); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	ea, ok := wire["equipmentAssignments"].(map[string]any)
	if !ok {
		t.Fatalf("equipmentAssignments missing or wrong type in JSON: %v", wire["equipmentAssignments"])
	}
	for _, key := range []string{"armor", "weaponsArrowsThrowables", "accessoriesConsumables", "preparedSpells"} {
		arr, ok := ea[key].([]any)
		if !ok {
			t.Fatalf("equipmentAssignments.%s = %v, want [] (non-nil array)", key, ea[key])
		}
		if len(arr) != 0 {
			t.Fatalf("equipmentAssignments.%s should default empty, got %v", key, arr)
		}
	}
}

func TestDefaultPlayerKindPC(t *testing.T) {
	p := DefaultPlayer("pid", "cid", "uid", "Alice", 1, PlayerKindPC)
	if p.Kind != string(PlayerKindPC) {
		t.Fatalf("Kind = %q, want %q", p.Kind, PlayerKindPC)
	}
	if p.ID != "pid" || p.CampaignID != "cid" || p.LinkedUserID != "uid" || p.Name != "Alice" {
		t.Fatalf("DefaultPlayer copied wrong fields: %+v", p)
	}
	if p.Level != 1 {
		t.Fatalf("Level = %d, want 1", p.Level)
	}
	if p.MaxHP != 10 || p.AC != 10 || p.Speed != 30 {
		t.Fatalf("expected D&D defaults; got HP=%d AC=%d Speed=%d", p.MaxHP, p.AC, p.Speed)
	}
}

func TestDefaultPlayerKindDM(t *testing.T) {
	p := DefaultPlayer("pid", "cid", "uid", "Bob", 0, PlayerKindDM)
	if p.Kind != string(PlayerKindDM) {
		t.Fatalf("Kind = %q, want %q", p.Kind, PlayerKindDM)
	}
	if p.Level != 0 {
		t.Fatalf("Level = %d, want 0 for DM stub", p.Level)
	}
}

func TestIsDMCreatableKind(t *testing.T) {
	cases := []struct {
		kind string
		want bool
	}{
		{"npc", true},
		{"enemy", true},
		{"pc", false},
		{"dm", false},
		{"", false},
		{"NPC", false},
	}
	for _, c := range cases {
		if got := IsDMCreatableKind(c.kind); got != c.want {
			t.Errorf("IsDMCreatableKind(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}
