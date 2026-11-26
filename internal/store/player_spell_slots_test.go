package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestSpendSpellSlot(t *testing.T) {
	s := newTestPlayerStore(t)
	id := randomPlayerID(t)
	insertTestPlayer(t, s, bson.M{
		"_id":        id,
		"campaignId": "c",
		"spellSlots": map[string]model.SpellSlot{"1": {Max: 2, Used: 1}, "2": {Max: 0, Used: 0}},
	})
	ctx := context.Background()

	slots, err := s.SpendSpellSlot(ctx, id, "1")
	if err != nil || slots["1"].Used != 2 || slots["1"].Max != 2 {
		t.Fatalf("spend: %+v, %v", slots, err)
	}
	if _, err := s.SpendSpellSlot(ctx, id, "1"); !errors.Is(err, ErrNoSpellSlot) {
		t.Fatalf("exhausted level: %v", err)
	}
	if _, err := s.SpendSpellSlot(ctx, id, "2"); !errors.Is(err, ErrNoSpellSlot) {
		t.Fatalf("zero-max level: %v", err)
	}
	if _, err := s.SpendSpellSlot(ctx, id, "3"); !errors.Is(err, ErrNoSpellSlot) {
		t.Fatalf("missing level: %v", err)
	}
}

func TestSpendSpellSlot_ConcurrentCastsNeverOverspend(t *testing.T) {
	s := newTestPlayerStore(t)
	id := randomPlayerID(t)
	insertTestPlayer(t, s, bson.M{
		"_id":        id,
		"campaignId": "c",
		"spellSlots": map[string]model.SpellSlot{"1": {Max: 3, Used: 0}},
	})
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.SpendSpellSlot(context.Background(), id, "1"); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 3 {
		t.Fatalf("%d casts succeeded, want 3", ok)
	}
}
