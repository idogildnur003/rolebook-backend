package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/elad/rolebook-backend/internal/email"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// AuthHandler handles user registration, login, and email verification.
type AuthHandler struct {
	users               *store.UserStore
	jwtSecret           []byte
	email               email.Sender
	verificationEnabled bool
	adminIDs            []string
}

// NewAuthHandler creates a new AuthHandler. verificationEnabled gates the
// entire OTP flow: when false (e.g. local dev with no Resend key), Register
// issues a JWT directly and Login skips the unverified gate. adminIDs is the
// allowlist used to stamp IsAdmin on auth responses.
func NewAuthHandler(users *store.UserStore, jwtSecret string, sender email.Sender, verificationEnabled bool, adminIDs []string) *AuthHandler {
	return &AuthHandler{
		users:               users,
		jwtSecret:           []byte(jwtSecret),
		email:               sender,
		verificationEnabled: verificationEnabled,
		adminIDs:            adminIDs,
	}
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token         string `json:"token"`
	UserID        string `json:"userId"`
	EmailVerified bool   `json:"emailVerified"`
	IsAdmin       bool   `json:"isAdmin"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

const minPasswordLength = 8

// validateNewPassword returns a human-readable error message if pw is not an
// acceptable new password, or "" if it is valid.
func validateNewPassword(pw string) string {
	if len(pw) < minPasswordLength {
		return "new password must be at least 8 characters"
	}
	return ""
}

type registerResponse struct {
	Status string `json:"status"`
	Email  string `json:"email"`
}

// Register handles POST /api/auth/register. It creates an unverified account,
// emails a 6-digit code, and returns {status:"verification_required"} — no JWT
// is issued until the code is confirmed via VerifyEmail.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required", "BAD_REQUEST")
		return
	}

	existing, err := h.users.FindByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered", "EMAIL_TAKEN")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Verification-disabled (local dev, no Resend key): create a verified
	// account and issue the token directly — no code, no email.
	if !h.verificationEnabled {
		user := &model.User{
			ID:            uuid.NewString(),
			Email:         req.Email,
			PasswordHash:  string(hash),
			EmailVerified: true,
		}
		if err := h.users.Create(r.Context(), user); err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		h.issueTokenWithStatus(w, user, http.StatusCreated)
		return
	}

	code, err := generateVerificationCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	codeHash, err := hashVerificationCode(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	now := time.Now()
	user := &model.User{
		ID:                  uuid.NewString(),
		Email:               req.Email,
		PasswordHash:        string(hash),
		EmailVerified:       false,
		VerifyCodeHash:      codeHash,
		VerifyCodeExpiresAt: now.Add(verifyCodeTTL),
		VerifyCodeSentAt:    now,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Best-effort send: if it fails the account still exists, and the user can
	// recover via /auth/resend-verification rather than being trapped behind
	// EMAIL_TAKEN on a retry.
	h.sendVerificationCode(r.Context(), user.Email, code)

	writeJSON(w, http.StatusCreated, registerResponse{Status: "verification_required", Email: user.Email})
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required", "BAD_REQUEST")
		return
	}

	user, err := h.users.FindByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "UNAUTHORIZED")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "UNAUTHORIZED")
		return
	}

	if emailVerificationBlocksLogin(user) {
		writeError(w, http.StatusForbidden, "email not verified", "EMAIL_NOT_VERIFIED")
		return
	}

	token, err := h.signToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified, IsAdmin: middleware.IsAdmin(h.adminIDs, user.ID)})
}

// sendVerificationCode emails an OTP, logging (but not surfacing) send errors.
func (h *AuthHandler) sendVerificationCode(ctx context.Context, to, code string) {
	subject, html, text := verificationEmailBody(code)
	if err := h.email.Send(ctx, to, subject, html, text); err != nil {
		log.Printf("[auth] failed to send verification email to %s: %v", to, err)
	}
}

// issueTokenWithStatus signs a JWT and writes an authResponse with the given
// HTTP status. Used by the verification-disabled register path (201) and any
// other direct-token outcomes; verify-email uses issueToken (200) instead.
func (h *AuthHandler) issueTokenWithStatus(w http.ResponseWriter, user *model.User, status int) {
	token, err := h.signToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, status, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified, IsAdmin: middleware.IsAdmin(h.adminIDs, user.ID)})
}

// ChangePassword handles POST /api/auth/change-password (authenticated).
// Verifies the caller's current password and stores a new bcrypt hash.
// The existing JWT remains valid — no token is re-issued.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid token", "UNAUTHORIZED")
		return
	}

	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current and new password are required", "BAD_REQUEST")
		return
	}
	if msg := validateNewPassword(req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg, "WEAK_PASSWORD")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "UNAUTHORIZED")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		// 400 (not 401) on purpose: the token is valid; only the supplied
		// current password is wrong. A 401 would trip the frontend's global
		// logout interceptor.
		writeError(w, http.StatusBadRequest, "current password is incorrect", "INVALID_CURRENT_PASSWORD")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	if err := h.users.UpdatePasswordHash(r.Context(), userID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Best-effort security heads-up to the account address.
	subject, html, text := passwordChangedNotificationBody()
	if err := h.email.Send(r.Context(), user.Email, subject, html, text); err != nil {
		log.Printf("[auth] failed to send password-change notice to %s: %v", user.Email, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) signToken(userID string) (string, error) {
	claims := &middleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}

type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// VerifyEmail handles POST /api/auth/verify-email. On success it marks the
// account verified and issues a JWT. Wrong/expired codes return 400 (never 401,
// which the frontend treats as a forced logout).
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if req.Email == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "email and code are required", "BAD_REQUEST")
		return
	}

	user, err := h.users.FindByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	// Generic error for unknown email — avoids account enumeration.
	if user == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}

	// Already-verified accounts must NOT short-circuit to a token here: this
	// endpoint is unauthenticated, so handing out a JWT for a known email
	// without checking the code would be an account-takeover vector. Tell the
	// caller to log in via /auth/login instead.
	if user.EmailVerified {
		writeError(w, http.StatusBadRequest, "email already verified", "ALREADY_VERIFIED")
		return
	}

	if user.VerifyCodeHash == "" || codeExpired(user.VerifyCodeExpiresAt, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}
	if user.VerifyCodeAttempts >= maxVerifyAttempts {
		writeError(w, http.StatusTooManyRequests, "too many attempts, request a new code", "TOO_MANY_ATTEMPTS")
		return
	}
	if !verificationCodeMatches(user.VerifyCodeHash, req.Code) {
		if err := h.users.IncrementVerifyAttempts(r.Context(), user.ID); err != nil {
			log.Printf("[auth] failed to increment verify attempts for %s: %v", user.ID, err)
		}
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}

	if err := h.users.MarkVerified(r.Context(), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	user.EmailVerified = true
	h.issueToken(w, user)
}

// issueToken signs a JWT for the user and writes the auth response.
func (h *AuthHandler) issueToken(w http.ResponseWriter, user *model.User) {
	token, err := h.signToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified, IsAdmin: middleware.IsAdmin(h.adminIDs, user.ID)})
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

// ResendVerification handles POST /api/auth/resend-verification. It always
// returns 200 with a generic body (no account enumeration), only actually
// sending a fresh code when the account exists, is unverified, and is past the
// resend cooldown.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendVerificationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required", "BAD_REQUEST")
		return
	}

	user, err := h.users.FindByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	if user != nil && !user.EmailVerified {
		withinCooldown := !user.VerifyCodeSentAt.IsZero() && time.Since(user.VerifyCodeSentAt) < resendCooldown
		if !withinCooldown {
			// We always return 200 to the caller (no account enumeration), but
			// inner failures must still be logged so a broken DB / RNG / sender
			// is visible to operators instead of looking like a successful send.
			code, gerr := generateVerificationCode()
			if gerr != nil {
				log.Printf("[auth] resend: generate code failed for %s: %v", user.ID, gerr)
			} else {
				codeHash, herr := hashVerificationCode(code)
				if herr != nil {
					log.Printf("[auth] resend: hash code failed for %s: %v", user.ID, herr)
				} else {
					now := time.Now()
					if serr := h.users.SetVerificationCode(r.Context(), user.ID, codeHash, now.Add(verifyCodeTTL), now); serr != nil {
						log.Printf("[auth] resend: persist code failed for %s: %v", user.ID, serr)
					} else {
						h.sendVerificationCode(r.Context(), user.Email, code)
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type changeEmailRequest struct {
	NewEmail        string `json:"newEmail"`
	CurrentPassword string `json:"currentPassword"`
}

// ChangeEmail handles POST /api/auth/change-email (authenticated). It re-auths
// with the current password, checks the new address is free, then stores it as
// pending and emails a 6-digit code to the NEW address. The account email is
// NOT changed until VerifyEmailChange confirms the code (pending swap).
func (h *AuthHandler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid token", "UNAUTHORIZED")
		return
	}
	var req changeEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.NewEmail = strings.ToLower(strings.TrimSpace(req.NewEmail))
	if req.NewEmail == "" || req.CurrentPassword == "" {
		writeError(w, http.StatusBadRequest, "new email and current password are required", "BAD_REQUEST")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "UNAUTHORIZED")
		return
	}
	if req.NewEmail == user.Email {
		writeError(w, http.StatusBadRequest, "new email matches the current email", "SAME_EMAIL")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		// 400 (not 401) on purpose: the token is valid; only the supplied current
		// password is wrong. A 401 would trip the frontend's global logout.
		writeError(w, http.StatusBadRequest, "current password is incorrect", "INVALID_CURRENT_PASSWORD")
		return
	}

	existing, err := h.users.FindByEmail(r.Context(), req.NewEmail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "email already registered", "EMAIL_TAKEN")
		return
	}

	code, err := generateVerificationCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	codeHash, err := hashVerificationCode(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	now := time.Now()
	if err := h.users.SetPendingEmailCode(r.Context(), userID, req.NewEmail, codeHash, now.Add(verifyCodeTTL), now); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	h.sendChangeEmailCode(r.Context(), req.NewEmail, code)

	writeJSON(w, http.StatusOK, map[string]string{"status": "verification_required", "email": req.NewEmail})
}

// sendChangeEmailCode emails the OTP to the new address, logging send errors.
func (h *AuthHandler) sendChangeEmailCode(ctx context.Context, to, code string) {
	subject, html, text := changeEmailCodeBody(code)
	if err := h.email.Send(ctx, to, subject, html, text); err != nil {
		log.Printf("[auth] failed to send email-change code to %s: %v", to, err)
	}
}

type verifyEmailChangeRequest struct {
	Code string `json:"code"`
}

// VerifyEmailChange handles POST /api/auth/verify-email-change (authenticated).
// On a correct code it swaps the account email to the pending address and emails
// a security heads-up to the OLD address. Wrong/expired codes return 400 (never
// 401, which the frontend treats as a forced logout).
func (h *AuthHandler) VerifyEmailChange(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid token", "UNAUTHORIZED")
		return
	}
	var req verifyEmailChangeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required", "BAD_REQUEST")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "UNAUTHORIZED")
		return
	}
	if user.PendingEmail == "" || user.VerifyCodeHash == "" {
		writeError(w, http.StatusBadRequest, "no pending email change", "NO_PENDING_CHANGE")
		return
	}
	if codeExpired(user.VerifyCodeExpiresAt, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}
	if user.VerifyCodeAttempts >= maxVerifyAttempts {
		writeError(w, http.StatusTooManyRequests, "too many attempts, request a new code", "TOO_MANY_ATTEMPTS")
		return
	}
	if !verificationCodeMatches(user.VerifyCodeHash, req.Code) {
		if err := h.users.IncrementVerifyAttempts(r.Context(), userID); err != nil {
			log.Printf("[auth] failed to increment email-change attempts for %s: %v", userID, err)
		}
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}

	// Re-check uniqueness at commit time (the address may have been taken after
	// the change was initiated).
	existing, err := h.users.FindByEmail(r.Context(), user.PendingEmail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if existing != nil && existing.ID != userID {
		writeError(w, http.StatusConflict, "email already registered", "EMAIL_TAKEN")
		return
	}

	oldEmail, newEmail := user.Email, user.PendingEmail
	if err := h.users.CommitEmailChange(r.Context(), userID, newEmail); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Best-effort security alert to the OLD address.
	subject, html, text := emailChangedNotificationBody(newEmail)
	if err := h.email.Send(r.Context(), oldEmail, subject, html, text); err != nil {
		log.Printf("[auth] failed to send email-change notice to %s: %v", oldEmail, err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"email": newEmail})
}

// ResendEmailChange handles POST /api/auth/resend-email-change (authenticated).
// Re-sends the code to the pending address if one is in flight and the resend
// cooldown has elapsed. Always 200 with {status:"ok"} when a change is pending.
func (h *AuthHandler) ResendEmailChange(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid token", "UNAUTHORIZED")
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil || user.PendingEmail == "" {
		writeError(w, http.StatusBadRequest, "no pending email change", "NO_PENDING_CHANGE")
		return
	}

	withinCooldown := !user.VerifyCodeSentAt.IsZero() && time.Since(user.VerifyCodeSentAt) < resendCooldown
	if !withinCooldown {
		if code, gerr := generateVerificationCode(); gerr != nil {
			log.Printf("[auth] resend email-change: generate failed for %s: %v", userID, gerr)
		} else if codeHash, herr := hashVerificationCode(code); herr != nil {
			log.Printf("[auth] resend email-change: hash failed for %s: %v", userID, herr)
		} else {
			now := time.Now()
			if serr := h.users.SetPendingEmailCode(r.Context(), userID, user.PendingEmail, codeHash, now.Add(verifyCodeTTL), now); serr != nil {
				log.Printf("[auth] resend email-change: persist failed for %s: %v", userID, serr)
			} else {
				h.sendChangeEmailCode(r.Context(), user.PendingEmail, code)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
