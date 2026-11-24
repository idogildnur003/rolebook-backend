package model

import "time"

// Activity categories, used by the History screen's filter chips.
const (
	ActivityCategoryCoins     = "coins"
	ActivityCategoryHP        = "hp"
	ActivityCategorySpells    = "spells"
	ActivityCategoryItems     = "items"
	ActivityCategoryAbilities = "abilities"
	ActivityCategoryNotes     = "notes"
	ActivityCategoryRequests  = "requests"
	ActivityCategorySchedule  = "schedule"
	ActivityCategoryCombat    = "combat"
	ActivityCategoryCharacter = "character"
	ActivityCategoryOther     = "other"
)

// ActivityChange is one before → after value on a history entry. A nil Before
// means the value did not exist; a nil After means it was removed. Both nil
// records that a private or free-text field changed without storing its value.
type ActivityChange struct {
	Field  string `bson:"field"  json:"field"`
	Before any    `bson:"before" json:"before"`
	After  any    `bson:"after"  json:"after"`
}

// ActivityEntry is one logged change, shown in SubjectUserID's history.
type ActivityEntry struct {
	ID            string           `bson:"_id"                   json:"id"`
	CampaignID    string           `bson:"campaignId"            json:"campaignId"`
	SubjectUserID string           `bson:"subjectUserId"         json:"-"`
	PlayerID      string           `bson:"playerId,omitempty"    json:"playerId,omitempty"`
	PlayerName    string           `bson:"playerName,omitempty"  json:"playerName,omitempty"`
	ActorUserID   string           `bson:"actorUserId"           json:"actorUserId"`
	ActorRole     string           `bson:"actorRole"             json:"actorRole"`
	ActorName     string           `bson:"actorName"             json:"actorName"`
	Type          string           `bson:"type"                  json:"type"`
	Category      string           `bson:"category"              json:"category"`
	Title         string           `bson:"title"                 json:"title"`
	Detail        string           `bson:"detail,omitempty"      json:"detail,omitempty"`
	Changes       []ActivityChange `bson:"changes,omitempty"     json:"changes,omitempty"`
	SessionID     string           `bson:"sessionId,omitempty"   json:"sessionId,omitempty"`
	SessionName   string           `bson:"sessionName,omitempty" json:"-"`
	SessionDate   string           `bson:"sessionDate,omitempty" json:"-"`
	Method        string           `bson:"method"                json:"-"`
	Route         string           `bson:"route"                 json:"-"`
	CreatedAt     time.Time        `bson:"createdAt"             json:"createdAt"`
}
