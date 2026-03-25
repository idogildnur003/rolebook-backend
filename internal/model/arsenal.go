package model

import "time"

// Spell is a reference spell in the arsenal database ("spells" collection).
// It is a global catalog entry, not tied to any player.
type Spell struct {
	ID          string    `bson:"_id"               json:"id"`
	Name        string    `bson:"name"              json:"name"`
	Level       int       `bson:"level"             json:"level"`
	School      string    `bson:"school,omitempty"  json:"school,omitempty"`
	CastingTime string    `bson:"castingTime,omitempty" json:"castingTime,omitempty"`
	Range       string    `bson:"range,omitempty"   json:"range,omitempty"`
	Components  []string  `bson:"components,omitempty" json:"components,omitempty"`
	Material    string    `bson:"material,omitempty" json:"material,omitempty"`
	Duration    string    `bson:"duration,omitempty" json:"duration,omitempty"`
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	IsRitual    bool      `bson:"isRitual,omitempty" json:"isRitual,omitempty"`
	Source      string    `bson:"source,omitempty"  json:"source,omitempty"`
	UpdatedAt   time.Time `bson:"updatedAt"         json:"updatedAt"`
}

// Equipment is a reference item in the arsenal database ("equipment" collection).
// It is a global catalog entry, not tied to any player.
type Equipment struct {
	ID       string   `bson:"_id"          json:"id"`
	Name     string   `bson:"name"         json:"name"`
	Category string   `bson:"category"     json:"category"`
	Tags     []string `bson:"tags"         json:"tags"`
	Notes    string   `bson:"notes,omitempty"    json:"notes,omitempty"`
	ImageURI string   `bson:"imageUri,omitempty" json:"imageUri,omitempty"`

	Damage     string   `bson:"damage,omitempty"     json:"damage,omitempty"`
	DamageType string   `bson:"damageType,omitempty" json:"damageType,omitempty"`
	WeaponType string   `bson:"weaponType,omitempty" json:"weaponType,omitempty"`
	Properties []string `bson:"properties,omitempty" json:"properties,omitempty"`

	ArmorClass          *int    `bson:"armorClass,omitempty"          json:"armorClass,omitempty"`
	ArmorBonus          *int    `bson:"armorBonus,omitempty"          json:"armorBonus,omitempty"`
	ShieldBonus         *int    `bson:"shieldBonus,omitempty"         json:"shieldBonus,omitempty"`
	ArmorType           string  `bson:"armorType,omitempty"           json:"armorType,omitempty"`
	StrengthRequirement *int    `bson:"strengthRequirement,omitempty" json:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool   `bson:"stealthDisadvantage,omitempty" json:"stealthDisadvantage,omitempty"`

	CompatibleWith *string  `bson:"compatibleWith,omitempty" json:"compatibleWith,omitempty"`
	EffectSummary  string   `bson:"effectSummary,omitempty"  json:"effectSummary,omitempty"`
	Value          *float64 `bson:"value,omitempty"          json:"value,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
