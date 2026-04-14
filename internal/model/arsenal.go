package model

// Spell is a reference spell in the arsenal catalog.
// It is a global catalog entry, not tied to any player.
type Spell struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Level       int      `json:"level"`
	School      string   `json:"school,omitempty"`
	CastingTime string   `json:"castingTime,omitempty"`
	Range       string   `json:"range,omitempty"`
	Components  []string `json:"components,omitempty"`
	Material    string   `json:"material,omitempty"`
	Duration    string   `json:"duration,omitempty"`
	Description string   `json:"description,omitempty"`
	IsRitual    bool     `json:"isRitual,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// Equipment is a reference item in the arsenal catalog.
// It is a global catalog entry, not tied to any player.
type Equipment struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Notes    string   `json:"notes,omitempty"`
	ImageURI string   `json:"imageUri,omitempty"`

	Damage     string   `json:"damage,omitempty"`
	DamageType string   `json:"damageType,omitempty"`
	WeaponType string   `json:"weaponType,omitempty"`
	Properties []string `json:"properties,omitempty"`

	ArmorClass          *int  `json:"armorClass,omitempty"`
	ArmorBonus          *int  `json:"armorBonus,omitempty"`
	ShieldBonus         *int  `json:"shieldBonus,omitempty"`
	ArmorType           string `json:"armorType,omitempty"`
	StrengthRequirement *int  `json:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool `json:"stealthDisadvantage,omitempty"`

	CompatibleWith *string  `json:"compatibleWith,omitempty"`
	EffectSummary  string   `json:"effectSummary,omitempty"`
	Cost           *float64 `json:"cost,omitempty"`
	Currency       string   `json:"currency,omitempty"`
}
