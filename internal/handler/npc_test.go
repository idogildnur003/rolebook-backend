package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
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

func TestNPCHandler_ResolveAvatars_PassesThroughWhenUnconfigured(t *testing.T) {
	// Unconfigured store → ResolveImageURI is identity.
	avatars := avatarstore.New(config.Config{})
	h := &NPCHandler{avatars: avatars}

	npcs := []model.NPC{
		{ID: "n1", AvatarURI: "campaigns/c1/npcs/abc.png"},
		{ID: "n2", AvatarURI: "https://cdn.example/npc.png"},
		{ID: "n3", AvatarURI: ""},
	}
	out := h.resolveAvatars(context.Background(), npcs)
	if out[0].AvatarURI != "campaigns/c1/npcs/abc.png" {
		t.Errorf("n1: got %q, want passthrough key (unconfigured store)", out[0].AvatarURI)
	}
	if out[1].AvatarURI != "https://cdn.example/npc.png" {
		t.Errorf("n2: got %q, want passthrough URL", out[1].AvatarURI)
	}
	if out[2].AvatarURI != "" {
		t.Errorf("n3: got %q, want empty", out[2].AvatarURI)
	}
}
