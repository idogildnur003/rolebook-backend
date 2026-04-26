package model

import "testing"

func TestDefaultPlayerKindPC(t *testing.T) {
	p := DefaultPlayer("pid", "cid", "uid", "Alice", 1, PlayerKindPC)
	if p.Kind != string(PlayerKindPC) {
		t.Fatalf("Kind = %q, want %q", p.Kind, PlayerKindPC)
	}
	if p.ID != "pid" || p.CampaignID != "cid" || p.LinkedUserID != "uid" || p.Name != "Alice" {
		t.Fatalf("DefaultPlayer copied wrong fields: %+v", p)
	}
	if p.Level != 1 {
		t.Fatalf("Level = %d, want 1", p.Level)
	}
	if p.MaxHP != 10 || p.AC != 10 || p.Speed != 30 {
		t.Fatalf("expected D&D defaults; got HP=%d AC=%d Speed=%d", p.MaxHP, p.AC, p.Speed)
	}
}

func TestDefaultPlayerKindDM(t *testing.T) {
	p := DefaultPlayer("pid", "cid", "uid", "Bob", 0, PlayerKindDM)
	if p.Kind != string(PlayerKindDM) {
		t.Fatalf("Kind = %q, want %q", p.Kind, PlayerKindDM)
	}
	if p.Level != 0 {
		t.Fatalf("Level = %d, want 0 for DM stub", p.Level)
	}
}
