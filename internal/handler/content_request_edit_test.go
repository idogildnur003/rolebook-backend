package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

// forbiddenPatchKeys are server-owned/immutable or DM-controlled fields that must
// never appear in an edit's content patch.
var forbiddenPatchKeys = []string{
	"_id", "campaignId", "createdBy", "createdAt", "updatedAt",
	"visibilityMode", "visiblePlayerIds",
}

func TestContentFieldsForEquipment_ContentOnly(t *testing.T) {
	weight := 3.5
	ac := 16
	item := &model.CustomEquipment{
		ID:               "custom-sword-abc123",
		CampaignID:       "camp-1",
		CreatedBy:        "u-1",
		Name:             "Flaming Sword",
		Category:         "weapon",
		Tags:             []string{"fire"},
		Notes:            "burns",
		Damage:           "1d8",
		DamageType:       "slashing",
		Weight:           &weight,
		ArmorClass:       &ac,
		VisibilityMode:   model.VisibilityPlayers,
		VisiblePlayerIDs: []string{"p-1"},
	}

	fields := contentFieldsForEquipment(item)

	for _, key := range []string{"name", "damage", "category"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("expected content key %q to be present", key)
		}
	}
	if got := fields["name"]; got != "Flaming Sword" {
		t.Errorf("name = %v, want Flaming Sword", got)
	}
	for _, key := range forbiddenPatchKeys {
		if _, ok := fields[key]; ok {
			t.Errorf("forbidden key %q must not be in equipment patch", key)
		}
	}
}

func TestContentFieldsForSpell_ContentOnly(t *testing.T) {
	spell := &model.CustomSpell{
		ID:               "custom-bolt-def456",
		CampaignID:       "camp-1",
		CreatedBy:        "u-1",
		Name:             "Fire Bolt",
		Level:            0,
		School:           "evocation",
		Components:       []string{"V", "S"},
		Damage:           "1d10",
		DamageType:       "fire",
		ImageURI:         "https://example.com/spell.png",
		VisibilityMode:   model.VisibilityPlayers,
		VisiblePlayerIDs: []string{"p-1"},
	}

	fields := contentFieldsForSpell(spell)

	for _, key := range []string{"name", "damage", "level", "imageUri"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("expected content key %q to be present", key)
		}
	}
	if got := fields["name"]; got != "Fire Bolt" {
		t.Errorf("name = %v, want Fire Bolt", got)
	}
	if got := fields["imageUri"]; got != "https://example.com/spell.png" {
		t.Errorf("imageUri = %v, want https://example.com/spell.png", got)
	}
	for _, key := range forbiddenPatchKeys {
		if _, ok := fields[key]; ok {
			t.Errorf("forbidden key %q must not be in spell patch", key)
		}
	}
}
