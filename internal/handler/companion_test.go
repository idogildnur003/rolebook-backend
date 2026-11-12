package handler

import "testing"

func TestResolveCompanionOwner(t *testing.T) {
	player := &campaignMembership{IsMember: true, UserID: "u1", PlayerID: "p1"}
	if id, st, _ := resolveCompanionOwner(player, "p-other", false); id != "p1" || st != 0 {
		t.Fatalf("player → %q %d", id, st)
	}
	if _, st, _ := resolveCompanionOwner(&campaignMembership{IsMember: true, UserID: "u1"}, "", false); st != 403 {
		t.Fatalf("no PC → %d", st)
	}
	dm := &campaignMembership{IsDM: true, IsMember: true, UserID: "u-dm", PlayerID: "p-dm"}
	if _, st, _ := resolveCompanionOwner(dm, "", false); st != 400 {
		t.Fatalf("dm no owner → %d", st)
	}
	if _, st, _ := resolveCompanionOwner(dm, "n1", false); st != 400 {
		t.Fatalf("dm non-pc owner → %d", st)
	}
	if id, st, _ := resolveCompanionOwner(dm, "p1", true); id != "p1" || st != 0 {
		t.Fatalf("dm ok → %q %d", id, st)
	}
}
