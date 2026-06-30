package model

import "time"

// Request kinds, targets, and statuses for the DM-moderated custom content queue.
const (
	RequestKindCreate = "create"
	RequestKindEdit   = "edit" // reserved for Phase 2 (player-proposed edits)

	RequestTargetItem  = "item"
	RequestTargetSpell = "spell"

	RequestStatusPending  = "pending"
	RequestStatusApproved = "approved"
	RequestStatusDenied   = "denied"
)

// ContentRequest is a durable record of a player's proposal in the moderation
// queue. It is NOT deleted on approve/deny — the resolved record stays so the
// player keeps a history (denied proposals, past approvals) in "My Requests".
// On approval of a create request, a live CustomEquipment/CustomSpell is created
// and its id is recorded in ResultID.
type ContentRequest struct {
	ID               string `json:"id"               bson:"_id"`
	CampaignID       string `json:"campaignId"       bson:"campaignId"`
	TargetType       string `json:"targetType"       bson:"targetType"` // item | spell
	Kind             string `json:"kind"             bson:"kind"`       // create | edit (Phase 2)
	Status           string `json:"status"           bson:"status"`     // pending | approved | denied
	ProposedByUserID string `json:"proposedByUserId" bson:"proposedByUserId"`

	// ProposedByName is resolved from campaign players at read time (not stored).
	ProposedByName string `json:"proposedByName,omitempty" bson:"-"`

	// Player's suggested visibility (the DM makes the final call at approval).
	SuggestedVisibilityMode   string   `json:"suggestedVisibilityMode,omitempty"   bson:"suggestedVisibilityMode,omitempty"`
	SuggestedVisiblePlayerIDs []string `json:"suggestedVisiblePlayerIds,omitempty" bson:"suggestedVisiblePlayerIds,omitempty"`

	// Proposed content (only one is set, by TargetType, for kind=create).
	ItemPayload  *CustomEquipment `json:"itemPayload,omitempty"  bson:"itemPayload,omitempty"`
	SpellPayload *CustomSpell     `json:"spellPayload,omitempty" bson:"spellPayload,omitempty"`

	ResultID         string     `json:"resultId,omitempty"         bson:"resultId,omitempty"` // live entry id after approval
	CreatedAt        time.Time  `json:"createdAt"                  bson:"createdAt"`
	ResolvedAt       *time.Time `json:"resolvedAt,omitempty"       bson:"resolvedAt,omitempty"`
	ResolvedByUserID string     `json:"resolvedByUserId,omitempty" bson:"resolvedByUserId,omitempty"`
}

// CanResolveRequest reports whether a request may be approved/denied (pending only).
func CanResolveRequest(status string) bool { return status == RequestStatusPending }

// CanWithdrawRequest reports whether the proposer may withdraw it (pending only).
func CanWithdrawRequest(status string) bool { return status == RequestStatusPending }

// IsValidRequestTarget reports whether target is a supported target type.
func IsValidRequestTarget(target string) bool {
	return target == RequestTargetItem || target == RequestTargetSpell
}

// IsValidRequestKind reports whether kind is a supported request kind.
func IsValidRequestKind(kind string) bool {
	return kind == RequestKindCreate || kind == RequestKindEdit
}

// IsEditRequest reports whether the request edits an existing entry (vs creating one).
func IsEditRequest(kind string) bool { return kind == RequestKindEdit }
