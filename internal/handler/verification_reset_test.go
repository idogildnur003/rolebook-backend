package handler

import (
	"strings"
	"testing"
)

func TestPasswordResetEmailBody_ContainsCode(t *testing.T) {
	subject, html, text := passwordResetEmailBody("123456")
	if subject == "" {
		t.Error("subject is empty")
	}
	if !strings.Contains(html, "123456") || !strings.Contains(text, "123456") {
		t.Errorf("body missing code: html=%q text=%q", html, text)
	}
}

func TestResetToken_GenerateHashMatch(t *testing.T) {
	tok, err := generateResetToken()
	if err != nil {
		t.Fatalf("generateResetToken: %v", err)
	}
	if len(tok) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(tok))
	}
	hash := hashResetToken(tok)
	if hash == tok {
		t.Error("hash equals token (not hashed)")
	}
	if !resetTokenMatches(hash, tok) {
		t.Error("resetTokenMatches = false for the correct token")
	}
	if resetTokenMatches(hash, "wrong") {
		t.Error("resetTokenMatches = true for a wrong token")
	}
}
