package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/avatarstore"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// ContentRequestHandler exposes the DM-moderation queue: players propose, the DM
// approves/denies. Approval of a create request materialises a live custom
// equipment/spell entry with the DM-chosen visibility.
type ContentRequestHandler struct {
	requests        *store.ContentRequestStore
	customEquipment *store.CustomEquipmentStore
	customSpells    *store.CustomSpellStore
	players         *store.PlayerStore
	campaigns       *store.CampaignStore
	avatars         *avatarstore.Store
}

func NewContentRequestHandler(
	requests *store.ContentRequestStore,
	customEquipment *store.CustomEquipmentStore,
	customSpells *store.CustomSpellStore,
	players *store.PlayerStore,
	campaigns *store.CampaignStore,
	avatars *avatarstore.Store,
) *ContentRequestHandler {
	return &ContentRequestHandler{
		requests:        requests,
		customEquipment: customEquipment,
		customSpells:    customSpells,
		players:         players,
		campaigns:       campaigns,
		avatars:         avatars,
	}
}

// buildRequestViews resolves each request's proposer display name from campaign
// players (matching ProposedByUserID against LinkedUserID). Pure over its inputs.
func buildRequestViews(reqs []model.ContentRequest, players []store.PlayerInventorySummary) []model.ContentRequest {
	nameByUserID := make(map[string]string, len(players))
	for _, p := range players {
		if p.LinkedUserID != "" {
			nameByUserID[p.LinkedUserID] = p.Name
		}
	}
	out := make([]model.ContentRequest, len(reqs))
	for i, req := range reqs {
		req.ProposedByName = nameByUserID[req.ProposedByUserID]
		out[i] = req
	}
	return out
}

// Create handles POST /api/campaigns/:campaignId/content-requests.
// Any campaign member may propose. Body: { targetType, suggestedVisibilityMode,
// suggestedVisiblePlayerIds, itemPayload | spellPayload }.
func (h *ContentRequestHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}

	var body model.ContentRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if !model.IsValidRequestTarget(body.TargetType) {
		writeError(w, http.StatusBadRequest, "targetType must be 'item' or 'spell'", "BAD_REQUEST")
		return
	}
	if body.TargetType == model.RequestTargetItem && (body.ItemPayload == nil || body.ItemPayload.Name == "") {
		writeError(w, http.StatusBadRequest, "itemPayload with a name is required", "BAD_REQUEST")
		return
	}
	if body.TargetType == model.RequestTargetItem && body.ItemPayload.Category == "" {
		// The live custom-equipment create requires a category; enforce it here so
		// an approved proposal can never materialise into an invalid entry.
		writeError(w, http.StatusBadRequest, "itemPayload.category is required", "BAD_REQUEST")
		return
	}
	if body.TargetType == model.RequestTargetSpell && (body.SpellPayload == nil || body.SpellPayload.Name == "") {
		writeError(w, http.StatusBadRequest, "spellPayload with a name is required", "BAD_REQUEST")
		return
	}
	body.SuggestedVisibilityMode = model.NormalizeVisibilityMode(body.SuggestedVisibilityMode)
	if !model.IsValidVisibilityMode(body.SuggestedVisibilityMode) {
		writeError(w, http.StatusBadRequest, "suggestedVisibilityMode must be 'campaign' or 'players'", "BAD_REQUEST")
		return
	}

	id, err := allocateRequestID(r, h.requests, campaignID, proposalName(&body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	body.ID = id
	body.CampaignID = campaignID
	body.Kind = model.RequestKindCreate
	body.Status = model.RequestStatusPending
	body.ProposedByUserID = membership.UserID
	body.ResultID = ""
	body.ResolvedAt = nil
	body.ResolvedByUserID = ""
	body.CreatedAt = time.Now().UTC()
	scrubPayloadForProposal(&body)

	if err := h.requests.Create(r.Context(), &body); err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "duplicate request id", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	body.ProposedByName = h.resolveName(r, campaignID, body.ProposedByUserID)
	writeJSON(w, http.StatusCreated, body)
}

// Mine handles GET /api/campaigns/:campaignId/content-requests/mine.
func (h *ContentRequestHandler) Mine(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	reqs, err := h.requests.ListByProposer(r.Context(), campaignID, membership.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, h.withNamesAndImages(r, campaignID, reqs))
}

// Pending handles GET /api/campaigns/:campaignId/content-requests/pending (DM only).
func (h *ContentRequestHandler) Pending(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can view the approval queue", "FORBIDDEN")
		return
	}
	reqs, err := h.requests.ListPending(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, h.withNamesAndImages(r, campaignID, reqs))
}

// PendingCount handles GET /api/campaigns/:campaignId/content-requests/pending-count (DM only).
func (h *ContentRequestHandler) PendingCount(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can view the approval queue", "FORBIDDEN")
		return
	}
	count, err := h.requests.CountPending(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}

// Withdraw handles DELETE /api/campaigns/:campaignId/content-requests/:id.
// The proposer may withdraw their own still-pending request.
func (h *ContentRequestHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	existing, err := h.requests.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "request not found", "NOT_FOUND")
		return
	}
	if existing.ProposedByUserID != membership.UserID {
		writeError(w, http.StatusForbidden, "only the proposer can withdraw this request", "FORBIDDEN")
		return
	}
	if !model.CanWithdrawRequest(existing.Status) {
		writeError(w, http.StatusBadRequest, "only a pending request can be withdrawn", "BAD_REQUEST")
		return
	}
	if _, err := h.requests.Delete(r.Context(), campaignID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func proposalName(req *model.ContentRequest) string {
	if req.TargetType == model.RequestTargetItem && req.ItemPayload != nil {
		return req.ItemPayload.Name
	}
	if req.TargetType == model.RequestTargetSpell && req.SpellPayload != nil {
		return req.SpellPayload.Name
	}
	return "request"
}

// allocateRequestID generates a unique id ("custom-{slug}-{hex}"), retrying on collisions.
func allocateRequestID(r *http.Request, requests *store.ContentRequestStore, campaignID, name string) (string, error) {
	const maxAttempts = 3
	for i := 0; i < maxAttempts; i++ {
		candidate, err := store.GenerateID(name)
		if err != nil {
			return "", err
		}
		existing, err := requests.GetByID(r.Context(), campaignID, candidate)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", errors.New("failed to allocate request id")
}

// scrubPayloadForProposal clears server-owned fields on the embedded payload so a
// client can't pre-set id/campaignId/createdBy/visibility (set at approval time).
func scrubPayloadForProposal(req *model.ContentRequest) {
	if req.ItemPayload != nil {
		req.ItemPayload.ID = ""
		req.ItemPayload.CampaignID = ""
		req.ItemPayload.CreatedBy = ""
		req.ItemPayload.VisibilityMode = ""
		req.ItemPayload.VisiblePlayerIDs = nil
	}
	if req.SpellPayload != nil {
		req.SpellPayload.ID = ""
		req.SpellPayload.CampaignID = ""
		req.SpellPayload.CreatedBy = ""
		req.SpellPayload.VisibilityMode = ""
		req.SpellPayload.VisiblePlayerIDs = nil
	}
}

func (h *ContentRequestHandler) resolveName(r *http.Request, campaignID, userID string) string {
	summaries, err := h.players.ListInventorySummaries(r.Context(), campaignID)
	if err != nil {
		return ""
	}
	for _, p := range summaries {
		if p.LinkedUserID == userID {
			return p.Name
		}
	}
	return ""
}

func (h *ContentRequestHandler) withNamesAndImages(r *http.Request, campaignID string, reqs []model.ContentRequest) []model.ContentRequest {
	// Best-effort: ProposedByName is cosmetic, so a summaries failure degrades to
	// empty names rather than failing the whole list request. (Intentional — do
	// not convert to a hard error.)
	summaries, err := h.players.ListInventorySummaries(r.Context(), campaignID)
	if err != nil {
		summaries = nil
	}
	views := buildRequestViews(reqs, summaries)
	for i := range views {
		if views[i].ItemPayload != nil {
			views[i].ItemPayload.ImageURI = h.avatars.ResolveImageURI(r.Context(), views[i].ItemPayload.ImageURI)
		}
		if views[i].SpellPayload != nil {
			views[i].SpellPayload.ImageURI = h.avatars.ResolveImageURI(r.Context(), views[i].SpellPayload.ImageURI)
		}
	}
	return views
}

// approveBody is the DM's visibility decision when approving a create request.
type approveBody struct {
	VisibilityMode   string   `json:"visibilityMode"`
	VisiblePlayerIDs []string `json:"visiblePlayerIds"`
}

// Approve handles POST /api/campaigns/:campaignId/content-requests/:id/approve (DM only).
// Materialises a live custom equipment/spell with the DM-chosen visibility, then
// marks the request approved with the resulting entry id.
func (h *ContentRequestHandler) Approve(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can approve requests", "FORBIDDEN")
		return
	}

	req, err := h.requests.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "request not found", "NOT_FOUND")
		return
	}
	if !model.CanResolveRequest(req.Status) {
		writeError(w, http.StatusBadRequest, "request is already resolved", "BAD_REQUEST")
		return
	}

	var body approveBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	body.VisibilityMode = model.NormalizeVisibilityMode(body.VisibilityMode)
	if !model.IsValidVisibilityMode(body.VisibilityMode) {
		writeError(w, http.StatusBadRequest, "visibilityMode must be 'campaign' or 'players'", "BAD_REQUEST")
		return
	}
	if body.VisibilityMode != model.VisibilityPlayers {
		body.VisiblePlayerIDs = nil
	}

	resultID, err := h.materialise(r, campaignID, req, body)
	if err != nil {
		// Mirror the direct-create handler: a slug collision on the live entry is
		// a 409, not a 500 (narrow TOCTOU window after the id-allocation check).
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "a custom entry with that id already exists", "DUPLICATE")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	updated, err := h.requests.Resolve(r.Context(), campaignID, id, model.RequestStatusApproved, membership.UserID, resultID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	updated.ProposedByName = h.resolveName(r, campaignID, updated.ProposedByUserID)
	writeJSON(w, http.StatusOK, updated)
}

// Deny handles POST /api/campaigns/:campaignId/content-requests/:id/deny (DM only).
// The request record persists with status=denied so the proposer keeps the history.
func (h *ContentRequestHandler) Deny(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can deny requests", "FORBIDDEN")
		return
	}
	existing, err := h.requests.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "request not found", "NOT_FOUND")
		return
	}
	if !model.CanResolveRequest(existing.Status) {
		writeError(w, http.StatusBadRequest, "request is already resolved", "BAD_REQUEST")
		return
	}
	updated, err := h.requests.Resolve(r.Context(), campaignID, id, model.RequestStatusDenied, membership.UserID, "")
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	updated.ProposedByName = h.resolveName(r, campaignID, updated.ProposedByUserID)
	writeJSON(w, http.StatusOK, updated)
}

// EditPending handles PATCH /api/campaigns/:campaignId/content-requests/:id (DM only).
// Updates a still-pending request's proposed payload and/or suggested visibility
// (edit-before-approve). The request stays pending.
func (h *ContentRequestHandler) EditPending(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	id := chi.URLParam(r, "id")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	existing, err := h.requests.GetByID(r.Context(), campaignID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "request not found", "NOT_FOUND")
		return
	}
	// The DM may edit any pending request (edit-before-approve); a player may
	// edit their own pending proposal. Anyone else is forbidden.
	if !membership.IsDM && existing.ProposedByUserID != membership.UserID {
		writeError(w, http.StatusForbidden, "you can only edit your own pending request", "FORBIDDEN")
		return
	}
	if !model.CanResolveRequest(existing.Status) {
		writeError(w, http.StatusBadRequest, "only a pending request can be edited", "BAD_REQUEST")
		return
	}

	var body model.ContentRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	scrubPayloadForProposal(&body)
	fields := bson.M{}
	if existing.TargetType == model.RequestTargetItem && body.ItemPayload != nil {
		if body.ItemPayload.Name == "" {
			writeError(w, http.StatusBadRequest, "itemPayload with a name is required", "BAD_REQUEST")
			return
		}
		if body.ItemPayload.Category == "" {
			writeError(w, http.StatusBadRequest, "itemPayload.category is required", "BAD_REQUEST")
			return
		}
		fields["itemPayload"] = body.ItemPayload
	}
	if existing.TargetType == model.RequestTargetSpell && body.SpellPayload != nil {
		if body.SpellPayload.Name == "" {
			writeError(w, http.StatusBadRequest, "spellPayload with a name is required", "BAD_REQUEST")
			return
		}
		fields["spellPayload"] = body.SpellPayload
	}
	if body.SuggestedVisibilityMode != "" {
		mode := model.NormalizeVisibilityMode(body.SuggestedVisibilityMode)
		if !model.IsValidVisibilityMode(mode) {
			writeError(w, http.StatusBadRequest, "suggestedVisibilityMode must be 'campaign' or 'players'", "BAD_REQUEST")
			return
		}
		fields["suggestedVisibilityMode"] = mode
		// Clear the suggested player list for campaign mode, matching Create/Approve.
		if mode == model.VisibilityPlayers {
			fields["suggestedVisiblePlayerIds"] = body.SuggestedVisiblePlayerIDs
		} else {
			fields["suggestedVisiblePlayerIds"] = []string{}
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "no editable fields provided", "BAD_REQUEST")
		return
	}

	updated, err := h.requests.UpdatePayload(r.Context(), campaignID, id, fields)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, h.withNamesAndImages(r, campaignID, []model.ContentRequest{*updated})[0])
}

// materialise creates the live custom entry from an approved create request and
// returns its id. Visibility is the DM's decision (body). The created entry's
// creator is the original proposer, not the approving DM.
func (h *ContentRequestHandler) materialise(r *http.Request, campaignID string, req *model.ContentRequest, body approveBody) (string, error) {
	now := time.Now().UTC()
	if req.TargetType == model.RequestTargetItem {
		if req.ItemPayload == nil {
			return "", errors.New("item request has no payload")
		}
		item := *req.ItemPayload
		id, err := allocateCustomEquipmentID(r, h.customEquipment, campaignID, item.Name)
		if err != nil {
			return "", err
		}
		item.ID = id
		item.CampaignID = campaignID
		item.CreatedBy = req.ProposedByUserID
		item.CreatedAt = now
		item.UpdatedAt = now
		item.VisibilityMode = body.VisibilityMode
		item.VisiblePlayerIDs = body.VisiblePlayerIDs
		if item.Tags == nil {
			item.Tags = []string{}
		}
		if err := h.customEquipment.Create(r.Context(), &item); err != nil {
			return "", err
		}
		return id, nil
	}
	if req.SpellPayload == nil {
		return "", errors.New("spell request has no payload")
	}
	spell := *req.SpellPayload
	id, err := allocateCustomSpellID(r, h.customSpells, campaignID, spell.Name)
	if err != nil {
		return "", err
	}
	spell.ID = id
	spell.CampaignID = campaignID
	spell.CreatedBy = req.ProposedByUserID
	spell.CreatedAt = now
	spell.UpdatedAt = now
	spell.VisibilityMode = body.VisibilityMode
	spell.VisiblePlayerIDs = body.VisiblePlayerIDs
	if spell.Components == nil {
		spell.Components = []string{}
	}
	if err := h.customSpells.Create(r.Context(), &spell); err != nil {
		return "", err
	}
	return id, nil
}

func allocateCustomEquipmentID(r *http.Request, s *store.CustomEquipmentStore, campaignID, name string) (string, error) {
	const maxAttempts = 3
	for i := 0; i < maxAttempts; i++ {
		candidate, err := store.GenerateID(name)
		if err != nil {
			return "", err
		}
		existing, err := s.GetByID(r.Context(), campaignID, candidate)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", errors.New("failed to allocate equipment id")
}

func allocateCustomSpellID(r *http.Request, s *store.CustomSpellStore, campaignID, name string) (string, error) {
	const maxAttempts = 3
	for i := 0; i < maxAttempts; i++ {
		candidate, err := store.GenerateID(name)
		if err != nil {
			return "", err
		}
		existing, err := s.GetByID(r.Context(), campaignID, candidate)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", errors.New("failed to allocate spell id")
}
