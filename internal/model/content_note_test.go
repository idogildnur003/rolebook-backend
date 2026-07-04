package model

import "testing"

func TestIsValidNoteTarget(t *testing.T) {
	if !IsValidNoteTarget(RequestTargetItem) {
		t.Fatal("item should be valid")
	}
	if !IsValidNoteTarget(RequestTargetSpell) {
		t.Fatal("spell should be valid")
	}
	if IsValidNoteTarget("bogus") {
		t.Fatal("bogus should be invalid")
	}
	if IsValidNoteTarget("") {
		t.Fatal("empty should be invalid")
	}
}

func TestContentNoteKey(t *testing.T) {
	a := ContentNoteKey("u1", "c1", "item", "e1")
	b := ContentNoteKey("u1", "c1", "item", "e1")
	if a != b {
		t.Fatal("key must be stable for the same inputs")
	}
	// Different entryId → different key
	if a == ContentNoteKey("u1", "c1", "item", "e2") {
		t.Fatal("different entryId must yield a different key")
	}
	// Different targetType → different key
	if a == ContentNoteKey("u1", "c1", "spell", "e1") {
		t.Fatal("different targetType must yield a different key")
	}
	// Different user → different key
	if a == ContentNoteKey("u2", "c1", "item", "e1") {
		t.Fatal("different user must yield a different key")
	}
}
