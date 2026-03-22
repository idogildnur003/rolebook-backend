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

// PlayerStore handles persistence for player characters.
type PlayerStore struct {
	col      *mongo.Collection
	invStore *InventoryStore // set after inventory store is created (wired in routes)
	spStore  *SpellStore     // set after spell store is created (wired in routes)
}

// NewPlayerStore creates a PlayerStore and ensures required indexes.
func NewPlayerStore(db *DB) *PlayerStore {
	col := db.Collection("players")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
		{Keys: bson.D{{Key: "linkedUserId", Value: 1}}},
	})
	return &PlayerStore{col: col}
}

// SetInventoryStore links the inventory store for cascade deletes.
func (s *PlayerStore) SetInventoryStore(inv *InventoryStore) { s.invStore = inv }

// SetSpellStore links the spell store for cascade deletes.
func (s *PlayerStore) SetSpellStore(sp *SpellStore) { s.spStore = sp }

// Create inserts a new player.
func (s *PlayerStore) Create(ctx context.Context, p *model.Player) error {
	_, err := s.col.InsertOne(ctx, p)
	return err
}

// Get returns a player by ID for the given role/userID.
// DM: no ownership filter. Non-DM: filters by linkedUserId.
func (s *PlayerStore) Get(ctx context.Context, id, userID string, isDM bool) (*model.Player, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	var p model.Player
	err := s.col.FindOne(ctx, filter).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListForCampaign returns players in a campaign.
// DM: all players. Non-DM: only the player whose linkedUserId matches.
func (s *PlayerStore) ListForCampaign(ctx context.Context, campaignID, userID string, isDM bool) ([]model.Player, error) {
	filter := bson.M{"campaignId": campaignID}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var players []model.Player
	if err := cursor.All(ctx, &players); err != nil {
		return nil, err
	}
	if players == nil {
		players = []model.Player{}
	}
	return players, nil
}

// Update applies a partial $set update and returns the updated player.
// Protected fields (campaignId, linkedUserId) must be stripped by the handler before calling.
// DM: no ownership filter. Non-DM: filters by linkedUserId.
func (s *PlayerStore) Update(ctx context.Context, id, userID string, isDM bool, fields bson.M) (*model.Player, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var p model.Player
	if err := res.Decode(&p); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Delete removes a player and cascades to their inventory and spells.
func (s *PlayerStore) Delete(ctx context.Context, id, userID string, isDM bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isDM {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return false, err
	}
	// Cascade
	if s.invStore != nil {
		_ = s.invStore.DeleteByPlayerID(ctx, id)
	}
	if s.spStore != nil {
		_ = s.spStore.DeleteByPlayerID(ctx, id)
	}
	return true, nil
}

// DeleteByIDs deletes multiple players (used by campaign cascade delete).
func (s *PlayerStore) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if s.invStore != nil {
			_ = s.invStore.DeleteByPlayerID(ctx, id)
		}
		if s.spStore != nil {
			_ = s.spStore.DeleteByPlayerID(ctx, id)
		}
	}
	_, err := s.col.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}

// IDsForCampaign returns all player IDs in a campaign (used by campaign cascade delete).
func (s *PlayerStore) IDsForCampaign(ctx context.Context, campaignID string) ([]string, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var results []struct {
		ID string `bson:"_id"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	return ids, nil
}

// CampaignIDsForUser returns all campaignIds for players linked to the given user.
func (s *PlayerStore) CampaignIDsForUser(ctx context.Context, userID string) ([]string, error) {
	cursor, err := s.col.Find(
		ctx,
		bson.M{"linkedUserId": userID},
		options.Find().SetProjection(bson.M{"campaignId": 1}),
	)
	if err != nil {
		return nil, err
	}
	var results []struct {
		CampaignID string `bson:"campaignId"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.CampaignID
	}
	return ids, nil
}

// UserHasPlayerInCampaign returns true if the user has a character in the given campaign.
func (s *PlayerStore) UserHasPlayerInCampaign(ctx context.Context, userID, campaignID string) (bool, error) {
	count, err := s.col.CountDocuments(ctx, bson.M{"campaignId": campaignID, "linkedUserId": userID})
	return count > 0, err
}
