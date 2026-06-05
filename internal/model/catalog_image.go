package model

import "time"

// CatalogImageType distinguishes which global catalog an image override targets.
type CatalogImageType string

const (
	CatalogImageEquipment CatalogImageType = "equipment"
	CatalogImageSpell     CatalogImageType = "spell"
)

// CatalogImage maps a global arsenal catalog item (which lives in read-only
// embedded JSON) to an admin-uploaded S3 image key. Stored in the
// "catalog_images" collection; _id is "<type>:<itemId>", making (type, itemId)
// unique by construction.
type CatalogImage struct {
	ID        string           `bson:"_id"       json:"id"`
	Type      CatalogImageType `bson:"type"      json:"type"`
	ItemID    string           `bson:"itemId"    json:"itemId"`
	ImageKey  string           `bson:"imageKey"  json:"imageKey"`
	UpdatedAt time.Time        `bson:"updatedAt" json:"updatedAt"`
}
