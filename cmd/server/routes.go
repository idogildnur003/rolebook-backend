package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/store"
)

func registerRoutes(r *chi.Mux, cfg config.Config, db *store.DB) {
	// routes wired incrementally in subsequent tasks
	_ = r
	_ = cfg
	_ = db
}
