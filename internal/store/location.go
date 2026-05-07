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

// LocationStore handles persistence for campaign locations.
type LocationStore struct {
	col *mongo.Collection
}

// NewLocationStore creates a LocationStore and ensures the campaignId index exists.
func NewLocationStore(db *DB) *LocationStore {
	col := db.Collection("campaign_locations")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "ownerPlayerId", Value: 1}}},
	})
	return &LocationStore{col: col}
}

// Create inserts a new location.
func (s *LocationStore) Create(ctx context.Context, l *model.Location) error {
	_, err := s.col.InsertOne(ctx, l)
	return err
}

// GetByID returns a location scoped to a campaign. Returns (nil, nil) when not found.
func (s *LocationStore) GetByID(ctx context.Context, campaignID, id string) (*model.Location, error) {
	var loc model.Location
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&loc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

// ListVisibleToCaller returns locations the caller can read: owner-or-shared.
// A location is visible when:
//   - ownerPlayerId == callerPlayerId, OR
//   - visibility.sharedWithAll == true, OR
//   - callerPlayerId is in visibility.sharedPlayerIds.
func (s *LocationStore) ListVisibleToCaller(ctx context.Context, campaignID, callerPlayerID string) ([]model.Location, error) {
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
	var out []model.Location
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.Location{}
	}
	return out, nil
}

// ListByOwner returns all locations owned by a specific player. Used by /share
// to look up "do I already have a clone of source X for recipient Y?"
func (s *LocationStore) ListByOwner(ctx context.Context, campaignID, ownerPlayerID string) ([]model.Location, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID, "ownerPlayerId": ownerPlayerID})
	if err != nil {
		return nil, err
	}
	var out []model.Location
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.Location{}
	}
	return out, nil
}

// FindCloneOf returns the existing clone of sourceID owned by ownerPlayerID, if any.
func (s *LocationStore) FindCloneOf(ctx context.Context, campaignID, sourceID, ownerPlayerID string) (*model.Location, error) {
	var loc model.Location
	err := s.col.FindOne(ctx, bson.M{
		"campaignId":              campaignID,
		"clone.clonedFromEntryId": sourceID,
		"ownerPlayerId":           ownerPlayerID,
	}).Decode(&loc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

// Update applies a partial update scoped by (campaignId, id) and bumps updatedAt.
// Caller is responsible for stripping immutable fields.
func (s *LocationStore) Update(ctx context.Context, campaignID, id string, fields bson.M) (*model.Location, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var loc model.Location
	if err := res.Decode(&loc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &loc, nil
}

// Delete removes a location scoped by (campaignId, id). Returns true if removed.
// Caller must run pin cleanup separately (see MapPinStore.DeleteByEntity).
func (s *LocationStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}
