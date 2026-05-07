package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestVisibilityAllowsRead_OwnerAlways(t *testing.T) {
	if !visibilityAllowsRead("p-1", model.Visibility{}, "p-1") {
		t.Error("owner must always be able to read")
	}
}

func TestVisibilityAllowsRead_NonOwnerWithoutShareDenied(t *testing.T) {
	v := model.Visibility{SharedPlayerIds: []string{}}
	if visibilityAllowsRead("p-1", v, "p-2") {
		t.Error("non-owner with no share must be denied")
	}
}

func TestVisibilityAllowsRead_SharedWithAll(t *testing.T) {
	v := model.Visibility{SharedWithAll: true}
	if !visibilityAllowsRead("p-1", v, "p-2") {
		t.Error("sharedWithAll must allow any caller to read")
	}
}

func TestVisibilityAllowsRead_SharedSpecific(t *testing.T) {
	v := model.Visibility{SharedPlayerIds: []string{"p-2", "p-3"}}
	if !visibilityAllowsRead("p-1", v, "p-3") {
		t.Error("listed player must be able to read")
	}
	if visibilityAllowsRead("p-1", v, "p-4") {
		t.Error("unlisted player must be denied")
	}
}

// Wire-shape contract: model.MapPin must not leak ownerUserId on the wire.
func TestMapPin_OmitsOwnerUserIDOnWire(t *testing.T) {
	p := &model.MapPin{
		ID:            "pin-1",
		CampaignID:    "c-1",
		OwnerPlayerID: "p-1",
		OwnerUserID:   "u-secret",
		Type:          model.MapPinLocation,
		EntityID:      "loc-1",
		X:             1.0,
		Y:             2.0,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "u-secret") {
		t.Errorf("MapPin leaked ownerUserId: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "owneruserid") {
		t.Errorf("MapPin emitted ownerUserId-shaped key: %s", got)
	}
	if !strings.Contains(got, "p-1") {
		t.Errorf("MapPin missing ownerPlayerId: %s", got)
	}
}
