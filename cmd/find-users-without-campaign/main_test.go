package main

import (
	"reflect"
	"testing"
)

func TestParseSelection(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		n           int
		wantSel     []int
		wantIgnored []string
	}{
		{"single", "3", 5, []int{3}, nil},
		{"list", "1,3,5", 5, []int{1, 3, 5}, nil},
		{"range", "2-4", 5, []int{2, 3, 4}, nil},
		{"mixed with spaces", " 1 , 3-4 ", 5, []int{1, 3, 4}, nil},
		{"all", "all", 3, []int{1, 2, 3}, nil},
		{"dedup overlapping", "2,2-3,3", 5, []int{2, 3}, nil},
		{"out of range dropped", "0,3,9", 5, []int{3}, []string{"0", "9"}},
		{"non-numeric dropped", "abc,2", 5, []int{2}, []string{"abc"}},
		{"reversed range dropped", "4-2", 5, nil, []string{"4-2"}},
		{"range past n dropped", "1-9", 5, nil, []string{"1-9"}},
		{"empty tokens ignored", " , ,2", 5, []int{2}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSel, gotIgnored := parseSelection(tc.input, tc.n)
			if !reflect.DeepEqual(gotSel, tc.wantSel) {
				t.Errorf("selected = %#v, want %#v", gotSel, tc.wantSel)
			}
			if !reflect.DeepEqual(gotIgnored, tc.wantIgnored) {
				t.Errorf("ignored = %#v, want %#v", gotIgnored, tc.wantIgnored)
			}
		})
	}
}
