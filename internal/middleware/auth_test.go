package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
