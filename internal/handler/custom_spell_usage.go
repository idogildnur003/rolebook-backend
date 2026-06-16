package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type customSpellHolder struct {
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
}

type customSpellUsage struct {
	model.CustomSpell
	Holders       []customSpellHolder `json:"holders"`
	CreatedByName string              `json:"createdByName"`
}

// buildCustomSpellUsage joins custom spells with the players who know each, and
// resolves the creator display name. Pure over its inputs (mirrors
// buildCustomEquipmentUsage). Holders sorted by name then id; never nil.
func buildCustomSpellUsage(spells []model.CustomSpell, players []store.PlayerSpellSummary) []customSpellUsage {
	holdersBySpell := make(map[string][]customSpellHolder)
	nameByUserID := make(map[string]string)
	for _, p := range players {
		if p.LinkedUserID != "" {
			nameByUserID[p.LinkedUserID] = p.Name
		}
		for _, sp := range p.Spells {
			holdersBySpell[sp.SpellID] = append(holdersBySpell[sp.SpellID], customSpellHolder{PlayerID: p.ID, PlayerName: p.Name})
		}
	}
	usage := make([]customSpellUsage, 0, len(spells))
	for _, spell := range spells {
		holders := holdersBySpell[spell.ID]
		if holders == nil {
			holders = []customSpellHolder{}
		}
		sort.Slice(holders, func(i, j int) bool {
			if holders[i].PlayerName != holders[j].PlayerName {
				return holders[i].PlayerName < holders[j].PlayerName
			}
			return holders[i].PlayerID < holders[j].PlayerID
		})
		usage = append(usage, customSpellUsage{CustomSpell: spell, Holders: holders, CreatedByName: nameByUserID[spell.CreatedBy]})
	}
	return usage
}

// Usage handles GET /api/campaigns/:campaignId/custom-spells/usage (DM only).
func (h *CustomSpellHandler) Usage(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	membership := resolveCampaignMembership(w, r, h.campaigns, campaignID)
	if membership == nil {
		return
	}
	if !membership.IsDM {
		writeError(w, http.StatusForbidden, "only the DM can view custom spell usage", "FORBIDDEN")
		return
	}
	spells, err := h.customSpells.ListByCampaign(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	summaries, err := h.players.ListSpellSummaries(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	usage := buildCustomSpellUsage(spells, summaries)
	for i := range usage {
		usage[i].ImageURI = h.avatars.ResolveImageURI(r.Context(), usage[i].ImageURI)
	}
	writeJSON(w, http.StatusOK, usage)
}
