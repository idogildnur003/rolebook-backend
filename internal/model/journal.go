package model

import "time"

// Visibility controls read-access widening for journal entries.
// The owner can always read; sharedWithAll grants read to all current members;
// sharedPlayerIds grants read to a specific subset.
type Visibility struct {
	SharedWithAll   bool     `bson:"sharedWithAll"   json:"sharedWithAll"`
	SharedPlayerIds []string `bson:"sharedPlayerIds" json:"sharedPlayerIds"`
}

// CloneAudit records the provenance of a cloned (forked) entry. Set on clones
// only; nil on originals. Carried by all three journal entity types.
type CloneAudit struct {
	ClonedFromEntryId       string    `bson:"clonedFromEntryId"       json:"clonedFromEntryId"`
	ClonedFromOwnerPlayerId string    `bson:"clonedFromOwnerPlayerId" json:"clonedFromOwnerPlayerId"`
	ClonedAt                time.Time `bson:"clonedAt"                json:"clonedAt"`
}

// Location is a journal entry stored in the campaign_locations collection.
type Location struct {
	ID                string     `bson:"_id"                          json:"id"`
	CampaignID        string     `bson:"campaignId"                   json:"campaignId"`
	OwnerPlayerID     string     `bson:"ownerPlayerId"                json:"ownerPlayerId"`
	OwnerUserID       string     `bson:"ownerUserId"                  json:"-"`
	Name              string     `bson:"name"                         json:"name"`
	ShortNotes        string     `bson:"shortNotes,omitempty"         json:"shortNotes,omitempty"`
	FullDescription   string     `bson:"fullDescription,omitempty"    json:"fullDescription,omitempty"`
	ThumbnailURI      string     `bson:"thumbnailUri,omitempty"       json:"thumbnailUri,omitempty"`
	SessionID         string     `bson:"sessionId,omitempty"          json:"sessionId,omitempty"`
	ParentLocationID  string     `bson:"parentLocationId,omitempty"   json:"parentLocationId,omitempty"`
	LinkedNpcIds      []string   `bson:"linkedNpcIds"                 json:"linkedNpcIds"`
	Visibility        Visibility `bson:"visibility"                   json:"visibility"`
	ShareNote         string     `bson:"shareNote,omitempty"          json:"shareNote,omitempty"`
	Clone             *CloneAudit `bson:"clone,omitempty"             json:"clone,omitempty"`
	CreatedAt         time.Time  `bson:"createdAt"                    json:"createdAt"`
	UpdatedAt         time.Time  `bson:"updatedAt"                    json:"updatedAt"`
}

// NPC is a journal entry stored in the campaign_npcs collection.
type NPC struct {
	ID                string     `bson:"_id"                          json:"id"`
	CampaignID        string     `bson:"campaignId"                   json:"campaignId"`
	OwnerPlayerID     string     `bson:"ownerPlayerId"                json:"ownerPlayerId"`
	OwnerUserID       string     `bson:"ownerUserId"                  json:"-"`
	Name              string     `bson:"name"                         json:"name"`
	ShortNotes        string     `bson:"shortNotes,omitempty"         json:"shortNotes,omitempty"`
	FullDescription   string     `bson:"fullDescription,omitempty"    json:"fullDescription,omitempty"`
	AvatarURI         string     `bson:"avatarUri,omitempty"          json:"avatarUri,omitempty"`
	SessionID         string     `bson:"sessionId,omitempty"          json:"sessionId,omitempty"`
	LinkedLocationIds []string   `bson:"linkedLocationIds"            json:"linkedLocationIds"`
	Visibility        Visibility `bson:"visibility"                   json:"visibility"`
	ShareNote         string     `bson:"shareNote,omitempty"          json:"shareNote,omitempty"`
	Clone             *CloneAudit `bson:"clone,omitempty"             json:"clone,omitempty"`
	CreatedAt         time.Time  `bson:"createdAt"                    json:"createdAt"`
	UpdatedAt         time.Time  `bson:"updatedAt"                    json:"updatedAt"`
}

// MapPinType enumerates the six pin types the client supports.
type MapPinType string

const (
	MapPinLocation     MapPinType = "location"
	MapPinNPC          MapPinType = "npc"
	MapPinItem         MapPinType = "item"
	MapPinMajorFinding MapPinType = "majorFinding"
	MapPinTravelMarker MapPinType = "travelMarker"
	MapPinCustom       MapPinType = "custom"
)

// IsValidMapPinType reports whether t is one of the six allowed pin types.
func IsValidMapPinType(t MapPinType) bool {
	switch t {
	case MapPinLocation, MapPinNPC, MapPinItem, MapPinMajorFinding, MapPinTravelMarker, MapPinCustom:
		return true
	}
	return false
}

// MapPinTypeReferencesEntity reports whether the pin type requires an entityId
// (location/npc) versus carrying its own title/notes (item/majorFinding/etc.).
func MapPinTypeReferencesEntity(t MapPinType) bool {
	return t == MapPinLocation || t == MapPinNPC
}

// MapPin is a journal entry stored in the campaign_map_pins collection.
type MapPin struct {
	ID            string     `bson:"_id"                  json:"id"`
	CampaignID    string     `bson:"campaignId"           json:"campaignId"`
	OwnerPlayerID string     `bson:"ownerPlayerId"        json:"ownerPlayerId"`
	OwnerUserID   string     `bson:"ownerUserId"          json:"-"`
	Type          MapPinType `bson:"type"                 json:"type"`
	EntityID      string     `bson:"entityId,omitempty"   json:"entityId,omitempty"`
	Title         string     `bson:"title,omitempty"      json:"title,omitempty"`
	Notes         string     `bson:"notes,omitempty"      json:"notes,omitempty"`
	X             float64    `bson:"x"                    json:"x"`
	Y             float64    `bson:"y"                    json:"y"`
	SessionID     string     `bson:"sessionId,omitempty"  json:"sessionId,omitempty"`
	Visibility    Visibility `bson:"visibility"           json:"visibility"`
	ShareNote     string     `bson:"shareNote,omitempty"  json:"shareNote,omitempty"`
	Clone         *CloneAudit `bson:"clone,omitempty"     json:"clone,omitempty"`
	CreatedAt     time.Time  `bson:"createdAt"            json:"createdAt"`
	UpdatedAt     time.Time  `bson:"updatedAt"            json:"updatedAt"`
}
