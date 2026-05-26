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
