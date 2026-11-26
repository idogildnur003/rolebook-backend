// Package activity records every successful mutating API call as a history
// entry. The Record middleware guarantees the entry exists; handlers enrich it
// through Describe.
package activity

import (
	"context"
	"net/http"
	"sync"

	"github.com/elad/rolebook-backend/internal/model"
)

// Desc is what a handler knows about its change. Empty fields keep the route
// registry's defaults or the values the middleware resolves.
type Desc struct {
	Type, Category, Title, Detail                   string
	CampaignID, PlayerID, PlayerName, SubjectUserID string
	Changes                                         []model.ActivityChange
}

// Holder is the request-scoped slot a handler writes its description into. The
// middleware installs it before the handler runs, so the description can flow
// back up despite contexts only flowing down.
type Holder struct {
	mu        sync.Mutex
	desc      Desc
	described bool
	skipped   bool
}

type holderKey struct{}

// WithHolder returns a context carrying a new empty Holder.
func WithHolder(ctx context.Context) (context.Context, *Holder) {
	h := &Holder{}
	return context.WithValue(ctx, holderKey{}, h), h
}

func holderFrom(r *http.Request) *Holder {
	h, _ := r.Context().Value(holderKey{}).(*Holder)
	return h
}

// Describe sets the entry's description. The last call wins. It is a no-op on
// requests that did not pass through the middleware.
func Describe(r *http.Request, d Desc) {
	h := holderFrom(r)
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.desc = d
	h.described = true
}

// AddChanges appends before → after values to the current description.
func AddChanges(r *http.Request, c ...model.ActivityChange) {
	h := holderFrom(r)
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.desc.Changes = append(h.desc.Changes, c...)
	h.described = true
}

// Skip marks a mutation that changes nothing user-visible, so no entry is written.
func Skip(r *http.Request) {
	h := holderFrom(r)
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.skipped = true
}

// Snapshot returns a copy of the holder's current state.
func (h *Holder) Snapshot() (d Desc, described, skipped bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d = h.desc
	d.Changes = append([]model.ActivityChange(nil), h.desc.Changes...)
	return d, h.described, h.skipped
}
