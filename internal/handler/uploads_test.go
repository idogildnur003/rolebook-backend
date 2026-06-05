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
	h := NewUploadsHandler(avatars, nil, nil, nil, nil)

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

// configured store + nil sub-stores reaches the kind switch in CreateURL.
// We're testing the routing/validation that runs BEFORE the stores are touched.
func newConfiguredUploadsHandler() *UploadsHandler {
	avatars := avatarstore.New(config.Config{
		AWSS3Endpoint:      "https://s3.example.com",
		AWSAccessKeyID:     "a",
		AWSSecretAccessKey: "s",
		AWSRegion:          "r",
		AWSS3Bucket:        "b",
	})
	return NewUploadsHandler(avatars, nil, nil, nil, nil)
}

func TestUploadsCreateURL_UnsupportedKind(t *testing.T) {
	h := newConfiguredUploadsHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/url",
		strings.NewReader(`{"kind":"wat","contentType":"image/png"}`))
	rr := httptest.NewRecorder()
	h.CreateURL(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unsupported upload kind") {
		t.Fatalf("body missing kind error: %q", rr.Body.String())
	}
}

func TestUploadsCreateURL_RejectsCampaignKindWithoutCampaignID(t *testing.T) {
	h := newConfiguredUploadsHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/url",
		strings.NewReader(`{"kind":"map","contentType":"image/png"}`))
	rr := httptest.NewRecorder()
	h.CreateURL(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "campaignId is required") {
		t.Fatalf("body missing campaignId error: %q", rr.Body.String())
	}
}

func TestUploadsCreateURL_RejectsBadContentType(t *testing.T) {
	h := newConfiguredUploadsHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/url",
		strings.NewReader(`{"kind":"player-avatar","playerId":"p1","contentType":"image/gif"}`))
	rr := httptest.NewRecorder()
	h.CreateURL(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "contentType must be") {
		t.Fatalf("body missing contentType error: %q", rr.Body.String())
	}
}

func TestUploadsCreateURL_ArsenalKindForbiddenForNonAdmin(t *testing.T) {
	h := newConfiguredUploadsHandler() // adminIDs = nil → nobody is admin
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/url",
		strings.NewReader(`{"kind":"arsenal-equipment","itemId":"longsword","contentType":"image/png"}`))
	// no userID in context → not admin
	rr := httptest.NewRecorder()
	h.CreateURL(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}
