package model

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIsCoinDenomination(t *testing.T) {
	for _, k := range []string{"cp", "sp", "ep", "gp", "pp"} {
		if !IsCoinDenomination(k) {
			t.Errorf("IsCoinDenomination(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"GP", "", "gold"} {
		if IsCoinDenomination(k) {
			t.Errorf("IsCoinDenomination(%q) = true, want false", k)
		}
	}
}

func TestConvertedAmount_Valid(t *testing.T) {
	tests := []struct {
		from, to string
		amount   int
		want     int
	}{
		{"pp", "gp", 1, 10},
		{"gp", "ep", 3, 6},
		{"cp", "gp", 100, 1},
		{"ep", "gp", 2, 1},
		{"gp", "cp", 7, 700},
		{"cp", "pp", 1000, 1},
	}
	for _, tt := range tests {
		got, err := ConvertedAmount(tt.from, tt.to, tt.amount)
		if err != nil {
			t.Errorf("ConvertedAmount(%s,%s,%d) error: %v", tt.from, tt.to, tt.amount, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ConvertedAmount(%s,%s,%d) = %d, want %d", tt.from, tt.to, tt.amount, got, tt.want)
		}
	}
}

func TestConvertedAmount_Uneven(t *testing.T) {
	tests := []struct {
		from, to string
		amount   int
	}{
		{"ep", "gp", 1},
		{"cp", "gp", 150},
		{"sp", "ep", 4},
	}
	for _, tt := range tests {
		_, err := ConvertedAmount(tt.from, tt.to, tt.amount)
		if !errors.Is(err, ErrUnevenConversion) {
			t.Errorf("ConvertedAmount(%s,%s,%d) err = %v, want ErrUnevenConversion", tt.from, tt.to, tt.amount, err)
		}
	}
}

func TestConvertedAmount_Invalid(t *testing.T) {
	tests := []struct {
		from, to string
		amount   int
	}{
		{"gp", "gp", 1},
		{"xx", "gp", 1},
		{"gp", "xx", 1},
		{"gp", "cp", 0},
		{"gp", "cp", -1},
		{"pp", "cp", MaxCoinAmount + 1},
	}
	for _, tt := range tests {
		_, err := ConvertedAmount(tt.from, tt.to, tt.amount)
		if !errors.Is(err, ErrInvalidConversion) {
			t.Errorf("ConvertedAmount(%s,%s,%d) err = %v, want ErrInvalidConversion", tt.from, tt.to, tt.amount, err)
		}
	}
}

func TestDefaultPlayer_ZeroCoins(t *testing.T) {
	p := DefaultPlayer("id", "camp", "user", "Name", 1, PlayerKindPC)
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `"coins":{"cp":0,"sp":0,"ep":0,"gp":0,"pp":0}`
	if !strings.Contains(string(b), want) {
		t.Errorf("player JSON missing %s", want)
	}
}
