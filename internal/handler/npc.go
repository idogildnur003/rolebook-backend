package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// NpcHandler handles NPC-related endpoints.
type NpcHandler struct {
	npcs      *store.NpcStore
	campaigns *store.CampaignStore
}

// NewNpcHandler creates an NpcHandler.
func NewNpcHandler(npcs *store.NpcStore, campaigns *store.CampaignStore) *NpcHandler {
	return &NpcHandler{npcs: npcs, campaigns: campaigns}
}

// List handles GET /api/campaigns/{campaignId}/npcs.
func (h *NpcHandler) List(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())

	// Check if user has access to campaign
	isDM, ok := isCampaignDM(r.Context(), h.campaigns, campaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !isDM {
		// Only DM can see the NPC list for now
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	npcs, err := h.npcs.ListByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, npcs)
}

// Get handles GET /api/npcs/{npcId}.
func (h *NpcHandler) Get(w http.ResponseWriter, r *http.Request) {
	npcID := chi.URLParam(r, "npcId")
	userID := middleware.UserIDFromContext(r.Context())

	npc, err := h.npcs.GetByID(r.Context(), npcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if npc == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}

	isDM, ok := isCampaignDM(r.Context(), h.campaigns, npc.CampaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !isDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	writeJSON(w, http.StatusOK, npc)
}

// Create handles POST /api/campaigns/{campaignId}/npcs.
func (h *NpcHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())

	isDM, ok := isCampaignDM(r.Context(), h.campaigns, campaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !isDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	npc := model.DefaultNPC(uuid.NewString(), campaignID, req.Name)

	if err := h.npcs.Create(r.Context(), npc); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, npc)
}

// Update handles PATCH /api/npcs/{npcId}.
func (h *NpcHandler) Update(w http.ResponseWriter, r *http.Request) {
	npcID := chi.URLParam(r, "npcId")
	userID := middleware.UserIDFromContext(r.Context())

	npc, err := h.npcs.GetByID(r.Context(), npcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if npc == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}

	isDM, ok := isCampaignDM(r.Context(), h.campaigns, npc.CampaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !isDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "campaignId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.npcs.Update(r.Context(), npcID, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/npcs/{npcId}.
func (h *NpcHandler) Delete(w http.ResponseWriter, r *http.Request) {
	npcID := chi.URLParam(r, "npcId")
	userID := middleware.UserIDFromContext(r.Context())

	npc, err := h.npcs.GetByID(r.Context(), npcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if npc == nil {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}

	isDM, ok := isCampaignDM(r.Context(), h.campaigns, npc.CampaignID, userID)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !isDM {
		writeError(w, http.StatusForbidden, "forbidden", "FORBIDDEN")
		return
	}

	found, err := h.npcs.Delete(r.Context(), npcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "npc not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
