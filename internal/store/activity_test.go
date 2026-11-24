package store

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
)

func clause(t *testing.T, q bson.M, key string) any {
	t.Helper()
	and, _ := q["$and"].([]bson.M)
	for _, c := range and {
		if v, ok := c[key]; ok {
			return v
		}
	}
	t.Fatalf("no %q clause in %v", key, q)
	return nil
}

func TestActivityQuery_AlwaysScopedToSubject(t *testing.T) {
	q := activityQuery(ActivityFilter{CampaignID: "c", SubjectUserID: "u"})
	if clause(t, q, "campaignId") != "c" || clause(t, q, "subjectUserId") != "u" {
		t.Fatalf("got %v", q)
	}
}

func TestActivityQuery_BetweenMeansNoSession(t *testing.T) {
	q := activityQuery(ActivityFilter{CampaignID: "c", SubjectUserID: "u", Session: "between"})
	if !reflect.DeepEqual(clause(t, q, "sessionId"), bson.M{"$exists": false}) {
		t.Fatalf("got %v", q)
	}
}

func TestActivityQuery_Filters(t *testing.T) {
	q := activityQuery(ActivityFilter{CampaignID: "c", SubjectUserID: "u", Session: "s1", Categories: []string{"coins", "hp"}, ByDM: true})
	if clause(t, q, "sessionId") != "s1" || clause(t, q, "actorRole") != "dm" {
		t.Fatalf("got %v", q)
	}
	if !reflect.DeepEqual(clause(t, q, "category"), bson.M{"$in": []string{"coins", "hp"}}) {
		t.Fatalf("got %v", q)
	}
}

func TestActivityQuery_SearchIsLiteral(t *testing.T) {
	q := activityQuery(ActivityFilter{CampaignID: "c", SubjectUserID: "u", Query: "(.*"})
	or := clause(t, q, "$or").([]bson.M)
	want := bson.M{"$regex": `\(\.\*`, "$options": "i"}
	if !reflect.DeepEqual(or[0]["title"], want) || !reflect.DeepEqual(or[1]["detail"], want) {
		t.Fatalf("got %v", or)
	}
}

func TestActivityQuery_SearchAndCursorCombine(t *testing.T) {
	at := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	q := activityQuery(ActivityFilter{CampaignID: "c", SubjectUserID: "u", Query: "gp", BeforeTime: at, BeforeID: "e5"})
	and := q["$and"].([]bson.M)
	ors := 0
	for _, c := range and {
		if _, ok := c["$or"]; ok {
			ors++
		}
	}
	if ors != 2 {
		t.Fatalf("expected separate search and cursor clauses, got %v", and)
	}
}

func newTestActivityStore(t *testing.T) (*ActivityStore, string) {
	t.Helper()
	uri := os.Getenv("ROLEBOOK_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("ROLEBOOK_TEST_MONGO_URI not set; skipping Mongo integration test")
	}
	db, err := NewDB(uri)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	s := NewActivityStore(db)
	campaignID := "test-activity-" + uuid.NewString()
	t.Cleanup(func() {
		_ = s.DeleteByCampaign(context.Background(), campaignID)
		_ = db.Disconnect(context.Background())
	})
	return s, campaignID
}

func insertActivity(t *testing.T, s *ActivityStore, e model.ActivityEntry) {
	t.Helper()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if err := s.Insert(context.Background(), &e); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func TestActivityList_OnlySubject(t *testing.T) {
	s, c := newTestActivityStore(t)
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", Title: "mine", CreatedAt: time.Now()})
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "b", Title: "theirs", CreatedAt: time.Now()})
	got, err := s.List(context.Background(), ActivityFilter{CampaignID: c, SubjectUserID: "a", Limit: 10})
	if err != nil || len(got) != 1 || got[0].Title != "mine" {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestActivityList_CursorNoDuplicates(t *testing.T) {
	s, c := newTestActivityStore(t)
	at := time.Now().UTC().Truncate(time.Millisecond)
	for i := 0; i < 5; i++ {
		insertActivity(t, s, model.ActivityEntry{ID: fmt.Sprintf("e%d", i), CampaignID: c, SubjectUserID: "a", CreatedAt: at})
	}
	seen := map[string]bool{}
	f := ActivityFilter{CampaignID: c, SubjectUserID: "a", Limit: 2}
	for {
		page, err := s.List(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			if seen[e.ID] {
				t.Fatalf("duplicate %s", e.ID)
			}
			seen[e.ID] = true
		}
		last := page[len(page)-1]
		f.BeforeTime, f.BeforeID = last.CreatedAt, last.ID
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d", len(seen))
	}
}

func TestActivityList_Filters(t *testing.T) {
	s, c := newTestActivityStore(t)
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", SessionID: "s1", Category: "coins", ActorRole: "dm", CreatedAt: time.Now()})
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", Category: "hp", ActorRole: "player", CreatedAt: time.Now()})
	for name, f := range map[string]ActivityFilter{
		"session":  {Session: "s1"},
		"between":  {Session: "between"},
		"category": {Categories: []string{"hp"}},
		"byDM":     {ByDM: true},
	} {
		f.CampaignID, f.SubjectUserID, f.Limit = c, "a", 10
		got, err := s.List(context.Background(), f)
		if err != nil || len(got) != 1 {
			t.Fatalf("%s: got %d err %v", name, len(got), err)
		}
	}
}

func TestActivityList_QueryIsLiteral(t *testing.T) {
	s, c := newTestActivityStore(t)
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", Title: "Coins adjusted", CreatedAt: time.Now()})
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", Title: "literal .* text", CreatedAt: time.Now()})
	got, err := s.List(context.Background(), ActivityFilter{CampaignID: c, SubjectUserID: "a", Query: ".*", Limit: 10})
	if err != nil || len(got) != 1 || got[0].Title != "literal .* text" {
		t.Fatalf("got %v err %v", got, err)
	}
	if _, err := s.List(context.Background(), ActivityFilter{CampaignID: c, SubjectUserID: "a", Query: "(", Limit: 10}); err != nil {
		t.Fatalf("paren: %v", err)
	}
}

func TestActivityDeleteByCampaign(t *testing.T) {
	s, c := newTestActivityStore(t)
	insertActivity(t, s, model.ActivityEntry{CampaignID: c, SubjectUserID: "a", CreatedAt: time.Now()})
	if err := s.DeleteByCampaign(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got, _ := s.List(context.Background(), ActivityFilter{CampaignID: c, SubjectUserID: "a", Limit: 10})
	if len(got) != 0 {
		t.Fatalf("got %d", len(got))
	}
}
