package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

func newLimitedUsePlayer(t *testing.T, s *PlayerStore, items ...model.LimitedUse) string {
	t.Helper()
	id := randomPlayerID(t)
	doc := bson.M{"_id": id, "campaignId": "c", "updatedAt": time.Now().UTC().Add(-time.Hour)}
	if items != nil {
		doc["limitedUses"] = items
	}
	insertTestPlayer(t, s, doc)
	return id
}

func readLimitedUses(t *testing.T, s *PlayerStore, id string) []model.LimitedUse {
	t.Helper()
	var doc struct {
		LimitedUses []model.LimitedUse `bson:"limitedUses"`
	}
	if err := s.col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatalf("read player: %v", err)
	}
	return doc.LimitedUses
}

func TestAddLimitedUse_AppendsWithRemainingEqualMax(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s)

	a, err := s.AddLimitedUse(ctx, id, model.LimitedUse{ID: "a", Name: "Rage", Max: 3, Resets: true})
	if err != nil {
		t.Fatalf("add a: %v", err)
	}
	if a.ID != "a" || a.Remaining != 3 {
		t.Fatalf("unexpected a: %+v", a)
	}
	if _, err := s.AddLimitedUse(ctx, id, model.LimitedUse{ID: "b", Name: "Potion", Max: 2}); err != nil {
		t.Fatalf("add b: %v", err)
	}
	got := readLimitedUses(t, s, id)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" || got[1].Remaining != 2 {
		t.Fatalf("unexpected list: %+v", got)
	}
}

func TestAddLimitedUse_LegacyDocWithoutField(t *testing.T) {
	s := newTestPlayerStore(t)
	id := randomPlayerID(t)
	insertTestPlayer(t, s, bson.M{"_id": id, "campaignId": "c"})
	if _, err := s.AddLimitedUse(context.Background(), id, model.LimitedUse{ID: "a", Name: "X", Max: 1}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := readLimitedUses(t, s, id); len(got) != 1 {
		t.Fatalf("unexpected list: %+v", got)
	}
}

func TestAddLimitedUse_LimitReached(t *testing.T) {
	s := newTestPlayerStore(t)
	items := make([]model.LimitedUse, model.LimitedUseMaxCount)
	for i := range items {
		items[i] = model.LimitedUse{ID: fmt.Sprintf("id%d", i), Name: "n", Max: 1, Remaining: 1}
	}
	id := newLimitedUsePlayer(t, s, items...)
	_, err := s.AddLimitedUse(context.Background(), id, model.LimitedUse{ID: "extra", Name: "n", Max: 1})
	if !errors.Is(err, ErrLimitReached) {
		t.Fatalf("want ErrLimitReached, got %v", err)
	}
}

func TestAddLimitedUse_MissingPlayer(t *testing.T) {
	s := newTestPlayerStore(t)
	_, err := s.AddLimitedUse(context.Background(), randomPlayerID(t), model.LimitedUse{ID: "a", Name: "n", Max: 1})
	if err == nil || errors.Is(err, ErrLimitReached) {
		t.Fatalf("want wrapped ErrNoDocuments, got %v", err)
	}
}

func TestUpdateLimitedUse_ClampsRemainingWhenMaxLowered(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s,
		model.LimitedUse{ID: "a", Name: "A", Max: 5, Remaining: 4, Resets: true},
		model.LimitedUse{ID: "b", Name: "B", Max: 3, Remaining: 1, Resets: true},
	)
	two, six := 2, 6
	got, err := s.UpdateLimitedUse(ctx, id, "a", LimitedUsePatch{Max: &two})
	if err != nil {
		t.Fatalf("update a: %v", err)
	}
	if got.Max != 2 || got.Remaining != 2 || got.Name != "A" {
		t.Fatalf("unexpected a: %+v", got)
	}
	name, note, resets := "B2", "n", false
	got, err = s.UpdateLimitedUse(ctx, id, "b", LimitedUsePatch{Max: &six, Name: &name, Note: &note, Resets: &resets})
	if err != nil {
		t.Fatalf("update b: %v", err)
	}
	if got.Max != 6 || got.Remaining != 1 || got.Name != "B2" || got.Note != "n" || got.Resets {
		t.Fatalf("unexpected b: %+v", got)
	}
}

func TestUpdateLimitedUse_NotFound(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 1, Remaining: 1})
	n := "x"
	_, err := s.UpdateLimitedUse(context.Background(), id, "nope", LimitedUsePatch{Name: &n})
	if !errors.Is(err, ErrLimitedUseNotFound) {
		t.Fatalf("want ErrLimitedUseNotFound, got %v", err)
	}
}

func TestRemoveLimitedUse(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 1, Remaining: 1})
	if err := s.RemoveLimitedUse(ctx, id, "a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := readLimitedUses(t, s, id); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestRemoveLimitedUse_NotFound(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newLimitedUsePlayer(t, s)
	if err := s.RemoveLimitedUse(context.Background(), id, "nope"); !errors.Is(err, ErrLimitedUseNotFound) {
		t.Fatalf("want ErrLimitedUseNotFound, got %v", err)
	}
}

func TestSpendLimitedUse_ResettableStaysAtZero(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 1, Remaining: 1, Resets: true})
	item, removed, err := s.SpendLimitedUse(ctx, id, "a")
	if err != nil || removed || item == nil || item.Remaining != 0 {
		t.Fatalf("spend: item=%+v removed=%v err=%v", item, removed, err)
	}
	if _, _, err := s.SpendLimitedUse(ctx, id, "a"); !errors.Is(err, ErrNoUsesLeft) {
		t.Fatalf("want ErrNoUsesLeft, got %v", err)
	}
}

func TestSpendLimitedUse_OneTimeDecrementsThenRemoved(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 2, Remaining: 2})
	item, removed, err := s.SpendLimitedUse(ctx, id, "a")
	if err != nil || removed || item == nil || item.Remaining != 1 {
		t.Fatalf("spend 1: item=%+v removed=%v err=%v", item, removed, err)
	}
	item, removed, err = s.SpendLimitedUse(ctx, id, "a")
	if err != nil || !removed || item != nil {
		t.Fatalf("spend 2: item=%+v removed=%v err=%v", item, removed, err)
	}
	if got := readLimitedUses(t, s, id); len(got) != 0 {
		t.Fatalf("expected gone, got %+v", got)
	}
	if _, _, err := s.SpendLimitedUse(ctx, id, "a"); !errors.Is(err, ErrLimitedUseNotFound) {
		t.Fatalf("want ErrLimitedUseNotFound, got %v", err)
	}
}

func TestSpendLimitedUse_LastOneTimeRemovedOnce(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 1, Remaining: 1})
	const n = 10
	type result struct {
		removed bool
		err     error
	}
	results := make([]result, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, removed, err := s.SpendLimitedUse(context.Background(), id, "a")
			results[i] = result{removed, err}
		}(i)
	}
	wg.Wait()
	removedCount := 0
	for i, r := range results {
		switch {
		case r.err == nil && r.removed:
			removedCount++
		case errors.Is(r.err, ErrNoUsesLeft), errors.Is(r.err, ErrLimitedUseNotFound):
		default:
			t.Errorf("goroutine %d: unexpected removed=%v err=%v", i, r.removed, r.err)
		}
	}
	if removedCount != 1 {
		t.Errorf("want exactly 1 removal, got %d", removedCount)
	}
	if got := readLimitedUses(t, s, id); len(got) != 0 {
		t.Errorf("expected gone, got %+v", got)
	}
}

func TestRestoreLimitedUse_IncrementsAndFull(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s, model.LimitedUse{ID: "a", Name: "A", Max: 2, Remaining: 1, Resets: true})
	item, err := s.RestoreLimitedUse(ctx, id, "a")
	if err != nil || item.Remaining != 2 {
		t.Fatalf("restore: item=%+v err=%v", item, err)
	}
	if _, err := s.RestoreLimitedUse(ctx, id, "a"); !errors.Is(err, ErrAlreadyFull) {
		t.Fatalf("want ErrAlreadyFull, got %v", err)
	}
	if _, err := s.RestoreLimitedUse(ctx, id, "nope"); !errors.Is(err, ErrLimitedUseNotFound) {
		t.Fatalf("want ErrLimitedUseNotFound, got %v", err)
	}
}

func TestResetLimitedUse_SetsMaxAndRejectsOneTime(t *testing.T) {
	s := newTestPlayerStore(t)
	ctx := context.Background()
	id := newLimitedUsePlayer(t, s,
		model.LimitedUse{ID: "a", Name: "A", Max: 3, Remaining: 0, Resets: true},
		model.LimitedUse{ID: "b", Name: "B", Max: 3, Remaining: 1},
	)
	item, err := s.ResetLimitedUse(ctx, id, "a")
	if err != nil || item.Remaining != 3 {
		t.Fatalf("reset: item=%+v err=%v", item, err)
	}
	if _, err := s.ResetLimitedUse(ctx, id, "b"); !errors.Is(err, ErrNotResettable) {
		t.Fatalf("want ErrNotResettable, got %v", err)
	}
	if _, err := s.ResetLimitedUse(ctx, id, "nope"); !errors.Is(err, ErrLimitedUseNotFound) {
		t.Fatalf("want ErrLimitedUseNotFound, got %v", err)
	}
}
