package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeVisibilityMode(t *testing.T) {
	cases := map[string]string{
		"":         VisibilityCampaign,
		"campaign": VisibilityCampaign,
		"CAMPAIGN": VisibilityCampaign,
		" players": VisibilityPlayers,
		"players":  VisibilityPlayers,
		"bogus":    "bogus",
	}
	for in, want := range cases {
		if got := NormalizeVisibilityMode(in); got != want {
			t.Errorf("NormalizeVisibilityMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidVisibilityMode(t *testing.T) {
	if !IsValidVisibilityMode(VisibilityCampaign) || !IsValidVisibilityMode(VisibilityPlayers) {
		t.Error("campaign/players must be valid")
	}
	if IsValidVisibilityMode("bogus") || IsValidVisibilityMode("") {
		t.Error("bogus/empty must be invalid (caller normalizes first)")
	}
}

func TestCanPlayerSeeEntry(t *testing.T) {
	if !CanPlayerSeeEntry(VisibilityCampaign, nil, "p-1", false) {
		t.Error("campaign must be visible to any player")
	}
	if !CanPlayerSeeEntry("", nil, "p-stranger", false) {
		t.Error("empty mode is treated as campaign")
	}
	if !CanPlayerSeeEntry(VisibilityPlayers, []string{"p-1", "p-2"}, "p-1", false) {
		t.Error("listed player must see it")
	}
	if CanPlayerSeeEntry(VisibilityPlayers, []string{"p-1"}, "p-2", false) {
		t.Error("unlisted player must NOT see it")
	}
	if !CanPlayerSeeEntry(VisibilityPlayers, []string{"p-1"}, "p-2", true) {
		t.Error("DM sees everything")
	}
}

func TestAllowedPlayerIDsForCascade(t *testing.T) {
	allowed, run := AllowedPlayerIDsForCascade(VisibilityCampaign, []string{"p-1"})
	if run {
		t.Error("campaign visibility must not trigger a cascade (nobody loses access)")
	}
	if allowed != nil {
		t.Errorf("campaign allowed = %v, want nil", allowed)
	}
	allowed, run = AllowedPlayerIDsForCascade(VisibilityPlayers, []string{"p-1", "p-2"})
	if !run {
		t.Error("players visibility must trigger a cascade")
	}
	if !reflect.DeepEqual(allowed, []string{"p-1", "p-2"}) {
		t.Errorf("players allowed = %v, want [p-1 p-2]", allowed)
	}
	// players-mode with no listed ids: cascade runs with an empty allow-list,
	// i.e. the entry is pulled from everyone (DM locked all players out). This
	// documents the intended contract for that edge case.
	allowed, run = AllowedPlayerIDsForCascade(VisibilityPlayers, nil)
	if !run {
		t.Error("players visibility must trigger a cascade even with no listed players")
	}
	if len(allowed) != 0 {
		t.Errorf("players-with-none allowed = %v, want empty", allowed)
	}
}

func TestCustomEquipment_EmitsVisibilityOnWire(t *testing.T) {
	b, _ := json.Marshal(CustomEquipment{VisibilityMode: VisibilityPlayers, VisiblePlayerIDs: []string{"p-1"}})
	s := string(b)
	if !strings.Contains(s, `"visibilityMode":"players"`) || !strings.Contains(s, `"visiblePlayerIds":["p-1"]`) {
		t.Errorf("CustomEquipment must emit visibility fields: %s", s)
	}
}

func TestCustomSpell_EmitsVisibilityOnWire(t *testing.T) {
	b, _ := json.Marshal(CustomSpell{VisibilityMode: VisibilityPlayers, VisiblePlayerIDs: []string{"p-1"}})
	s := string(b)
	if !strings.Contains(s, `"visibilityMode":"players"`) || !strings.Contains(s, `"visiblePlayerIds":["p-1"]`) {
		t.Errorf("CustomSpell must emit visibility fields: %s", s)
	}
}
