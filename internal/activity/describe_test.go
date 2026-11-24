package activity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func requestWithHolder() (*Holder, func() *http.Request) {
	r := httptest.NewRequest("POST", "/", nil)
	ctx, h := WithHolder(r.Context())
	req := r.WithContext(ctx)
	return h, func() *http.Request { return req }
}

func TestDescribeWithoutHolderIsNoOp(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	Describe(r, Desc{Title: "x"})
	AddChanges(r, model.ActivityChange{Field: "gp"})
	Skip(r)
}

func TestDescribeLastCallWins(t *testing.T) {
	h, req := requestWithHolder()
	Describe(req(), Desc{Title: "first"})
	Describe(req(), Desc{Title: "second"})
	d, described, skipped := h.Snapshot()
	if !described || skipped || d.Title != "second" {
		t.Fatalf("got %+v described=%v skipped=%v", d, described, skipped)
	}
}

func TestAddChangesAppends(t *testing.T) {
	h, req := requestWithHolder()
	Describe(req(), Desc{Changes: []model.ActivityChange{{Field: "gp"}}})
	AddChanges(req(), model.ActivityChange{Field: "sp"})
	d, _, _ := h.Snapshot()
	if len(d.Changes) != 2 || d.Changes[1].Field != "sp" {
		t.Fatalf("got %+v", d.Changes)
	}
}

func TestSkipMarksHolder(t *testing.T) {
	h, req := requestWithHolder()
	Skip(req())
	if _, _, skipped := h.Snapshot(); !skipped {
		t.Fatal("expected skipped")
	}
}
