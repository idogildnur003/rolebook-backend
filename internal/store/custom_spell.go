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

// CustomSpellStore handles persistence for per-campaign homebrew spells.
type CustomSpellStore struct {
	col *mongo.Collection
}

// NewCustomSpellStore creates a CustomSpellStore and ensures the campaignId
// index exists (every lookup is scoped by campaign).
func NewCustomSpellStore(db *DB) *CustomSpellStore {
	col := db.Collection("custom_spells")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
	})
	return &CustomSpellStore{col: col}
}

// Create inserts a new custom spell entry. The caller is expected to have
// populated an ID via GenerateID; Create does not generate one itself so that
// collision-retry can live in the handler where it has full context.
func (s *CustomSpellStore) Create(ctx context.Context, spell *model.CustomSpell) error {
	_, err := s.col.InsertOne(ctx, spell)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEntry
	}
	return err
}

// GetByID returns a custom spell entry scoped to a campaign.
// Returns (nil, nil) when not found.
func (s *CustomSpellStore) GetByID(ctx context.Context, campaignID, id string) (*model.CustomSpell, error) {
	var spell model.CustomSpell
	err := s.col.FindOne(ctx, bson.M{"_id": id, "campaignId": campaignID}).Decode(&spell)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spell, nil
}

// ListByCampaign returns all custom spell entries for a campaign.
func (s *CustomSpellStore) ListByCampaign(ctx context.Context, campaignID string) ([]model.CustomSpell, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID})
	if err != nil {
		return nil, err
	}
	var spells []model.CustomSpell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.CustomSpell{}
	}
	return spells, nil
}

// Update applies a partial update scoped by (campaignId, id) and bumps
// updatedAt. The caller is responsible for stripping immutable fields
// (_id, campaignId, createdBy, createdAt) from the patch before calling.
// Returns the updated spell, or (nil, nil) when no match.
func (s *CustomSpellStore) Update(ctx context.Context, campaignID, id string, fields bson.M) (*model.CustomSpell, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id, "campaignId": campaignID},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var spell model.CustomSpell
	if err := res.Decode(&spell); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &spell, nil
}

// Delete removes a custom spell entry scoped by (campaignId, id).
// Returns true if a document was removed. Does NOT cascade to player spell
// lists — use DeleteWithCascade for that.
func (s *CustomSpellStore) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id, "campaignId": campaignID})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// CustomSpellCascadeResult captures the outcome of DeleteWithCascade.
type CustomSpellCascadeResult struct {
	CatalogDeleted  bool
	PlayersAffected int64
	CleanupErr      error // non-nil if the catalog delete succeeded but the
	//                     player spell cleanup failed. Best-effort semantics —
	//                     the catalog entry is already gone and surviving
	//                     references are tolerated by the spell list handler.
}

// DeleteWithCascade removes a custom spell entry AND pulls it out of every
// player's spell list in the campaign. Runs as two sequential writes: catalog
// delete first, then cleanup. If the cleanup fails, the catalog entry is still
// considered deleted (no rollback) and the error is surfaced via the result
// so the caller can log or alert.
func (s *CustomSpellStore) DeleteWithCascade(
	ctx context.Context,
	campaignID, id string,
	players *PlayerStore,
) (CustomSpellCascadeResult, error) {
	deleted, err := s.Delete(ctx, campaignID, id)
	if err != nil {
		return CustomSpellCascadeResult{}, err
	}
	if !deleted {
		return CustomSpellCascadeResult{CatalogDeleted: false}, nil
	}
	affected, cleanupErr := players.RemoveSpellFromAllPlayers(ctx, campaignID, id)
	return CustomSpellCascadeResult{
		CatalogDeleted:  true,
		PlayersAffected: affected,
		CleanupErr:      cleanupErr,
	}, nil
}
