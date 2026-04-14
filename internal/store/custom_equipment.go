package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// CustomEquipmentStore handles persistence for per-campaign homebrew equipment.
type CustomEquipmentStore struct {
	col *mongo.Collection
}

// NewCustomEquipmentStore creates a CustomEquipmentStore and ensures the
// campaignId index exists (every lookup is scoped by campaign).
func NewCustomEquipmentStore(db *DB) *CustomEquipmentStore {
	col := db.Collection("custom_equipment")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
	})
	return &CustomEquipmentStore{col: col}
}

// Create inserts a new custom equipment entry. The caller is expected to have
// populated an ID via GenerateID; Create does not generate one itself so that
// collision-retry can live in the handler where it has full context.
func (s *CustomEquipmentStore) Create(ctx context.Context, item *model.CustomEquipment) error {
	_, err := s.col.InsertOne(ctx, item)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEntry
	}
	return err
}

// GetByID returns a custom equipment entry scoped to a campaign.
// Returns (nil, nil) when not found.
func (s *CustomEquipmentStore) GetByID(ctx context.Context, campaignID, id string) (*model.CustomEquipment, error) {
	var item model.CustomEquipment
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&item)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListByCampaign returns all custom equipment entries for a campaign.
func (s *CustomEquipmentStore) ListByCampaign(ctx context.Context, campaignID string) ([]model.CustomEquipment, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID})
	if err != nil {
		return nil, err
	}
	var items []model.CustomEquipment
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.CustomEquipment{}
	}
	return items, nil
}

// Update applies a partial update scoped by (campaignId, id) and bumps updatedAt.
// The caller is responsible for stripping immutable fields (_id, campaignId,
// createdBy, createdAt) from the patch before calling.
// Returns the updated item, or (nil, nil) when no match.
func (s *CustomEquipmentStore) Update(ctx context.Context, campaignID, id string, fields bson.M) (*model.CustomEquipment, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var item model.CustomEquipment
	if err := res.Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// Delete removes a custom equipment entry scoped by (campaignId, id).
// Returns true if a document was removed. Does NOT cascade to player
// inventories — use DeleteWithCascade for that.
func (s *CustomEquipmentStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// CascadeDeleteResult captures the outcome of DeleteWithCascade.
type CascadeDeleteResult struct {
	CatalogDeleted     bool
	PlayersAffected    int64
	InventoryCleanupErr error // non-nil if the catalog delete succeeded but the
	//                         player inventory cleanup failed. Best-effort
	//                         semantics — the catalog entry is already gone and
	//                         surviving inventory references are tolerated by
	//                         the inventory list handler.
}

// DeleteWithCascade removes a custom equipment entry AND pulls it out of every
// player inventory in the campaign. Runs as two sequential writes: catalog
// delete first, then cleanup. If the cleanup fails, the catalog entry is still
// considered deleted (no rollback) and the error is surfaced via the result
// so the caller can log or alert.
func (s *CustomEquipmentStore) DeleteWithCascade(
	ctx context.Context,
	campaignID, id string,
	players *PlayerStore,
) (CascadeDeleteResult, error) {
	deleted, err := s.Delete(ctx, campaignID, id)
	if err != nil {
		return CascadeDeleteResult{}, err
	}
	if !deleted {
		return CascadeDeleteResult{CatalogDeleted: false}, nil
	}
	affected, cleanupErr := players.RemoveEquipmentFromAllInventories(ctx, campaignID, id)
	return CascadeDeleteResult{
		CatalogDeleted:      true,
		PlayersAffected:     affected,
		InventoryCleanupErr: cleanupErr,
	}, nil
}

// --- ID generation ---------------------------------------------------------

var customIDSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateID produces a server-owned custom equipment id of the form
// "custom-{slug(name)}-{6-hex-random}". On the rare slug collision the caller
// should retry — GenerateID itself is stateless.
func GenerateID(name string) (string, error) {
	slug := slugify(name)
	if slug == "" {
		slug = "item"
	}
	suffix, err := randomHex(3)
	if err != nil {
		return "", fmt.Errorf("custom equipment id suffix: %w", err)
	}
	return fmt.Sprintf("custom-%s-%s", slug, suffix), nil
}

func slugify(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	hyphenated := customIDSlugPattern.ReplaceAllString(lower, "-")
	return strings.Trim(hyphenated, "-")
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
