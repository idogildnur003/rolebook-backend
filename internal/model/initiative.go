package model

import "time"

// InitiativeParticipant is one combatant in an initiative call.
type InitiativeParticipant struct {
	ID                  string `bson:"id"                           json:"id"`
	ParticipantType     string `bson:"participantType"              json:"participantType"` // "player" | "enemy"
	Name                string `bson:"name"                         json:"name"`
	Initiative          *int   `bson:"initiative"                   json:"initiative"` // nil = not yet rolled
	PlayerID            string `bson:"playerId,omitempty"           json:"playerId,omitempty"`
	AvatarURI           string `bson:"avatarUri,omitempty"          json:"avatarUri,omitempty"`
	SubmittedByPlayerID string `bson:"submittedByPlayerId,omitempty" json:"submittedByPlayerId,omitempty"`
	SubmittedAt         *int64 `bson:"submittedAt,omitempty"        json:"submittedAt,omitempty"`
}

// InitiativeCall is the single active/last combat tracker for a campaign.
// Stored in "initiative_calls" with _id == campaignId (one per campaign).
type InitiativeCall struct {
	CampaignID               string                  `bson:"_id"                          json:"-"`
	ID                       string                  `bson:"id"                           json:"id"`
	Status                   string                  `bson:"status"                       json:"status"` // "open" | "resolved"
	StartedAt                int64                   `bson:"startedAt"                    json:"startedAt"`
	UpdatedAt                int64                   `bson:"updatedAt"                    json:"updatedAt"`
	StartedByPlayerID        string                  `bson:"startedByPlayerId"            json:"startedByPlayerId"`
	Participants             []InitiativeParticipant `bson:"participants"                 json:"participants"`
	TurnOrderParticipantIDs  []string                `bson:"turnOrderParticipantIds"      json:"turnOrderParticipantIds"`
	CurrentTurnParticipantID string                  `bson:"currentTurnParticipantId,omitempty" json:"currentTurnParticipantId,omitempty"`
	HasTurnCycleStarted      bool                    `bson:"hasTurnCycleStarted"          json:"hasTurnCycleStarted"`
	Version                  int                     `bson:"version"                      json:"-"` // optimistic concurrency
	ResolvedAt               *time.Time              `bson:"resolvedAt,omitempty"         json:"-"` // TTL key; absent while open
}
