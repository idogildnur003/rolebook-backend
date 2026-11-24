package activity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
)

type fakeInserter struct {
	ch    chan model.ActivityEntry
	block chan struct{}
	err   error
}

func newFakeInserter() *fakeInserter { return &fakeInserter{ch: make(chan model.ActivityEntry, 4)} }

func (f *fakeInserter) Insert(ctx context.Context, e *model.ActivityEntry) error {
	if f.block != nil {
		<-f.block
	}
	f.ch <- *e
	return f.err
}

func (f *fakeInserter) one(t *testing.T) model.ActivityEntry {
	t.Helper()
	select {
	case e := <-f.ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("no entry recorded")
		return model.ActivityEntry{}
	}
}

func (f *fakeInserter) none(t *testing.T) {
	t.Helper()
	select {
	case e := <-f.ch:
		t.Fatalf("unexpected entry: %+v", e)
	case <-time.After(60 * time.Millisecond):
	}
}

type fakeCampaigns struct{ c *model.Campaign }

func (f fakeCampaigns) GetByID(context.Context, string) (*model.Campaign, error) { return f.c, nil }

type fakePlayers map[string]*model.Player

func (f fakePlayers) Get(_ context.Context, id, _ string, _ bool) (*model.Player, error) {
	return f[id], nil
}

var testCampaign = &model.Campaign{ID: "c1", Members: []model.CampaignMember{
	{UserID: "dm", PlayerID: "dmp", Role: model.RoleDM},
	{UserID: "u1", PlayerID: "p1", Role: model.RolePlayer},
}}

var testPlayers = fakePlayers{
	"dmp": {ID: "dmp", CampaignID: "c1", LinkedUserID: "dm", Name: "Gil", Kind: "dm"},
	"p1":  {ID: "p1", CampaignID: "c1", LinkedUserID: "u1", Name: "Thalia", Kind: "pc"},
	"pet": {ID: "pet", CampaignID: "c1", Name: "Biscuit", Kind: "companion", OwnerPlayerID: "p1"},
}

// actorAuth is a test stand-in for Authenticate.
func serve(t *testing.T, s Inserter, method, pattern, target, actor string, body io.Reader, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(middleware.ContextWithUserID(req.Context(), actor)))
		})
	})
	r.Use(Record(s, fakeCampaigns{testCampaign}, testPlayers))
	r.Method(method, pattern, h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, body))
	return w
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

const inventoryRoute = "/api/players/{playerId}/inventory/{equipmentId}"

func TestRecord_IgnoresGet(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "GET", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, ok)
	s.none(t)
}

func TestRecord_Only2xx(t *testing.T) {
	for _, status := range []int{400, 500} {
		s := newFakeInserter()
		serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		})
		s.none(t)
	}
}

func TestRecord_ImplicitOK(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{}"))
	})
	s.one(t)
}

func TestRecord_Skip(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, func(w http.ResponseWriter, r *http.Request) {
		Skip(r)
		w.WriteHeader(http.StatusOK)
	})
	s.none(t)
}

func TestRecord_GenericFallback(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, ok)
	e := s.one(t)
	if e.Title != "Updated inventory item" || e.Category != model.ActivityCategoryItems || e.CampaignID != "c1" || e.PlayerName != "Thalia" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_UnregisteredRoute(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "POST", "/api/campaigns/{campaignId}/brand-new", "/api/campaigns/c1/brand-new", "u1", nil, ok)
	e := s.one(t)
	if e.Type != "generic" || e.Category != model.ActivityCategoryOther || e.Title == "" || e.SubjectUserID != "u1" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_DMOnPlayerIsBadged(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "dm", nil, ok)
	e := s.one(t)
	if e.ActorRole != "dm" || e.SubjectUserID != "u1" || e.ActorName != "Gil" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_DMOnCompanionGoesToOwner(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/pet/inventory/e1", "dm", nil, ok)
	e := s.one(t)
	if e.ActorRole != "dm" || e.SubjectUserID != "u1" || e.PlayerName != "Biscuit" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_PlayerOnOwnCharacter(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, ok)
	if e := s.one(t); e.ActorRole != "player" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_DMOwnActionNotBadged(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "POST", "/api/campaigns/{campaignId}/locations", "/api/campaigns/c1/locations", "dm", nil, ok)
	e := s.one(t)
	if e.ActorRole != "player" || e.SubjectUserID != "dm" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_IDParamNotCampaignOnSubroutes(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", "/api/uploads/{id}", "/api/uploads/not-a-campaign", "u1", nil, ok)
	if e := s.one(t); e.CampaignID != "" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_CampaignIDParamOnCampaignRoute(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "PATCH", "/api/campaigns/{id}", "/api/campaigns/c1", "dm", nil, ok)
	if e := s.one(t); e.CampaignID != "c1" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_AccountLevelHasNoCampaign(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "DELETE", "/api/campaigns/{id}", "/api/campaigns/c1", "dm", nil, ok)
	if e := s.one(t); e.CampaignID != "" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_DescribeOverridesSubject(t *testing.T) {
	s := newFakeInserter()
	serve(t, s, "POST", "/api/campaigns/{campaignId}/content-requests/{id}/approve", "/api/campaigns/c1/content-requests/r1/approve", "dm", nil, func(w http.ResponseWriter, r *http.Request) {
		Describe(r, Desc{SubjectUserID: "u1", Detail: "Moonlit Dagger"})
		w.WriteHeader(http.StatusOK)
	})
	e := s.one(t)
	if e.SubjectUserID != "u1" || e.ActorRole != "dm" || e.Detail != "Moonlit Dagger" {
		t.Fatalf("got %+v", e)
	}
}

func TestRecord_SlowInsertDoesNotBlock(t *testing.T) {
	s := newFakeInserter()
	s.block = make(chan struct{})
	defer close(s.block)
	start := time.Now()
	w := serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, ok)
	if d := time.Since(start); d > 100*time.Millisecond || w.Code != 200 {
		t.Fatalf("took %v code %d", d, w.Code)
	}
}

func TestRecord_InsertErrorKeepsResponse(t *testing.T) {
	s := newFakeInserter()
	s.err = errors.New("mongo down")
	w := serve(t, s, "PATCH", inventoryRoute, "/api/players/p1/inventory/e1", "u1", nil, ok)
	s.one(t)
	if w.Code != 200 {
		t.Fatalf("code %d", w.Code)
	}
}

type trackedBody struct {
	mu   sync.Mutex
	read bool
	r    io.Reader
}

func (b *trackedBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.read = true
	b.mu.Unlock()
	return b.r.Read(p)
}

func TestRecord_NeverReadsBody(t *testing.T) {
	s := newFakeInserter()
	body := &trackedBody{r: strings.NewReader(`{"password":"hunter2"}`)}
	serve(t, s, "POST", "/api/auth/change-password", "/api/auth/change-password", "u1", body, ok)
	e := s.one(t)
	body.mu.Lock()
	defer body.mu.Unlock()
	if body.read || len(e.Changes) != 0 || e.Detail != "" || e.CampaignID != "" {
		t.Fatalf("read=%v entry=%+v", body.read, e)
	}
}
