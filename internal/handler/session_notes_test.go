package handler

import (
	"encoding/json"
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

func TestSessionNotesGetResponse_ShapeAndOmissions(t *testing.T) {
	resp := sessionNotesGetResponse{
		Notes: map[string]string{
			"s-1": "hello",
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"notes"`) {
		t.Errorf("response missing 'notes' key: %s", got)
	}
	if !strings.Contains(got, `"s-1":"hello"`) {
		t.Errorf("response missing note entry: %s", got)
	}
}

func TestSessionNotesGetResponse_EmptyNotesIsEmptyObject(t *testing.T) {
	resp := sessionNotesGetResponse{Notes: map[string]string{}}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if got != `{"notes":{}}` {
		t.Errorf("empty response = %s, want {\"notes\":{}}", got)
	}
}

func TestSessionNotesPutResponse_Shape(t *testing.T) {
	resp := sessionNotesPutResponse{SessionID: "s-1", Text: "ok"}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"sessionId":"s-1"`) {
		t.Errorf("missing sessionId: %s", got)
	}
	if !strings.Contains(got, `"text":"ok"`) {
		t.Errorf("missing text: %s", got)
	}
}

func TestNormalizeSessionNoteDirection(t *testing.T) {
	cases := map[string]string{
		"rtl":     "rtl",
		"ltr":     "ltr",
		"":        "ltr",
		"RTL":     "ltr",
		"garbage": "ltr",
	}
	for in, want := range cases {
		if got := normalizeSessionNoteDirection(in); got != want {
			t.Errorf("normalizeSessionNoteDirection(%q) = %q, want %q", in, got, want)
		}
	}
}
