package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/resetstore"
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

// Concurrent wrong guesses must not get more than maxVerifyAttempts real code
// comparisons: the attempt is reserved atomically before comparing, so the
// excess requests are turned away with TOO_MANY_ATTEMPTS. Uses the real
// (mutex-guarded) Memory store since the stub isn't safe for concurrent use.
func TestVerifyResetCode_ConcurrentGuesses_CappedAtMaxAttempts(t *testing.T) {
	const email = "a@b.com"
	user := &model.User{ID: "u1", Email: email, EmailVerified: true}
	repo := &stubUserRepo{byEmail: map[string]*model.User{email: user}}
	resets := resetstore.NewMemory()
	h := NewAuthHandler(repo, "secret", &recordingSender{}, true, nil, resets)
	hash, err := hashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_ = resets.SetCode(context.Background(), email, hash)

	const guesses = 4 * maxVerifyAttempts
	codes := make([]string, guesses)
	var wg sync.WaitGroup
	for i := 0; i < guesses; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := doVerifyReset(t, h, email, "000000")
			var body struct{ Code string `json:"code"` }
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			codes[i] = body.Code
		}(i)
	}
	wg.Wait()

	invalid, tooMany := 0, 0
	for _, c := range codes {
		switch c {
		case "INVALID_CODE":
			invalid++
		case "TOO_MANY_ATTEMPTS":
			tooMany++
		default:
			t.Errorf("unexpected error code %q", c)
		}
	}
	if invalid != maxVerifyAttempts {
		t.Errorf("INVALID_CODE responses = %d, want exactly %d (one per allowed attempt)", invalid, maxVerifyAttempts)
	}
	if tooMany != guesses-maxVerifyAttempts {
		t.Errorf("TOO_MANY_ATTEMPTS responses = %d, want %d", tooMany, guesses-maxVerifyAttempts)
	}

	// The correct code is now locked out too.
	w := doVerifyReset(t, h, email, "123456")
	var body struct{ Code string `json:"code"` }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "TOO_MANY_ATTEMPTS" {
		t.Errorf("correct code after cap: code = %q, want TOO_MANY_ATTEMPTS", body.Code)
	}
}
