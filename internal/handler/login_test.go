package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/elad/rolebook-backend/internal/model"
)

const loginTestPassword = "correct-horse-battery"

// stubUserRepo is an in-memory userRepo for auth handler tests. Only the methods
// the login path exercises do real work; the rest are inert.
type stubUserRepo struct {
	byEmail      map[string]*model.User
	setCodeCalls int
}

func (s *stubUserRepo) byID(id string) *model.User {
	for _, u := range s.byEmail {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (s *stubUserRepo) FindByEmail(_ context.Context, email string) (*model.User, error) {
	return s.byEmail[email], nil
}

func (s *stubUserRepo) SetVerificationCode(_ context.Context, userID, codeHash string, expiresAt, sentAt time.Time) error {
	s.setCodeCalls++
	if u := s.byID(userID); u != nil {
		u.VerifyCodeHash = codeHash
		u.VerifyCodeExpiresAt = expiresAt
		u.VerifyCodeSentAt = sentAt
		u.VerifyCodeAttempts = 0
	}
	return nil
}

func (s *stubUserRepo) Create(context.Context, *model.User) error         { return nil }
func (s *stubUserRepo) GetByID(_ context.Context, id string) (*model.User, error) {
	return s.byID(id), nil
}
func (s *stubUserRepo) IncrementVerifyAttempts(context.Context, string) error { return nil }
func (s *stubUserRepo) MarkVerified(context.Context, string) error           { return nil }
func (s *stubUserRepo) UpdatePasswordHash(context.Context, string, string) error {
	return nil
}
func (s *stubUserRepo) SetPendingEmailCode(context.Context, string, string, string, time.Time, time.Time) error {
	return nil
}
func (s *stubUserRepo) CommitEmailChange(context.Context, string, string) error { return nil }

// recordingSender counts the emails it is asked to deliver.
type recordingSender struct {
	sends  int
	lastTo string
}

func (r *recordingSender) Send(_ context.Context, to, _, _, _ string) error {
	r.sends++
	r.lastTo = to
	return nil
}

func newUserWithPassword(t *testing.T, id, email string, verified bool) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(loginTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return &model.User{ID: id, Email: email, PasswordHash: string(hash), EmailVerified: verified}
}

func doLogin(t *testing.T, h *AuthHandler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, r)
	return w
}

// Fix 1: with verification disabled, an existing unverified account must be able
// to log in (the flag applies to login, not just registration).
func TestLogin_VerificationDisabled_AllowsUnverifiedUser(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", false)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, false, nil)

	w := doLogin(t, h, "a@b.com", loginTestPassword)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unverified login allowed when verification disabled)", w.Code)
	}
	var resp authResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected a token in the login response")
	}
	if sender.sends != 0 {
		t.Errorf("sends = %d, want 0 (no code should be sent when verification is disabled)", sender.sends)
	}
}

// Fix 2: with verification enabled, a blocked (unverified) login must both refuse
// with EMAIL_NOT_VERIFIED and proactively send a fresh code, reporting codeSent.
func TestLogin_VerificationEnabled_BlocksUnverifiedAndSendsCode(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", false) // no prior code (zero VerifyCodeSentAt)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, true, nil)

	w := doLogin(t, h, "a@b.com", loginTestPassword)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	var body struct {
		Code     string `json:"code"`
		CodeSent bool   `json:"codeSent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "EMAIL_NOT_VERIFIED" {
		t.Errorf("code = %q, want EMAIL_NOT_VERIFIED", body.Code)
	}
	if !body.CodeSent {
		t.Error("codeSent = false, want true (a code should be sent on the blocking login)")
	}
	if sender.sends != 1 {
		t.Errorf("sends = %d, want 1", sender.sends)
	}
	if repo.setCodeCalls != 1 {
		t.Errorf("SetVerificationCode calls = %d, want 1", repo.setCodeCalls)
	}
}

// Fix 2 guard: a login within the resend cooldown must not send a second code,
// but should still report a code is available (the recent one is still valid).
func TestLogin_VerificationEnabled_WithinCooldown_DoesNotResend(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", false)
	user.VerifyCodeSentAt = time.Now() // just sent → inside the 60s cooldown
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, true, nil)

	w := doLogin(t, h, "a@b.com", loginTestPassword)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	var body struct {
		CodeSent bool `json:"codeSent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.CodeSent {
		t.Error("codeSent = false, want true (a recently-sent code is still valid)")
	}
	if sender.sends != 0 {
		t.Errorf("sends = %d, want 0 (resend cooldown has not elapsed)", sender.sends)
	}
	if repo.setCodeCalls != 0 {
		t.Errorf("SetVerificationCode calls = %d, want 0 (resend cooldown has not elapsed)", repo.setCodeCalls)
	}
}

func doResend(t *testing.T, h *AuthHandler, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/resend-verification", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ResendVerification(w, r)
	return w
}

// Guards the shared-helper refactor: resend still sends for an unverified account.
func TestResendVerification_UnverifiedUser_SendsCode(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", false)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, true, nil)

	w := doResend(t, h, "a@b.com")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if sender.sends != 1 {
		t.Errorf("sends = %d, want 1", sender.sends)
	}
}

// Guards the shared-helper refactor: resend never sends for a verified account,
// and still returns a generic 200 (no account enumeration).
func TestResendVerification_VerifiedUser_NoSend(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	sender := &recordingSender{}
	h := NewAuthHandler(repo, "secret", sender, true, nil)

	w := doResend(t, h, "a@b.com")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if sender.sends != 0 {
		t.Errorf("sends = %d, want 0 (verified user)", sender.sends)
	}
}

// Regression guard: a verified account always logs in, even with verification on.
func TestLogin_VerifiedUser_Succeeds(t *testing.T) {
	user := newUserWithPassword(t, "u1", "a@b.com", true)
	repo := &stubUserRepo{byEmail: map[string]*model.User{"a@b.com": user}}
	h := NewAuthHandler(repo, "secret", &recordingSender{}, true, nil)

	w := doLogin(t, h, "a@b.com", loginTestPassword)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
