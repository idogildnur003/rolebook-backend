package model

import "time"

// Session is a play session embedded inside a Campaign document.
type Session struct {
	ID          string           `bson:"id"                 json:"id"`
	Name        string           `bson:"name"               json:"name"`
	Description string           `bson:"description"        json:"description"`
	UpdatedAt   time.Time        `bson:"updatedAt"          json:"updatedAt"`
	Schedule    *SessionSchedule `bson:"schedule,omitempty" json:"schedule,omitempty"`
}

// SessionDayParts flags which day-parts a participant can make for a date.
type SessionDayParts struct {
	Morning bool `bson:"morning" json:"morning"`
	Noon    bool `bson:"noon"    json:"noon"`
	Evening bool `bson:"evening" json:"evening"`
}

// SessionAvailability records one campaign member's availability across dates.
// Identity is the member's playerId. UserID never appears here, on the wire or
// in storage; auth is resolved by mapping the authenticated user to their
// member's playerId in the existing members[] array.
type SessionAvailability struct {
	PlayerID           string                     `bson:"playerId"           json:"playerId"`
	AvailabilityByDate map[string]SessionDayParts `bson:"availabilityByDate" json:"availabilityByDate"`
	UpdatedAt          time.Time                  `bson:"updatedAt"          json:"updatedAt"`
}

// SessionConfirmedSlot is the DM-picked slot for a session. Date is naive
// calendar date in dmTimezone; StartAt (when present) is UTC.
type SessionConfirmedSlot struct {
	Date            string     `bson:"date"                      json:"date"`
	DayPart         string     `bson:"dayPart"                   json:"dayPart"`
	StartAt         *time.Time `bson:"startAt,omitempty"         json:"startAt,omitempty"`
	DurationMinutes *int       `bson:"durationMinutes,omitempty" json:"durationMinutes,omitempty"`
	ConfirmedAt     time.Time  `bson:"confirmedAt"               json:"confirmedAt"`
}

// SessionSchedule is an optional subdocument on Session.
type SessionSchedule struct {
	DMTimezone                string                `bson:"dmTimezone"                json:"dmTimezone"`
	ParticipantAvailabilities []SessionAvailability `bson:"participantAvailabilities" json:"participantAvailabilities"`
	ConfirmedSlot             *SessionConfirmedSlot `bson:"confirmedSlot,omitempty"   json:"confirmedSlot,omitempty"`
	UpdatedAt                 time.Time             `bson:"updatedAt"                 json:"updatedAt"`
}

// CampaignMember represents a member of a campaign — the DM or a player.
// Every member has a backing Player record (DM's has kind: "dm").
type CampaignMember struct {
	UserID   string `bson:"userId"   json:"-"`
	PlayerID string `bson:"playerId" json:"playerId"`
	Role     Role   `bson:"role"     json:"role"`
	IsActive bool   `bson:"isActive" json:"isActive"`
}

// Campaign is stored in the "campaigns" collection.
// Sessions are embedded to avoid cross-collection joins.
// Membership (DM + players) is unified in Members.
type Campaign struct {
	ID                string           `bson:"_id"               json:"id"`
	Name              string           `bson:"name"              json:"name"`
	ThemeImage        string           `bson:"themeImage"        json:"themeImage"`
	MapImageURI       *string          `bson:"mapImageUri"       json:"mapImageUri"`
	Sessions          []Session        `bson:"sessions"          json:"sessions"`
	Members           []CampaignMember `bson:"members"           json:"members"`
	DisabledSpells    []string         `bson:"disabledSpells"    json:"disabledSpells"`
	DisabledEquipment []string         `bson:"disabledEquipment" json:"disabledEquipment"`
	UpdatedAt         time.Time        `bson:"updatedAt"         json:"updatedAt"`
}

// Campaign-level role constants used in API responses.
type Role string

const (
	RoleDM     Role = "dm"
	RolePlayer Role = "player"
)
