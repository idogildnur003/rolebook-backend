package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
)

func doForgot(t *testing.T, h *AuthHandler, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/forgot-password", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, r)
	return w
}

func TestForgotPassword_ExistingAccount_SendsCodeAndStores(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", sender, true, nil, resets)

	w := doForgot(t, h, "A@b.com") // mixed case -> normalized

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if sender.sends != 1 {
		t.Errorf("sends = %d, want 1", sender.sends)
	}
	if s, _ := resets.Get(context.Background(), "a@b.com"); s == nil || s.CodeHash == "" {
		t.Errorf("no code stored for normalized email")
	}
}

func TestForgotPassword_UnknownAccount_Returns200SendsNothing(t *testing.T) {
	repo := &stubUserRepo{byEmail: map[string]*model.User{}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, true, nil, newStubResetStore())

	w := doForgot(t, h, "nobody@b.com")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (anti-enumeration)", w.Code)
	}
	if sender.sends != 0 {
		t.Errorf("sends = %d, want 0 for unknown account", sender.sends)
	}
}

func TestForgotPassword_Cooldown_DoesNotResend(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", sender, true, nil, resets)

	_ = doForgot(t, h, "a@b.com")
	w := doForgot(t, h, "a@b.com") // second within cooldown

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if sender.sends != 1 {
		t.Errorf("sends = %d, want 1 (cooldown suppresses the 2nd)", sender.sends)
	}
}
