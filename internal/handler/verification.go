package handler

import (
	"crypto/rand"
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

// emailVerificationBlocksLogin reports whether login must be refused: only for
// unverified accounts that are subject to the hard gate (new signups). Legacy
// accounts (LegacyUnverified=true) are exempt and may log in unverified.
func emailVerificationBlocksLogin(u *model.User) bool {
	return !u.EmailVerified && !u.LegacyUnverified
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
