package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const contextKeyUserID contextKey = "userID"

// Claims extends jwt.RegisteredClaims with the user ID (Subject).
type Claims struct {
	jwt.RegisteredClaims
}

// Authenticate validates the Bearer JWT and injects userID into the request context.
// Returns 401 if the token is missing, malformed, or expired.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	secret := []byte(jwtSecret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeUnauthorized(w)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrTokenSignatureInvalid
				}
				return secret, nil
			})
			if err != nil || !token.Valid {
				writeUnauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), contextKeyUserID, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext extracts the userID injected by Authenticate middleware.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyUserID).(string)
	return v
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"missing or invalid token","code":"UNAUTHORIZED"}`))
}

// IsAdmin reports whether userID is in the admin allowlist. An empty userID or
// empty allowlist is never admin.
func IsAdmin(adminIDs []string, userID string) bool {
	if userID == "" {
		return false
	}
	for _, id := range adminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// RequireAdmin allows only requests whose authenticated user ID is in adminIDs.
// Must run inside an Authenticate group (it reads the userID from context).
func RequireAdmin(adminIDs []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsAdmin(adminIDs, UserIDFromContext(r.Context())) {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":"forbidden","code":"FORBIDDEN"}`))
}
