package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/activity"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

// publicAuthRoutes run before authentication, so they have no actor to log.
var publicAuthRoutes = map[string]bool{
	"POST /api/auth/register":            true,
	"POST /api/auth/login":               true,
	"POST /api/auth/verify-email":        true,
	"POST /api/auth/resend-verification": true,
	"POST /api/auth/forgot-password":     true,
	"POST /api/auth/verify-reset-code":   true,
	"POST /api/auth/reset-password":      true,
}

func mutatingRouteKeys(t *testing.T) map[string]bool {
	t.Helper()
	// An unreachable Mongo: the driver connects lazily and index creation fails fast.
	db, err := store.NewDB("mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Disconnect(context.Background()) })
	r := chi.NewRouter()
	registerRoutes(r, config.Config{}, db, &middleware.RateLimiters{})

	keys := map[string]bool{}
	err = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			route = strings.ReplaceAll(route, "/*/", "/")
			if key := activity.RouteKey(method, route); !publicAuthRoutes[key] {
				keys[key] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestEveryMutatingRouteIsRegistered(t *testing.T) {
	var missing []string
	for key := range mutatingRouteKeys(t) {
		if _, ok := activity.Registry[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%d mutating routes missing from activity.Registry:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func TestRegistryHasNoStaleRoutes(t *testing.T) {
	keys := mutatingRouteKeys(t)
	var stale []string
	for key := range activity.Registry {
		if !keys[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("registry routes that no longer exist:\n%s", strings.Join(stale, "\n"))
	}
}
