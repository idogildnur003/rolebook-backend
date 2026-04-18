package model

import "time"

// CustomEquipment is a homebrew equipment entry scoped to a single campaign.
// It mirrors the shape of Equipment but is stored in MongoDB and carries
// ownership and timestamps.
//
// IDs are server-generated slugs of the form "custom-{slug}-{random}" and are
// globally unique, but all lookups must always include campaignId to prevent
// cross-campaign leakage.
type CustomEquipment struct {
	ID         string    `json:"id"         bson:"_id"`
	CampaignID string    `json:"campaignId" bson:"campaignId"`
	CreatedBy  string    `json:"createdBy"  bson:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"  bson:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"  bson:"updatedAt"`

	Name     string   `json:"name"               bson:"name"`
	Category string   `json:"category"           bson:"category"`
	Tags     []string `json:"tags"               bson:"tags"`
	Notes    string   `json:"notes,omitempty"    bson:"notes,omitempty"`
	ImageURI string   `json:"imageUri,omitempty" bson:"imageUri,omitempty"`

	Damage     string   `json:"damage,omitempty"     bson:"damage,omitempty"`
	DamageType string   `json:"damageType,omitempty" bson:"damageType,omitempty"`
	WeaponType string   `json:"weaponType,omitempty" bson:"weaponType,omitempty"`
	Properties []string `json:"properties,omitempty" bson:"properties,omitempty"`

	ArmorClass          *int   `json:"armorClass,omitempty"          bson:"armorClass,omitempty"`
	ArmorBonus          *int   `json:"armorBonus,omitempty"          bson:"armorBonus,omitempty"`
	ShieldBonus         *int   `json:"shieldBonus,omitempty"         bson:"shieldBonus,omitempty"`
	ArmorType           string `json:"armorType,omitempty"           bson:"armorType,omitempty"`
	StrengthRequirement *int   `json:"strengthRequirement,omitempty" bson:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool  `json:"stealthDisadvantage,omitempty" bson:"stealthDisadvantage,omitempty"`

	CompatibleWith *string  `json:"compatibleWith,omitempty" bson:"compatibleWith,omitempty"`
	EffectSummary  string   `json:"effectSummary,omitempty"  bson:"effectSummary,omitempty"`
	Cost           *float64 `json:"cost,omitempty"           bson:"cost,omitempty"`
	Currency       string   `json:"currency,omitempty"       bson:"currency,omitempty"`
}
