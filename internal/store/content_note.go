package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// ContentNoteStore persists players' PRIVATE per-entry notes (content_notes).
// One note per (userId, campaignId, targetType, entryId) — enforced by a unique index.
type ContentNoteStore struct {
	col *mongo.Collection
}

// NewContentNoteStore creates the store and ensures the unique 4-tuple index
// (plus an owner-scoped lookup index) exist.
func NewContentNoteStore(db *DB) *ContentNoteStore {
	col := db.Collection("content_notes")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "userId", Value: 1},
				{Key: "campaignId", Value: 1},
				{Key: "targetType", Value: 1},
				{Key: "entryId", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "campaignId", Value: 1}}},
	})
	return &ContentNoteStore{col: col}
}

// ListForUser returns all of one user's notes in a campaign.
func (s *ContentNoteStore) ListForUser(ctx context.Context, campaignID, userID string) ([]model.ContentNote, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID, "userId": userID})
	if err != nil {
		return nil, err
	}
	var notes []model.ContentNote
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}
	if notes == nil {
		notes = []model.ContentNote{}
	}
	return notes, nil
}

// Upsert creates or updates a user's note for one entry, keyed by the 4-tuple.
// Returns the stored note.
func (s *ContentNoteStore) Upsert(ctx context.Context, campaignID, userID, targetType, entryID, body string) (*model.ContentNote, error) {
	now := time.Now().UTC()
	filter := bson.M{
		"campaignId": campaignID,
		"userId":     userID,
		"targetType": targetType,
		"entryId":    entryID,
	}
	newID, err := GenerateID(entryID)
	if err != nil {
		return nil, err
	}
	update := bson.M{
		"$set": bson.M{"body": body, "updatedAt": now},
		"$setOnInsert": bson.M{
			"_id":        newID,
			"campaignId": campaignID,
			"userId":     userID,
			"targetType": targetType,
			"entryId":    entryID,
		},
	}
	res := s.col.FindOneAndUpdate(
		ctx, filter, update,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)
	var note model.ContentNote
	if err := res.Decode(&note); err != nil {
		return nil, err
	}
	return &note, nil
}

// Delete removes a user's note for one entry. Returns whether a note was deleted.
func (s *ContentNoteStore) Delete(ctx context.Context, campaignID, userID, targetType, entryID string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{
		"campaignId": campaignID,
		"userId":     userID,
		"targetType": targetType,
		"entryId":    entryID,
	})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}
