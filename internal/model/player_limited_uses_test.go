package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLimitedUseList_MarshalNilAsEmpty(t *testing.T) {
	b, err := json.Marshal(Player{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"limitedUses":[]`) {
		t.Errorf("expected limitedUses to marshal as [], got %s", b)
	}
}

func TestDefaultPlayer_LimitedUsesEmpty(t *testing.T) {
	p := DefaultPlayer("id", "c", "u", "Name", 1, PlayerKindPC)
	if p.LimitedUses == nil || len(p.LimitedUses) != 0 {
		t.Errorf("expected non-nil empty LimitedUses, got %#v", p.LimitedUses)
	}
}
