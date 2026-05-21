package initiativehub

import (
	"sync"
	"testing"
	"time"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestPublishFanout_DeliversToEverySubscriber(t *testing.T) {
	h := New()
	chA, unsubA := h.Subscribe("c1")
	defer unsubA()
	chB, unsubB := h.Subscribe("c1")
	defer unsubB()

	call := &model.InitiativeCall{ID: "call-1", Status: "open"}
	h.Publish("c1", call)

	for i, ch := range []<-chan *model.InitiativeCall{chA, chB} {
		select {
		case got := <-ch:
			if got != call {
				t.Fatalf("subscriber %d: got %v want %v", i, got, call)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: did not receive within 100ms", i)
		}
	}
}

func TestPublishScope_OnlyDeliversToMatchingCampaign(t *testing.T) {
	h := New()
	chA, unsubA := h.Subscribe("c1")
	defer unsubA()
	chB, unsubB := h.Subscribe("c2")
	defer unsubB()

	h.Publish("c1", &model.InitiativeCall{ID: "x"})

	select {
	case <-chA:
		// expected
	case <-time.After(50 * time.Millisecond):
		t.Fatal("c1 subscriber should have received")
	}

	select {
	case got := <-chB:
		t.Fatalf("c2 subscriber should not have received, got %v", got)
	case <-time.After(20 * time.Millisecond):
		// expected
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) {
	h := New()
	ch, unsub := h.Subscribe("c1")
	unsub()

	// Publish after unsub must not panic and must not be delivered to the
	// now-unmapped channel. The channel is intentionally NOT closed by unsub
	// (closing races with concurrent publishers); receivers exit via their
	// own ctx-done signal.
	h.Publish("c1", &model.InitiativeCall{ID: "y"})

	select {
	case got := <-ch:
		t.Fatalf("expected no delivery after unsub, got %v", got)
	case <-time.After(30 * time.Millisecond):
		// expected — nothing delivered
	}
}

// A subscriber whose buffer fills must be dropped on overflow rather than
// blocking the publisher.
func TestPublishNonBlocking_DropsSlowSubscriber(t *testing.T) {
	h := New()
	ch, unsub := h.Subscribe("c1")
	defer unsub()

	// Saturate the buffer.
	for i := 0; i < subBufferSize; i++ {
		h.Publish("c1", &model.InitiativeCall{ID: "fill"})
	}

	// The next Publish must return promptly even though the buffer is full.
	done := make(chan struct{})
	go func() {
		h.Publish("c1", &model.InitiativeCall{ID: "overflow"})
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked when subscriber buffer was full")
	}

	// Drain to confirm the slow-subscriber drop kept earlier items intact.
	if len(ch) != subBufferSize {
		t.Fatalf("expected %d buffered items, got %d", subBufferSize, len(ch))
	}
}

// Concurrent publishers + concurrent subscribers must not race.
func TestConcurrentPublishAndSubscribe(t *testing.T) {
	h := New()
	var wg sync.WaitGroup
	// 20 subscribers, each receives from a campaign and unsubs.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := h.Subscribe("c1")
			defer unsub()
			select {
			case <-ch:
			case <-time.After(200 * time.Millisecond):
			}
		}()
	}
	// 50 publishers in parallel.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Publish("c1", &model.InitiativeCall{ID: "x"})
		}()
	}
	wg.Wait()
}
