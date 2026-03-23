package model

import "time"

// MapPin is a labelled point on a campaign map image.
type MapPin struct {
	ID    string  `bson:"id"    json:"id"`
	X     float64 `bson:"x"     json:"x"`
	Y     float64 `bson:"y"     json:"y"`
	Label string  `bson:"label" json:"label"`
}

// Session is a play session embedded inside a Campaign document.
type Session struct {
	ID          string    `bson:"id"          json:"id"`
	Name        string    `bson:"name"        json:"name"`
	Description string    `bson:"description" json:"description"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}

// CampaignPlayer represents a player in a campaign.
type CampaignPlayer struct {
	UserID   string `bson:"userId"   json:"userId"`
	IsActive bool   `bson:"isActive" json:"isActive"`
}

// Campaign is stored in the "campaigns" collection.
// Sessions are embedded to avoid cross-collection joins.
// DM is the user ID of the campaign's Dungeon Master (the user who created it).
type Campaign struct {
	ID          string           `bson:"_id"         json:"id"`
	DM          string           `bson:"dm"          json:"dm"`
	Name        string           `bson:"name"        json:"name"`
	ThemeImage  string           `bson:"themeImage"  json:"themeImage"`
	MapImageURI *string          `bson:"mapImageUri" json:"mapImageUri"`
	MapPins     []MapPin         `bson:"mapPins"     json:"mapPins"`
	Sessions    []Session        `bson:"sessions"    json:"sessions"`
	Players     []CampaignPlayer `bson:"players"     json:"players"`
	DisabledSpells    []string `bson:"disabledSpells"    json:"disabledSpells"`
	DisabledEquipment []string `bson:"disabledEquipment" json:"disabledEquipment"`
	UpdatedAt         time.Time        `bson:"updatedAt"   json:"updatedAt"`
}
