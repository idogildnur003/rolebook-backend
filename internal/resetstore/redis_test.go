package resetstore

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func newTestRedis(t *testing.T) *Redis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return NewRedis("redis://" + mr.Addr())
}

func TestRedis_SetCodeGetPromoteClear(t *testing.T) {
	s := newTestRedis(t)
	ctx := context.Background()
	email := "a@b.com"

	if got, _ := s.Get(ctx, email); got != nil {
		t.Fatalf("Get on empty = %+v, want nil", got)
	}
	if err := s.SetCode(ctx, email, "codehash"); err != nil {
		t.Fatalf("SetCode: %v", err)
	}
	sess, err := s.Get(ctx, email)
	if err != nil || sess == nil || sess.CodeHash != "codehash" || sess.Attempts != 0 {
		t.Fatalf("Get after SetCode = %+v, %v", sess, err)
	}
	if n, _ := s.IncrAttempts(ctx, email); n != 1 {
		t.Fatalf("IncrAttempts = %d, want 1", n)
	}
	if err := s.PromoteToToken(ctx, email, "tokhash"); err != nil {
		t.Fatalf("PromoteToToken: %v", err)
	}
	sess, _ = s.Get(ctx, email)
	if sess == nil || sess.TokenHash != "tokhash" || sess.CodeHash != "" {
		t.Fatalf("after promote = %+v, want token set & code cleared", sess)
	}
	if err := s.Clear(ctx, email); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got, _ := s.Get(ctx, email); got != nil {
		t.Fatalf("Get after Clear = %+v, want nil", got)
	}
}

func TestRedis_MarkSentCooldown(t *testing.T) {
	s := newTestRedis(t)
	ctx := context.Background()
	if ok, err := s.MarkSent(ctx, "a@b.com"); err != nil || !ok {
		t.Fatalf("first MarkSent = %v, %v, want true", ok, err)
	}
	if ok, _ := s.MarkSent(ctx, "a@b.com"); ok {
		t.Fatal("second MarkSent = true, want false (cooldown)")
	}
}
