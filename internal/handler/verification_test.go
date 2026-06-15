package handler

import (
	"testing"
	"time"

	"github.com/elad/rolebook-backend/internal/model"
)

func TestGenerateVerificationCode_SixDigits(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := generateVerificationCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("code %q has length %d, want 6", code, len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("code %q contains non-digit", code)
			}
		}
	}
}

func TestHashAndMatchVerificationCode(t *testing.T) {
	hash, err := hashVerificationCode("123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !verificationCodeMatches(hash, "123456") {
		t.Error("matching code reported as mismatch")
	}
	if verificationCodeMatches(hash, "000000") {
		t.Error("wrong code reported as match")
	}
}

func TestCodeExpired(t *testing.T) {
	now := time.Now()
	if codeExpired(now.Add(time.Minute), now) {
		t.Error("future expiry reported as expired")
	}
	if !codeExpired(now.Add(-time.Minute), now) {
		t.Error("past expiry reported as not expired")
	}
}

func TestEmailVerificationBlocksLogin(t *testing.T) {
	cases := []struct {
		name string
		user model.User
		want bool
	}{
		{"verified new account", model.User{EmailVerified: true}, false},
		{"verified legacy account", model.User{EmailVerified: true, LegacyUnverified: true}, false},
		{"unverified new signup (gated)", model.User{}, true},
		{"unverified legacy account (exempt)", model.User{LegacyUnverified: true}, false},
	}
	for _, c := range cases {
		if got := emailVerificationBlocksLogin(&c.user); got != c.want {
			t.Errorf("%s: blocks = %v, want %v", c.name, got, c.want)
		}
	}
}
