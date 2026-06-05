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
