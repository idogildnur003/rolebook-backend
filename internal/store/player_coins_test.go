package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

// newTestPlayerStore connects to the Mongo at ROLEBOOK_TEST_MONGO_URI and
// returns a PlayerStore. Tests skip when the variable is unset.
// WARNING: NewDB always uses the "rolebook" database, so point the URI at a
// local or disposable Mongo only. Each test inserts and removes its own player.
func newTestPlayerStore(t *testing.T) *PlayerStore {
	t.Helper()
	uri := os.Getenv("ROLEBOOK_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("ROLEBOOK_TEST_MONGO_URI not set; skipping Mongo integration test")
	}
	db, err := NewDB(uri)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Disconnect(context.Background()) })
	return NewPlayerStore(db)
}

func randomPlayerID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "test-coins-" + hex.EncodeToString(b)
}

// insertTestPlayer inserts doc (which must carry an _id) and registers cleanup.
func insertTestPlayer(t *testing.T, s *PlayerStore, doc bson.M) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.col.InsertOne(ctx, doc); err != nil {
		t.Fatalf("insert player: %v", err)
	}
	t.Cleanup(func() { _, _ = s.col.DeleteOne(context.Background(), bson.M{"_id": doc["_id"]}) })
}

func newCoinPlayer(t *testing.T, s *PlayerStore, coins model.Coins) string {
	t.Helper()
	id := randomPlayerID(t)
	insertTestPlayer(t, s, bson.M{
		"_id":        id,
		"campaignId": "c",
		"coins":      coins,
		"updatedAt":  time.Now().UTC().Add(-time.Hour),
	})
	return id
}

func readCoins(t *testing.T, s *PlayerStore, id string) model.Coins {
	t.Helper()
	var doc struct {
		Coins model.Coins `bson:"coins"`
	}
	if err := s.col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatalf("read player: %v", err)
	}
	return doc.Coins
}

func TestAdjustCoins_AddAndSpend(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newCoinPlayer(t, s, model.Coins{})

	got, err := s.AdjustCoins(context.Background(), id, map[string]int{"gp": 5, "sp": 3})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if want := (model.Coins{SP: 3, GP: 5}); *got != want {
		t.Fatalf("after add: got %+v, want %+v", *got, want)
	}

	got, err = s.AdjustCoins(context.Background(), id, map[string]int{"gp": -2, "sp": -3})
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	if want := (model.Coins{GP: 3}); *got != want {
		t.Fatalf("after spend: got %+v, want %+v", *got, want)
	}
}

func TestAdjustCoins_Insufficient(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newCoinPlayer(t, s, model.Coins{GP: 1})

	_, err := s.AdjustCoins(context.Background(), id, map[string]int{"gp": -2, "sp": 1})
	if !errors.Is(err, ErrInsufficientCoins) {
		t.Fatalf("err = %v, want ErrInsufficientCoins", err)
	}
	if got, want := readCoins(t, s, id), (model.Coins{GP: 1}); got != want {
		t.Fatalf("coins changed: got %+v, want %+v", got, want)
	}
}

func TestAdjustCoins_LegacyDocument(t *testing.T) {
	s := newTestPlayerStore(t)
	id := randomPlayerID(t)
	insertTestPlayer(t, s, bson.M{"_id": id, "campaignId": "c"})

	_, err := s.AdjustCoins(context.Background(), id, map[string]int{"gp": -1})
	if !errors.Is(err, ErrInsufficientCoins) {
		t.Fatalf("err = %v, want ErrInsufficientCoins", err)
	}

	got, err := s.AdjustCoins(context.Background(), id, map[string]int{"pp": 2})
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if want := (model.Coins{PP: 2}); *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
}

func TestConvertCoins_Success(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newCoinPlayer(t, s, model.Coins{PP: 1})

	got, err := s.ConvertCoins(context.Background(), id, "pp", "gp", 1, 10)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if want := (model.Coins{GP: 10}); *got != want {
		t.Fatalf("got %+v, want %+v", *got, want)
	}
}

func TestConvertCoins_Insufficient(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newCoinPlayer(t, s, model.Coins{CP: 50})

	_, err := s.ConvertCoins(context.Background(), id, "cp", "gp", 100, 1)
	if !errors.Is(err, ErrInsufficientCoins) {
		t.Fatalf("err = %v, want ErrInsufficientCoins", err)
	}
	if got, want := readCoins(t, s, id), (model.Coins{CP: 50}); got != want {
		t.Fatalf("coins changed: got %+v, want %+v", got, want)
	}
}

func TestAdjustCoins_BumpsUpdatedAt(t *testing.T) {
	s := newTestPlayerStore(t)
	id := newCoinPlayer(t, s, model.Coins{})

	var before, after struct {
		UpdatedAt time.Time `bson:"updatedAt"`
	}
	if err := s.col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&before); err != nil {
		t.Fatalf("read before: %v", err)
	}
	if _, err := s.AdjustCoins(context.Background(), id, map[string]int{"cp": 1}); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if err := s.col.FindOne(context.Background(), bson.M{"_id": id}).Decode(&after); err != nil {
		t.Fatalf("read after: %v", err)
	}
	if !after.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("updatedAt not bumped: before %v, after %v", before.UpdatedAt, after.UpdatedAt)
	}
}
