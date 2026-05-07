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

func main() {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("MONGO_URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("rolebook")
	res, err := db.Collection("campaigns").UpdateMany(ctx, bson.M{}, bson.M{
		"$unset": bson.M{"mapPins": ""},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("dropped mapPins from %d campaigns\n", res.ModifiedCount)
}
