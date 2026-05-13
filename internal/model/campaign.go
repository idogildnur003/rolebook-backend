package model

import "time"

// Session is a play session embedded inside a Campaign document.
type Session struct {
	ID          string    `bson:"id"          json:"id"`
	Name        string    `bson:"name"        json:"name"`
	Description string    `bson:"description" json:"description"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}

// CampaignMember represents a member of a campaign — the DM or a player.
// Every member has a backing Player record (DM's has kind: "dm").
type CampaignMember struct {
	UserID       string            `bson:"userId"                 json:"-"`
	PlayerID     string            `bson:"playerId"               json:"playerId"`
	Role         Role              `bson:"role"                   json:"role"`
	IsActive     bool              `bson:"isActive"               json:"isActive"`
	SessionNotes map[string]string `bson:"sessionNotes,omitempty" json:"-"`
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
