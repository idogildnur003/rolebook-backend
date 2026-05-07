package handler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// visibilityAllowsRead encodes the read filter shared by all journal entities.
// The owner can always read; sharedWithAll opens to every member; sharedPlayerIds
// opens to specific members. Lives here (rather than in any single handler file)
// so all three entity handlers can share it without an import cycle.
func visibilityAllowsRead(ownerPlayerID string, v model.Visibility, callerPlayerID string) bool {
	if ownerPlayerID == callerPlayerID {
		return true
	}
	if v.SharedWithAll {
		return true
	}
	for _, id := range v.SharedPlayerIds {
		if id == callerPlayerID {
			return true
		}
	}
	return false
}

// isValidVisibilityPatch checks that v decodes to the expected
// {sharedWithAll bool, sharedPlayerIds []string} shape. Used by all three
// journal Update handlers to reject malformed visibility patches before they
// reach Mongo.
//
// Tolerates a partial patch (only one of the two fields present). The two
// fields are independently optional in JSON; both being absent is a no-op
// patch. Extra keys are silently ignored — the caller's $set will simply not
// touch them.
func isValidVisibilityPatch(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if sw, ok := m["sharedWithAll"]; ok {
		if _, isBool := sw.(bool); !isBool {
			return false
		}
	}
	if ids, ok := m["sharedPlayerIds"]; ok {
		arr, isArr := ids.([]any)
		if !isArr {
			return false
		}
		for _, e := range arr {
			if _, isString := e.(string); !isString {
				return false
			}
		}
	}
	return true
}

// recipient holds the resolved playerId+userId of a share target.
type recipient struct {
	PlayerID string
	UserID   string
}

// resolveShareRecipients expands a request's recipientPlayerIds and
// sharedWithAll flag against the campaign's current members. The owner is
// excluded automatically. Returns an error if any recipientPlayerIds entry is
// not a current campaign member.
func resolveShareRecipients(ctx context.Context, campaigns *store.CampaignStore, campaignID, ownerPlayerID string, ids []string, all bool) ([]recipient, error) {
	camp, err := campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return nil, errors.New("internal error")
	}
	if camp == nil {
		return nil, errors.New("campaign not found")
	}
	if all {
		out := make([]recipient, 0, len(camp.Members))
		for _, m := range camp.Members {
			if m.PlayerID == ownerPlayerID {
				continue
			}
			out = append(out, recipient{PlayerID: m.PlayerID, UserID: m.UserID})
		}
		return out, nil
	}
	byPlayerID := make(map[string]model.CampaignMember, len(camp.Members))
	for _, m := range camp.Members {
		byPlayerID[m.PlayerID] = m
	}
	out := make([]recipient, 0, len(ids))
	for _, id := range ids {
		if id == ownerPlayerID {
			continue
		}
		m, ok := byPlayerID[id]
		if !ok {
			return nil, errors.New("recipient not in campaign: " + id)
		}
		out = append(out, recipient{PlayerID: m.PlayerID, UserID: m.UserID})
	}
	return out, nil
}

// rewriteIds replaces each id in src with idMap[id] when present; passes
// through otherwise. Order preserved.
func rewriteIds(src []string, idMap map[string]string) []string {
	out := make([]string, 0, len(src))
	for _, id := range src {
		if next, ok := idMap[id]; ok {
			out = append(out, next)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// cloneNpcForRecipient produces (and persists) a clone of an NPC for the given
// recipient, idempotently. Returns the clone id (or the existing clone's id if
// one already existed). Used by both LocationHandler.Share (NPC cascade) and
// MapPinHandler.Share (entity cascade for npc pins).
func cloneNpcForRecipient(ctx context.Context, npcs *store.NPCStore, campaignID, sourceNpcID string, recip recipient) (string, error) {
	existing, err := npcs.FindCloneOf(ctx, campaignID, sourceNpcID, recip.PlayerID)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}
	source, err := npcs.GetByID(ctx, campaignID, sourceNpcID)
	if err != nil {
		return "", err
	}
	if source == nil {
		// Source missing — caller's cascade list referenced an id we can't see.
		// Skip silently; caller continues without that link rewritten.
		return "", nil
	}
	now := time.Now().UTC()
	clone := *source
	clone.ID = uuid.NewString()
	clone.OwnerPlayerID = recip.PlayerID
	clone.OwnerUserID = recip.UserID
	clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
	clone.ShareNote = ""
	clone.Clone = &model.CloneAudit{
		ClonedFromEntryId:       source.ID,
		ClonedFromOwnerPlayerId: source.OwnerPlayerID,
		ClonedAt:                now,
	}
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if err := npcs.Create(ctx, &clone); err != nil {
		return "", err
	}
	return clone.ID, nil
}

// cascadeLinkedNpcs walks sourceLinkedNpcIds and, for each id present in
// cascadeIds, calls cloneNpcForRecipient. Returns a map of original-id → clone-id
// used to rewrite the parent location clone's linkedNpcIds.
func cascadeLinkedNpcs(ctx context.Context, npcs *store.NPCStore, campaignID string, sourceLinkedNpcIds []string, recip recipient, cascadeIds []string) (map[string]string, error) {
	wanted := make(map[string]struct{}, len(cascadeIds))
	for _, id := range cascadeIds {
		wanted[id] = struct{}{}
	}
	out := make(map[string]string, len(cascadeIds))
	for _, originalNpcID := range sourceLinkedNpcIds {
		if _, ok := wanted[originalNpcID]; !ok {
			continue
		}
		cloneID, err := cloneNpcForRecipient(ctx, npcs, campaignID, originalNpcID, recip)
		if err != nil {
			return nil, err
		}
		if cloneID != "" {
			out[originalNpcID] = cloneID
		}
	}
	return out, nil
}

// cascadeLinkedLocations is the NPC-side mirror of cascadeLinkedNpcs. Used by
// NPCHandler.Share (Task 16). Defined here so both share handlers can call it.
func cascadeLinkedLocations(ctx context.Context, locations *store.LocationStore, campaignID string, sourceLinkedLocationIds []string, recip recipient, cascadeIds []string) (map[string]string, error) {
	wanted := make(map[string]struct{}, len(cascadeIds))
	for _, id := range cascadeIds {
		wanted[id] = struct{}{}
	}
	out := make(map[string]string, len(cascadeIds))
	for _, originalLocationID := range sourceLinkedLocationIds {
		if _, ok := wanted[originalLocationID]; !ok {
			continue
		}
		cloneID, err := cloneLocationForRecipientShallow(ctx, locations, campaignID, originalLocationID, recip)
		if err != nil {
			return nil, err
		}
		if cloneID != "" {
			out[originalLocationID] = cloneID
		}
	}
	return out, nil
}

// cloneLocationForRecipientShallow is the location-cloning sibling of
// cloneNpcForRecipient: idempotent, no recursive NPC cascade. Used by
// NPCHandler's cascadeLinkedLocations (Task 16) — when an NPC clone references
// a location, we shallow-clone the location (no nested cascades) so the
// recipient gets something readable.
func cloneLocationForRecipientShallow(ctx context.Context, locations *store.LocationStore, campaignID, sourceID string, recip recipient) (string, error) {
	existing, err := locations.FindCloneOf(ctx, campaignID, sourceID, recip.PlayerID)
	if err != nil {
		return "", err
	}
	if existing != nil {
		return existing.ID, nil
	}
	source, err := locations.GetByID(ctx, campaignID, sourceID)
	if err != nil {
		return "", err
	}
	if source == nil {
		return "", nil
	}
	now := time.Now().UTC()
	clone := *source
	clone.ID = uuid.NewString()
	clone.OwnerPlayerID = recip.PlayerID
	clone.OwnerUserID = recip.UserID
	clone.LinkedNpcIds = []string{} // shallow: don't carry NPC links
	clone.Visibility = model.Visibility{SharedPlayerIds: []string{}}
	clone.ShareNote = ""
	clone.Clone = &model.CloneAudit{
		ClonedFromEntryId:       source.ID,
		ClonedFromOwnerPlayerId: source.OwnerPlayerID,
		ClonedAt:                now,
	}
	clone.CreatedAt = now
	clone.UpdatedAt = now
	if err := locations.Create(ctx, &clone); err != nil {
		return "", err
	}
	return clone.ID, nil
}
