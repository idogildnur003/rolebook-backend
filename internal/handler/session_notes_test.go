package handler

import (
	"strings"
	"testing"
)

func TestNormalizeSessionNoteText_TrimsAndKeepsContent(t *testing.T) {
	text, cleared, err := normalizeSessionNoteText("  hello world  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleared {
		t.Errorf("cleared = true, want false")
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
}

func TestNormalizeSessionNoteText_EmptyIsCleared(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n  "} {
		text, cleared, err := normalizeSessionNoteText(in)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", in, err)
		}
		if !cleared {
			t.Errorf("input %q: cleared = false, want true", in)
		}
		if text != "" {
			t.Errorf("input %q: text = %q, want empty", in, text)
		}
	}
}

func TestNormalizeSessionNoteText_RejectsOverLimit(t *testing.T) {
	huge := strings.Repeat("a", 10001)
	_, _, err := normalizeSessionNoteText(huge)
	if err == nil {
		t.Errorf("expected error for input of length 10001, got nil")
	}
}

func TestNormalizeSessionNoteText_AcceptsAtLimit(t *testing.T) {
	atLimit := strings.Repeat("a", 10000)
	text, cleared, err := normalizeSessionNoteText(atLimit)
	if err != nil {
		t.Fatalf("unexpected error at limit: %v", err)
	}
	if cleared {
		t.Errorf("cleared = true at limit, want false")
	}
	if len(text) != 10000 {
		t.Errorf("text length = %d, want 10000", len(text))
	}
}
