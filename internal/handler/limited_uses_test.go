package handler

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/elad/rolebook-backend/internal/store"
)

func TestParseLimitedUseCreate_Valid(t *testing.T) {
	got, err := parseLimitedUseCreate(map[string]any{
		"name": "  Bardic Inspiration ", "max": 3.0, "resets": true, "note": " d8 ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Bardic Inspiration" || got.Note != "d8" || got.Max != 3 || got.Remaining != 3 || !got.Resets || got.ID != "" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestParseLimitedUseCreate_Rejects(t *testing.T) {
	ok := func() map[string]any {
		return map[string]any{"name": "X", "max": 3.0, "resets": true}
	}
	with := func(k string, v any) map[string]any {
		m := ok()
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	cases := map[string]map[string]any{
		"empty name":      with("name", ""),
		"blank name":      with("name", "   "),
		"long name":       with("name", strings.Repeat("a", 61)),
		"missing name":    with("name", nil),
		"long note":       with("note", strings.Repeat("a", 201)),
		"non-string note": with("note", 5.0),
		"max 0":           with("max", 0.0),
		"max 100":         with("max", 100.0),
		"max 1.5":         with("max", 1.5),
		"max string":      with("max", "3"),
		"max missing":     with("max", nil),
		"resets string":   with("resets", "yes"),
		"resets missing":  with("resets", nil),
	}
	for name, body := range cases {
		if _, err := parseLimitedUseCreate(body); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseLimitedUseCreate_BoundariesAccepted(t *testing.T) {
	_, err := parseLimitedUseCreate(map[string]any{
		"name": strings.Repeat("a", 60), "note": strings.Repeat("b", 200), "max": 99.0, "resets": false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLimitedUsePatch(t *testing.T) {
	if _, err := parseLimitedUsePatch(map[string]any{}); err == nil {
		t.Error("empty body should error")
	}
	if _, err := parseLimitedUsePatch(map[string]any{"foo": 1.0}); err == nil {
		t.Error("unknown keys alone should error")
	}
	p, err := parseLimitedUsePatch(map[string]any{"max": 2.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Max == nil || *p.Max != 2 || p.Name != nil || p.Note != nil || p.Resets != nil {
		t.Errorf("unexpected patch: %+v", p)
	}
	p, err = parseLimitedUsePatch(map[string]any{"name": " N ", "note": "", "resets": false, "ignored": 1.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *p.Name != "N" || *p.Note != "" || *p.Resets != false {
		t.Errorf("unexpected patch: %+v", p)
	}
	bad := []map[string]any{
		{"name": ""}, {"name": strings.Repeat("a", 61)}, {"name": 1.0},
		{"note": strings.Repeat("a", 201)}, {"max": 0.0}, {"max": 100.0}, {"max": 1.5}, {"max": "3"},
		{"resets": "yes"}, {"resets": nil},
	}
	for _, b := range bad {
		if _, err := parseLimitedUsePatch(b); err == nil {
			t.Errorf("expected error for %v", b)
		}
	}
}

func TestWriteLimitedUseStoreError(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{store.ErrLimitedUseNotFound, 404, "LIMITED_USE_NOT_FOUND"},
		{store.ErrNoUsesLeft, 409, "NO_USES_LEFT"},
		{store.ErrAlreadyFull, 409, "ALREADY_FULL"},
		{store.ErrNotResettable, 400, "NOT_RESETTABLE"},
		{store.ErrLimitReached, 400, "LIMIT_REACHED"},
		{store.ErrLimitedUseChanged, 409, "LIMITED_USE_CHANGED"},
		{fmt.Errorf("player x: %w", mongo.ErrNoDocuments), 404, "NOT_FOUND"},
		{errors.New("boom"), 500, "INTERNAL_ERROR"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		writeLimitedUseStoreError(w, c.err)
		if w.Code != c.status || !strings.Contains(w.Body.String(), `"`+c.code+`"`) {
			t.Errorf("%v: got %d %s, want %d %s", c.err, w.Code, w.Body.String(), c.status, c.code)
		}
	}
}
