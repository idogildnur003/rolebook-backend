package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestStatusGuards(t *testing.T) {
	if !CanResolveRequest(RequestStatusPending) {
		t.Error("pending request must be resolvable")
	}
	if CanResolveRequest(RequestStatusApproved) || CanResolveRequest(RequestStatusDenied) {
		t.Error("already-resolved request must not be resolvable again")
	}
	if !CanWithdrawRequest(RequestStatusPending) {
		t.Error("pending request must be withdrawable")
	}
	if CanWithdrawRequest(RequestStatusApproved) {
		t.Error("approved request must not be withdrawable")
	}
	if CanWithdrawRequest(RequestStatusDenied) {
		t.Error("denied request must not be withdrawable")
	}
}

func TestIsValidRequestTarget(t *testing.T) {
	if !IsValidRequestTarget(RequestTargetItem) || !IsValidRequestTarget(RequestTargetSpell) {
		t.Error("item/spell must be valid targets")
	}
	if IsValidRequestTarget("bogus") {
		t.Error("bogus target must be invalid")
	}
}

func TestContentRequest_OmitsBsonOnlyFieldsOnWire(t *testing.T) {
	b, _ := json.Marshal(ContentRequest{
		ID: "req-1", Status: RequestStatusPending, TargetType: RequestTargetItem,
		Kind: RequestKindCreate, ProposedByName: "Aria",
	})
	s := string(b)
	if !strings.Contains(s, `"proposedByName":"Aria"`) {
		t.Errorf("request must emit proposedByName: %s", s)
	}
	if !strings.Contains(s, `"status":"pending"`) {
		t.Errorf("request must emit status: %s", s)
	}
}
