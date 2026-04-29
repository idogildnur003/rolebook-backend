package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// ErrCannotArchiveDM is returned by SetPlayerActive when the target member is
// the campaign DM. Handlers translate this into a 400 BAD_REQUEST. The DM is a
// first-class campaign member but cannot be archived (unset isActive); only
// role:"player" entries support it.
var ErrCannotArchiveDM = errors.New("cannot archive the campaign DM")

// CampaignStore handles persistence for campaigns and embedded sessions.
type CampaignStore struct {
	col *mongo.Collection
}

// NewCampaignStore creates a CampaignStore.
func NewCampaignStore(db *DB) *CampaignStore {
	return &CampaignStore{col: db.Collection("campaigns")}
}

// ListAll returns all campaigns (no filter).
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

// ListByUser returns campaigns where the user is the DM or a player.
func (s *CampaignStore) ListByUser(ctx context.Context, userID string) ([]model.Campaign, error) {
	return s.find(ctx, bson.M{"members.userId": userID})
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

// AddMember appends a CampaignMember to the campaign's embedded members array.
func (s *CampaignStore) AddMember(ctx context.Context, campaignID string, member model.CampaignMember) error {
	_, err := s.col.UpdateOne(
		ctx,
		bson.M{"_id": campaignID},
		bson.M{
			"$push": bson.M{"members": member},
			"$set":  bson.M{"updatedAt": time.Now().UTC()},
		},
	)
	return err
}

// SetPlayerActive flips isActive on a player member entry.
// Returns:
//   - (true, nil) on a successful flip.
//   - (false, ErrCannotArchiveDM) when the member exists but is the DM.
//   - (false, nil) when no member with that playerId exists in the campaign.
//   - (false, err) on a real Mongo error.
//
// The DM-archive guard is double-enforced: the $elemMatch role:"player" filter
// prevents the write atomically, and a follow-up lookup distinguishes
// "member-is-DM" from "no-such-member" so callers (and handlers) get a
// non-ambiguous result.
func (s *CampaignStore) SetPlayerActive(ctx context.Context, campaignID, playerID string, active bool) (bool, error) {
	res, err := s.col.UpdateOne(
		ctx,
		bson.M{
			"_id": campaignID,
			"members": bson.M{"$elemMatch": bson.M{
				"playerId": playerID,
				"role":     model.RolePlayer,
			}},
		},
		bson.M{
			"$set": bson.M{
				"members.$.isActive": active,
				"updatedAt":          time.Now().UTC(),
			},
		},
	)
	if err != nil {
		return false, err
	}
	if res.MatchedCount > 0 {
		return true, nil
	}

	// No match. The member may not exist, OR it may exist but be the DM.
	// Resolve the ambiguity with a second query so the caller can react.
	var c model.Campaign
	err = s.col.FindOne(ctx, bson.M{"_id": campaignID, "members.playerId": playerID}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, m := range c.Members {
		if m.PlayerID == playerID && m.Role == model.RoleDM {
			return false, ErrCannotArchiveDM
		}
	}
	return false, nil
}

// CreateWithDMMember inserts a new Campaign together with the DM's stub Player,
// and seeds the campaign's members[] with a single DM entry pointing at that
// player. The two writes are wrapped in a Mongo transaction; on any error,
// nothing is persisted.
//
// The supplied Campaign must have its Members slice already containing exactly
// one entry: {UserID: dmUserID, PlayerID: dmPlayer.ID, Role: RoleDM, IsActive: true}.
//
// Falls back to a non-transactional sequence (insert player, then insert campaign)
// when the deployment does not support transactions (standalone Mongo). On a
// retry of an interrupted run, the migration tool's idempotency check covers
// the orphan case; for live Create traffic, the fallback also rolls back the
// player insert on a campaign-insert failure.
func (s *CampaignStore) CreateWithDMMember(ctx context.Context, client *mongo.Client, campaign *model.Campaign, dmPlayer *model.Player, players *PlayerStore) error {
	sess, err := client.StartSession()
	if err == nil {
		defer sess.EndSession(ctx)
		_, txErr := sess.WithTransaction(ctx, func(sc context.Context) (interface{}, error) {
			if _, err := players.col.InsertOne(sc, dmPlayer); err != nil {
				return nil, err
			}
			if _, err := s.col.InsertOne(sc, campaign); err != nil {
				return nil, err
			}
			return nil, nil
		})
		if txErr == nil {
			return nil
		}
		// Fall through to compensating sequence on transaction-not-supported
		// errors; rethrow other errors.
		if !isTxnUnsupported(txErr) {
			return txErr
		}
	}

	// Standalone Mongo path: insert player, then campaign; on failure, delete the orphan.
	if _, err := players.col.InsertOne(ctx, dmPlayer); err != nil {
		return err
	}
	if _, err := s.col.InsertOne(ctx, campaign); err != nil {
		_, _ = players.col.DeleteOne(ctx, bson.M{"_id": dmPlayer.ID})
		return err
	}
	return nil
}

// isTxnUnsupported reports whether err signals that the deployment does not
// support transactions (e.g. standalone Mongo). Prefers the driver's structured
// error code; falls back to substring matching if the error doesn't unwrap to
// a CommandError (for forward-compat with future driver wrapping changes).
func isTxnUnsupported(err error) bool {
	if err == nil {
		return false
	}
	// Mongo server returns IllegalOperation (code 20) on standalone deployments
	// when a transaction is attempted. The mongo-driver/v2 wraps server errors
	// in CommandError, which exposes the numeric code.
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == 20 {
		return true
	}
	// Fallback: substring match against the historical wording.
	// "Transaction numbers are only allowed on a replica set member or mongos."
	msg := err.Error()
	return strings.Contains(msg, "replica set") || strings.Contains(msg, "Transaction numbers")
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
