package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/elad/rolebook-backend/internal/model"
)

const (
	verifyCodeTTL     = 10 * time.Minute
	maxVerifyAttempts = 5
	resendCooldown    = 60 * time.Second
)

// generateVerificationCode returns a cryptographically random, zero-padded
// 6-digit code, e.g. "004217".
func generateVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashVerificationCode bcrypt-hashes a code for storage at rest.
func hashVerificationCode(code string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// verificationCodeMatches reports whether code matches the stored hash.
func verificationCodeMatches(hash, code string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)) == nil
}

// codeExpired reports whether expiresAt is in the past relative to now.
func codeExpired(expiresAt, now time.Time) bool {
	return now.After(expiresAt)
}

// emailVerificationBlocksLogin reports whether login must be refused: any account
// whose email has not been verified is blocked (the hard gate applies to all).
func emailVerificationBlocksLogin(u *model.User) bool {
	return !u.EmailVerified
}

// changeEmailCodeBody returns the (subject, html, text) for the OTP sent to a
// user's NEW address when they request an email change.
func changeEmailCodeBody(code string) (subject, html, text string) {
	subject = "Confirm your new Rolebook email"
	html = fmt.Sprintf(
		"<p>Use this code to confirm your new Rolebook email address:</p>"+
			"<p style=\"font-size:28px;font-weight:bold;letter-spacing:4px\">%s</p>"+
			"<p>It expires in 10 minutes. If you didn't request this, ignore this email.</p>",
		code,
	)
	text = fmt.Sprintf("Your Rolebook email-change confirmation code is %s. It expires in 10 minutes.", code)
	return subject, html, text
}

// emailChangedNotificationBody returns the (subject, html, text) for the alert
// sent to a user's OLD address after their email is changed. It is a security
// heads-up with a recovery hint and never contains the OTP.
func emailChangedNotificationBody(newEmail string) (subject, html, text string) {
	subject = "Your Rolebook email was changed"
	html = fmt.Sprintf(
		"<p>The email address on your Rolebook account was just changed to <strong>%s</strong>.</p>"+
			"<p>If this was you, no action is needed. If it wasn't, reset your password immediately or contact support.</p>",
		newEmail,
	)
	text = fmt.Sprintf("Your Rolebook account email was changed to %s. If this wasn't you, reset your password immediately or contact support.", newEmail)
	return subject, html, text
}

// passwordChangedNotificationBody returns the (subject, html, text) for the
// security heads-up sent to a user's address after their password is changed.
// It carries no password or code material.
func passwordChangedNotificationBody() (subject, html, text string) {
	subject = "Your Rolebook password was changed"
	html = "<p>The password on your Rolebook account was just changed.</p>" +
		"<p>If this was you, no action is needed. If it wasn't, secure your account or contact support right away.</p>"
	text = "The password on your Rolebook account was just changed. If this wasn't you, secure your account or contact support right away."
	return subject, html, text
}

// verificationEmailBody returns the (subject, htmlBody, textBody) for an OTP.
func verificationEmailBody(code string) (subject, html, text string) {
	subject = "Your Rolebook verification code"
	html = fmt.Sprintf(
		"<p>Welcome to Rolebook!</p><p>Your verification code is:</p>"+
			"<p style=\"font-size:28px;font-weight:bold;letter-spacing:4px\">%s</p>"+
			"<p>It expires in 10 minutes. If you didn't request this, ignore this email.</p>",
		code,
	)
	text = fmt.Sprintf("Your Rolebook verification code is %s. It expires in 10 minutes.", code)
	return subject, html, text
}

// passwordResetEmailBody returns the (subject, html, text) for the OTP sent when
// a user requests a password reset.
func passwordResetEmailBody(code string) (subject, html, text string) {
	subject = "Reset your Rolebook password"
	html = fmt.Sprintf(
		"<p>We received a request to reset your Rolebook password.</p>"+
			"<p>Your reset code is:</p>"+
			"<p style=\"font-size:28px;font-weight:bold;letter-spacing:4px\">%s</p>"+
			"<p>It expires in 10 minutes. If you didn't request this, ignore this email — your password is unchanged.</p>",
		code,
	)
	text = fmt.Sprintf("Your Rolebook password reset code is %s. It expires in 10 minutes. If you didn't request this, ignore this email.", code)
	return subject, html, text
}

// generateResetToken returns a 32-byte cryptographically-random token, hex-encoded.
func generateResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashResetToken returns the SHA-256 hex digest of a reset token, for storage
// at rest. The token is high-entropy, so a fast hash (not bcrypt) is appropriate.
func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// resetTokenMatches reports whether token hashes to the stored hash, using a
// constant-time comparison.
func resetTokenMatches(hash, token string) bool {
	return subtle.ConstantTimeCompare([]byte(hash), []byte(hashResetToken(token))) == 1
}
