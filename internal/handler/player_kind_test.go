package handler

import (
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestListKindForRole(t *testing.T) {
	if got := listKindForRole(true); got != model.PlayerKind("") {
		t.Errorf("listKindForRole(isDM=true) = %q, want \"\" (all kinds)", got)
	}
	if got := listKindForRole(false); got != model.PlayerKindPC {
		t.Errorf("listKindForRole(isDM=false) = %q, want %q", got, model.PlayerKindPC)
	}
}
