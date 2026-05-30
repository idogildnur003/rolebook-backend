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
}

// NewAuthHandler creates a new AuthHandler. verificationEnabled gates the
// entire OTP flow: when false (e.g. local dev with no Resend key), Register
// issues a JWT directly and Login skips the unverified gate.
func NewAuthHandler(users *store.UserStore, jwtSecret string, sender email.Sender, verificationEnabled bool) *AuthHandler {
	return &AuthHandler{
		users:               users,
		jwtSecret:           []byte(jwtSecret),
		email:               sender,
		verificationEnabled: verificationEnabled,
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
		ID:                   uuid.NewString(),
		Email:                req.Email,
		PasswordHash:         string(hash),
		EmailVerified:        false,
		VerificationRequired: true,
		VerifyCodeHash:       codeHash,
		VerifyCodeExpiresAt:  now.Add(verifyCodeTTL),
		VerifyCodeSentAt:     now,
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

	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified})
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
	writeJSON(w, status, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified})
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

	// Idempotent: already-verified accounts just get a fresh token.
	if user.EmailVerified {
		h.issueToken(w, user)
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
	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, EmailVerified: user.EmailVerified})
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
			if code, gerr := generateVerificationCode(); gerr == nil {
				if codeHash, herr := hashVerificationCode(code); herr == nil {
					now := time.Now()
					if serr := h.users.SetVerificationCode(r.Context(), user.ID, codeHash, now.Add(verifyCodeTTL), now); serr == nil {
						h.sendVerificationCode(r.Context(), user.Email, code)
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
