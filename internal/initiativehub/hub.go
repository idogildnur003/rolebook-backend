// Package initiativehub is an in-process pub/sub broadcaster for
// per-campaign initiative-call updates. Mutation handlers Publish a snapshot
// of the updated call; SSE handlers Subscribe to receive snapshots for the
// campaign they're streaming.
//
// Single Go instance only. Multi-instance horizontal scaling would need a
// shared broker (Redis pub/sub, NATS, etc.) — out of scope for now.
package initiativehub

import (
	"sync"

	"github.com/elad/rolebook-backend/internal/model"
)

// subBufferSize is the per-subscriber channel buffer. Slow consumers that
// fall this far behind are dropped on the next publish rather than blocking
// the mutation path. 16 absorbs a reasonable burst (~16 mutations in flight)
// without holding back the publisher.
const subBufferSize = 16

// Hub fans out *model.InitiativeCall snapshots to subscribers, keyed by
// campaignID. Methods are safe to call from any goroutine.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[int]chan *model.InitiativeCall
	next int // monotonically increasing subscriber id; never reused
}

// New returns a ready-to-use Hub.
func New() *Hub {
	return &Hub{subs: make(map[string]map[int]chan *model.InitiativeCall)}
}

// Subscribe registers a new subscriber for campaignID and returns the channel
// it will receive snapshots on plus an unsub func the caller MUST invoke
// (typically with defer) when finished. The channel is buffered; if the
// subscriber falls behind, Publish drops the message for that subscriber
// rather than blocking other subscribers or the publisher.
//
// The returned channel is NOT closed by unsub — closing would race with
// concurrent Publish goroutines (which hold the channel value after dropping
// the lock to avoid blocking other subscribers). Receivers must instead
// signal exit via select-on-ctx.Done() (or another sentinel) and treat the
// channel as garbage once unmapped.
func (h *Hub) Subscribe(campaignID string) (<-chan *model.InitiativeCall, func()) {
	ch := make(chan *model.InitiativeCall, subBufferSize)

	h.mu.Lock()
	id := h.next
	h.next++
	bucket, ok := h.subs[campaignID]
	if !ok {
		bucket = make(map[int]chan *model.InitiativeCall)
		h.subs[campaignID] = bucket
	}
	bucket[id] = ch
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if bucket, ok := h.subs[campaignID]; ok {
			delete(bucket, id)
			if len(bucket) == 0 {
				delete(h.subs, campaignID)
			}
		}
		h.mu.Unlock()
	}

	return ch, unsub
}

// Publish sends call to every subscriber of campaignID. Non-blocking: if a
// subscriber's buffer is full, that subscriber misses this message. The next
// successful send will give them a fresher snapshot anyway, since each event
// is a full snapshot (not a delta).
func (h *Hub) Publish(campaignID string, call *model.InitiativeCall) {
	h.mu.RLock()
	bucket, ok := h.subs[campaignID]
	if !ok || len(bucket) == 0 {
		h.mu.RUnlock()
		return
	}
	// Snapshot the channels under the read lock so Publish doesn't hold the
	// lock while sending. Channels themselves are safe to send on without
	// the map lock.
	targets := make([]chan *model.InitiativeCall, 0, len(bucket))
	for _, ch := range bucket {
		targets = append(targets, ch)
	}
	h.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- call:
		default:
			// Subscriber is behind; drop. The next publish will catch them
			// up with a fresher snapshot.
		}
	}
}
