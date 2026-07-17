package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestKeyByTrustedIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		realIP     string
		xff        string
		want       string
	}{
		{"x-real-ip wins over xff", "10.0.0.1:1234", "203.0.113.5", "1.2.3.4, 9.9.9.9", "203.0.113.5"},
		{"left-most xff when no x-real-ip", "10.0.0.1:1234", "", "203.0.113.5, 152.99.99.99", "203.0.113.5"},
		{"single xff", "10.0.0.1:1234", "", "203.0.113.5", "203.0.113.5"},
		{"remoteaddr fallback", "203.0.113.9:5555", "", "", "203.0.113.9"},
		{"x-real-ip with port", "10.0.0.1:1234", "203.0.113.5:443", "", "203.0.113.5"},
		// IPv6 clients are bucketed to their /64 so they can't rotate within it.
		{"ipv6 bucketed to /64", "10.0.0.1:1234", "2001:db8:abcd:1234::1", "", "2001:db8:abcd:1234::"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			got, err := KeyByTrustedIP(r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("KeyByTrustedIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// fire sends n requests through h, applying decorate to each, and returns the
// status codes. RemoteAddr is a constant proxy address (as in production).
func fire(t *testing.T, h http.Handler, n int, decorate func(*http.Request)) []int {
	t.Helper()
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "10.0.0.1:1000"
		if decorate != nil {
			decorate(r)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes[i] = w.Code
	}
	return codes
}

// realip sets X-Real-IP, which is how Railway's edge proxy conveys the true
// client IP to the app (see KeyByTrustedIP).
func realip(ip string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("X-Real-IP", ip) }
}

func TestRateLimiter_GlobalOverLimit(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{
		Enabled: true, GlobalRequests: 3, GlobalWindow: time.Minute,
		UserRequests: 3, UserWindow: time.Minute, AuthRequests: 3, AuthWindow: time.Minute,
	})
	defer rl.Close()

	// Same client IP: exactly GlobalRequests pass, the next is limited.
	codes := fire(t, rl.Global(okHandler), 4, realip("203.0.113.5"))
	want := []int{200, 200, 200, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("request %d: got %d, want %d (all=%v)", i+1, codes[i], want[i], codes)
		}
	}
}

func TestRateLimiter_429ResponseShape(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{
		Enabled: true, GlobalRequests: 100, GlobalWindow: time.Minute,
		UserRequests: 100, UserWindow: time.Minute, AuthRequests: 1, AuthWindow: time.Minute,
	})
	defer rl.Close()
	h := rl.Auth(okHandler)

	_ = fire(t, h, 1, realip("203.0.113.5")) // exhaust (limit 1)

	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:1000"
	r.Header.Set("X-Real-IP", "203.0.113.5")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header not set")
	}
	if body := w.Body.String(); !strings.Contains(body, `"code":"rate_limited"`) {
		t.Errorf("body missing rate_limited code: %s", body)
	}
}

func TestRateLimiter_PerIPIsolation(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{
		Enabled: true, GlobalRequests: 2, GlobalWindow: time.Minute,
		UserRequests: 2, UserWindow: time.Minute, AuthRequests: 2, AuthWindow: time.Minute,
	})
	defer rl.Close()
	h := rl.Global(okHandler)

	a := fire(t, h, 3, realip("203.0.113.1")) // 200,200,429
	if a[2] != http.StatusTooManyRequests {
		t.Fatalf("IP A not limited: %v", a)
	}
	b := fire(t, h, 2, realip("203.0.113.2")) // different IP, own bucket
	if b[0] != http.StatusOK || b[1] != http.StatusOK {
		t.Fatalf("IP B wrongly limited: %v", b)
	}
}

// A malicious client that rotates X-Forwarded-For must not win fresh buckets:
// Railway's X-Real-IP is authoritative, so all requests share one bucket even as
// the client-supplied XFF changes.
func TestRateLimiter_SpoofedXFFCannotBypass(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{
		Enabled: true, GlobalRequests: 2, GlobalWindow: time.Minute,
		UserRequests: 2, UserWindow: time.Minute, AuthRequests: 2, AuthWindow: time.Minute,
	})
	defer rl.Close()
	h := rl.Global(okHandler)

	codes := make([]int, 3)
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.RemoteAddr = "10.0.0.1:1000"
		r.Header.Set("X-Real-IP", "203.0.113.5")                    // constant real client (Railway-set)
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("9.9.9.%d", i)) // attacker rotates
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		codes[i] = w.Code
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Errorf("spoofed X-Forwarded-For bypassed the limiter: %v", codes)
	}
}

func TestRateLimiter_UserKeyingIsolatesUsersOnSameIP(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{
		Enabled: true, GlobalRequests: 100, GlobalWindow: time.Minute,
		UserRequests: 2, UserWindow: time.Minute, AuthRequests: 100, AuthWindow: time.Minute,
	})
	defer rl.Close()
	h := rl.User(okHandler)

	fireUser := func(uid string, n int) []int {
		codes := make([]int, n)
		for i := 0; i < n; i++ {
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			r.RemoteAddr = "10.0.0.1:1000" // same IP for both users
			r = r.WithContext(context.WithValue(r.Context(), contextKeyUserID, uid))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes[i] = w.Code
		}
		return codes
	}

	if a := fireUser("userA", 3); a[0] != 200 || a[1] != 200 || a[2] != 429 {
		t.Errorf("userA codes = %v, want [200 200 429]", a)
	}
	if b := fireUser("userB", 2); b[0] != 200 || b[1] != 200 {
		t.Errorf("userB (same IP) codes = %v, want [200 200]", b)
	}
}

func TestRateLimiter_DisabledPassthrough(t *testing.T) {
	rl := NewRateLimiters(RateLimitOptions{Enabled: false})
	defer rl.Close()
	if rl.Enabled {
		t.Fatal("expected Enabled=false")
	}
	for _, mw := range []func(http.Handler) http.Handler{rl.Global, rl.User, rl.Auth} {
		codes := fire(t, mw(okHandler), 50, realip("203.0.113.5"))
		for i, c := range codes {
			if c != http.StatusOK {
				t.Fatalf("request %d limited while disabled: %d", i, c)
			}
		}
	}
}
