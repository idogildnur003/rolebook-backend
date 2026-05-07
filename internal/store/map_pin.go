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

// MapPinStore handles persistence for campaign map pins.
type MapPinStore struct {
	col *mongo.Collection
}

// NewMapPinStore creates a MapPinStore and ensures the campaignId index exists.
func NewMapPinStore(db *DB) *MapPinStore {
	col := db.Collection("campaign_map_pins")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "ownerPlayerId", Value: 1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "entityId", Value: 1}}},
	})
	return &MapPinStore{col: col}
}

// Create inserts a new map pin.
func (s *MapPinStore) Create(ctx context.Context, m *model.MapPin) error {
	_, err := s.col.InsertOne(ctx, m)
	return err
}

// GetByID returns a map pin scoped to a campaign. Returns (nil, nil) when not found.
func (s *MapPinStore) GetByID(ctx context.Context, campaignID, id string) (*model.MapPin, error) {
	var pin model.MapPin
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&pin)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pin, nil
}

// ListVisibleToCaller returns map pins the caller can read: owner-or-shared.
// A map pin is visible when:
//   - ownerPlayerId == callerPlayerId, OR
//   - visibility.sharedWithAll == true, OR
//   - callerPlayerId is in visibility.sharedPlayerIds.
func (s *MapPinStore) ListVisibleToCaller(ctx context.Context, campaignID, callerPlayerID string) ([]model.MapPin, error) {
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
	var out []model.MapPin
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.MapPin{}
	}
	return out, nil
}

// ListByOwner returns all map pins owned by a specific player. Used by /share
// to look up "do I already have a clone of source X for recipient Y?"
func (s *MapPinStore) ListByOwner(ctx context.Context, campaignID, ownerPlayerID string) ([]model.MapPin, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID, "ownerPlayerId": ownerPlayerID})
	if err != nil {
		return nil, err
	}
	var out []model.MapPin
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.MapPin{}
	}
	return out, nil
}

// FindCloneOf returns the existing clone of sourceID owned by ownerPlayerID, if any.
func (s *MapPinStore) FindCloneOf(ctx context.Context, campaignID, sourceID, ownerPlayerID string) (*model.MapPin, error) {
	var pin model.MapPin
	err := s.col.FindOne(ctx, bson.M{
		"campaignId":              campaignID,
		"clone.clonedFromEntryId": sourceID,
		"ownerPlayerId":           ownerPlayerID,
	}).Decode(&pin)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pin, nil
}

// Update applies a partial update scoped by (campaignId, id) and bumps updatedAt.
// Caller is responsible for stripping immutable fields.
func (s *MapPinStore) Update(ctx context.Context, campaignID, id string, fields bson.M) (*model.MapPin, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var pin model.MapPin
	if err := res.Decode(&pin); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &pin, nil
}

// Delete removes a map pin scoped by (campaignId, id). Returns true if removed.
func (s *MapPinStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// DeleteByEntity removes all pins whose entityId matches the given id (used
// when a Location or NPC is deleted, to cascade-delete pointing pins).
// Returns the number of pins deleted.
func (s *MapPinStore) DeleteByEntity(ctx context.Context, campaignID, entityID string) (int64, error) {
	res, err := s.col.DeleteMany(ctx, bson.M{"campaignId": campaignID, "entityId": entityID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
