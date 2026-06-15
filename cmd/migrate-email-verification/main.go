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

// Backfills email verification state on accounts created before verification
// shipped. Existing users become {emailVerified:false, legacyUnverified:true} so
// they stay exempt from the hard gate (soft prompt) while only new signups are
// gated. Idempotent: only users missing the emailVerified field are touched.
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
	res, err := col.UpdateMany(ctx,
		bson.M{"emailVerified": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"emailVerified": false, "legacyUnverified": true}},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("backfilled email verification on %d users\n", res.ModifiedCount)
}
