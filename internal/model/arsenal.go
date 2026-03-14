package model

import "time"

// ArsenalSpell is a reference spell in the global catalog ("arsenal_spells" collection).
// It does not have PlayerID or LinkedUserID fields — it is a global reference, not tied to any player.
type ArsenalSpell struct {
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

// ArsenalEquipment is a reference item in the global catalog ("arsenal_equipment" collection).
// It does not have PlayerID or LinkedUserID fields.
type ArsenalEquipment struct {
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
