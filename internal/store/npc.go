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

// NPCStore handles persistence for campaign NPCs.
type NPCStore struct {
	col *mongo.Collection
}

// NewNPCStore creates an NPCStore and ensures the campaignId index exists.
func NewNPCStore(db *DB) *NPCStore {
	col := db.Collection("campaign_npcs")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "ownerPlayerId", Value: 1}}},
	})
	return &NPCStore{col: col}
}

// Create inserts a new NPC.
func (s *NPCStore) Create(ctx context.Context, n *model.NPC) error {
	_, err := s.col.InsertOne(ctx, n)
	return err
}

// GetByID returns an NPC scoped to a campaign. Returns (nil, nil) when not found.
func (s *NPCStore) GetByID(ctx context.Context, campaignID, id string) (*model.NPC, error) {
	var npc model.NPC
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&npc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &npc, nil
}

// ListVisibleToCaller returns NPCs the caller can read: owner-or-shared.
// An NPC is visible when:
//   - ownerPlayerId == callerPlayerId, OR
//   - visibility.sharedWithAll == true, OR
//   - callerPlayerId is in visibility.sharedPlayerIds.
func (s *NPCStore) ListVisibleToCaller(ctx context.Context, campaignID, callerPlayerID string) ([]model.NPC, error) {
	filter := bson.M{
		"campaignId": campaignID,
		"$or": bson.A{
			bson.M{"ownerPlayerId": callerPlayerID},
			bson.M{"visibility.sharedWithAll": true},
			bson.M{"visibility.sharedPlayerIds": callerPlayerID},
		},
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var out []model.NPC
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.NPC{}
	}
	return out, nil
}

// ListByOwner returns all NPCs owned by a specific player. Used by /share
// to look up "do I already have a clone of source X for recipient Y?"
func (s *NPCStore) ListByOwner(ctx context.Context, campaignID, ownerPlayerID string) ([]model.NPC, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID, "ownerPlayerId": ownerPlayerID})
	if err != nil {
		return nil, err
	}
	var out []model.NPC
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.NPC{}
	}
	return out, nil
}

// FindCloneOf returns the existing clone of sourceID owned by ownerPlayerID, if any.
func (s *NPCStore) FindCloneOf(ctx context.Context, campaignID, sourceID, ownerPlayerID string) (*model.NPC, error) {
	var npc model.NPC
	err := s.col.FindOne(ctx, bson.M{
		"campaignId":              campaignID,
		"clone.clonedFromEntryId": sourceID,
		"ownerPlayerId":           ownerPlayerID,
	}).Decode(&npc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &npc, nil
}

// Update applies a partial update scoped by (campaignId, id) and bumps updatedAt.
// Caller is responsible for stripping immutable fields.
func (s *NPCStore) Update(ctx context.Context, campaignID, id string, fields bson.M) (*model.NPC, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var npc model.NPC
	if err := res.Decode(&npc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &npc, nil
}

// Delete removes an NPC scoped by (campaignId, id). Returns true if removed.
// Caller must run pin cleanup separately (see MapPinStore.DeleteByEntity).
func (s *NPCStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}
