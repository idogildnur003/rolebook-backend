package activity

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
)

const insertTimeout = 3 * time.Second

// Inserter persists a history entry.
type Inserter interface {
	Insert(ctx context.Context, e *model.ActivityEntry) error
}

// CampaignLookup loads a campaign.
type CampaignLookup interface {
	GetByID(ctx context.Context, id string) (*model.Campaign, error)
}

// PlayerLookup loads a character. Calls use the unfiltered form (userID "", isDM true).
type PlayerLookup interface {
	Get(ctx context.Context, id, userID string, isDM bool) (*model.Player, error)
}

// recordInput is everything captured from the request before the handler's
// goroutine ends. chi pools its route context, so nothing may read it later.
type recordInput struct {
	method, pattern, actor     string
	campaignParam, playerParam string
	desc                       Desc
	at                         time.Time
}

// Record writes one history entry for each successful mutation. It never reads
// the request body, and logging cannot fail or delay the response.
func Record(store Inserter, campaigns CampaignLookup, players PlayerLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			at := time.Now().UTC()
			ctx, holder := WithHolder(r.Context())
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			if status < 200 || status >= 300 {
				return
			}
			desc, _, skipped := holder.Snapshot()
			if skipped {
				return
			}
			in := recordInput{method: r.Method, actor: middleware.UserIDFromContext(r.Context()), desc: desc, at: at}
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				in.pattern = rctx.RoutePattern()
				in.playerParam = rctx.URLParam("playerId")
				in.campaignParam = rctx.URLParam("campaignId")
				if in.campaignParam == "" && strings.HasPrefix(in.pattern, "/api/campaigns/{id}") {
					in.campaignParam = rctx.URLParam("id")
				}
			}
			go persist(store, campaigns, players, in)
		})
	}
}

func persist(store Inserter, campaigns CampaignLookup, players PlayerLookup, in recordInput) {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[activity] panic recording %s %s: %v", in.method, in.pattern, p)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
	defer cancel()

	entry := buildEntry(ctx, campaigns, players, in)
	if entry == nil {
		return
	}
	if err := store.Insert(ctx, entry); err != nil {
		log.Printf("[activity] insert failed for %s %s: %v", in.method, in.pattern, err)
	}
}

func buildEntry(ctx context.Context, campaigns CampaignLookup, players PlayerLookup, in recordInput) *model.ActivityEntry {
	info := routeInfo(in.method, in.pattern)
	if info.Skip {
		return nil
	}
	d := in.desc
	e := &model.ActivityEntry{
		ID:          uuid.NewString(),
		PlayerID:    firstNonEmpty(d.PlayerID, in.playerParam),
		PlayerName:  d.PlayerName,
		ActorUserID: in.actor,
		Type:        firstNonEmpty(d.Type, info.Type),
		Category:    firstNonEmpty(d.Category, info.Category),
		Title:       firstNonEmpty(d.Title, info.Title),
		Detail:      d.Detail,
		Changes:     d.Changes,
		Method:      in.method,
		Route:       in.pattern,
		CreatedAt:   in.at,
	}

	subject := d.SubjectUserID
	campaignID := d.CampaignID
	if e.PlayerID != "" {
		if p := lookupPlayer(ctx, players, e.PlayerID); p != nil {
			e.PlayerName = firstNonEmpty(e.PlayerName, p.Name)
			campaignID = firstNonEmpty(campaignID, p.CampaignID)
			if subject == "" {
				subject = characterOwner(ctx, players, p)
			}
		}
	}
	e.SubjectUserID = firstNonEmpty(subject, in.actor)

	if !info.AccountLevel {
		e.CampaignID = firstNonEmpty(campaignID, in.campaignParam)
	}
	e.ActorRole = string(model.RolePlayer)
	if e.CampaignID == "" {
		return e
	}

	campaign, err := campaigns.GetByID(ctx, e.CampaignID)
	if err != nil {
		log.Printf("[activity] campaign lookup failed: %v", err)
	}
	if campaign == nil {
		return e
	}
	e.SessionID, e.SessionName, e.SessionDate = StampSession(campaign, in.at)
	for i := range campaign.Members {
		m := &campaign.Members[i]
		if m.UserID != in.actor {
			continue
		}
		if m.Role == model.RoleDM && in.actor != e.SubjectUserID {
			e.ActorRole = string(model.RoleDM)
		}
		if p := lookupPlayer(ctx, players, m.PlayerID); p != nil {
			e.ActorName = p.Name
		}
		if e.ActorName == "" {
			e.ActorName = "Player"
			if m.Role == model.RoleDM {
				e.ActorName = "DM"
			}
		}
		break
	}
	return e
}

func lookupPlayer(ctx context.Context, players PlayerLookup, id string) *model.Player {
	p, err := players.Get(ctx, id, "", true)
	if err != nil {
		log.Printf("[activity] player lookup failed: %v", err)
		return nil
	}
	return p
}

// characterOwner returns the user whose history a character's changes belong to.
func characterOwner(ctx context.Context, players PlayerLookup, p *model.Player) string {
	if p.LinkedUserID != "" || p.OwnerPlayerID == "" {
		return p.LinkedUserID
	}
	if owner := lookupPlayer(ctx, players, p.OwnerPlayerID); owner != nil {
		return owner.LinkedUserID
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
