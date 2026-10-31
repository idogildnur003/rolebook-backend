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

var (
	ErrLimitedUseNotFound = errors.New("limited use not found")
	ErrNoUsesLeft         = errors.New("no uses left")
	ErrAlreadyFull        = errors.New("limited use already full")
	ErrNotResettable      = errors.New("limited use is not resettable")
	ErrLimitReached       = errors.New("limited use limit reached")
	ErrLimitedUseChanged  = errors.New("limited use changed concurrently")
)

// LimitedUsePatch holds the optional fields of a limited-use update.
type LimitedUsePatch struct {
	Name   *string
	Note   *string
	Max    *int
	Resets *bool
}

type limitedUseDoc struct {
	LimitedUses []model.LimitedUse `bson:"limitedUses"`
}

// decodeLimitedUse extracts the single projected element from res. A filter
// miss returns (nil, nil) so callers can map it to the right sentinel.
func decodeLimitedUse(res *mongo.SingleResult) (*model.LimitedUse, error) {
	var doc limitedUseDoc
	if err := res.Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	if len(doc.LimitedUses) != 1 {
		return nil, nil
	}
	return &doc.LimitedUses[0], nil
}

func limitedUseProjection(id string) bson.M {
	return bson.M{"limitedUses": bson.M{"$elemMatch": bson.M{"id": id}}}
}

// updateLimitedUse runs an atomic update and returns the updated element, or
// (nil, nil) when the filter matched nothing.
func (s *PlayerStore) updateLimitedUse(ctx context.Context, filter bson.M, update bson.M, id string) (*model.LimitedUse, error) {
	update["$set"] = mergeSet(update["$set"], bson.M{"updatedAt": time.Now().UTC()})
	res := s.col.FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After).SetProjection(limitedUseProjection(id)))
	return decodeLimitedUse(res)
}

func mergeSet(existing any, extra bson.M) bson.M {
	out := bson.M{}
	if m, ok := existing.(bson.M); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// getLimitedUse reads one element; (nil, nil) when the player or element is missing.
func (s *PlayerStore) getLimitedUse(ctx context.Context, playerID, id string) (*model.LimitedUse, error) {
	res := s.col.FindOne(ctx,
		bson.M{"_id": playerID, "limitedUses.id": id},
		options.FindOne().SetProjection(limitedUseProjection(id)))
	return decodeLimitedUse(res)
}

// AddLimitedUse appends lu (with Remaining forced to Max) unless the player
// already holds LimitedUseMaxCount entries. A missing player returns a wrapped
// mongo.ErrNoDocuments.
func (s *PlayerStore) AddLimitedUse(ctx context.Context, playerID string, lu model.LimitedUse) (*model.LimitedUse, error) {
	lu.Remaining = lu.Max
	filter := bson.M{
		"_id": playerID,
		fmt.Sprintf("limitedUses.%d", model.LimitedUseMaxCount-1): bson.M{"$exists": false},
	}
	item, err := s.updateLimitedUse(ctx, filter, bson.M{"$push": bson.M{"limitedUses": lu}}, lu.ID)
	if err != nil {
		return nil, err
	}
	if item != nil {
		return item, nil
	}
	n, err := s.col.CountDocuments(ctx, bson.M{"_id": playerID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("player %s: %w", playerID, mongo.ErrNoDocuments)
	}
	return nil, ErrLimitReached
}

// UpdateLimitedUse applies the non-nil patch fields. Lowering Max clamps
// Remaining down to the new Max.
func (s *PlayerStore) UpdateLimitedUse(ctx context.Context, playerID, id string, p LimitedUsePatch) (*model.LimitedUse, error) {
	set := bson.M{}
	if p.Name != nil {
		set["limitedUses.$.name"] = *p.Name
	}
	if p.Note != nil {
		set["limitedUses.$.note"] = *p.Note
	}
	if p.Max != nil {
		set["limitedUses.$.max"] = *p.Max
	}
	if p.Resets != nil {
		set["limitedUses.$.resets"] = *p.Resets
	}
	update := bson.M{"$set": set}
	if p.Max != nil {
		update["$min"] = bson.M{"limitedUses.$.remaining": *p.Max}
	}
	item, err := s.updateLimitedUse(ctx, bson.M{"_id": playerID, "limitedUses.id": id}, update, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrLimitedUseNotFound
	}
	return item, nil
}

// RemoveLimitedUse deletes one entry.
func (s *PlayerStore) RemoveLimitedUse(ctx context.Context, playerID, id string) error {
	res, err := s.col.UpdateOne(ctx,
		bson.M{"_id": playerID, "limitedUses.id": id},
		bson.M{
			"$pull": bson.M{"limitedUses": bson.M{"id": id}},
			"$set":  bson.M{"updatedAt": time.Now().UTC()},
		})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrLimitedUseNotFound
	}
	return nil
}

// SpendLimitedUse spends one use. The last use of a one-time (non-resetting)
// entry removes it (removed == true, item == nil); a resettable entry stays at 0.
func (s *PlayerStore) SpendLimitedUse(ctx context.Context, playerID, id string) (item *model.LimitedUse, removed bool, err error) {
	res, err := s.col.UpdateOne(ctx,
		bson.M{"_id": playerID, "limitedUses": bson.M{"$elemMatch": bson.M{"id": id, "resets": false, "remaining": 1}}},
		bson.M{
			"$pull": bson.M{"limitedUses": bson.M{"id": id}},
			"$set":  bson.M{"updatedAt": time.Now().UTC()},
		})
	if err != nil {
		return nil, false, err
	}
	if res.ModifiedCount > 0 {
		return nil, true, nil
	}

	item, err = s.updateLimitedUse(ctx,
		bson.M{"_id": playerID, "limitedUses": bson.M{"$elemMatch": bson.M{
			"id":        id,
			"remaining": bson.M{"$gt": 0},
			"$or": bson.A{
				bson.M{"resets": true},
				bson.M{"remaining": bson.M{"$gt": 1}},
			},
		}}},
		bson.M{"$inc": bson.M{"limitedUses.$.remaining": -1}}, id)
	if err != nil {
		return nil, false, err
	}
	if item != nil {
		return item, false, nil
	}

	cur, err := s.getLimitedUse(ctx, playerID, id)
	if err != nil {
		return nil, false, err
	}
	if cur == nil {
		return nil, false, ErrLimitedUseNotFound
	}
	return nil, false, ErrNoUsesLeft
}

// RestoreLimitedUse gives back one use (compare-and-set, one retry).
func (s *PlayerStore) RestoreLimitedUse(ctx context.Context, playerID, id string) (*model.LimitedUse, error) {
	return s.casLimitedUse(ctx, playerID, id, func(cur *model.LimitedUse) (int, error) {
		if cur.Remaining >= cur.Max {
			return 0, ErrAlreadyFull
		}
		return cur.Remaining + 1, nil
	})
}

// ResetLimitedUse refills a resettable entry to Max (compare-and-set, one retry).
func (s *PlayerStore) ResetLimitedUse(ctx context.Context, playerID, id string) (*model.LimitedUse, error) {
	return s.casLimitedUse(ctx, playerID, id, func(cur *model.LimitedUse) (int, error) {
		if !cur.Resets {
			return 0, ErrNotResettable
		}
		return cur.Max, nil
	})
}

// casLimitedUse reads the element, lets next pick the new remaining value, and
// writes it only if remaining and max are unchanged. Retries once on a miss.
func (s *PlayerStore) casLimitedUse(ctx context.Context, playerID, id string, next func(*model.LimitedUse) (int, error)) (*model.LimitedUse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		cur, err := s.getLimitedUse(ctx, playerID, id)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, ErrLimitedUseNotFound
		}
		remaining, err := next(cur)
		if err != nil {
			return nil, err
		}
		item, err := s.updateLimitedUse(ctx,
			bson.M{"_id": playerID, "limitedUses": bson.M{"$elemMatch": bson.M{
				"id": id, "remaining": cur.Remaining, "max": cur.Max,
			}}},
			bson.M{"$set": bson.M{"limitedUses.$.remaining": remaining}}, id)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}
	return nil, ErrLimitedUseChanged
}
