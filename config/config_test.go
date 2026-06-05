package config

import (
	"reflect"
	"testing"
)

func TestParseCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,, c ", []string{"a", "b", "c"}},
		{",,", nil},
		{" , , ", nil},
	}
	for _, tc := range cases {
		got := parseCSV(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseCSV(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"false", false},
		{"0", false},
		{"true", true},
		{"TRUE", true},
		{" True ", true},
		{"1", true},
		{"yes", true},
		{"on", true},
	}
	for _, tc := range cases {
		if got := envBool(tc.in); got != tc.want {
			t.Errorf("envBool(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

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
