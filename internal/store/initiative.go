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

// ErrInitiativeVersionConflict signals a concurrent write; caller should retry.
var ErrInitiativeVersionConflict = errors.New("initiative version conflict")

// InitiativeStore persists one initiative call per campaign (_id == campaignId).
type InitiativeStore struct {
	col *mongo.Collection
}

// NewInitiativeStore creates the store and ensures the resolvedAt TTL index.
// TTL deletes resolved calls 24h after resolvedAt. Open calls have no
// resolvedAt, so Mongo's TTL never touches them.
func NewInitiativeStore(db *DB) *InitiativeStore {
	col := db.Collection("initiative_calls")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "resolvedAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(86400),
	})
	return &InitiativeStore{col: col}
}

// Get returns the campaign's call, or (nil, nil) when none exists.
func (s *InitiativeStore) Get(ctx context.Context, campaignID string) (*model.InitiativeCall, error) {
	var c model.InitiativeCall
	err := s.col.FindOne(ctx, bson.M{"_id": campaignID}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// StartReplace overwrites any existing call for the campaign with a fresh one
// (version reset to 1). This is the only "clearing" of a prior doc.
func (s *InitiativeStore) StartReplace(ctx context.Context, c *model.InitiativeCall) error {
	c.Version = 1
	c.ResolvedAt = nil
	_, err := s.col.ReplaceOne(
		ctx,
		bson.M{"_id": c.CampaignID},
		c,
		options.Replace().SetUpsert(true),
	)
	return err
}

// UpdateWithVersion writes next only if the stored version still equals
// expectedVersion, bumping it. Returns ErrInitiativeVersionConflict on mismatch.
// Note: next.Version is overwritten in place; on error next.Version is left
// at the attempted value and must not be relied on — use the returned call.
func (s *InitiativeStore) UpdateWithVersion(ctx context.Context, next *model.InitiativeCall, expectedVersion int) (*model.InitiativeCall, error) {
	next.Version = expectedVersion + 1
	res := s.col.FindOneAndReplace(
		ctx,
		bson.M{"_id": next.CampaignID, "version": expectedVersion},
		next,
		options.FindOneAndReplace().SetReturnDocument(options.After),
	)
	var out model.InitiativeCall
	if err := res.Decode(&out); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrInitiativeVersionConflict
		}
		return nil, err
	}
	return &out, nil
}

// Resolve marks an open call resolved and stamps resolvedAt (arms the TTL).
// DM-only and terminal. Idempotent: if the call is already resolved it is
// returned unchanged (the TTL is NOT reset). Returns (nil, nil) when no call
// exists for the campaign (same not-found convention as Get); the handler
// maps that to 404.
func (s *InitiativeStore) Resolve(ctx context.Context, campaignID string) (*model.InitiativeCall, error) {
	now := time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": campaignID, "status": "open"},
		bson.M{"$set": bson.M{
			"status":     "resolved",
			"updatedAt":  now.UnixMilli(),
			"resolvedAt": now,
		}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var out model.InitiativeCall
	err := res.Decode(&out)
	if err == nil {
		return &out, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	// No OPEN call matched: either it's already resolved, or there is no
	// call at all. Re-read to disambiguate — return an already-resolved
	// call unchanged (idempotent, TTL untouched); nil when truly absent.
	return s.Get(ctx, campaignID)
}
