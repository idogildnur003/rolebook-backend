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

// The wire-shape contract for this refactor: NO userIds appear on the wire.
// Identity is by playerId. Caller-specific fields (myRole, myPlayerId) are
// computed per request from the JWT. These tests lock the contract by
// JSON-marshaling the wire types and asserting the result contains no
// userId-shaped tokens. They would have caught a regression where someone
// adds a UserID field with a default json tag, or removes the json:"-" tag
// from a model type that ends up being marshaled.

func TestCampaignMemberSummary_OmitsUserIDOnWire(t *testing.T) {
	s := campaignMemberSummary{
		PlayerID: "p-1",
		Role:     model.RolePlayer,
		IsActive: true,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(strings.ToLower(got), "userid") {
		t.Errorf("campaignMemberSummary leaked userId: %s", got)
	}
}

func TestCampaignDetail_OmitsCallerUserIDOnWire(t *testing.T) {
	c := &model.Campaign{
		ID:   "c-1",
		Name: "Test",
		Members: []model.CampaignMember{
			{UserID: "u-dm-secret", PlayerID: "p-dm", Role: model.RoleDM, IsActive: true},
			{UserID: "u-1-secret", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
		},
	}
	detail := toCampaignDetail(c, "u-dm-secret")
	b, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	// Neither the caller's userId nor any other member's userId should appear.
	if strings.Contains(got, "u-dm-secret") || strings.Contains(got, "u-1-secret") {
		t.Errorf("campaignDetail leaked a userId on the wire: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "userid") {
		t.Errorf("campaignDetail emitted a userId-shaped key: %s", got)
	}
	// Sanity: the playerId-keyed identity *is* present.
	if !strings.Contains(got, "p-dm") || !strings.Contains(got, "p-1") {
		t.Errorf("campaignDetail missing expected playerIds: %s", got)
	}
}

func TestToMemberSummaries_OmitsUserIDOnWire(t *testing.T) {
	in := []model.CampaignMember{
		{UserID: "u-secret", PlayerID: "p-1", Role: model.RolePlayer, IsActive: true},
	}
	out := toMemberSummaries(in)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "u-secret") {
		t.Errorf("toMemberSummaries leaked userId: %s", got)
	}
}

func TestToCampaignDetailWithImages_PassesThroughWhenUnconfigured(t *testing.T) {
	avatars := avatarstore.New(config.Config{})
	mapKey := "campaigns/c1/maps/abc.png"
	c := &model.Campaign{
		ID:          "c1",
		Name:        "Test",
		MapImageURI: &mapKey,
		Members: []model.CampaignMember{
			{UserID: "u1", PlayerID: "p1", Role: model.RoleDM, IsActive: true},
		},
	}
	d := toCampaignDetailWithImages(context.Background(), c, "u1", avatars)
	if d.MapImageURI == nil || *d.MapImageURI != "campaigns/c1/maps/abc.png" {
		t.Errorf("MapImageURI = %v, want passthrough key", d.MapImageURI)
	}
}

func TestToCampaignDetailWithImages_NilMapImage(t *testing.T) {
	avatars := avatarstore.New(config.Config{})
	c := &model.Campaign{ID: "c1", Name: "Test"}
	d := toCampaignDetailWithImages(context.Background(), c, "", avatars)
	if d.MapImageURI != nil {
		t.Errorf("MapImageURI = %v, want nil", d.MapImageURI)
	}
}

func TestCampaignListItem_IncludesMapImageURIWhenSet(t *testing.T) {
	mapKey := "campaigns/c1/maps/abc.png"
	item := campaignListItem{ID: "c1", Name: "Test", MapImageURI: &mapKey}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"mapImageUri":"campaigns/c1/maps/abc.png"`) {
		t.Errorf("campaignListItem missing mapImageUri: %s", got)
	}
}

func TestCampaignListItem_OmitsMapImageURIWhenNil(t *testing.T) {
	item := campaignListItem{ID: "c1", Name: "Test"}
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "mapImageUri") {
		t.Errorf("campaignListItem leaked nil mapImageUri: %s", b)
	}
}

func TestCampaignUpdateFields_AllowsMapImageDimensionsAsInts(t *testing.T) {
	fields := campaignUpdateFields(map[string]any{
		"name":           "New",
		"mapImageWidth":  float64(1024),
		"mapImageHeight": float64(768),
		"bogus":          "nope",
	})

	if fields["name"] != "New" {
		t.Errorf("name = %v, want New", fields["name"])
	}
	if _, ok := fields["bogus"]; ok {
		t.Errorf("bogus field was not filtered out: %v", fields)
	}
	w, ok := fields["mapImageWidth"].(int)
	if !ok || w != 1024 {
		t.Errorf("mapImageWidth = %v (%T), want int 1024", fields["mapImageWidth"], fields["mapImageWidth"])
	}
	h, ok := fields["mapImageHeight"].(int)
	if !ok || h != 768 {
		t.Errorf("mapImageHeight = %v (%T), want int 768", fields["mapImageHeight"], fields["mapImageHeight"])
	}
}

func TestCampaignUpdateFields_RejectsEmptyAndUnknown(t *testing.T) {
	fields := campaignUpdateFields(map[string]any{"userId": "hax", "role": "dm"})
	if len(fields) != 0 {
		t.Errorf("expected no allow-listed fields, got %v", fields)
	}
}

func TestCampaignDetail_IncludesMapImageDimensionsWhenSet(t *testing.T) {
	w, h := 1024, 768
	d := campaignDetail{ID: "c1", Name: "Test", MapImageWidth: &w, MapImageHeight: &h}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"mapImageWidth":1024`) || !strings.Contains(got, `"mapImageHeight":768`) {
		t.Errorf("campaignDetail missing map image dimensions: %s", got)
	}
}

func TestCampaignDetail_OmitsMapImageDimensionsWhenNil(t *testing.T) {
	d := campaignDetail{ID: "c1", Name: "Test"}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "mapImageWidth") || strings.Contains(string(b), "mapImageHeight") {
		t.Errorf("campaignDetail leaked nil map image dimensions: %s", b)
	}
}

func TestToCampaignDetail_CopiesMapImageDimensions(t *testing.T) {
	w, h := 800, 600
	c := &model.Campaign{
		ID:             "c1",
		Name:           "Test",
		MapImageWidth:  &w,
		MapImageHeight: &h,
		Members: []model.CampaignMember{
			{UserID: "u1", PlayerID: "p1", Role: model.RoleDM, IsActive: true},
		},
	}
	d := toCampaignDetail(c, "u1")
	if d.MapImageWidth == nil || *d.MapImageWidth != 800 {
		t.Errorf("MapImageWidth = %v, want 800", d.MapImageWidth)
	}
	if d.MapImageHeight == nil || *d.MapImageHeight != 600 {
		t.Errorf("MapImageHeight = %v, want 600", d.MapImageHeight)
	}
}
