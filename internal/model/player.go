package model

import "time"

// SpellSlot holds the max and used count for a spell slot level.
type SpellSlot struct {
	Max  int `bson:"max"  json:"max"`
	Used int `bson:"used" json:"used"`
}

// Note: spell slot level keys are strings "1"–"9" in the map.

// PlayerSpell is a lightweight spell reference embedded in the Player document.
type PlayerSpell struct {
	SpellID    string `bson:"spellId"    json:"spellId"`
	Name       string `bson:"name"       json:"name"`
	IsPrepared bool   `bson:"isPrepared" json:"isPrepared"`
}

// PlayerInventoryItem is a lightweight equipment reference embedded in the Player document.
type PlayerInventoryItem struct {
	EquipmentID string `bson:"equipmentId" json:"equipmentId"`
	Name        string `bson:"name"        json:"name"`
	Quantity    int    `bson:"quantity"    json:"quantity"`
}

// Player represents a D&D character sheet stored in the "players" collection.
type Player struct {
	ID           string `bson:"_id"          json:"id"`
	CampaignID   string `bson:"campaignId"   json:"campaignId"`
	LinkedUserID string `bson:"linkedUserId" json:"-"` // internal access-control field; not exposed in API responses

	Name             string  `bson:"name"             json:"name"`
	ClassName        *string `bson:"className"        json:"className"`
	Level            int     `bson:"level"            json:"level"`
	ExperiencePoints int     `bson:"experiencePoints" json:"experiencePoints"`
	Race             string  `bson:"race,omitempty"   json:"race,omitempty"`
	Notes           string  `bson:"notes"           json:"notes"`
	AvatarURI       string  `bson:"avatarUri,omitempty" json:"avatarUri,omitempty"`
	BackgroundStory string  `bson:"backgroundStory" json:"backgroundStory"`
	Alignment       *string `bson:"alignment"       json:"alignment"`
	SpeciesOrRegion *string `bson:"speciesOrRegion" json:"speciesOrRegion"`
	Subclass        *string `bson:"subclass"        json:"subclass"`
	Region          *string `bson:"region"          json:"region"`
	Size            string  `bson:"size"            json:"size"`

	// Combat stats
	CurrentHP          int `bson:"currentHp"          json:"currentHp"`
	MaxHP              int `bson:"maxHp"              json:"maxHp"`
	TempHP             int `bson:"tempHp"             json:"tempHp"`
	AC                 int `bson:"ac"                 json:"ac"`
	Speed              int `bson:"speed"              json:"speed"`
	InitiativeBonus    int `bson:"initiativeBonus"    json:"initiativeBonus"`
	ProficiencyBonus   int `bson:"proficiencyBonus"   json:"proficiencyBonus"`
	DeathSaveSuccesses int `bson:"deathSaveSuccesses" json:"deathSaveSuccesses"`
	DeathSaveFailures  int `bson:"deathSaveFailures"  json:"deathSaveFailures"`

	// Ability scores — keys: STR, DEX, CON, INT, WIS, CHA
	AbilityScores             map[string]int `bson:"abilityScores"             json:"abilityScores"`
	AbilityTemporaryModifiers map[string]int `bson:"abilityTemporaryModifiers" json:"abilityTemporaryModifiers"`
	SkillTemporaryModifiers   map[string]int `bson:"skillTemporaryModifiers"   json:"skillTemporaryModifiers"`

	// Proficiencies
	ProficientSavingThrows []string `bson:"proficientSavingThrows" json:"proficientSavingThrows"`
	ProficientSkills       []string `bson:"proficientSkills"       json:"proficientSkills"`
	ExpertiseSkills        []string `bson:"expertiseSkills"        json:"expertiseSkills"`
	FeaturesAndFeats       []string `bson:"featuresAndFeats"       json:"featuresAndFeats"`

	// Spell slots — keys "1"–"9"
	SpellSlots map[string]SpellSlot `bson:"spellSlots" json:"spellSlots"`

	// Conditions — e.g. {"poisoned": true}
	Conditions map[string]bool `bson:"conditions" json:"conditions"`

	// Spells — embedded references to arsenal spells
	Spells []PlayerSpell `bson:"spells" json:"spells"`

	// Inventory — embedded references to arsenal equipment
	Inventory []PlayerInventoryItem `bson:"inventory" json:"inventory"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// DefaultPlayer returns a new Player with sensible D&D 5e defaults.
// All maps and slices are initialized (never nil) to ensure clean JSON serialisation.
func DefaultPlayer(id, campaignID, linkedUserID, name string, level int) *Player {
	return &Player{
		ID:           id,
		CampaignID:   campaignID,
		LinkedUserID: linkedUserID,
		Name:         name,
		Level:        level,
		Size:         "Medium",
		CurrentHP:    10,
		MaxHP:        10,
		TempHP:       0,
		AC:           10,
		Speed:        30,
		ProficiencyBonus: 2,
		AbilityScores: map[string]int{
			"STR": 10, "DEX": 10, "CON": 10,
			"INT": 10, "WIS": 10, "CHA": 10,
		},
		AbilityTemporaryModifiers: make(map[string]int),
		SkillTemporaryModifiers:   make(map[string]int),
		ProficientSavingThrows:    []string{},
		ProficientSkills:          []string{},
		ExpertiseSkills:           []string{},
		FeaturesAndFeats:          []string{},
		// All 9 spell slot levels pre-populated so client never sees null
		SpellSlots: map[string]SpellSlot{
			"1": {}, "2": {}, "3": {}, "4": {}, "5": {},
			"6": {}, "7": {}, "8": {}, "9": {},
		},
		Conditions: make(map[string]bool),
		Spells:    []PlayerSpell{},
		Inventory: []PlayerInventoryItem{},
		UpdatedAt:  time.Now().UTC(),
	}
}
