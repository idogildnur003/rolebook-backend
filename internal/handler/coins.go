package handler

import (
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

type CoinsHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

func NewCoinsHandler(players *store.PlayerStore, campaigns *store.CampaignStore) *CoinsHandler {
	return &CoinsHandler{players: players, campaigns: campaigns}
}

// parseCoinDeltas validates a coin adjustment body. It drops zero entries and
// rejects unknown keys, non-integer values, out-of-range values and an empty result.
func parseCoinDeltas(body map[string]any) (map[string]int, error) {
	deltas := make(map[string]int, len(body))
	for key, raw := range body {
		if !model.IsCoinDenomination(key) {
			return nil, fmt.Errorf("unknown coin denomination %q", key)
		}
		f, ok := raw.(float64)
		if !ok || f != math.Trunc(f) {
			return nil, fmt.Errorf("%s must be an integer", key)
		}
		if math.Abs(f) > model.MaxCoinAmount {
			return nil, fmt.Errorf("%s is out of range", key)
		}
		if f != 0 {
			deltas[key] = int(f)
		}
	}
	if len(deltas) == 0 {
		return nil, errors.New("at least one non-zero coin amount is required")
	}
	return deltas, nil
}

// writeCoinStoreError maps a store error from a coin mutation to a response.
func writeCoinStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrInsufficientCoins) {
		writeError(w, http.StatusConflict, "not enough coins", "INSUFFICIENT_COINS")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
}

// Get handles GET /api/players/:playerId/coins.
func (h *CoinsHandler) Get(w http.ResponseWriter, r *http.Request) {
	access := resolvePlayerAccess(w, r, h.players, h.campaigns, chi.URLParam(r, "playerId"))
	if access == nil {
		return
	}
	writeJSON(w, http.StatusOK, access.Player.Coins)
}

// Adjust handles PATCH /api/players/:playerId/coins.
func (h *CoinsHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	deltas, err := parseCoinDeltas(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}

	coins, err := h.players.AdjustCoins(r.Context(), playerID, deltas)
	if err != nil {
		writeCoinStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coins)
}

// Convert handles POST /api/players/:playerId/coins/convert.
func (h *CoinsHandler) Convert(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")

	access := resolvePlayerAccess(w, r, h.players, h.campaigns, playerID)
	if access == nil {
		return
	}

	var req struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Amount int    `json:"amount"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	gained, err := model.ConvertedAmount(req.From, req.To, req.Amount)
	if err != nil {
		if errors.Is(err, model.ErrUnevenConversion) {
			writeError(w, http.StatusBadRequest, "conversion does not divide evenly", "UNEVEN_CONVERSION")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid conversion", "BAD_REQUEST")
		return
	}

	coins, err := h.players.ConvertCoins(r.Context(), playerID, req.From, req.To, req.Amount, gained)
	if err != nil {
		writeCoinStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, coins)
}
