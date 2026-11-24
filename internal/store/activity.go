package store

import (
	"context"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// ActivitySessionBetween selects entries outside every game session.
const ActivitySessionBetween = "between"

// ActivityFilter selects one user's history in a campaign.
type ActivityFilter struct {
	CampaignID, SubjectUserID, Session, Query string
	Categories                                []string
	ByDM                                      bool
	BeforeTime                                time.Time
	BeforeID                                  string
	Limit                                     int
}

// ActivityStore persists history entries (activity).
type ActivityStore struct {
	col *mongo.Collection
}

// NewActivityStore creates the store and ensures its read indexes exist.
func NewActivityStore(db *DB) *ActivityStore {
	col := db.Collection("activity")
	_, _ = col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "subjectUserId", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}, {Key: "subjectUserId", Value: 1}, {Key: "sessionId", Value: 1}}},
	})
	return &ActivityStore{col: col}
}

// Insert writes one entry.
func (s *ActivityStore) Insert(ctx context.Context, e *model.ActivityEntry) error {
	_, err := s.col.InsertOne(ctx, e)
	return err
}

// activityQuery builds the Mongo filter. Clauses sit under $and so the search
// and cursor can each use $or.
func activityQuery(f ActivityFilter) bson.M {
	and := []bson.M{{"campaignId": f.CampaignID}, {"subjectUserId": f.SubjectUserID}}
	switch f.Session {
	case "":
	case ActivitySessionBetween:
		and = append(and, bson.M{"sessionId": bson.M{"$exists": false}})
	default:
		and = append(and, bson.M{"sessionId": f.Session})
	}
	if len(f.Categories) > 0 {
		and = append(and, bson.M{"category": bson.M{"$in": f.Categories}})
	}
	if f.ByDM {
		and = append(and, bson.M{"actorRole": string(model.RoleDM)})
	}
	if f.Query != "" {
		match := bson.M{"$regex": regexp.QuoteMeta(f.Query), "$options": "i"}
		and = append(and, bson.M{"$or": []bson.M{{"title": match}, {"detail": match}}})
	}
	if !f.BeforeTime.IsZero() {
		and = append(and, bson.M{"$or": []bson.M{
			{"createdAt": bson.M{"$lt": f.BeforeTime}},
			{"createdAt": f.BeforeTime, "_id": bson.M{"$lt": f.BeforeID}},
		}})
	}
	return bson.M{"$and": and}
}

// List returns matching entries, newest first.
func (s *ActivityStore) List(ctx context.Context, f ActivityFilter) ([]model.ActivityEntry, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(f.Limit))
	return s.find(ctx, activityQuery(f), opts)
}

// ListForSummary returns all of a user's entries in a campaign, newest first,
// with only the fields session summaries need.
func (s *ActivityStore) ListForSummary(ctx context.Context, campaignID, subjectUserID string) ([]model.ActivityEntry, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetProjection(bson.M{
			"playerId": 1, "actorRole": 1, "type": 1, "category": 1, "changes": 1,
			"sessionId": 1, "sessionName": 1, "sessionDate": 1, "createdAt": 1,
		})
	return s.find(ctx, bson.M{"campaignId": campaignID, "subjectUserId": subjectUserID}, opts)
}

func (s *ActivityStore) find(ctx context.Context, filter bson.M, opts *options.FindOptionsBuilder) ([]model.ActivityEntry, error) {
	cursor, err := s.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	entries := []model.ActivityEntry{}
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// DeleteByCampaign removes a campaign's whole history.
func (s *ActivityStore) DeleteByCampaign(ctx context.Context, campaignID string) error {
	_, err := s.col.DeleteMany(ctx, bson.M{"campaignId": campaignID})
	return err
}
