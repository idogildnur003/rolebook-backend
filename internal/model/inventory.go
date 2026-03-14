package model

import "time"

// InventoryItem is stored in the "inventory" collection.
type InventoryItem struct {
	ID           string    `bson:"_id"          json:"id"`
	PlayerID     string    `bson:"playerId"     json:"playerId"`
	LinkedUserID string    `bson:"linkedUserId" json:"linkedUserId"` // denormalised from player

	Name     string   `bson:"name"     json:"name"`
	Quantity int      `bson:"quantity" json:"quantity"`
	Category string   `bson:"category" json:"category"`
	Tags     []string `bson:"tags"     json:"tags"`
	Notes    string   `bson:"notes,omitempty"    json:"notes,omitempty"`
	ImageURI string   `bson:"imageUri,omitempty" json:"imageUri,omitempty"`

	// Weapon fields
	Damage     string   `bson:"damage,omitempty"     json:"damage,omitempty"`
	DamageType string   `bson:"damageType,omitempty" json:"damageType,omitempty"`
	WeaponType string   `bson:"weaponType,omitempty" json:"weaponType,omitempty"`
	Properties []string `bson:"properties,omitempty" json:"properties,omitempty"`

	// Armor fields
	ArmorClass          *int    `bson:"armorClass,omitempty"          json:"armorClass,omitempty"`
	ArmorBonus          *int    `bson:"armorBonus,omitempty"          json:"armorBonus,omitempty"`
	ShieldBonus         *int    `bson:"shieldBonus,omitempty"         json:"shieldBonus,omitempty"`
	ArmorType           string  `bson:"armorType,omitempty"           json:"armorType,omitempty"`
	StrengthRequirement *int    `bson:"strengthRequirement,omitempty" json:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool   `bson:"stealthDisadvantage,omitempty" json:"stealthDisadvantage,omitempty"`

	// Magic item fields
	CompatibleWith *string  `bson:"compatibleWith,omitempty" json:"compatibleWith,omitempty"`
	EffectSummary  string   `bson:"effectSummary,omitempty"  json:"effectSummary,omitempty"`
	Value          *float64 `bson:"value,omitempty"          json:"value,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
