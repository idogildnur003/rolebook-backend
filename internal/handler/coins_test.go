package handler

import (
	"reflect"
	"testing"
)

func TestParseCoinDeltas_Valid(t *testing.T) {
	got, err := parseCoinDeltas(map[string]any{"gp": 5.0, "sp": -3.0, "cp": 0.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]int{"gp": 5, "sp": -3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseCoinDeltas_RejectsUnknownKey(t *testing.T) {
	for _, body := range []map[string]any{{"GP": 1.0}, {"gold": 1.0}} {
		if _, err := parseCoinDeltas(body); err == nil {
			t.Errorf("expected error for %v", body)
		}
	}
}

func TestParseCoinDeltas_RejectsNonInteger(t *testing.T) {
	for _, body := range []map[string]any{{"gp": 1.5}, {"gp": "5"}, {"gp": true}, {"gp": nil}} {
		if _, err := parseCoinDeltas(body); err == nil {
			t.Errorf("expected error for %v", body)
		}
	}
}

func TestParseCoinDeltas_RejectsOutOfRange(t *testing.T) {
	for _, body := range []map[string]any{{"pp": 1e9 + 1}, {"pp": -9e18}} {
		if _, err := parseCoinDeltas(body); err == nil {
			t.Errorf("expected error for %v", body)
		}
	}
}

func TestParseCoinDeltas_RejectsEmpty(t *testing.T) {
	for _, body := range []map[string]any{{}, {"gp": 0.0}} {
		if _, err := parseCoinDeltas(body); err == nil {
			t.Errorf("expected error for %v", body)
		}
	}
}

func TestStripProtectedPlayerFields(t *testing.T) {
	req := map[string]any{
		"campaignId":   "c",
		"linkedUserId": "u",
		"_id":          "x",
		"id":           "y",
		"coins":        map[string]any{"gp": 999.0},
		"coins.gp":     999.0,
		"name":         "x",
	}
	stripProtectedPlayerFields(req)
	if len(req) != 1 || req["name"] != "x" {
		t.Errorf("expected only name to remain, got %v", req)
	}
}
