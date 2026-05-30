package handler

import (
	"strings"
	"testing"
)

func TestValidateNewPassword_RejectsTooShort(t *testing.T) {
	for _, pw := range []string{"", "1234567", "short"} {
		if msg := validateNewPassword(pw); msg == "" {
			t.Errorf("validateNewPassword(%q) = \"\", want non-empty error", pw)
		}
	}
}

func TestValidateNewPassword_AcceptsAtMinimum(t *testing.T) {
	if msg := validateNewPassword("12345678"); msg != "" {
		t.Errorf("validateNewPassword(8 chars) = %q, want \"\"", msg)
	}
}

func TestValidateNewPassword_AcceptsLong(t *testing.T) {
	if msg := validateNewPassword(strings.Repeat("a", 50)); msg != "" {
		t.Errorf("validateNewPassword(50 chars) = %q, want \"\"", msg)
	}
}
