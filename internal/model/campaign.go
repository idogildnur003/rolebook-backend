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

// CampaignMembership defines a player's role in a campaign.
type CampaignMembership struct {
	PlayerID string `bson:"playerId" json:"playerId"`
	IsDM     bool   `bson:"isDM"    json:"isDM"`
}

// Campaign is stored in the "campaigns" collection.
// Sessions are embedded to avoid cross-collection joins.
// createdBy records the admin who created the campaign (informational; not used for access control).
type Campaign struct {
	ID          string    `bson:"_id"         json:"id"`
	CreatedBy   string    `bson:"createdBy"   json:"createdBy"`
	Name        string    `bson:"name"        json:"name"`
	ThemeImage  string    `bson:"themeImage"  json:"themeImage"`
	MapImageURI *string   `bson:"mapImageUri" json:"mapImageUri"`
	MapPins     []MapPin  `bson:"mapPins"     json:"mapPins"`
	Sessions    []Session `bson:"sessions"    json:"sessions"`
	Memberships []CampaignMembership `bson:"memberships" json:"memberships"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}
