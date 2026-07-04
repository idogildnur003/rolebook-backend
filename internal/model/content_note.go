package model

import "time"

// ContentNote is a player's PRIVATE note on an item or spell they hold.
// Owner-scoped: it is stored in its own collection and is NEVER included in any
// DM-visible payload (privacy holds by construction, not by field-stripping).
type ContentNote struct {
	ID         string    `json:"id"         bson:"_id"`
	CampaignID string    `json:"campaignId" bson:"campaignId"`
	UserID     string    `json:"userId"     bson:"userId"`
	TargetType string    `json:"targetType" bson:"targetType"` // item | spell
	EntryID    string    `json:"entryId"    bson:"entryId"`
	Body       string    `json:"body"       bson:"body"`
	UpdatedAt  time.Time `json:"updatedAt"  bson:"updatedAt"`
}

// IsValidNoteTarget reports whether t is a supported note target type.
// Reuses the request target vocabulary (item | spell).
func IsValidNoteTarget(t string) bool {
	return t == RequestTargetItem || t == RequestTargetSpell
}

// ContentNoteKey is a stable composite key identifying one note
// (one per user+campaign+targetType+entry). For logging/dedup only.
func ContentNoteKey(userID, campaignID, targetType, entryID string) string {
	return userID + "|" + campaignID + "|" + targetType + "|" + entryID
}
