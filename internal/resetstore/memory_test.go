package resetstore

import (
	"context"
	"testing"
)

func TestMemory_SetCodeGetPromoteClear(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	email := "a@b.com"

	if got, _ := s.Get(ctx, email); got != nil {
		t.Fatalf("Get on empty = %+v, want nil", got)
	}
	if err := s.SetCode(ctx, email, "codehash"); err != nil {
		t.Fatalf("SetCode: %v", err)
	}
	sess, err := s.Get(ctx, email)
	if err != nil || sess == nil {
		t.Fatalf("Get after SetCode = %+v, %v", sess, err)
	}
	if sess.CodeHash != "codehash" || sess.Attempts != 0 || sess.TokenHash != "" {
		t.Fatalf("session = %+v, want {codehash 0 \"\"}", sess)
	}

	n, err := s.IncrAttempts(ctx, email)
	if err != nil || n != 1 {
		t.Fatalf("IncrAttempts = %d, %v, want 1", n, err)
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

func TestMemory_MarkSentCooldown(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	ok, err := s.MarkSent(ctx, "a@b.com")
	if err != nil || !ok {
		t.Fatalf("first MarkSent = %v, %v, want true", ok, err)
	}
	ok, _ = s.MarkSent(ctx, "a@b.com")
	if ok {
		t.Fatal("second MarkSent within window = true, want false (cooldown)")
	}
}
