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

// SpellStore handles persistence for player spells.
type SpellStore struct {
	col *mongo.Collection
}

// NewSpellStore creates a SpellStore and ensures the playerId index.
func NewSpellStore(db *DB) *SpellStore {
	col := db.Collection("spells")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "playerId", Value: 1}},
	})
	return &SpellStore{col: col}
}

// ListForPlayer returns all spells for a player.
func (s *SpellStore) ListForPlayer(ctx context.Context, playerID, userID string, isDM bool) ([]model.Spell, error) {
	filter := bson.M{"playerId": playerID}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var spells []model.Spell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.Spell{}
	}
	return spells, nil
}

// Create inserts a new spell.
func (s *SpellStore) Create(ctx context.Context, spell *model.Spell) error {
	_, err := s.col.InsertOne(ctx, spell)
	return err
}

// Update applies a partial $set update to a spell.
func (s *SpellStore) Update(ctx context.Context, id, userID string, isDM bool, fields bson.M) (*model.Spell, error) {
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
	var spell model.Spell
	if err := res.Decode(&spell); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &spell, nil
}

// Delete removes a spell by ID.
func (s *SpellStore) Delete(ctx context.Context, id, userID string, isDM bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	return res.DeletedCount > 0, err
}

// DeleteByPlayerID removes all spells for a player (cascade delete).
func (s *SpellStore) DeleteByPlayerID(ctx context.Context, playerID string) error {
	_, err := s.col.DeleteMany(ctx, bson.M{"playerId": playerID})
	return err
}
