package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

// Wire-shape contract: model.NPC must not leak ownerUserId on the wire.
func TestNPC_OmitsOwnerUserIDOnWire(t *testing.T) {
	n := &model.NPC{
		ID:            "npc-1",
		CampaignID:    "c-1",
		OwnerPlayerID: "p-1",
		OwnerUserID:   "u-secret",
		Name:          "Test NPC",
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "u-secret") {
		t.Errorf("NPC leaked ownerUserId: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "owneruserid") {
		t.Errorf("NPC emitted ownerUserId-shaped key: %s", got)
	}
	if !strings.Contains(got, "p-1") {
		t.Errorf("NPC missing ownerPlayerId: %s", got)
	}
}
