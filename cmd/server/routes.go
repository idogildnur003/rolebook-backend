package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

func registerRoutes(r *chi.Mux, cfg config.Config, db *store.DB) {
	// Stores
	userStore := store.NewUserStore(db)
	campaignStore := store.NewCampaignStore(db)
	playerStore := store.NewPlayerStore(db)
	customEquipmentStore := store.NewCustomEquipmentStore(db)
	customSpellStore := store.NewCustomSpellStore(db)

	// Catalog
	arsenalCatalog, err := catalog.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load arsenal catalog: %v", err))
	}

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, cfg.JWTSecret)
	campaignHandler := handler.NewCampaignHandler(campaignStore, playerStore, userStore, db)
	sessionHandler := handler.NewSessionHandler(campaignStore)
	playerHandler := handler.NewPlayerHandler(playerStore, campaignStore, userStore)
	spellHandler := handler.NewSpellHandler(playerStore, campaignStore, arsenalCatalog, customSpellStore)
	inventoryHandler := handler.NewInventoryHandler(playerStore, campaignStore, arsenalCatalog, customEquipmentStore)
	arsenalHandler := handler.NewArsenalHandler(arsenalCatalog)
	customEquipmentHandler := handler.NewCustomEquipmentHandler(customEquipmentStore, playerStore, campaignStore)
	customSpellHandler := handler.NewCustomSpellHandler(customSpellStore, playerStore, campaignStore)

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
				// Player archive/restore (DM only)
				r.Patch("/players/{playerId}", campaignHandler.SetPlayerActive)
			})

			// Sessions (campaign DM only — enforced in handler)
			r.Route("/campaigns/{campaignId}/sessions", func(r chi.Router) {
				r.Post("/", sessionHandler.Create)
				r.Patch("/{sessionId}", sessionHandler.Update)
				r.Delete("/{sessionId}", sessionHandler.Delete)
			})

			// Players
			r.Get("/campaigns/{campaignId}/player", playerHandler.GetMyPlayer)
			r.Get("/campaigns/{campaignId}/players", playerHandler.ListForCampaign)
			r.Post("/players", playerHandler.Create)
			r.Route("/players/{playerId}", func(r chi.Router) {
				r.Get("/", playerHandler.Get)
				r.Patch("/", playerHandler.Update)
				r.Delete("/", playerHandler.Delete)

				// Spells sub-resource
				r.Get("/spells", spellHandler.List)
				r.Post("/spells", spellHandler.Create)
				r.Patch("/spells/{spellId}", spellHandler.Update)
				r.Delete("/spells/{spellId}", spellHandler.Delete)

				// Inventory sub-resource
				r.Get("/inventory", inventoryHandler.List)
				r.Post("/inventory", inventoryHandler.Create)
				r.Patch("/inventory/{equipmentId}", inventoryHandler.Update)
				r.Delete("/inventory/{equipmentId}", inventoryHandler.Delete)

				// Spell slots
				r.Put("/spell-slots", spellHandler.UpdateSpellSlots)
			})

			// Arsenal (read-only catalog)
			r.Route("/arsenal/spells", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListSpells)
				r.Get("/{spellId}", arsenalHandler.GetSpell)
			})
			r.Route("/arsenal/equipment", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListEquipment)
				r.Get("/{equipmentId}", arsenalHandler.GetEquipment)
			})

			// Per-campaign custom equipment (homebrew)
			r.Route("/campaigns/{campaignId}/custom-equipment", func(r chi.Router) {
				r.Get("/", customEquipmentHandler.List)
				r.Post("/", customEquipmentHandler.Create)
				r.Patch("/{id}", customEquipmentHandler.Update)
				r.Delete("/{id}", customEquipmentHandler.Delete)
			})

			// Per-campaign custom spells (homebrew)
			r.Route("/campaigns/{campaignId}/custom-spells", func(r chi.Router) {
				r.Get("/", customSpellHandler.List)
				r.Post("/", customSpellHandler.Create)
				r.Patch("/{id}", customSpellHandler.Update)
				r.Delete("/{id}", customSpellHandler.Delete)
			})
		})
	})

	// Health check (Railway uses this)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}
