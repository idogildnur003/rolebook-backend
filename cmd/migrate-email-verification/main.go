package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Backfills email verification state so accounts that predate verification stay
// exempt from the hard gate (soft prompt) while only new signups are gated, and
// scrubs the obsolete `verificationRequired` field left by earlier code/migrations.
// Idempotent: re-running is a no-op once every account is in the new shape.
//
// It is defensive about every prior state a document might be in:
//   - pre-feature (no emailVerified)         → {emailVerified:false, legacyUnverified:true}
//   - old grandfathered (verificationRequired:false) → set legacyUnverified:true (stay exempt)
//   - any doc still carrying verificationRequired    → drop the dead field
//
// IMPORTANT: the login gate is fail-closed for un-migrated accounts (absent
// legacyUnverified → not exempt → blocked when unverified). Run this migration
// as part of the deploy, before the new server serves traffic, so existing
// unverified users are not locked out.
func main() {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)

	col := client.Database("rolebook").Collection("users")

	// 1. Pre-feature accounts (never had any verification fields): make them
	//    verified=false and exempt from the gate.
	preFeature, err := col.UpdateMany(ctx,
		bson.M{"emailVerified": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"emailVerified": false, "legacyUnverified": true}},
	)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Accounts the OLD migration grandfathered (verificationRequired:false):
	//    keep them exempt under the new flag, and drop the obsolete field.
	grandfathered, err := col.UpdateMany(ctx,
		bson.M{"verificationRequired": false},
		bson.M{
			"$set":   bson.M{"legacyUnverified": true},
			"$unset": bson.M{"verificationRequired": ""},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// 3. Any remaining doc still carrying the dead field (e.g. new signups under
	//    old code, verificationRequired:true): just remove it. emailVerified
	//    already governs their gating.
	scrubbed, err := col.UpdateMany(ctx,
		bson.M{"verificationRequired": bson.M{"$exists": true}},
		bson.M{"$unset": bson.M{"verificationRequired": ""}},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("pre-feature backfilled: %d | grandfathered re-marked: %d | stale verificationRequired scrubbed: %d\n",
		preFeature.ModifiedCount, grandfathered.ModifiedCount, scrubbed.ModifiedCount)
}
