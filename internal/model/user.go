package model

import "time"

// User is stored in the "users" collection.
// No createdAt field — consistent with all other resources which expose only
// updatedAt. The VerifyCode* fields are operational (not audit) state for email
// verification and are never serialized to clients.
type User struct {
	ID           string `bson:"_id"          json:"id"`
	Email        string `bson:"email"        json:"email"`
	PasswordHash string `bson:"passwordHash" json:"-"`

	// EmailVerified is true once the address has been confirmed via OTP.
	EmailVerified bool `bson:"emailVerified" json:"emailVerified"`
	// LegacyUnverified marks the small, fixed set of accounts that predate email
	// verification (set once by the backfill migration). They are exempt from the
	// hard gate (soft prompt instead). New signups leave it false and must verify.
	LegacyUnverified bool `bson:"legacyUnverified,omitempty" json:"-"`

	VerifyCodeHash      string    `bson:"verifyCodeHash,omitempty"      json:"-"`
	VerifyCodeExpiresAt time.Time `bson:"verifyCodeExpiresAt,omitempty" json:"-"`
	VerifyCodeAttempts  int       `bson:"verifyCodeAttempts,omitempty"  json:"-"`
	VerifyCodeSentAt    time.Time `bson:"verifyCodeSentAt,omitempty"    json:"-"`
}
