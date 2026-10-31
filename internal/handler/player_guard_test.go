package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestStripProtectedPlayerFields_RemovesKindAndOwner(t *testing.T) {
	req := map[string]any{"kind": "dm", "ownerPlayerId": "x", "campaignId": "c", "linkedUserId": "u", "_id": "1", "id": "1", "name": "Ash"}
	stripProtectedPlayerFields(req)
	if len(req) != 1 || req["name"] != "Ash" {
		t.Fatalf("got %v", req)
	}
}

func TestCanDeletePlayer(t *testing.T) {
	pet := &model.Player{Kind: "companion", LinkedUserID: "u1"}
	pc := &model.Player{Kind: "pc", LinkedUserID: "u1"}
	cases := []struct {
		dm   bool
		p    *model.Player
		user string
		want bool
	}{
		{true, pc, "u9", true}, {false, pet, "u1", true}, {false, pet, "u2", false}, {false, pc, "u1", false},
	}
	for _, c := range cases {
		if got := canDeletePlayer(c.dm, c.p, c.user); got != c.want {
			t.Errorf("%+v → %v", c, got)
		}
	}
}

func TestCascadeOnDelete(t *testing.T) {
	if cascadeOnDelete(&model.Player{ID: "p1", Kind: "pc"}) != "p1" {
		t.Fatal("pc must cascade")
	}
	if cascadeOnDelete(&model.Player{ID: "c1", Kind: "companion"}) != "" {
		t.Fatal("companion must not cascade")
	}
	if cascadeOnDelete(&model.Player{ID: "n1", Kind: "npc"}) != "" {
		t.Fatal("npc must not cascade")
	}
}

func TestValidateCompanionFields(t *testing.T) {
	ok := []map[string]any{
		{"traits": []any{map[string]any{"name": "Pack Tactics", "description": "Adv."}}},
		{"actions": []any{map[string]any{"name": "Bite", "attackBonus": float64(4), "damage": "2d4+2", "description": ""}}},
		{"actions": []any{map[string]any{"name": "Howl", "attackBonus": nil, "damage": "", "description": "scare"}}},
		{"name": "Ash"},
	}
	bad := []map[string]any{
		{"traits": "Pack Tactics"},
		{"traits": []any{map[string]any{"name": 5, "description": ""}}},
		{"traits": []any{map[string]any{"name": "", "description": "x"}}},
		{"actions": []any{map[string]any{"name": "Bite", "attackBonus": 4.5, "damage": "", "description": ""}}},
		{"senses": 13},
	}
	for _, r := range ok {
		if err := validateCompanionFields(r); err != nil {
			t.Errorf("%v: %v", r, err)
		}
	}
	for _, r := range bad {
		if validateCompanionFields(r) == nil {
			t.Errorf("%v: want error", r)
		}
	}
}
