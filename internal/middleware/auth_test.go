package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIsAdmin(t *testing.T) {
	admins := []string{"u-admin", "u-2"}
	if !IsAdmin(admins, "u-admin") {
		t.Error("expected u-admin to be admin")
	}
	if IsAdmin(admins, "u-other") {
		t.Error("expected u-other not to be admin")
	}
	if IsAdmin(admins, "") {
		t.Error("empty userID must never be admin")
	}
	if IsAdmin(nil, "u-admin") {
		t.Error("empty allowlist must never be admin")
	}
}

func TestRequireAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RequireAdmin([]string{"u-admin"})(next)

	// admin → 200
	req := httptest.NewRequest(http.MethodPut, "/x", nil).
		WithContext(context.WithValue(context.Background(), contextKeyUserID, "u-admin"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: status = %d, want 200", rr.Code)
	}

	// non-admin → 403 FORBIDDEN
	req = httptest.NewRequest(http.MethodPut, "/x", nil).
		WithContext(context.WithValue(context.Background(), contextKeyUserID, "u-other"))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-admin: status = %d, want 403", rr.Code)
	}
}

type fakeLookup struct {
	changedAt time.Time
	err       error
}

func (f fakeLookup) PasswordChangedAt(_ context.Context, _ string) (time.Time, error) {
	return f.changedAt, f.err
}

// signTestToken mints an HS256 token for userID with the given issued-at.
// ExpiresAt is anchored to time.Now() (not issuedAt) so tokens with an
// issuedAt already hours in the past aren't already expired by the time the
// test parses them — these tests are exercising the revocation check, not
// standard expiry.
func signTestToken(t *testing.T, secret, userID string, issuedAt time.Time) string {
	t.Helper()
	claims := &Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func TestAuthenticate_RevokesTokenIssuedBeforePasswordChange(t *testing.T) {
	secret := "secret"
	issuedAt := time.Now().Add(-2 * time.Hour)
	changedAt := time.Now().Add(-1 * time.Hour) // password changed AFTER token issued
	token := signTestToken(t, secret, "u1", issuedAt)

	var reached bool
	h := Authenticate(secret, fakeLookup{changedAt: changedAt})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestWithToken(token))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (revoked)", w.Code)
	}
	if reached {
		t.Error("handler was reached for a revoked token")
	}
}

func TestAuthenticate_AllowsTokenIssuedAfterPasswordChange(t *testing.T) {
	secret := "secret"
	changedAt := time.Now().Add(-2 * time.Hour)
	issuedAt := time.Now().Add(-1 * time.Hour) // token newer than the change
	token := signTestToken(t, secret, "u1", issuedAt)

	var reached bool
	h := Authenticate(secret, fakeLookup{changedAt: changedAt})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestWithToken(token))

	if !reached || w.Code == http.StatusUnauthorized {
		t.Fatalf("token should be allowed; reached=%v status=%d", reached, w.Code)
	}
}

func TestAuthenticate_ZeroPasswordChangedAt_Allows(t *testing.T) {
	secret := "secret"
	token := signTestToken(t, secret, "u1", time.Now().Add(-time.Hour))
	var reached bool
	h := Authenticate(secret, fakeLookup{})( // zero changedAt
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestWithToken(token))
	if !reached {
		t.Errorf("token should be allowed when passwordChangedAt is zero; status=%d", w.Code)
	}
}

func TestAuthenticate_LookupError_FailsOpen(t *testing.T) {
	secret := "secret"
	token := signTestToken(t, secret, "u1", time.Now().Add(-time.Hour))
	var reached bool
	h := Authenticate(secret, fakeLookup{err: context.DeadlineExceeded})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, requestWithToken(token))
	if !reached {
		t.Errorf("lookup error should fail open (allow); status=%d", w.Code)
	}
}
