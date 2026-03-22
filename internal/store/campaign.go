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

// CampaignStore handles persistence for campaigns and embedded sessions.
type CampaignStore struct {
	col *mongo.Collection
}

// NewCampaignStore creates a CampaignStore.
func NewCampaignStore(db *DB) *CampaignStore {
	return &CampaignStore{col: db.Collection("campaigns")}
}

// ListAll returns all campaigns (admin use — no filter).
func (s *CampaignStore) ListAll(ctx context.Context) ([]model.Campaign, error) {
	return s.find(ctx, bson.M{})
}

// ListByIDs returns campaigns whose IDs are in the given set (player visibility).
func (s *CampaignStore) ListByIDs(ctx context.Context, ids []string) ([]model.Campaign, error) {
	if len(ids) == 0 {
		return []model.Campaign{}, nil
	}
	return s.find(ctx, bson.M{"_id": bson.M{"$in": ids}})
}

// ListByUser returns campaigns where the user is a member.
func (s *CampaignStore) ListByUser(ctx context.Context, userID string) ([]model.Campaign, error) {
	return s.find(ctx, bson.M{"memberships.playerId": userID})
}

// GetByID returns a single campaign, or nil if not found.
func (s *CampaignStore) GetByID(ctx context.Context, id string) (*model.Campaign, error) {
	var c model.Campaign
	err := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Create inserts a new campaign.
func (s *CampaignStore) Create(ctx context.Context, c *model.Campaign) error {
	_, err := s.col.InsertOne(ctx, c)
	return err
}

// Update applies a partial update to a campaign. updatedAt is always set to now.
func (s *CampaignStore) Update(ctx context.Context, id string, fields bson.M) (*model.Campaign, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var c model.Campaign
	if err := res.Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Delete removes a campaign by ID. Returns true if it existed.
func (s *CampaignStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}

// AddSession appends a session to the campaign's embedded sessions array and updates updatedAt.
func (s *CampaignStore) AddSession(ctx context.Context, campaignID string, sess model.Session) (*model.Session, error) {
	now := time.Now().UTC()
	sess.UpdatedAt = now
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": campaignID},
		bson.M{
			"$push": bson.M{"sessions": sess},
			"$set":  bson.M{"updatedAt": now},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var c model.Campaign
	if err := res.Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	// Return the appended session
	for _, s := range c.Sessions {
		if s.ID == sess.ID {
			return &s, nil
		}
	}
	return nil, nil
}

// UpdateSession updates fields on an embedded session using a fetch-modify-replace approach.
// Bumps both the session's updatedAt and the campaign's updatedAt.
func (s *CampaignStore) UpdateSession(ctx context.Context, campaignID, sessionID string, fields bson.M) (*model.Session, error) {
	campaign, err := s.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	var updatedSession *model.Session
	for i := range campaign.Sessions {
		if campaign.Sessions[i].ID == sessionID {
			if v, ok := fields["name"].(string); ok {
				campaign.Sessions[i].Name = v
			}
			if v, ok := fields["description"].(string); ok {
				campaign.Sessions[i].Description = v
			}
			campaign.Sessions[i].UpdatedAt = now
			sess := campaign.Sessions[i]
			updatedSession = &sess
			break
		}
	}
	if updatedSession == nil {
		return nil, nil // session not found
	}
	_, err = s.col.UpdateOne(ctx, bson.M{"_id": campaignID}, bson.M{
		"$set": bson.M{"sessions": campaign.Sessions, "updatedAt": now},
	})
	if err != nil {
		return nil, err
	}
	return updatedSession, nil
}

// DeleteSession removes an embedded session from a campaign and bumps updatedAt.
// Returns (false, nil) if the campaign or session is not found.
func (s *CampaignStore) DeleteSession(ctx context.Context, campaignID, sessionID string) (bool, error) {
	campaign, err := s.GetByID(ctx, campaignID)
	if err != nil {
		return false, err
	}
	if campaign == nil {
		return false, nil
	}
	now := time.Now().UTC()
	res, err := s.col.UpdateOne(
		ctx,
		bson.M{"_id": campaignID},
		bson.M{
			"$pull": bson.M{"sessions": bson.M{"id": sessionID}},
			"$set":  bson.M{"updatedAt": now},
		},
	)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

func (s *CampaignStore) find(ctx context.Context, filter bson.M) ([]model.Campaign, error) {
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var campaigns []model.Campaign
	if err := cursor.All(ctx, &campaigns); err != nil {
		return nil, err
	}
	if campaigns == nil {
		campaigns = []model.Campaign{}
	}
	return campaigns, nil
}
