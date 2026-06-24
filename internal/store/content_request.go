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

// ContentRequestStore persists the DM-moderation queue (collection content_requests).
type ContentRequestStore struct {
	col *mongo.Collection
}

// NewContentRequestStore creates the store and ensures lookup indexes exist.
func NewContentRequestStore(db *DB) *ContentRequestStore {
	col := db.Collection("content_requests")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "proposedByUserId", Value: 1}}},
	})
	return &ContentRequestStore{col: col}
}

// Create inserts a new request. The caller populates ID (via GenerateID).
func (s *ContentRequestStore) Create(ctx context.Context, req *model.ContentRequest) error {
	_, err := s.col.InsertOne(ctx, req)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEntry
	}
	return err
}

// GetByID returns a request scoped to a campaign, or (nil, nil) when not found.
func (s *ContentRequestStore) GetByID(ctx context.Context, campaignID, id string) (*model.ContentRequest, error) {
	var req model.ContentRequest
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&req)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// ListPending returns the campaign's pending requests, oldest first (DM queue).
func (s *ContentRequestStore) ListPending(ctx context.Context, campaignID string) ([]model.ContentRequest, error) {
	return s.find(ctx,
		bson.M{"campaignId": campaignID, "status": model.RequestStatusPending},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}),
	)
}

// ListByProposer returns all of a user's requests in a campaign, newest first
// (powers the player's "My Requests" screen — pending + resolved history).
func (s *ContentRequestStore) ListByProposer(ctx context.Context, campaignID, userID string) ([]model.ContentRequest, error) {
	return s.find(ctx,
		bson.M{"campaignId": campaignID, "proposedByUserId": userID},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	)
}

// CountPending returns the number of pending requests (for the DM badge).
func (s *ContentRequestStore) CountPending(ctx context.Context, campaignID string) (int64, error) {
	return s.col.CountDocuments(ctx, bson.M{"campaignId": campaignID, "status": model.RequestStatusPending})
}

// Resolve marks a request approved/denied and records the resolver + optional
// result id. Returns the updated request, or (nil, nil) when not found.
func (s *ContentRequestStore) Resolve(ctx context.Context, campaignID, id, status, resolvedByUserID, resultID string) (*model.ContentRequest, error) {
	now := time.Now().UTC()
	set := bson.M{"status": status, "resolvedAt": &now, "resolvedByUserId": resolvedByUserID}
	if resultID != "" {
		set["resultId"] = resultID
	}
	res := s.col.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": set},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var req model.ContentRequest
	if err := res.Decode(&req); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

// Delete removes a request (withdraw). Returns true if a document was removed.
func (s *ContentRequestStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// UpdatePayload applies a partial update to a request (used for the DM's
// edit-before-approve). Caller strips immutable fields. Returns the updated
// request, or (nil, nil) when no match.
func (s *ContentRequestStore) UpdatePayload(ctx context.Context, campaignID, id string, fields bson.M) (*model.ContentRequest, error) {
	res := s.col.FindOneAndUpdate(ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var req model.ContentRequest
	if err := res.Decode(&req); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

func (s *ContentRequestStore) find(ctx context.Context, filter bson.M, opts ...options.Lister[options.FindOptions]) ([]model.ContentRequest, error) {
	cursor, err := s.col.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	var reqs []model.ContentRequest
	if err := cursor.All(ctx, &reqs); err != nil {
		return nil, err
	}
	if reqs == nil {
		reqs = []model.ContentRequest{}
	}
	return reqs, nil
}
