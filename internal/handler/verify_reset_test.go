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

func doVerifyReset(t *testing.T, h *AuthHandler, email, code string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "code": code})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/verify-reset-code", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.VerifyResetCode(w, r)
	return w
}

// seedResetCode stores a bcrypt hash of `code` for email in the stub store.
func seedResetCode(t *testing.T, resets *stubResetStore, email, code string) {
	t.Helper()
	hash, err := hashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_ = resets.SetCode(context.Background(), email, hash)
}

func newVerifyHandler(email string) (*AuthHandler, *stubResetStore) {
	user := &model.User{ID: "u1", Email: email, EmailVerified: true}
	repo := &stubUserRepo{byEmail: map[string]*model.User{email: user}}
	resets := newStubResetStore()
	h := NewAuthHandler(repo, "secret", &recordingSender{}, true, nil, resets)
	return h, resets
}

func TestVerifyResetCode_CorrectCode_ReturnsToken(t *testing.T) {
	h, resets := newVerifyHandler("a@b.com")
	seedResetCode(t, resets, "a@b.com", "123456")

	w := doVerifyReset(t, h, "a@b.com", "123456")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		ResetToken string `json:"resetToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.ResetToken) != 64 {
		t.Errorf("resetToken = %q, want 64 hex chars", body.ResetToken)
	}
	s, _ := resets.Get(context.Background(), "a@b.com")
	if s == nil || s.TokenHash == "" || s.CodeHash != "" {
		t.Errorf("session after verify = %+v, want token set & code cleared", s)
	}
}

func TestVerifyResetCode_WrongCode_400IncrementsAttempts(t *testing.T) {
	h, resets := newVerifyHandler("a@b.com")
	seedResetCode(t, resets, "a@b.com", "123456")

	w := doVerifyReset(t, h, "a@b.com", "000000")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "INVALID_CODE" {
		t.Errorf("code = %q, want INVALID_CODE", body.Code)
	}
	s, _ := resets.Get(context.Background(), "a@b.com")
	if s.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", s.Attempts)
	}
}

func TestVerifyResetCode_NoSession_400Generic(t *testing.T) {
	h, _ := newVerifyHandler("a@b.com")
	w := doVerifyReset(t, h, "a@b.com", "123456")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "INVALID_CODE" {
		t.Errorf("code = %q, want INVALID_CODE (generic)", body.Code)
	}
}

func TestVerifyResetCode_TooManyAttempts_400(t *testing.T) {
	h, resets := newVerifyHandler("a@b.com")
	seedResetCode(t, resets, "a@b.com", "123456")
	resets.sessions["a@b.com"].Attempts = maxVerifyAttempts // already at cap

	w := doVerifyReset(t, h, "a@b.com", "123456")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "TOO_MANY_ATTEMPTS" {
		t.Errorf("code = %q, want TOO_MANY_ATTEMPTS", body.Code)
	}
}
