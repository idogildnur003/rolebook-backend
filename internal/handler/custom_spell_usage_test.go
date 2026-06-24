package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

func TestBuildCustomSpellUsage(t *testing.T) {
	spells := []model.CustomSpell{
		{ID: "custom-bolt-aaa", Name: "Bolt", CreatedBy: "u-1"},
		{ID: "custom-ward-bbb", Name: "Ward", CreatedBy: "u-ghost"},
	}
	players := []store.PlayerSpellSummary{
		{ID: "p-1", Name: "Aria", LinkedUserID: "u-1", Spells: []model.PlayerSpell{{SpellID: "custom-bolt-aaa"}}},
		{ID: "p-2", Name: "Bram", LinkedUserID: "u-2", Spells: []model.PlayerSpell{{SpellID: "custom-bolt-aaa"}}},
	}
	usage := buildCustomSpellUsage(spells, players)
	if len(usage) != 2 {
		t.Fatalf("len = %d, want 2", len(usage))
	}
	if usage[0].ID != "custom-bolt-aaa" || len(usage[0].Holders) != 2 {
		t.Fatalf("bolt holders = %+v, want 2", usage[0].Holders)
	}
	if usage[0].Holders[0].PlayerName != "Aria" {
		t.Errorf("first holder = %q, want Aria", usage[0].Holders[0].PlayerName)
	}
	if usage[0].CreatedByName != "Aria" {
		t.Errorf("bolt creator = %q, want Aria", usage[0].CreatedByName)
	}
	if usage[1].Holders == nil || len(usage[1].Holders) != 0 {
		t.Errorf("ward holders = %+v, want empty non-nil", usage[1].Holders)
	}
}
