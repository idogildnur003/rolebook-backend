package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/elad/rolebook-backend/internal/model"
)

func doReset(t *testing.T, h *AuthHandler, email, token, newPassword string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "resetToken": token, "newPassword": newPassword})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, r)
	return w
}

// seedResetToken stores the SHA-256 of `token` in the stub store for email.
func seedResetToken(resets *stubResetStore, email, token string) {
	_ = resets.PromoteToToken(context.Background(), email, hashResetToken(token))
}

func TestResetPassword_ValidToken_UpdatesPasswordAndClears(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", sender, true, nil, resets)
	seedResetToken(resets, "a@b.com", "tok-abc")

	w := doReset(t, h, "a@b.com", "tok-abc", "brand-new-pass")

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	// Password now matches the new one.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("brand-new-pass")); err != nil {
		t.Errorf("password not updated: %v", err)
	}
	// Reset session cleared.
	if s, _ := resets.Get(context.Background(), "a@b.com"); s != nil {
		t.Errorf("session not cleared: %+v", s)
	}
	// Security notice sent.
	if sender.sends != 1 {
		t.Errorf("sends = %d, want 1 (password-changed notice)", sender.sends)
	}
}

func TestResetPassword_WrongToken_400(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", &recordingSender{}, true, nil, resets)
	seedResetToken(resets, "a@b.com", "tok-abc")

	w := doReset(t, h, "a@b.com", "WRONG", "brand-new-pass")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "INVALID_TOKEN" {
		t.Errorf("code = %q, want INVALID_TOKEN", body.Code)
	}
}

func TestResetPassword_WeakPassword_400(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", &recordingSender{}, true, nil, resets)
	seedResetToken(resets, "a@b.com", "tok-abc")

	w := doReset(t, h, "a@b.com", "tok-abc", "short")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "WEAK_PASSWORD" {
		t.Errorf("code = %q, want WEAK_PASSWORD", body.Code)
	}
}
