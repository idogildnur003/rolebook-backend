package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// InventoryStore handles persistence for inventory items.
type InventoryStore struct {
	col *mongo.Collection
}

// NewInventoryStore creates an InventoryStore and ensures the playerId index.
func NewInventoryStore(db *DB) *InventoryStore {
	col := db.Collection("inventory")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "playerId", Value: 1}},
	})
	return &InventoryStore{col: col}
}

// ListForPlayer returns all inventory items for a player.
// DM: no linkedUserId filter. Player: requires linkedUserId match.
func (s *InventoryStore) ListForPlayer(ctx context.Context, playerID, userID string, isDM bool) ([]model.InventoryItem, error) {
	filter := bson.M{"playerId": playerID}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var items []model.InventoryItem
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.InventoryItem{}
	}
	return items, nil
}

// Create inserts a new inventory item.
func (s *InventoryStore) Create(ctx context.Context, item *model.InventoryItem) error {
	_, err := s.col.InsertOne(ctx, item)
	return err
}

// Update applies a partial $set update. DM: no linkedUserId filter. Player: requires match.
func (s *InventoryStore) Update(ctx context.Context, id, userID string, isDM bool, fields bson.M) (*model.InventoryItem, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	// updatedAt is set in-place on the caller's map; callers must not reuse fields after this call.
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var item model.InventoryItem
	if err := res.Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// Delete removes an inventory item by ID.
func (s *InventoryStore) Delete(ctx context.Context, id, userID string, isDM bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	return res.DeletedCount > 0, err
}

// DeleteByPlayerID removes all inventory items for a player (cascade delete).
func (s *InventoryStore) DeleteByPlayerID(ctx context.Context, playerID string) error {
	_, err := s.col.DeleteMany(ctx, bson.M{"playerId": playerID})
	return err
}
