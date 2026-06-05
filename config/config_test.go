package config

import "testing"

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name     string
		flagVal  string
		envVal   string
		expected string
	}{
		{name: "flag wins over env", flagVal: "8080", envVal: "5000", expected: "8080"},
		{name: "env used when flag empty", flagVal: "", envVal: "5000", expected: "5000"},
		{name: "default when both empty", flagVal: "", envVal: "", expected: "3000"},
		{name: "flag used when env empty", flagVal: "9090", envVal: "", expected: "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePort(tt.flagVal, tt.envVal); got != tt.expected {
				t.Errorf("resolvePort(%q, %q) = %q, want %q", tt.flagVal, tt.envVal, got, tt.expected)
			}
		})
	}
}
