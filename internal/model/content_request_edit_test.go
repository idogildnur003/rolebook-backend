package model

import "testing"

func TestIsValidRequestKind(t *testing.T) {
	for _, k := range []string{RequestKindCreate, RequestKindEdit} {
		if !IsValidRequestKind(k) {
			t.Fatalf("expected %q valid", k)
		}
	}
	if IsValidRequestKind("bogus") {
		t.Fatal("expected bogus invalid")
	}
	if IsValidRequestKind("") {
		t.Fatal("expected empty invalid")
	}
}

func TestIsEditRequest(t *testing.T) {
	if !IsEditRequest(RequestKindEdit) {
		t.Fatal("edit should be edit")
	}
	if IsEditRequest(RequestKindCreate) {
		t.Fatal("create is not edit")
	}
}
