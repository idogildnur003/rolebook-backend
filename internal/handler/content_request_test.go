package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

func TestBuildRequestViews_ResolvesProposerName(t *testing.T) {
	reqs := []model.ContentRequest{
		{ID: "r-1", ProposedByUserID: "u-1", TargetType: model.RequestTargetItem, Kind: model.RequestKindCreate},
		{ID: "r-2", ProposedByUserID: "u-ghost", TargetType: model.RequestTargetSpell, Kind: model.RequestKindCreate},
	}
	players := []store.PlayerInventorySummary{
		{ID: "p-1", Name: "Aria", LinkedUserID: "u-1"},
	}
	out := buildRequestViews(reqs, players)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].ProposedByName != "Aria" {
		t.Errorf("r-1 proposer = %q, want Aria", out[0].ProposedByName)
	}
	if out[1].ProposedByName != "" {
		t.Errorf("r-2 proposer (no player record) = %q, want empty", out[1].ProposedByName)
	}
}
