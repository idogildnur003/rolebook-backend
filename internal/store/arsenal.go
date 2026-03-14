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

// ArsenalStore handles the global spell and equipment catalog.
type ArsenalStore struct {
	spells    *mongo.Collection
	equipment *mongo.Collection
}

// NewArsenalStore creates an ArsenalStore for both catalog collections.
func NewArsenalStore(db *DB) *ArsenalStore {
	return &ArsenalStore{
		spells:    db.Collection("arsenal_spells"),
		equipment: db.Collection("arsenal_equipment"),
	}
}

// --- Spell catalog ---

func (s *ArsenalStore) ListSpells(ctx context.Context) ([]model.ArsenalSpell, error) {
	cursor, err := s.spells.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var spells []model.ArsenalSpell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.ArsenalSpell{}
	}
	return spells, nil
}

func (s *ArsenalStore) CreateSpell(ctx context.Context, spell *model.ArsenalSpell) error {
	_, err := s.spells.InsertOne(ctx, spell)
	return err
}

// GetSpell returns a single arsenal spell by ID, or nil if not found.
func (s *ArsenalStore) GetSpell(ctx context.Context, id string) (*model.ArsenalSpell, error) {
	var spell model.ArsenalSpell
	err := s.spells.FindOne(ctx, bson.M{"_id": id}).Decode(&spell)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spell, nil
}

func (s *ArsenalStore) UpdateSpell(ctx context.Context, id string, fields bson.M) (*model.ArsenalSpell, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.spells.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var spell model.ArsenalSpell
	if err := res.Decode(&spell); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &spell, nil
}

func (s *ArsenalStore) DeleteSpell(ctx context.Context, id string) (bool, error) {
	res, err := s.spells.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}

// --- Equipment catalog ---

func (s *ArsenalStore) ListEquipment(ctx context.Context) ([]model.ArsenalEquipment, error) {
	cursor, err := s.equipment.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var items []model.ArsenalEquipment
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.ArsenalEquipment{}
	}
	return items, nil
}

func (s *ArsenalStore) CreateEquipment(ctx context.Context, item *model.ArsenalEquipment) error {
	_, err := s.equipment.InsertOne(ctx, item)
	return err
}

// GetEquipment returns a single arsenal equipment item by ID, or nil if not found.
func (s *ArsenalStore) GetEquipment(ctx context.Context, id string) (*model.ArsenalEquipment, error) {
	var item model.ArsenalEquipment
	err := s.equipment.FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ArsenalStore) UpdateEquipment(ctx context.Context, id string, fields bson.M) (*model.ArsenalEquipment, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.equipment.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var item model.ArsenalEquipment
	if err := res.Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (s *ArsenalStore) DeleteEquipment(ctx context.Context, id string) (bool, error) {
	res, err := s.equipment.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}
