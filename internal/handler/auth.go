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
	"github.com/elad/rolebook-backend/internal/resetstore"
)

// userRepo is the subset of *store.UserStore that AuthHandler depends on.
// Declaring it as an interface here keeps the handler testable with an in-memory
// fake (mirrors catalogImageRepo in arsenal.go); *store.UserStore satisfies it,
// so the production wiring in routes.go is unchanged.
type userRepo interface {
	Create(ctx context.Context, u *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
	SetVerificationCode(ctx context.Context, userID, codeHash string, expiresAt, sentAt time.Time) error
	IncrementVerifyAttempts(ctx context.Context, userID string) error
	MarkVerified(ctx context.Context, userID string) error
	UpdatePasswordHash(ctx context.Context, id, hash string) error
	SetPendingEmailCode(ctx context.Context, userID, pendingEmail, codeHash string, expiresAt, sentAt time.Time) error
	CommitEmailChange(ctx context.Context, userID, newEmail string) error
	MarkPasswordReset(ctx context.Context, userID, hash string, changedAt time.Time) error
}

// AuthHandler handles user registration, login, and email verification.
type AuthHandler struct {
	users               userRepo
	jwtSecret           []byte
	email               email.Sender
	verificationEnabled bool
	adminIDs            []string
	resets              resetstore.Store
}

// NewAuthHandler creates a new AuthHandler. verificationEnabled gates the
// entire OTP flow: when false (e.g. local dev with no Resend key), Register
// issues a JWT directly and Login skips the unverified gate. adminIDs is the
// allowlist used to stamp IsAdmin on auth responses. resets backs the
// forgot-password flow (reset codes, cooldowns, and — later — reset tokens).
func NewAuthHandler(users userRepo, jwtSecret string, sender email.Sender, verificationEnabled bool, adminIDs []string, resets resetstore.Store) *AuthHandler {
	return &AuthHandler{
		users:               users,
		jwtSecret:           []byte(jwtSecret),
		email:               sender,
		verificationEnabled: verificationEnabled,
		adminIDs:            adminIDs,
		resets:              resets,
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

	// The gate only applies when verification is enabled. With it disabled (local
	// dev, or an incident switch), existing unverified accounts log in normally —
	// otherwise flipping the flag off would strand every account created while it
	// was on.
	if h.verificationEnabled && emailVerificationBlocksLogin(user) {
		// Proactively (re)issue a code so the client's verify screen has one
		// waiting, instead of forcing the user to tap "Resend". Cooldown-guarded.
		sent := h.issueVerificationCode(r.Context(), user)
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":    "email not verified",
			"code":     "EMAIL_NOT_VERIFIED",
			"codeSent": sent,
		})
		return
	}

	token, err := h.signToken(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified, IsAdmin: middleware.IsAdmin(h.adminIDs, user.ID)})
}

// issueVerificationCode ensures an unverified user has a usable OTP: if the
// resend cooldown has elapsed it generates, persists, and emails a fresh code;
// otherwise the previously-sent code still stands (its TTL exceeds the cooldown).
// Returns whether a code is available to the user afterwards. All inner failures
// are logged, never surfaced — a false return means no code could be issued now.
func (h *AuthHandler) issueVerificationCode(ctx context.Context, user *model.User) bool {
	if !user.VerifyCodeSentAt.IsZero() && time.Since(user.VerifyCodeSentAt) < resendCooldown {
		return true // a recently-sent code is still valid
	}
	code, err := generateVerificationCode()
	if err != nil {
		log.Printf("[auth] issue code: generate failed for %s: %v", user.ID, err)
		return false
	}
	codeHash, err := hashVerificationCode(code)
	if err != nil {
		log.Printf("[auth] issue code: hash failed for %s: %v", user.ID, err)
		return false
	}
	now := time.Now()
	if err := h.users.SetVerificationCode(ctx, user.ID, codeHash, now.Add(verifyCodeTTL), now); err != nil {
		log.Printf("[auth] issue code: persist failed for %s: %v", user.ID, err)
		return false
	}
	h.sendVerificationCode(ctx, user.Email, code)
	return true
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

	// Always return 200 (no account enumeration). issueVerificationCode is
	// cooldown-guarded and logs any inner failure, so a broken DB / RNG / sender
	// stays visible to operators instead of looking like a successful send.
	if user != nil && !user.EmailVerified {
		h.issueVerificationCode(r.Context(), user)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// ForgotPassword handles POST /api/auth/forgot-password (public). It always
// responds 200 with a generic body — never revealing whether the account
// exists. When it does exist and the resend cooldown has elapsed, it stores a
// fresh reset code and emails it.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Generic OK regardless of outcome (anti-enumeration). Real work is
	// best-effort and its failures are logged, never surfaced.
	if email != "" {
		if user, err := h.users.FindByEmail(r.Context(), email); err == nil && user != nil {
			h.issueResetCode(r.Context(), email)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "reset_requested",
	})
}

// issueResetCode generates, stores, and emails a reset code, guarded by the
// resend cooldown. All failures are logged, never surfaced.
func (h *AuthHandler) issueResetCode(ctx context.Context, email string) {
	allowed, err := h.resets.MarkSent(ctx, email)
	if err != nil {
		log.Printf("[auth] reset: cooldown check failed for %s: %v", email, err)
		return
	}
	if !allowed {
		return // still within the 60s resend window
	}
	code, err := generateVerificationCode()
	if err != nil {
		log.Printf("[auth] reset: generate code failed for %s: %v", email, err)
		return
	}
	codeHash, err := hashVerificationCode(code)
	if err != nil {
		log.Printf("[auth] reset: hash code failed for %s: %v", email, err)
		return
	}
	if err := h.resets.SetCode(ctx, email, codeHash); err != nil {
		log.Printf("[auth] reset: store code failed for %s: %v", email, err)
		return
	}
	subject, html, text := passwordResetEmailBody(code)
	if err := h.email.Send(ctx, email, subject, html, text); err != nil {
		log.Printf("[auth] reset: send email failed for %s: %v", email, err)
	}
}

type verifyResetCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// VerifyResetCode handles POST /api/auth/verify-reset-code (public). On a valid
// code it consumes the code and returns a single-use reset token. All failures
// use a generic 400 to avoid revealing whether an account or code exists.
func (h *AuthHandler) VerifyResetCode(w http.ResponseWriter, r *http.Request) {
	var req verifyResetCodeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	if email == "" || code == "" {
		writeError(w, http.StatusBadRequest, "email and code are required", "BAD_REQUEST")
		return
	}

	sess, err := h.resets.Get(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	// No session, or already promoted to a token (CodeHash cleared) -> generic invalid.
	if sess == nil || sess.CodeHash == "" {
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}
	if sess.Attempts >= maxVerifyAttempts {
		writeError(w, http.StatusBadRequest, "too many attempts; request a new code", "TOO_MANY_ATTEMPTS")
		return
	}
	if !verificationCodeMatches(sess.CodeHash, code) {
		if _, err := h.resets.IncrAttempts(r.Context(), email); err != nil {
			log.Printf("[auth] reset: incr attempts failed for %s: %v", email, err)
		}
		writeError(w, http.StatusBadRequest, "invalid or expired code", "INVALID_CODE")
		return
	}

	token, err := generateResetToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if err := h.resets.PromoteToToken(r.Context(), email, hashResetToken(token)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"resetToken": token})
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	ResetToken  string `json:"resetToken"`
	NewPassword string `json:"newPassword"`
}

// ResetPassword handles POST /api/auth/reset-password (public). It exchanges a
// single-use reset token for a password change, stamps passwordChangedAt to
// revoke pre-reset sessions, and clears the reset session. Email is included so
// the user is looked up by the indexed email field rather than by token.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.ResetToken == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "email, resetToken and newPassword are required", "BAD_REQUEST")
		return
	}
	if msg := validateNewPassword(req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg, "WEAK_PASSWORD")
		return
	}

	sess, err := h.resets.Get(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if sess == nil || sess.TokenHash == "" || !resetTokenMatches(sess.TokenHash, req.ResetToken) {
		writeError(w, http.StatusBadRequest, "invalid or expired reset token", "INVALID_TOKEN")
		return
	}

	user, err := h.users.FindByEmail(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if user == nil {
		// Token existed but account vanished — treat as invalid.
		writeError(w, http.StatusBadRequest, "invalid or expired reset token", "INVALID_TOKEN")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if err := h.users.MarkPasswordReset(r.Context(), user.ID, string(hash), time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if err := h.resets.Clear(r.Context(), email); err != nil {
		log.Printf("[auth] reset: clear session failed for %s: %v", email, err)
	}

	// Best-effort security heads-up.
	subject, html, text := passwordChangedNotificationBody()
	if err := h.email.Send(r.Context(), user.Email, subject, html, text); err != nil {
		log.Printf("[auth] reset: send notice failed for %s: %v", user.Email, err)
	}

	w.WriteHeader(http.StatusNoContent)
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
