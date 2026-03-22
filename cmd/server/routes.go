package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

func registerRoutes(r *chi.Mux, cfg config.Config, db *store.DB) {
	// Stores
	userStore := store.NewUserStore(db)
	campaignStore := store.NewCampaignStore(db)
	playerStore := store.NewPlayerStore(db)
	inventoryStore := store.NewInventoryStore(db)
	spellStore := store.NewSpellStore(db)
	arsenalStore := store.NewArsenalStore(db)

	// Wire cascade dependencies
	playerStore.SetInventoryStore(inventoryStore)
	playerStore.SetSpellStore(spellStore)

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, cfg.JWTSecret)
	campaignHandler := handler.NewCampaignHandler(campaignStore, playerStore)
	sessionHandler := handler.NewSessionHandler(campaignStore)
	playerHandler := handler.NewPlayerHandler(playerStore, campaignStore)
	inventoryHandler := handler.NewInventoryHandler(inventoryStore, playerStore, campaignStore, arsenalStore)
	spellHandler := handler.NewSpellHandler(spellStore, playerStore, campaignStore, arsenalStore)
	arsenalHandler := handler.NewArsenalHandler(arsenalStore)

	dmOnly := middleware.RequireRole(model.RoleDM)

	r.Route("/api", func(r chi.Router) {
		// Public
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected (JWT required)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Campaigns
			r.Get("/campaigns", campaignHandler.List)
			r.Post("/campaigns", campaignHandler.Create)
			r.Route("/campaigns/{id}", func(r chi.Router) {
				r.Get("/", campaignHandler.Get)
				r.Patch("/", campaignHandler.Update)
				r.Delete("/", campaignHandler.Delete)
			})

			// Sessions (campaign DM only — enforced in handler)
			r.Route("/campaigns/{campaignId}/sessions", func(r chi.Router) {
				r.Post("/", sessionHandler.Create)
				r.Patch("/{sessionId}", sessionHandler.Update)
				r.Delete("/{sessionId}", sessionHandler.Delete)
			})

			// Players (campaign DM check enforced in handler)
			r.Get("/campaigns/{campaignId}/players", playerHandler.ListForCampaign)
			r.Post("/players", playerHandler.Create)
			r.Route("/players/{playerId}", func(r chi.Router) {
				r.Get("/", playerHandler.Get)
				r.Patch("/", playerHandler.Update)
				r.Delete("/", playerHandler.Delete)
				// Inventory sub-resource
				r.Get("/inventory", inventoryHandler.List)
				r.Post("/inventory", inventoryHandler.Create)
				// Spells sub-resource
				r.Get("/spells", spellHandler.List)
				r.Post("/spells", spellHandler.Create)
				// Spell slots
				r.Put("/spell-slots", spellHandler.UpdateSpellSlots)
			})

			// Flat inventory routes (access enforced in handler)
			r.Patch("/inventory/{itemId}", inventoryHandler.Update)
			r.Delete("/inventory/{itemId}", inventoryHandler.Delete)

			// Flat spell routes (access enforced in handler)
			r.Patch("/spells/{spellId}", spellHandler.Update)
			r.Delete("/spells/{spellId}", spellHandler.Delete)

			// Arsenal (global DM-only via JWT role — not campaign-specific)
			r.Route("/arsenal/spells", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListSpells)
				r.With(dmOnly).Post("/", arsenalHandler.CreateSpell)
				r.With(dmOnly).Patch("/{id}", arsenalHandler.UpdateSpell)
				r.With(dmOnly).Delete("/{id}", arsenalHandler.DeleteSpell)
			})
			r.Route("/arsenal/equipment", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListEquipment)
				r.With(dmOnly).Post("/", arsenalHandler.CreateEquipment)
				r.With(dmOnly).Patch("/{id}", arsenalHandler.UpdateEquipment)
				r.With(dmOnly).Delete("/{id}", arsenalHandler.DeleteEquipment)
			})
		})
	})

	// Health check (Railway uses this)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}
