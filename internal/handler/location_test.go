package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestNormalizeStringSlice_NilAndEmptyReturnEmptyNotNil(t *testing.T) {
	if got := normalizeStringSlice(nil); got == nil || len(got) != 0 {
		t.Errorf("nil input → %#v, want empty non-nil slice", got)
	}
	if got := normalizeStringSlice([]string{}); got == nil || len(got) != 0 {
		t.Errorf("empty input → %#v, want empty non-nil slice", got)
	}
}

func TestNormalizeStringSlice_DedupesAndDropsEmpty(t *testing.T) {
	got := normalizeStringSlice([]string{"a", "", "b", "a", "c", "", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("length: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNormalizeStringSlice_PreservesFirstSeenOrder(t *testing.T) {
	got := normalizeStringSlice([]string{"c", "a", "b", "a"})
	want := []string{"c", "a", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order not preserved: got %v, want %v", got, want)
		}
	}
}

// Wire-shape contract: model.Location must not leak ownerUserId on the wire.
// The model already has json:"-" on OwnerUserID, but this test guards against
// regressions if someone adds a userId field in the future or removes the tag.
func TestLocation_OmitsOwnerUserIDOnWire(t *testing.T) {
	l := &model.Location{
		ID:            "loc-1",
		CampaignID:    "c-1",
		OwnerPlayerID: "p-1",
		OwnerUserID:   "u-secret",
		Name:          "Test Location",
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "u-secret") {
		t.Errorf("Location leaked ownerUserId: %s", got)
	}
	if strings.Contains(strings.ToLower(got), "owneruserid") {
		t.Errorf("Location emitted ownerUserId-shaped key: %s", got)
	}
	// Sanity: the ownerPlayerId-keyed identity *is* present.
	if !strings.Contains(got, "p-1") {
		t.Errorf("Location missing ownerPlayerId: %s", got)
	}
}

func TestNormalizeAnySlice_FiltersAndDedupes(t *testing.T) {
	got := normalizeAnySlice([]any{"a", "", "b", 42, "a", nil, "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("length: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestNormalizeAnySlice_EmptyInputReturnsEmptyNotNil(t *testing.T) {
	got := normalizeAnySlice(nil)
	if got == nil {
		t.Errorf("nil input → nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("nil input → %v, want empty", got)
	}
}
