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

// NpcStore handles persistence for non-player characters and monsters.
type NpcStore struct {
	col *mongo.Collection
}

// NewNpcStore creates a NpcStore and ensures required indexes.
func NewNpcStore(db *DB) *NpcStore {
	col := db.Collection("npcs")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
	})
	return &NpcStore{col: col}
}

// Create inserts a new NPC.
func (s *NpcStore) Create(ctx context.Context, n *model.NPC) error {
	_, err := s.col.InsertOne(ctx, n)
	return err
}

// GetByID returns an NPC by ID.
func (s *NpcStore) GetByID(ctx context.Context, id string) (*model.NPC, error) {
	var n model.NPC
	err := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&n)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListByCampaign returns all NPCs for a given campaign.
func (s *NpcStore) ListByCampaign(ctx context.Context, campaignID string) ([]model.NPC, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID})
	if err != nil {
		return nil, err
	}
	var npcs []model.NPC
	if err := cursor.All(ctx, &npcs); err != nil {
		return nil, err
	}
	if npcs == nil {
		npcs = []model.NPC{}
	}
	return npcs, nil
}

// Update applies a partial $set update and returns the updated NPC.
func (s *NpcStore) Update(ctx context.Context, id string, fields bson.M) (*model.NPC, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var n model.NPC
	if err := res.Decode(&n); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

// Delete removes an NPC by ID.
func (s *NpcStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil || res.DeletedCount == 0 {
		return false, err
	}
	return true, nil
}
