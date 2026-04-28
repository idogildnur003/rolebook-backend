// Command migrate-members rewrites every campaign document into the new
// unified-membership shape. Idempotent: safe to run multiple times.
//
// Per campaign:
//  1. Skip if already migrated.
//  2. Look up the DM user's email; derive a display name.
//  3. Reuse an existing kind:"dm" Player record for this campaign if one
//     exists (orphan from an interrupted run); otherwise create a new one.
//  4. Set members[] on the campaign and unset the legacy dm and players fields.
package main

import (
	stderrors "errors"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/migrate"
	"github.com/elad/rolebook-backend/internal/store"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := store.NewDB(cfg.MongoURI)
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = db.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	campaigns := db.Collection("campaigns")
	players := db.Collection("players")
	users := db.Collection("users")

	cursor, err := campaigns.Find(ctx, bson.M{})
	if err != nil {
		log.Fatalf("list campaigns: %v", err)
	}

	var (
		scanned, migrated, alreadyMigrated, reused int
		errors                                     []string
	)

	for cursor.Next(ctx) {
		scanned++
		var legacy migrate.LegacyCampaign
		if err := cursor.Decode(&legacy); err != nil {
			errors = append(errors, fmt.Sprintf("decode: %v", err))
			continue
		}

		// 1) Look up DM display name (best-effort; falls back to "DM").
		displayName := "DM"
		if legacy.DM != "" {
			var u struct {
				Email string `bson:"email"`
			}
			if err := users.FindOne(ctx, bson.M{"_id": legacy.DM}).Decode(&u); err == nil && u.Email != "" {
				displayName = handler.DisplayNameFromEmailExported(u.Email)
			}
		}

		// 2) Look for an orphan DM Player in this campaign (interrupted run).
		// Distinguish "no orphan exists" (proceed with a new ID) from a real
		// driver/connection error (record and skip — silent fall-through would
		// create a duplicate DM Player on a retry).
		var orphanDMID string
		var orphan struct {
			ID string `bson:"_id"`
		}
		lookupErr := players.FindOne(ctx, bson.M{
			"campaignId": legacy.ID,
			"kind":       "dm",
		}).Decode(&orphan)
		switch {
		case lookupErr == nil:
			orphanDMID = orphan.ID
		case stderrors.Is(lookupErr, mongo.ErrNoDocuments):
			// no orphan; proceed with a fresh ID
		default:
			errors = append(errors, fmt.Sprintf("%s: lookup orphan dm player: %v", legacy.ID, lookupErr))
			continue
		}

		newPlayerID := uuid.NewString()
		plan := migrate.PlanCampaign(legacy, displayName, orphanDMID, newPlayerID)

		switch plan.Status {
		case migrate.StatusAlreadyMigrated:
			alreadyMigrated++
			continue

		case migrate.StatusMigrate, migrate.StatusMigrateReuseOrphan:
			// Insert (or replace if reusing) the DM Player.
			if plan.Status == migrate.StatusMigrate {
				if _, err := players.InsertOne(ctx, plan.DMPlayer); err != nil {
					errors = append(errors, fmt.Sprintf("%s: insert dm player: %v", legacy.ID, err))
					continue
				}
			} else {
				// Reuse: replace the orphan with a fully-populated record so its
				// fields match what a fresh insert would have produced.
				if _, err := players.ReplaceOne(ctx, bson.M{"_id": plan.DMPlayer.ID}, plan.DMPlayer); err != nil {
					errors = append(errors, fmt.Sprintf("%s: replace orphan dm player: %v", legacy.ID, err))
					continue
				}
				reused++
			}

			// Update the campaign: set members, unset legacy dm + players.
			update := bson.M{
				"$set":   bson.M{"members": plan.NewMembers, "updatedAt": time.Now().UTC()},
				"$unset": bson.M{"dm": "", "players": ""},
			}
			if _, err := campaigns.UpdateOne(ctx, bson.M{"_id": legacy.ID}, update); err != nil {
				errors = append(errors, fmt.Sprintf("%s: update campaign: %v", legacy.ID, err))
				// Compensate: delete the just-inserted DM player so a retry sees a clean slate.
				if plan.Status == migrate.StatusMigrate {
					_, _ = players.DeleteOne(ctx, bson.M{"_id": plan.DMPlayer.ID})
				}
				continue
			}
			migrated++
		}
	}
	if err := cursor.Err(); err != nil {
		log.Fatalf("cursor error: %v", err)
	}

	fmt.Printf("scanned=%d migrated=%d already=%d reused=%d errors=%d\n", scanned, migrated, alreadyMigrated, reused, len(errors))
	for _, e := range errors {
		fmt.Fprintln(os.Stderr, "ERROR:", e)
	}
	if len(errors) > 0 {
		os.Exit(1)
	}
}
