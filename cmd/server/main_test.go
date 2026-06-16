package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A preflight OPTIONS must carry Access-Control-Max-Age so the browser caches
// the result and stops sending an OPTIONS before every authenticated request.
func TestCorsMiddleware_PreflightSetsMaxAge(t *testing.T) {
	nextCalled := false
	h := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/players/abc", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Max-Age"); got != "7200" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", got, "7200")
	}
	if nextCalled {
		t.Errorf("preflight should short-circuit and not call next handler")
	}
}

// Non-OPTIONS requests still pass through to the next handler with CORS headers.
func TestCorsMiddleware_PassThrough(t *testing.T) {
	h := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/players/abc", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}
