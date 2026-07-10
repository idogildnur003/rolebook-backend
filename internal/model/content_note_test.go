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
