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

// Converts each campaign member's sessionNotes entries from a bare string
// (legacy) into a {text, direction:"ltr"} document. Idempotent: entries that
// are already documents are left untouched.
//
// Decoding into bson.M yields bson.D for nested documents (members and the
// sessionNotes map), so we navigate bson.D, falling back to bson.M defensively.
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

	col := client.Database("rolebook").Collection("campaigns")
	cur, err := col.Find(ctx, bson.M{})
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)

	migrated := 0
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			log.Fatal(err)
		}
		members, ok := doc["members"].(bson.A)
		if !ok {
			continue
		}
		changed := false
		for ci := range members {
			notesRaw, present := memberField(members[ci], "sessionNotes")
			if !present {
				continue
			}
			if upgradeNotes(notesRaw) {
				changed = true
			}
		}
		if !changed {
			continue
		}
		if _, err := col.UpdateByID(ctx, doc["_id"], bson.M{"$set": bson.M{"members": members}}); err != nil {
			log.Fatal(err)
		}
		migrated++
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("migrated session-note direction on %d campaigns\n", migrated)
}

// memberField returns the value for key on a member document (bson.D or bson.M).
func memberField(member any, key string) (any, bool) {
	switch m := member.(type) {
	case bson.D:
		for i := range m {
			if m[i].Key == key {
				return m[i].Value, true
			}
		}
	case bson.M:
		v, ok := m[key]
		return v, ok
	}
	return nil, false
}

// upgradeNotes rewrites any bare-string entries in a sessionNotes map
// (bson.D or bson.M) to {text, direction:"ltr"} in place. Returns whether
// anything changed. bson.D/bson.M are mutated through their shared backing
// store, so the change is visible to the enclosing members array.
func upgradeNotes(notes any) bool {
	changed := false
	switch n := notes.(type) {
	case bson.D:
		for i := range n {
			if s, ok := n[i].Value.(string); ok {
				n[i].Value = bson.D{{Key: "text", Value: s}, {Key: "direction", Value: "ltr"}}
				changed = true
			}
		}
	case bson.M:
		for k, v := range n {
			if s, ok := v.(string); ok {
				n[k] = bson.D{{Key: "text", Value: s}, {Key: "direction", Value: "ltr"}}
				changed = true
			}
		}
	}
	return changed
}
