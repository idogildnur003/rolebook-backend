package store

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DB wraps the MongoDB client and database handle.
type DB struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewDB connects to MongoDB and returns a DB handle for the "rolebook" database.
func NewDB(uri string) (*DB, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	return &DB{
		client: client,
		db:     client.Database("rolebook"),
	}, nil
}

// Collection returns a handle for the named collection.
func (d *DB) Collection(name string) *mongo.Collection {
	return d.db.Collection(name)
}

// Rolebook returns the rolebook database handle.
func (d *DB) Rolebook() *mongo.Database {
	return d.client.Database("rolebook")
}

// Disconnect closes the MongoDB connection.
func (d *DB) Disconnect(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}
