package model

import "time"

// MapPin is a labelled point on a campaign map image.
type MapPin struct {
	ID    string  `bson:"id"    json:"id"`
	X     float64 `bson:"x"     json:"x"`
	Y     float64 `bson:"y"     json:"y"`
	Label string  `bson:"label" json:"label"`
}

// SessionAvailabilityByPart holds per-day-part availability flags for a
// single participant on a single date.
type SessionAvailabilityByPart struct {
	Morning *bool `bson:"morning,omitempty" json:"morning,omitempty"`
	Noon    *bool `bson:"noon,omitempty"    json:"noon,omitempty"`
	Evening *bool `bson:"evening,omitempty" json:"evening,omitempty"`
}

// SessionParticipantAvailability stores one participant's availability
// keyed by date string (e.g. "2026-04-15").
type SessionParticipantAvailability struct {
	UserID             string                               `bson:"userId"             json:"userId"`
	AvailabilityByDate map[string]SessionAvailabilityByPart `bson:"availabilityByDate" json:"availabilityByDate"`
	UpdatedAt          int64                                `bson:"updatedAt"          json:"updatedAt"`
}

// SessionSchedule holds the scheduling data for a session.
// Timestamps (CreatedAt, UpdatedAt) are Unix milliseconds to match the
// frontend convention.
type SessionSchedule struct {
	CreatedAt                 int64                            `bson:"createdAt"                 json:"createdAt"`
	UpdatedAt                 int64                            `bson:"updatedAt"                 json:"updatedAt"`
	ParticipantAvailabilities []SessionParticipantAvailability `bson:"participantAvailabilities" json:"participantAvailabilities"`
}

// Session is a play session embedded inside a Campaign document.
type Session struct {
	ID          string           `bson:"id"          json:"id"`
	Name        string           `bson:"name"        json:"name"`
	Description string           `bson:"description" json:"description"`
	Schedule    *SessionSchedule `bson:"schedule,omitempty" json:"schedule,omitempty"`
	UpdatedAt   time.Time        `bson:"updatedAt"   json:"updatedAt"`
}

// CampaignPlayer represents a player in a campaign.
type CampaignPlayer struct {
	UserID   string `bson:"userId"   json:"-"`
	PlayerID string `bson:"playerId" json:"playerId"`
	IsActive bool   `bson:"isActive" json:"isActive"`
}

// Campaign is stored in the "campaigns" collection.
// Sessions are embedded to avoid cross-collection joins.
// DM is the user ID of the campaign's Dungeon Master (the user who created it).
type Campaign struct {
	ID                string           `bson:"_id"         json:"id"`
	DM                string           `bson:"dm"          json:"dm"`
	Name              string           `bson:"name"        json:"name"`
	ThemeImage        string           `bson:"themeImage"  json:"themeImage"`
	MapImageURI       *string          `bson:"mapImageUri" json:"mapImageUri"`
	MapPins           []MapPin         `bson:"mapPins"     json:"mapPins"`
	Sessions          []Session        `bson:"sessions"    json:"sessions"`
	Players           []CampaignPlayer `bson:"players"     json:"players"`
	DisabledSpells    []string         `bson:"disabledSpells"    json:"disabledSpells"`
	DisabledEquipment []string         `bson:"disabledEquipment" json:"disabledEquipment"`
	UpdatedAt         time.Time        `bson:"updatedAt"   json:"updatedAt"`
}

// Campaign-level role constants used in API responses.
type Role string

const (
	RoleDM     Role = "dm"
	RolePlayer Role = "player"
)
