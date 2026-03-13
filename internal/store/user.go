package store

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// UserStore handles persistence for user accounts.
type UserStore struct {
	col *mongo.Collection
}

// NewUserStore creates a UserStore and ensures the unique email index exists.
func NewUserStore(db *DB) *UserStore {
	col := db.Collection("users")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &UserStore{col: col}
}

// Create inserts a new user. Returns an error if the email already exists.
func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	_, err := s.col.InsertOne(ctx, u)
	return err
}

// FindByEmail returns the user with the given email, or nil if not found.
func (s *UserStore) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := s.col.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
