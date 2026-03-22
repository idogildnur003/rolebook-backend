package model

import "time"

// NPC represents a non-player character or monster managed by the DM.
type NPC struct {
	ID         string `bson:"_id"        json:"id"`
	CampaignID string `bson:"campaignId" json:"campaignId"`

	Name      string  `bson:"name"      json:"name"`
	Type      string  `bson:"type"      json:"type"`      // e.g. "Monster", "NPC", "Boss"
	Challenge string  `bson:"challenge" json:"challenge"` // CR
	Notes     string  `bson:"notes"     json:"notes"`
	AvatarURI string  `bson:"avatarUri" json:"avatarUri"`

	// Stats
	CurrentHP int `bson:"currentHp" json:"currentHp"`
	MaxHP     int `bson:"maxHp"     json:"maxHp"`
	TempHP    int `bson:"tempHp"    json:"tempHp"`
	AC        int `bson:"ac"        json:"ac"`
	Speed     int `bson:"speed"     json:"speed"`

	AbilityScores map[string]int `bson:"abilityScores" json:"abilityScores"`

	// NPC specific fields
	Actions   []string `bson:"actions"   json:"actions"`
	Reactions []string `bson:"reactions" json:"reactions"`
	Legendary []string `bson:"legendary" json:"legendary"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// DefaultNPC returns a new NPC with defaults.
func DefaultNPC(id, campaignID, name string) *NPC {
	return &NPC{
		ID:         id,
		CampaignID: campaignID,
		Name:       name,
		Type:       "NPC",
		CurrentHP:  10,
		MaxHP:      10,
		AC:         10,
		Speed:      30,
		AbilityScores: map[string]int{
			"STR": 10, "DEX": 10, "CON": 10,
			"INT": 10, "WIS": 10, "CHA": 10,
		},
		Actions:   []string{},
		Reactions: []string{},
		Legendary: []string{},
		UpdatedAt: time.Now().UTC(),
	}
}
