package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// CatalogImageStore persists admin image overrides for the global arsenal
// catalog. The catalog itself is read-only embedded JSON; this collection holds
// only the (type, itemId) → S3 key association.
type CatalogImageStore struct {
	col *mongo.Collection
}

// NewCatalogImageStore creates the store and ensures the `type` index exists
// (KeysByType is the hot path for list endpoints).
func NewCatalogImageStore(db *DB) *CatalogImageStore {
	col := db.Collection("catalog_images")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "type", Value: 1}}},
	})
	return &CatalogImageStore{col: col}
}

func catalogImageDocID(t model.CatalogImageType, itemID string) string {
	return fmt.Sprintf("%s:%s", t, itemID)
}

// SetImage upserts the image key for a catalog item and returns the previous
// key ("" if none), so the caller can delete the now-orphaned S3 object.
func (s *CatalogImageStore) SetImage(ctx context.Context, t model.CatalogImageType, itemID, key string) (prevKey string, err error) {
	id := catalogImageDocID(t, itemID)
	var prev model.CatalogImage
	findErr := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&prev)
	if findErr == nil {
		prevKey = prev.ImageKey
	} else if !errors.Is(findErr, mongo.ErrNoDocuments) {
		return "", findErr
	}
	doc := model.CatalogImage{
		ID:        id,
		Type:      t,
		ItemID:    itemID,
		ImageKey:  key,
		UpdatedAt: time.Now().UTC(),
	}
	_, err = s.col.ReplaceOne(ctx, bson.M{"_id": id}, doc, options.Replace().SetUpsert(true))
	return prevKey, err
}

// DeleteImage removes the override for a catalog item and returns the deleted
// key ("" if none existed).
func (s *CatalogImageStore) DeleteImage(ctx context.Context, t model.CatalogImageType, itemID string) (deletedKey string, err error) {
	var prev model.CatalogImage
	res := s.col.FindOneAndDelete(ctx, bson.M{"_id": catalogImageDocID(t, itemID)})
	if err := res.Decode(&prev); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil
		}
		return "", err
	}
	return prev.ImageKey, nil
}

// ImageKey returns the stored key for a single item ("" if none).
func (s *CatalogImageStore) ImageKey(ctx context.Context, t model.CatalogImageType, itemID string) (string, error) {
	var doc model.CatalogImage
	err := s.col.FindOne(ctx, bson.M{"_id": catalogImageDocID(t, itemID)}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return doc.ImageKey, nil
}

// KeysByType returns itemID → imageKey for every override of the given type, in
// a single query.
func (s *CatalogImageStore) KeysByType(ctx context.Context, t model.CatalogImageType) (map[string]string, error) {
	cursor, err := s.col.Find(ctx, bson.M{"type": t})
	if err != nil {
		return nil, err
	}
	var docs []model.CatalogImage
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(docs))
	for _, d := range docs {
		out[d.ItemID] = d.ImageKey
	}
	return out, nil
}
