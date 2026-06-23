package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/catalog"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/initiativehub"
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
	contentRequestStore := store.NewContentRequestStore(db)
	catalogImageStore := store.NewCatalogImageStore(db)
	locationStore := store.NewLocationStore(db)
	npcStore := store.NewNPCStore(db)
	mapPinStore := store.NewMapPinStore(db)
	initiativeStore := store.NewInitiativeStore(db)
	initiativeBroadcast := initiativehub.New()

	// Avatar storage (S3). Unconfigured in local dev — uploads disabled,
	// avatarUri pass-through on Player reads.
	avatars := avatarstore.New(cfg)

	// Catalog
	arsenalCatalog, err := catalog.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load arsenal catalog: %v", err))
	}

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, cfg.JWTSecret, cfg.AdminUserIDs)
	campaignHandler := handler.NewCampaignHandler(campaignStore, playerStore, userStore, db, avatars)
	sessionHandler := handler.NewSessionHandler(campaignStore)
	sessionNotesHandler := handler.NewSessionNotesHandler(campaignStore)
	sessionScheduleHandler := handler.NewSessionScheduleHandler(campaignStore)
	playerHandler := handler.NewPlayerHandler(playerStore, campaignStore, userStore, avatars)
	spellHandler := handler.NewSpellHandler(playerStore, campaignStore, arsenalCatalog, customSpellStore)
	inventoryHandler := handler.NewInventoryHandler(playerStore, campaignStore, arsenalCatalog, customEquipmentStore)
	arsenalHandler := handler.NewArsenalHandler(arsenalCatalog, catalogImageStore, avatars)
	customEquipmentHandler := handler.NewCustomEquipmentHandler(customEquipmentStore, playerStore, campaignStore, avatars)
	customSpellHandler := handler.NewCustomSpellHandler(customSpellStore, playerStore, campaignStore, avatars)
	contentRequestHandler := handler.NewContentRequestHandler(contentRequestStore, customEquipmentStore, customSpellStore, playerStore, campaignStore, avatars)
	locationHandler := handler.NewLocationHandler(locationStore, npcStore, mapPinStore, campaignStore, avatars)
	npcHandler := handler.NewNPCHandler(npcStore, locationStore, mapPinStore, campaignStore, avatars)
	mapPinHandler := handler.NewMapPinHandler(mapPinStore, locationStore, npcStore, campaignStore)
	initiativeHandler := handler.NewInitiativeHandler(initiativeStore, playerStore, campaignStore, initiativeBroadcast)
	uploadsHandler := handler.NewUploadsHandler(avatars, playerStore, campaignStore, arsenalCatalog, cfg.AdminUserIDs)

	r.Route("/api", func(r chi.Router) {
		// Public
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected (JWT required)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Account
			r.Post("/auth/change-password", authHandler.ChangePassword)

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

				// Session schedule sub-resources
				r.Put("/{sessionId}/availability", sessionScheduleHandler.PutAvailability)
				r.Delete("/{sessionId}/availability", sessionScheduleHandler.DeleteAvailability)
				r.Put("/{sessionId}/confirmed-slot", sessionScheduleHandler.PutConfirmedSlot)
				r.Delete("/{sessionId}/confirmed-slot", sessionScheduleHandler.DeleteConfirmedSlot)
			})

			// Per-user session notes (any active member can write; any member can read).
			r.Get("/campaigns/{campaignId}/my-session-notes", sessionNotesHandler.GetMine)
			r.Put("/campaigns/{campaignId}/sessions/{sessionId}/my-notes", sessionNotesHandler.PutMine)

			// Uploads (presigned S3 URLs)
			r.Post("/uploads/url", uploadsHandler.CreateURL)

			// Players
			r.Get("/campaigns/{campaignId}/player", playerHandler.GetMyPlayer)
			r.Get("/campaigns/{campaignId}/players", playerHandler.ListForCampaign)
			r.Get("/campaigns/{campaignId}/roster", playerHandler.Roster)
			r.Post("/players", playerHandler.Create)
			r.Post("/campaigns/{campaignId}/players", playerHandler.CreateNPC)
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

			// Admin-only: arsenal catalog images (env allowlist)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAdmin(cfg.AdminUserIDs))
				r.Put("/admin/arsenal/equipment/{equipmentId}/image", arsenalHandler.SetEquipmentImage)
				r.Delete("/admin/arsenal/equipment/{equipmentId}/image", arsenalHandler.DeleteEquipmentImage)
				r.Put("/admin/arsenal/spells/{spellId}/image", arsenalHandler.SetSpellImage)
				r.Delete("/admin/arsenal/spells/{spellId}/image", arsenalHandler.DeleteSpellImage)
			})

			// Per-campaign custom equipment (homebrew)
			r.Route("/campaigns/{campaignId}/custom-equipment", func(r chi.Router) {
				r.Get("/", customEquipmentHandler.List)
				r.Get("/usage", customEquipmentHandler.Usage)
				r.Post("/", customEquipmentHandler.Create)
				r.Patch("/{id}", customEquipmentHandler.Update)
				r.Delete("/{id}", customEquipmentHandler.Delete)
			})

			// Per-campaign custom spells (homebrew)
			r.Route("/campaigns/{campaignId}/custom-spells", func(r chi.Router) {
				r.Get("/", customSpellHandler.List)
				r.Get("/usage", customSpellHandler.Usage)
				r.Post("/", customSpellHandler.Create)
				r.Patch("/{id}", customSpellHandler.Update)
				r.Delete("/{id}", customSpellHandler.Delete)
			})

			// Per-campaign DM-moderated content requests (proposals + history)
			r.Route("/campaigns/{campaignId}/content-requests", func(r chi.Router) {
				r.Get("/mine", contentRequestHandler.Mine)
				r.Get("/pending", contentRequestHandler.Pending)
				r.Get("/pending-count", contentRequestHandler.PendingCount)
				r.Post("/", contentRequestHandler.Create)
				r.Delete("/{id}", contentRequestHandler.Withdraw)
				r.Patch("/{id}", contentRequestHandler.EditPending)
				r.Post("/{id}/approve", contentRequestHandler.Approve)
				r.Post("/{id}/deny", contentRequestHandler.Deny)
			})

			// Per-campaign locations
			r.Route("/campaigns/{campaignId}/locations", func(r chi.Router) {
				r.Get("/", locationHandler.List)
				r.Post("/", locationHandler.Create)
				r.Patch("/{id}", locationHandler.Update)
				r.Delete("/{id}", locationHandler.Delete)
				r.Post("/{id}/share", locationHandler.Share)
			})

			// Per-campaign NPCs
			r.Route("/campaigns/{campaignId}/npcs", func(r chi.Router) {
				r.Get("/", npcHandler.List)
				r.Post("/", npcHandler.Create)
				r.Patch("/{id}", npcHandler.Update)
				r.Delete("/{id}", npcHandler.Delete)
				r.Post("/{id}/share", npcHandler.Share)
			})

			// Per-campaign initiative tracker
			r.Route("/campaigns/{campaignId}/initiative", func(r chi.Router) {
				r.Get("/", initiativeHandler.Get)
				r.Get("/stream", initiativeHandler.Stream)
				r.Post("/", initiativeHandler.Start)
				r.Post("/submit", initiativeHandler.Submit)
				r.Post("/enemies", initiativeHandler.Enemy)
				r.Delete("/enemies/{participantId}", initiativeHandler.RemoveEnemy)
				r.Post("/participants/{participantId}/skip", initiativeHandler.Skip)
				r.Post("/end-turn", initiativeHandler.EndTurn)
				r.Post("/resolve", initiativeHandler.Resolve)
			})

			// Per-campaign map pins
			r.Route("/campaigns/{campaignId}/map-pins", func(r chi.Router) {
				r.Get("/", mapPinHandler.List)
				r.Post("/", mapPinHandler.Create)
				r.Patch("/{id}", mapPinHandler.Update)
				r.Delete("/{id}", mapPinHandler.Delete)
				r.Post("/{id}/share", mapPinHandler.Share)
			})
		})
	})

	// Health check (Railway uses this)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}
