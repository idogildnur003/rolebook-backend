package handler

import (
	"reflect"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestCoinsAdjustDesc(t *testing.T) {
	d := coinsAdjustDesc(model.Coins{GP: 105, SP: 3}, map[string]int{"gp": -15})
	want := []model.ActivityChange{{Field: "gp", Before: 120, After: 105}}
	if !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d.Changes)
	}
}

func TestCoinsConvertDesc(t *testing.T) {
	d := coinsConvertDesc(model.Coins{SP: 0, GP: 85}, "sp", "gp", 50, 5)
	want := []model.ActivityChange{
		{Field: "sp", Before: 50, After: 0},
		{Field: "gp", Before: 80, After: 85},
	}
	if !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d.Changes)
	}
}

func TestLimitedUseDesc(t *testing.T) {
	list := model.LimitedUseList{{ID: "a", Name: "Second Wind", Remaining: 1, Max: 1}}
	d := limitedUseDesc(list, "a", &model.LimitedUse{ID: "a", Name: "Second Wind", Remaining: 0, Max: 1})
	want := []model.ActivityChange{{Field: "uses", Before: 1, After: 0}}
	if d.Detail != "Second Wind" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func TestLimitedUseDesc_RemovedOnLastUse(t *testing.T) {
	list := model.LimitedUseList{{ID: "a", Name: "Wish", Remaining: 1, Max: 1}}
	d := limitedUseDesc(list, "a", nil)
	want := []model.ActivityChange{{Field: "uses", Before: 1, After: nil}}
	if d.Detail != "Wish" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("got %+v", d)
	}
}

func TestSpellDesc(t *testing.T) {
	spells := []model.PlayerSpell{{SpellID: "fb", Name: "Fireball", IsPrepared: false}}
	if d := spellDesc(spells, "fb", nil); d.Detail != "Fireball" || len(d.Changes) != 0 {
		t.Fatalf("delete: %+v", d)
	}
	d := spellDesc(spells, "fb", map[string]any{"isPrepared": true})
	want := []model.ActivityChange{{Field: "prepared", Before: false, After: true}}
	if d.Title != "Prepared spell" || !reflect.DeepEqual(d.Changes, want) {
		t.Fatalf("update: %+v", d)
	}
	if d := spellDesc(spells, "missing", nil); d.Detail != "" {
		t.Fatalf("missing: %+v", d)
	}
}

func TestInventoryDesc(t *testing.T) {
	item := model.PlayerInventoryItem{EquipmentID: "p", Name: "Potion of Healing", Quantity: 2}
	cases := []struct {
		name string
		d    func() []model.ActivityChange
		want model.ActivityChange
	}{
		{"create", func() []model.ActivityChange { return inventoryCreateDesc(item).Changes }, model.ActivityChange{Field: "qty", Before: nil, After: 2}},
		{"update", func() []model.ActivityChange {
			return inventoryUpdateDesc([]model.PlayerInventoryItem{item}, "p", map[string]any{"quantity": 3.0}).Changes
		}, model.ActivityChange{Field: "qty", Before: 2, After: 3}},
		{"delete", func() []model.ActivityChange {
			return inventoryDeleteDesc([]model.PlayerInventoryItem{item}, "p").Changes
		}, model.ActivityChange{Field: "qty", Before: 2, After: nil}},
	}
	for _, tc := range cases {
		got := tc.d()
		if len(got) != 1 || !reflect.DeepEqual(got[0], tc.want) {
			t.Fatalf("%s: got %+v", tc.name, got)
		}
	}
	if d := inventoryCreateDesc(item); d.Detail != "Potion of Healing" {
		t.Fatalf("detail: %+v", d)
	}
}
