package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
)

// Test 503 short-circuit when the avatar store is unconfigured. The handler
// must return early without touching the player store.
func TestUploadsCreateURL_ServiceUnavailableWhenUnconfigured(t *testing.T) {
	avatars := avatarstore.New(config.Config{})
	h := NewUploadsHandler(avatars, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/uploads/url",
		strings.NewReader(`{"kind":"player-avatar","playerId":"p1","contentType":"image/png"}`))
	rr := httptest.NewRecorder()
	h.CreateURL(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "UPLOAD_NOT_CONFIGURED") {
		t.Fatalf("body missing UPLOAD_NOT_CONFIGURED code: %q", rr.Body.String())
	}
}
