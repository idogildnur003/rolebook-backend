package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

// TestRegisterRoutes_RateLimitWiring exercises the REAL route tree from
// registerRoutes. The limiter middleware runs before any handler, so we observe
// 429s without a live database (mongo.Connect is lazy and never pinged; a tiny
// serverSelectionTimeout keeps the never-reached handler paths from stalling).
//
// It locks two wiring decisions that the middleware unit tests can't see:
//   - /health is NOT rate limited (it's on the root mux, outside /api).
//   - the tight auth limiter fronts the public auth routes.
func TestRegisterRoutes_RateLimitWiring(t *testing.T) {
	db, err := store.NewDB("mongodb://127.0.0.1:27017/?serverSelectionTimeoutMS=200")
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}

	cfg := config.Config{JWTSecret: "test-secret"}
	rl := middleware.NewRateLimiters(middleware.RateLimitOptions{
		Enabled:        true,
		GlobalRequests: 1000, GlobalWindow: time.Minute,
		UserRequests:   1000, UserWindow: time.Minute,
		AuthRequests:   3, AuthWindow: time.Minute,
	})
	defer rl.Close()

	r := chi.NewRouter()
	registerRoutes(r, cfg, db, rl)

	call := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "10.0.0.1:1000"
		req.Header.Set("X-Envoy-External-Address", "203.0.113.7")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr.Code
	}

	// /health is never throttled (Railway health checks must not 429).
	for i := 0; i < 8; i++ {
		if code := call(http.MethodGet, "/health"); code != http.StatusOK {
			t.Fatalf("/health request %d = %d, want 200 (health must be excluded from limiting)", i+1, code)
		}
	}

	// Public auth route: tight limit is 3, so the 4th+ requests are 429 from the
	// limiter, ahead of the (never-reached) DB-backed login handler.
	var last int
	limited := false
	for i := 0; i < 6; i++ {
		last = call(http.MethodPost, "/api/auth/login")
		if i >= 3 && last == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatalf("auth route not rate limited after 3 requests; last status = %d", last)
	}
}
