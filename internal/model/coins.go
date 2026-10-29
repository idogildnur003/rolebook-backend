package model

import "errors"

// Coins holds a player's purse, one count per D&D 5e denomination.
type Coins struct {
	CP int `bson:"cp" json:"cp"`
	SP int `bson:"sp" json:"sp"`
	EP int `bson:"ep" json:"ep"`
	GP int `bson:"gp" json:"gp"`
	PP int `bson:"pp" json:"pp"`
}

// CoinDenominations lists the valid denomination keys, lowest value first.
var CoinDenominations = []string{"cp", "sp", "ep", "gp", "pp"}

// CoinValueCP is each denomination's value in copper pieces.
var CoinValueCP = map[string]int{"cp": 1, "sp": 10, "ep": 50, "gp": 100, "pp": 1000}

// MaxCoinAmount caps both a single adjustment/conversion amount and a stored balance.
const MaxCoinAmount = 1_000_000_000

var (
	// ErrInvalidConversion — unknown denomination, same from/to, or amount out of range.
	ErrInvalidConversion = errors.New("invalid conversion")
	// ErrUnevenConversion — the amount does not convert to a whole number of target coins.
	ErrUnevenConversion = errors.New("uneven conversion")
)

// IsCoinDenomination reports whether key is one of cp, sp, ep, gp, pp.
func IsCoinDenomination(key string) bool {
	_, ok := CoinValueCP[key]
	return ok
}

// ConvertedAmount returns how many `to` coins result from converting `amount`
// `from` coins. It returns ErrUnevenConversion when the result is not whole.
func ConvertedAmount(from, to string, amount int) (int, error) {
	if !IsCoinDenomination(from) || !IsCoinDenomination(to) || from == to ||
		amount < 1 || amount > MaxCoinAmount {
		return 0, ErrInvalidConversion
	}
	total := amount * CoinValueCP[from]
	if total%CoinValueCP[to] != 0 {
		return 0, ErrUnevenConversion
	}
	return total / CoinValueCP[to], nil
}
