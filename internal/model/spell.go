package model

import "time"

// Spell is a known spell stored in the "spells" collection.
type Spell struct {
	ID           string    `bson:"_id"          json:"id"`
	PlayerID     string    `bson:"playerId"     json:"playerId"`
	LinkedUserID string    `bson:"linkedUserId" json:"linkedUserId"` // denormalised from player

	Name        string   `bson:"name"               json:"name"`
	Level       int      `bson:"level"              json:"level"` // 0 = cantrip
	School      string   `bson:"school,omitempty"   json:"school,omitempty"`
	CastingTime string   `bson:"castingTime,omitempty" json:"castingTime,omitempty"`
	Range       string   `bson:"range,omitempty"    json:"range,omitempty"`
	Components  []string `bson:"components,omitempty" json:"components,omitempty"`
	Material    string   `bson:"material,omitempty" json:"material,omitempty"`
	Duration    string   `bson:"duration,omitempty" json:"duration,omitempty"`
	Description string   `bson:"description,omitempty" json:"description,omitempty"`
	IsPrepared  bool     `bson:"isPrepared"         json:"isPrepared"`
	IsRitual    bool     `bson:"isRitual,omitempty" json:"isRitual,omitempty"`
	Source      string   `bson:"source,omitempty"   json:"source,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
