package handler

import (
	"strings"
	"testing"
)

func TestValidateSheetNoteFields(t *testing.T) {
	r := func(n int, s string) string { return strings.Repeat(s, n) }
	tests := []struct {
		name    string
		req     map[string]any
		stored  int
		wantErr bool
	}{
		{"empty", map[string]any{}, 0, false},
		{"hitDie ok", map[string]any{"hitDie": "d10"}, 0, false},
		{"hitDie null", map[string]any{"hitDie": nil}, 0, false},
		{"hitDiceMax 20", map[string]any{"hitDiceMax": 20.0}, 0, false},
		{"max and spent equal", map[string]any{"hitDiceMax": 5.0, "hitDiceSpent": 5.0}, 0, false},
		{"spent within stored", map[string]any{"hitDiceSpent": 2.0}, 3, false},
		{"ability WIS", map[string]any{"spellcastingAbility": "WIS"}, 0, false},
		{"ability null", map[string]any{"spellcastingAbility": nil}, 0, false},
		{"armor empty", map[string]any{"armorTraining": []any{}}, 0, false},
		{"armor two", map[string]any{"armorTraining": []any{"light", "shields"}}, 0, false},
		{"weapon 200", map[string]any{"weaponProficiencies": r(200, "a")}, 0, false},
		{"languages 200 runes", map[string]any{"languages": r(200, "é")}, 0, false},
		{"appearance 2000", map[string]any{"appearance": r(2000, "a")}, 0, false},

		{"hitDie d20", map[string]any{"hitDie": "d20"}, 0, true},
		{"hitDie number", map[string]any{"hitDie": 6.0}, 0, true},
		{"hitDiceMax 21", map[string]any{"hitDiceMax": 21.0}, 0, true},
		{"hitDiceMax -1", map[string]any{"hitDiceMax": -1.0}, 0, true},
		{"hitDiceMax 2.5", map[string]any{"hitDiceMax": 2.5}, 0, true},
		{"spent over stored", map[string]any{"hitDiceSpent": 4.0}, 3, true},
		{"spent over request max", map[string]any{"hitDiceMax": 2.0, "hitDiceSpent": 3.0}, 10, true},
		{"spent fractional", map[string]any{"hitDiceSpent": 1.5}, 3, true},
		{"ability STR", map[string]any{"spellcastingAbility": "STR"}, 0, true},
		{"armor string", map[string]any{"armorTraining": "light"}, 0, true},
		{"armor dup", map[string]any{"armorTraining": []any{"light", "light"}}, 0, true},
		{"armor unknown", map[string]any{"armorTraining": []any{"plate"}}, 0, true},
		{"tool 201", map[string]any{"toolProficiencies": r(201, "a")}, 0, true},
		{"languages 201", map[string]any{"languages": r(201, "a")}, 0, true},
		{"appearance 2001", map[string]any{"appearance": r(2001, "a")}, 0, true},
		{"appearance number", map[string]any{"appearance": 7.0}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSheetNoteFields(tt.req, tt.stored)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
