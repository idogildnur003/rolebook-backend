package store

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

type ArsenalStore struct {
	spells    *mongo.Collection
	equipment *mongo.Collection
}

func NewArsenalStore(db *DB) *ArsenalStore {
	return &ArsenalStore{
		spells:    db.Arsenal().Collection("spells"),
		equipment: db.Arsenal().Collection("equipment"),
	}
}

// PaginatedResult holds a page of results with total count.
type PaginatedResult[T any] struct {
	Data  []T   `json:"data"`
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
	Total int64 `json:"total"`
}

func (s *ArsenalStore) ListSpells(ctx context.Context, page, limit int64) (*PaginatedResult[model.Spell], error) {
	total, err := s.spells.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	skip := (page - 1) * limit
	cursor, err := s.spells.Find(ctx, bson.M{},
		options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "name", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	var spells []model.Spell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.Spell{}
	}
	return &PaginatedResult[model.Spell]{Data: spells, Page: page, Limit: limit, Total: total}, nil
}

func (s *ArsenalStore) GetSpell(ctx context.Context, id string) (*model.Spell, error) {
	var spell model.Spell
	err := s.spells.FindOne(ctx, bson.M{"_id": id}).Decode(&spell)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spell, nil
}

func (s *ArsenalStore) ListEquipment(ctx context.Context, page, limit int64) (*PaginatedResult[model.Equipment], error) {
	total, err := s.equipment.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	skip := (page - 1) * limit
	cursor, err := s.equipment.Find(ctx, bson.M{},
		options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "name", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	var items []model.Equipment
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.Equipment{}
	}
	return &PaginatedResult[model.Equipment]{Data: items, Page: page, Limit: limit, Total: total}, nil
}

func (s *ArsenalStore) GetEquipment(ctx context.Context, id string) (*model.Equipment, error) {
	var item model.Equipment
	err := s.equipment.FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
