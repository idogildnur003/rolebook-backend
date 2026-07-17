// Package resetstore holds the ephemeral state of a password-reset flow: the
// emailed OTP (hashed), its attempt counter, and — once the OTP is verified —
// a single-use reset token (hashed). State is short-lived (10 min) and carries
// its own TTL, so Redis is the natural backing store; an in-memory fallback is
// used when REDIS_URL is unset (local dev / CI).
package resetstore

import (
	"context"
	"time"

	"github.com/elad/rolebook-backend/config"
)

const (
	CodeTTL     = 10 * time.Minute
	CooldownTTL = 60 * time.Second
)

// Session is the current reset state for one email. CodeHash is empty once the
// code has been promoted to a token; TokenHash is empty before that.
type Session struct {
	CodeHash  string
	Attempts  int
	TokenHash string
}

// Store persists reset sessions keyed by (normalized) email.
type Store interface {
	// MarkSent starts a resend cooldown and reports whether a send is allowed
	// now: true on the first call in a window, false while a code was recently sent.
	MarkSent(ctx context.Context, email string) (bool, error)
	// SetCode starts or replaces a session with a fresh code hash, attempts=0,
	// and no token. Refreshes the TTL.
	SetCode(ctx context.Context, email, codeHash string) error
	// Get returns the session, or nil if none exists / it has expired.
	Get(ctx context.Context, email string) (*Session, error)
	// IncrAttempts increments and returns the failed-attempt counter.
	IncrAttempts(ctx context.Context, email string) (int, error)
	// PromoteToToken consumes the code (clears CodeHash) and stores a single-use
	// token hash, refreshing the TTL.
	PromoteToToken(ctx context.Context, email, tokenHash string) error
	// Clear removes all reset state for the email.
	Clear(ctx context.Context, email string) error
}

// New returns a Store backed by an in-memory implementation. A Redis-backed
// implementation (used when cfg.RedisURL is set) is added in a later task; for
// now New always returns the in-memory Store regardless of cfg.
func New(cfg config.Config) Store {
	_ = cfg.RedisURL // reserved for the Redis-backed impl added in a later task
	return NewMemory()
}
