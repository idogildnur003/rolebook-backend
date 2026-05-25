package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

func TestBuildCustomEquipmentUsage(t *testing.T) {
	items := []model.CustomEquipment{
		{ID: "custom-sword-aaa", Name: "Sword"},
		{ID: "custom-shield-bbb", Name: "Shield"},
	}
	players := []store.PlayerInventorySummary{
		{ID: "p-2", Name: "Bob", Inventory: []model.PlayerInventoryItem{
			{EquipmentID: "custom-sword-aaa", Quantity: 1},
		}},
		{ID: "p-1", Name: "Alice", Inventory: []model.PlayerInventoryItem{
			{EquipmentID: "custom-sword-aaa", Quantity: 2},
		}},
	}

	usage := buildCustomEquipmentUsage(items, players)

	if len(usage) != 2 {
		t.Fatalf("len(usage) = %d, want 2", len(usage))
	}

	sword := usage[0]
	if sword.ID != "custom-sword-aaa" {
		t.Fatalf("usage[0].ID = %q, want custom-sword-aaa", sword.ID)
	}
	if len(sword.Holders) != 2 {
		t.Fatalf("sword holders = %d, want 2", len(sword.Holders))
	}
	if sword.Holders[0].PlayerName != "Alice" || sword.Holders[0].Quantity != 2 {
		t.Fatalf("first holder = %+v, want Alice qty 2", sword.Holders[0])
	}
	if sword.Holders[1].PlayerName != "Bob" {
		t.Fatalf("second holder = %+v, want Bob", sword.Holders[1])
	}

	shield := usage[1]
	if shield.Holders == nil || len(shield.Holders) != 0 {
		t.Fatalf("shield holders = %+v, want empty non-nil slice", shield.Holders)
	}
}
