package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	// Embed the IANA timezone database in the binary so time.LoadLocation
	// resolves zones like "Asia/Jerusalem" on the CGO-disabled alpine runtime,
	// which ships no /usr/share/zoneinfo. Without this, every availability /
	// confirmed-slot write with an X-Timezone header 400s (INVALID_TIMEZONE).
	_ "time/tzdata"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/store"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Timezone")
		// Let the browser cache the preflight result so it doesn't send an
		// OPTIONS before every authenticated request. Without this, each
		// cross-origin call costs two requests (preflight + actual), which over
		// HTTP/1.1 (dev) exhausts the ~6-connections-per-origin pool and makes
		// requests stall. Chrome caps this at 7200s (2h); larger values are
		// clamped, not rejected.
		w.Header().Set("Access-Control-Max-Age", "7200")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := store.NewDB(cfg.MongoURI)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := db.Disconnect(context.Background()); err != nil {
			log.Printf("error disconnecting from MongoDB: %v", err)
		}
	}()

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(corsMiddleware)

	// Routes registered in subsequent tasks via registerRoutes
	registerRoutes(r, cfg, db)

	// baseCtx is the parent of every request's context. Cancelling it on
	// shutdown trips r.Context().Done() in all in-flight handlers — notably the
	// long-lived initiative SSE stream — so they return promptly instead of
	// holding the connection "active" until srv.Shutdown hits its deadline.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	srv := &http.Server{
		Addr:        ":" + cfg.Port,
		Handler:     r,
		BaseContext: func(net.Listener) context.Context { return baseCtx },
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Cancel in-flight request contexts first so streaming handlers exit and
	// their connections go idle, letting Shutdown drain in milliseconds.
	cancelBase()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// registerRoutes is populated incrementally as handlers are added.
// It lives in cmd/server/routes.go (created in Task 11).
