package handler

import (
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

// TestPlayerDecode_CaseVariantShadowsID documents why the PATCH stripping is
// case-insensitive: MongoDB stores "_ID" as a separate field, but the driver's
// struct decoder falls back to a lowercase match and lets it overwrite ID.
func TestPlayerDecode_CaseVariantShadowsID(t *testing.T) {
	raw, err := bson.Marshal(bson.D{{Key: "_id", Value: "real"}, {Key: "_ID", Value: "spoofed"}})
	if err != nil {
		t.Fatal(err)
	}
	var p model.Player
	if err := bson.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != "spoofed" {
		t.Skipf("driver no longer shadows _id with _ID (got %q); stripping is still defence in depth", p.ID)
	}
}

func TestStripProtectedPlayerFields_CaseVariants(t *testing.T) {
	req := map[string]any{
		"name":              "Aria",
		"abilityScores.STR": 14,
		"campaignId":        "x",
		"CAMPAIGNID":        "x",
		"linkedUserId":      "x",
		"LinkedUserId":      "x",
		"linkedUserId.foo":  "x",
		"_id":               "x",
		"_ID":               "x",
		"_Id.x":             "x",
		"id":                "x",
		"ID":                "x",
	}
	stripProtectedPlayerFields(req)

	got := make([]string, 0, len(req))
	for k := range req {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"abilityScores.STR", "name"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("remaining keys = %v, want %v", got, want)
	}
}

func TestHasOperatorKey(t *testing.T) {
	cases := map[string]bool{
		"name":               false,
		"abilityScores.STR":  false,
		"$where":             true,
		"spells.$[].name":    true,
		"inventory.$.amount": true,
		"cost$":              false,
	}
	for key, want := range cases {
		if got := hasOperatorKey(map[string]any{key: 1}); got != want {
			t.Errorf("hasOperatorKey(%q) = %v, want %v", key, got, want)
		}
	}
}
