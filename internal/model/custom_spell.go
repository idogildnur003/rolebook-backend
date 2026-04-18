package model

import "time"

// CustomSpell is a homebrew spell entry scoped to a single campaign.
// It mirrors the shape of Spell but is stored in MongoDB and carries
// ownership and timestamps.
//
// IDs are server-generated slugs of the form "custom-{slug}-{random}" and are
// globally unique, but all lookups must always include campaignId to prevent
// cross-campaign leakage.
type CustomSpell struct {
	ID         string    `json:"id"         bson:"_id"`
	CampaignID string    `json:"campaignId" bson:"campaignId"`
	CreatedBy  string    `json:"createdBy"  bson:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"  bson:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"  bson:"updatedAt"`

	Name        string   `json:"name"                  bson:"name"`
	Level       int      `json:"level"                 bson:"level"`
	School      string   `json:"school,omitempty"      bson:"school,omitempty"`
	CastingTime string   `json:"castingTime,omitempty" bson:"castingTime,omitempty"`
	Range       string   `json:"range,omitempty"       bson:"range,omitempty"`
	Components  []string `json:"components,omitempty"  bson:"components,omitempty"`
	Material    string   `json:"material,omitempty"    bson:"material,omitempty"`
	Duration    string   `json:"duration,omitempty"    bson:"duration,omitempty"`
	Description string   `json:"description,omitempty" bson:"description,omitempty"`
	IsRitual    bool     `json:"isRitual,omitempty"    bson:"isRitual,omitempty"`
	Source      string   `json:"source,omitempty"      bson:"source,omitempty"`
}
