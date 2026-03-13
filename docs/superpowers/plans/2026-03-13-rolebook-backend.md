# Rolebook Backend Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a Go 1.26 REST API for the D&D 5e Companion app with JWT auth, roles, MongoDB Atlas persistence, and Railway deployment.

**Architecture:** Flat package layout — `handler/`, `middleware/`, `model/`, `store/` under `internal/`. All routes under `/api` prefix with JWT middleware. Single admin via `ADMIN_EMAIL` env var. Players scoped to their character via `linkedUserId`. No tests in this iteration.

**Tech Stack:** Go 1.26, chi v5, mongo-driver v2, golang-jwt v5, bcrypt, google/uuid

---

## Chunk 1: Scaffolding

### Task 1: Initialize Go module and install dependencies

**Files:**
- Create: `go.mod`
- Create: `go.sum` (auto-generated)

- [ ] **Step 1: Initialize module**

```bash
cd /path/to/rolebook-backend
go mod init github.com/elad/rolebook-backend
```

- [ ] **Step 2: Install dependencies**

```bash
go get github.com/go-chi/chi/v5
go get go.mongodb.org/mongo-driver/v2/mongo
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
go get github.com/google/uuid
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize Go module and add dependencies"
```

---

### Task 2: Config package

**Files:**
- Create: `config/config.go`

- [ ] **Step 1: Create `config/config.go`**

```go
package config

import "os"

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port       string
	MongoURI   string
	JWTSecret  string
	AdminEmail string
}

// Load reads configuration from environment variables.
// Panics if MONGO_URI or JWT_SECRET are missing.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	return Config{
		Port:       port,
		MongoURI:   mustEnv("MONGO_URI"),
		JWTSecret:  mustEnv("JWT_SECRET"),
		AdminEmail: os.Getenv("ADMIN_EMAIL"),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}
```

- [ ] **Step 2: Commit**

```bash
git add config/config.go
git commit -m "feat: add config package"
```

---

### Task 3: MongoDB connection helper

**Files:**
- Create: `internal/store/mongo.go`

- [ ] **Step 1: Create `internal/store/mongo.go`**

```go
package store

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DB wraps the MongoDB client and database handle.
type DB struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewDB connects to MongoDB and returns a DB handle for the "rolebook" database.
func NewDB(uri string) (*DB, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	return &DB{
		client: client,
		db:     client.Database("rolebook"),
	}, nil
}

// Collection returns a handle for the named collection.
func (d *DB) Collection(name string) *mongo.Collection {
	return d.db.Collection(name)
}

// Disconnect closes the MongoDB connection.
func (d *DB) Disconnect(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/mongo.go
git commit -m "feat: add MongoDB connection helper"
```

---

### Task 4: Main entry point skeleton

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: Create `cmd/server/main.go`**

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.NewDB(cfg.MongoURI)
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := db.Disconnect(context.Background()); err != nil {
			log.Printf("error disconnecting from MongoDB: %v", err)
		}
	}()

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Routes registered in subsequent tasks via registerRoutes
	registerRoutes(r, cfg, db)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Println("server stopped")
}

// registerRoutes is populated incrementally as handlers are added.
// It lives in cmd/server/routes.go (created in Task 11).
```

- [ ] **Step 2: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: add server entry point skeleton"
```

---

### Task 5: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
# Stage 1: build
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Stage 2: minimal runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 3000
CMD ["./server"]
```

- [ ] **Step 2: Commit**

```bash
git add Dockerfile
git commit -m "feat: add two-stage Dockerfile for Railway deployment"
```

---

## Chunk 2: Auth Infrastructure

### Task 6: Shared response helpers

**Files:**
- Create: `internal/handler/response.go`

- [ ] **Step 1: Create `internal/handler/response.go`**

```go
package handler

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, map[string]string{
		"error": message,
		"code":  code,
	})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/response.go
git commit -m "feat: add shared JSON response helpers"
```

---

### Task 7: User model

**Files:**
- Create: `internal/model/user.go`

- [ ] **Step 1: Create `internal/model/user.go`**

```go
package model

// Role represents a user's access level.
type Role string

const (
	RoleAdmin  Role = "admin"
	RolePlayer Role = "player"
)

// User is stored in the "users" collection.
// No createdAt field — consistent with all other resources which expose only updatedAt.
type User struct {
	ID           string `bson:"_id"          json:"id"`
	Email        string `bson:"email"        json:"email"`
	PasswordHash string `bson:"passwordHash" json:"-"`
	Role         Role   `bson:"role"         json:"role"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/user.go
git commit -m "feat: add user model"
```

---

### Task 8: User store

**Files:**
- Create: `internal/store/user.go`

- [ ] **Step 1: Create `internal/store/user.go`**

```go
package store

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// UserStore handles persistence for user accounts.
type UserStore struct {
	col *mongo.Collection
}

// NewUserStore creates a UserStore and ensures the unique email index exists.
func NewUserStore(db *DB) *UserStore {
	col := db.Collection("users")
	_, _ = col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &UserStore{col: col}
}

// Create inserts a new user. Returns an error if the email already exists.
func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	_, err := s.col.InsertOne(ctx, u)
	return err
}

// FindByEmail returns the user with the given email, or nil if not found.
func (s *UserStore) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := s.col.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/user.go
git commit -m "feat: add user store"
```

---

### Task 9: JWT middleware

**Files:**
- Create: `internal/middleware/auth.go`

- [ ] **Step 1: Create `internal/middleware/auth.go`**

```go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/elad/rolebook-backend/internal/model"
)

type contextKey string

const (
	contextKeyUserID contextKey = "userID"
	contextKeyRole   contextKey = "role"
)

// Claims extends jwt.RegisteredClaims with the user's role.
type Claims struct {
	Role model.Role `json:"role"`
	jwt.RegisteredClaims
}

// Authenticate validates the Bearer JWT and injects userID and role into the request context.
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
			ctx = context.WithValue(ctx, contextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns 403 if the authenticated user does not have the specified role.
// Must be used after Authenticate middleware.
func RequireRole(role model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole := RoleFromContext(r.Context())
			if userRole != role {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserIDFromContext extracts the userID injected by Authenticate middleware.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyUserID).(string)
	return v
}

// RoleFromContext extracts the role injected by Authenticate middleware.
func RoleFromContext(ctx context.Context) model.Role {
	v, _ := ctx.Value(contextKeyRole).(model.Role)
	return v
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"missing or invalid token","code":"UNAUTHORIZED"}`))
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":"insufficient permissions","code":"FORBIDDEN"}`))
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/middleware/auth.go
git commit -m "feat: add JWT authentication middleware"
```

---

### Task 10: Auth handler (register + login)

**Files:**
- Create: `internal/handler/auth.go`

- [ ] **Step 1: Create `internal/handler/auth.go`**

```go
package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// AuthHandler handles user registration and login.
type AuthHandler struct {
	users      *store.UserStore
	jwtSecret  []byte
	adminEmail string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(users *store.UserStore, jwtSecret, adminEmail string) *AuthHandler {
	return &AuthHandler{
		users:      users,
		jwtSecret:  []byte(jwtSecret),
		adminEmail: strings.ToLower(adminEmail),
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token  string     `json:"token"`
	UserID string     `json:"userId"`
	Role   model.Role `json:"role"`
}

// Register handles POST /api/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
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

	role := model.RolePlayer
	if req.Email == h.adminEmail {
		role = model.RoleAdmin
	}

	user := &model.User{
		ID:           uuid.NewString(),
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         role,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	token, err := h.signToken(user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{Token: token, UserID: user.ID, Role: user.Role})
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
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

	token, err := h.signToken(user.ID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, UserID: user.ID, Role: user.Role})
}

func (h *AuthHandler) signToken(userID string, role model.Role) (string, error) {
	claims := &middleware.Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(h.jwtSecret)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/auth.go
git commit -m "feat: add auth handler (register + login)"
```

---

### Task 11: Routes file + wire auth

**Files:**
- Create: `cmd/server/routes.go`

- [ ] **Step 1: Create `cmd/server/routes.go`**

```go
package main

import (
	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/store"
)

func registerRoutes(r *chi.Mux, cfg config.Config, db *store.DB) {
	userStore := store.NewUserStore(db)
	authHandler := handler.NewAuthHandler(userStore, cfg.JWTSecret, cfg.AdminEmail)

	r.Route("/api", func(r chi.Router) {
		// Public auth routes
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected routes (JWT required)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))
			// Campaign, session, player, inventory, spell, arsenal routes added in later tasks
		})
	})
}
```

- [ ] **Step 2: Verify the project compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/routes.go
git commit -m "feat: wire auth routes into server"
```

---

## Chunk 3: Campaigns & Sessions

### Task 12: Campaign model

**Files:**
- Create: `internal/model/campaign.go`

- [ ] **Step 1: Create `internal/model/campaign.go`**

```go
package model

import "time"

// MapPin is a labelled point on a campaign map image.
type MapPin struct {
	ID    string  `bson:"id"    json:"id"`
	X     float64 `bson:"x"     json:"x"`
	Y     float64 `bson:"y"     json:"y"`
	Label string  `bson:"label" json:"label"`
}

// Session is a play session embedded inside a Campaign document.
type Session struct {
	ID          string    `bson:"id"          json:"id"`
	Name        string    `bson:"name"        json:"name"`
	Description string    `bson:"description" json:"description"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}

// Campaign is stored in the "campaigns" collection.
// Sessions are embedded to avoid cross-collection joins.
// createdBy records the admin who created the campaign (informational; not used for access control).
type Campaign struct {
	ID          string    `bson:"_id"         json:"id"`
	CreatedBy   string    `bson:"createdBy"   json:"createdBy"`
	Name        string    `bson:"name"        json:"name"`
	ThemeImage  string    `bson:"themeImage"  json:"themeImage"`
	MapImageURI *string   `bson:"mapImageUri" json:"mapImageUri"`
	MapPins     []MapPin  `bson:"mapPins"     json:"mapPins"`
	Sessions    []Session `bson:"sessions"    json:"sessions"`
	UpdatedAt   time.Time `bson:"updatedAt"   json:"updatedAt"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/campaign.go
git commit -m "feat: add campaign model"
```

---

### Task 13: Campaign store

**Files:**
- Create: `internal/store/campaign.go`

- [ ] **Step 1: Create `internal/store/campaign.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/elad/rolebook-backend/internal/model"
)

// CampaignStore handles persistence for campaigns and embedded sessions.
type CampaignStore struct {
	col *mongo.Collection
}

// NewCampaignStore creates a CampaignStore with a campaignId index on the players collection
// (campaigns themselves have no ownership index since admin access is global).
func NewCampaignStore(db *DB) *CampaignStore {
	return &CampaignStore{col: db.Collection("campaigns")}
}

// ListAll returns all campaigns (admin use — no filter).
func (s *CampaignStore) ListAll(ctx context.Context) ([]model.Campaign, error) {
	return s.find(ctx, bson.M{})
}

// ListByIDs returns campaigns whose IDs are in the given set (player visibility).
func (s *CampaignStore) ListByIDs(ctx context.Context, ids []string) ([]model.Campaign, error) {
	if len(ids) == 0 {
		return []model.Campaign{}, nil
	}
	return s.find(ctx, bson.M{"_id": bson.M{"$in": ids}})
}

// GetByID returns a single campaign, or nil if not found.
func (s *CampaignStore) GetByID(ctx context.Context, id string) (*model.Campaign, error) {
	var c model.Campaign
	err := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Create inserts a new campaign.
func (s *CampaignStore) Create(ctx context.Context, c *model.Campaign) error {
	_, err := s.col.InsertOne(ctx, c)
	return err
}

// Update applies a partial update to a campaign. updatedAt is always set to now.
func (s *CampaignStore) Update(ctx context.Context, id string, fields bson.M) (*model.Campaign, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var c model.Campaign
	if err := res.Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Delete removes a campaign by ID. Returns true if it existed.
func (s *CampaignStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.col.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}

// AddSession appends a session to the campaign's embedded sessions array and updates updatedAt.
func (s *CampaignStore) AddSession(ctx context.Context, campaignID string, sess model.Session) (*model.Session, error) {
	now := time.Now().UTC()
	sess.UpdatedAt = now
	res := s.col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": campaignID},
		bson.M{
			"$push": bson.M{"sessions": sess},
			"$set":  bson.M{"updatedAt": now},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var c model.Campaign
	if err := res.Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	// Return the appended session
	for _, s := range c.Sessions {
		if s.ID == sess.ID {
			return &s, nil
		}
	}
	return nil, nil
}

// UpdateSession updates fields on an embedded session using a fetch-modify-replace approach.
// Bumps both the session's updatedAt and the campaign's updatedAt.
func (s *CampaignStore) UpdateSession(ctx context.Context, campaignID, sessionID string, fields bson.M) (*model.Session, error) {
	campaign, err := s.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	var updatedSession *model.Session
	for i := range campaign.Sessions {
		if campaign.Sessions[i].ID == sessionID {
			if v, ok := fields["name"].(string); ok {
				campaign.Sessions[i].Name = v
			}
			if v, ok := fields["description"].(string); ok {
				campaign.Sessions[i].Description = v
			}
			campaign.Sessions[i].UpdatedAt = now
			sess := campaign.Sessions[i]
			updatedSession = &sess
			break
		}
	}
	if updatedSession == nil {
		return nil, nil // session not found
	}
	_, err = s.col.UpdateOne(ctx, bson.M{"_id": campaignID}, bson.M{
		"$set": bson.M{"sessions": campaign.Sessions, "updatedAt": now},
	})
	if err != nil {
		return nil, err
	}
	return updatedSession, nil
}

// DeleteSession removes an embedded session from a campaign and bumps updatedAt.
func (s *CampaignStore) DeleteSession(ctx context.Context, campaignID, sessionID string) (bool, error) {
	now := time.Now().UTC()
	res, err := s.col.UpdateOne(
		ctx,
		bson.M{"_id": campaignID},
		bson.M{
			"$pull": bson.M{"sessions": bson.M{"id": sessionID}},
			"$set":  bson.M{"updatedAt": now},
		},
	)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount > 0, nil
}

func (s *CampaignStore) find(ctx context.Context, filter bson.M) ([]model.Campaign, error) {
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var campaigns []model.Campaign
	if err := cursor.All(ctx, &campaigns); err != nil {
		return nil, err
	}
	if campaigns == nil {
		campaigns = []model.Campaign{}
	}
	return campaigns, nil
}

// Note: use options.FindOneAndUpdate() inline at each call site.
// In mongo-driver v2, options.FindOneAndUpdate() returns *FindOneAndUpdateOptionsBuilder
// which implements options.Lister[FindOneAndUpdateOptions] — the type accepted by
// Collection.FindOneAndUpdate and siblings. Do NOT store it as *FindOneAndUpdateOptions.
```

> **Note:** The `options` package from `go.mongodb.org/mongo-driver/v2/mongo/options` must be imported. The helper functions at the bottom avoid aliasing issues — rename or inline them as needed.

- [ ] **Step 2: Commit**

```bash
git add internal/store/campaign.go
git commit -m "feat: add campaign store (includes embedded session operations)"
```

---

### Task 14: Campaign handler

**Files:**
- Create: `internal/handler/campaign.go`

- [ ] **Step 1: Create `internal/handler/campaign.go`**

```go
package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// CampaignHandler handles all campaign CRUD endpoints.
type CampaignHandler struct {
	campaigns *store.CampaignStore
	players   *store.PlayerStore // needed for player-role campaign visibility
}

// NewCampaignHandler creates a CampaignHandler.
func NewCampaignHandler(campaigns *store.CampaignStore, players *store.PlayerStore) *CampaignHandler {
	return &CampaignHandler{campaigns: campaigns, players: players}
}

// List handles GET /api/campaigns.
func (h *CampaignHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := middleware.RoleFromContext(ctx)
	userID := middleware.UserIDFromContext(ctx)

	var campaigns []model.Campaign
	var err error

	if role == model.RoleAdmin {
		campaigns, err = h.campaigns.ListAll(ctx)
	} else {
		// Player: find campaigns where they have a character
		campaignIDs, e := h.players.CampaignIDsForUser(ctx, userID)
		if e != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		campaigns, err = h.campaigns.ListByIDs(ctx, campaignIDs)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, campaigns)
}

// Get handles GET /api/campaigns/:id.
func (h *CampaignHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	campaign, err := h.campaigns.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	// Player role: verify they have a character in this campaign
	if middleware.RoleFromContext(r.Context()) == model.RolePlayer {
		userID := middleware.UserIDFromContext(r.Context())
		ok, err := h.players.UserHasPlayerInCampaign(r.Context(), userID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
			return
		}
	}
	writeJSON(w, http.StatusOK, campaign)
}

// Create handles POST /api/campaigns (admin only — enforced by middleware).
func (h *CampaignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		ThemeImage  string  `json:"themeImage"`
		MapImageURI *string `json:"mapImageUri"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	campaign := &model.Campaign{
		ID:          uuid.NewString(),
		CreatedBy:   middleware.UserIDFromContext(r.Context()),
		Name:        req.Name,
		ThemeImage:  req.ThemeImage,
		MapImageURI: req.MapImageURI,
		MapPins:     []model.MapPin{},
		Sessions:    []model.Session{},
		UpdatedAt:   time.Now().UTC(),
	}
	if err := h.campaigns.Create(r.Context(), campaign); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, campaign)
}

// Update handles PATCH /api/campaigns/:id (admin only — enforced by middleware).
func (h *CampaignHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Only allow mutable fields
	allowed := map[string]bool{"name": true, "themeImage": true, "mapImageUri": true, "mapPins": true}
	fields := bson.M{}
	for k, v := range req {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.campaigns.Update(r.Context(), id, fields)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:id (admin only — enforced by middleware).
// Cascade: deletes all players in the campaign, then their inventory and spells.
func (h *CampaignHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Get all player IDs in this campaign for cascade
	playerIDs, err := h.players.IDsForCampaign(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	// Cascade via PlayerStore (deletes players + their inventory + spells)
	if err := h.players.DeleteByIDs(ctx, playerIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	found, err := h.campaigns.Delete(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/campaign.go
git commit -m "feat: add campaign handler"
```

---

### Task 15: Session handler

**Files:**
- Create: `internal/handler/session.go`

- [ ] **Step 1: Create `internal/handler/session.go`**

```go
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// SessionHandler handles session sub-resource operations on campaigns.
type SessionHandler struct {
	campaigns *store.CampaignStore
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(campaigns *store.CampaignStore) *SessionHandler {
	return &SessionHandler{campaigns: campaigns}
}

// Create handles POST /api/campaigns/:campaignId/sessions (admin only).
func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")

	// Verify campaign exists
	campaign, err := h.campaigns.GetByID(r.Context(), campaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	sess := model.Session{
		ID:          uuid.NewString(),
		Name:        req.Name,
		Description: req.Description,
	}
	created, err := h.campaigns.AddSession(r.Context(), campaignID, sess)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if created == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// Update handles PATCH /api/campaigns/:campaignId/sessions/:sessionId (admin only).
func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	allowed := map[string]bool{"name": true, "description": true}
	fields := bson.M{}
	for k, v := range req {
		if allowed[k] {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.campaigns.UpdateSession(r.Context(), campaignID, sessionID, fields)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/campaigns/:campaignId/sessions/:sessionId (admin only).
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	sessionID := chi.URLParam(r, "sessionId")

	found, err := h.campaigns.DeleteSession(r.Context(), campaignID, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "session not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/session.go
git commit -m "feat: add session handler"
```

---

### Task 16: Wire campaign and session routes

**Files:**
- Modify: `cmd/server/routes.go`

- [ ] **Step 1: Update `cmd/server/routes.go`** to add campaign and session routes inside the protected group

```go
// NOTE: Do not add campaign/session routes to routes.go yet at this step.
// Campaign handler requires playerStore (defined in Chunk 4) to avoid nil panics.
// All routes are wired together in Task 30 (final routes.go).
// This task only confirms the session and campaign handlers compile correctly.
```

Verify the handlers compile in isolation:

```bash
go build ./internal/handler/... ./internal/store/... ./internal/model/...
```

- [ ] **Step 2: Verify compiles**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/routes.go
git commit -m "feat: wire campaign and session routes"
```

---

## Chunk 4: Players

### Task 17: Player model

**Files:**
- Create: `internal/model/player.go`

- [ ] **Step 1: Create `internal/model/player.go`**

```go
package model

import "time"

// SpellSlot holds the max and used count for a spell slot level.
type SpellSlot struct {
	Max  int `bson:"max"  json:"max"`
	Used int `bson:"used" json:"used"`
}

// Note: spell slot level keys are strings "1"–"9" in the map.

// Player represents a D&D character sheet stored in the "players" collection.
type Player struct {
	ID           string `bson:"_id"          json:"id"`
	CampaignID   string `bson:"campaignId"   json:"campaignId"`
	LinkedUserID string `bson:"linkedUserId" json:"-"` // internal access-control field; not exposed in API responses

	Name            string  `bson:"name"            json:"name"`
	ClassName       *string `bson:"className"       json:"className"`
	Level           int     `bson:"level"           json:"level"`
	Race            string  `bson:"race,omitempty"  json:"race,omitempty"`
	Notes           string  `bson:"notes"           json:"notes"`
	AvatarURI       string  `bson:"avatarUri,omitempty" json:"avatarUri,omitempty"`
	BackgroundStory string  `bson:"backgroundStory" json:"backgroundStory"`
	Alignment       *string `bson:"alignment"       json:"alignment"`
	SpeciesOrRegion *string `bson:"speciesOrRegion" json:"speciesOrRegion"`
	Subclass        *string `bson:"subclass"        json:"subclass"`
	Region          *string `bson:"region"          json:"region"`
	Size            string  `bson:"size"            json:"size"`

	// Combat stats
	CurrentHP          int `bson:"currentHp"          json:"currentHp"`
	MaxHP              int `bson:"maxHp"              json:"maxHp"`
	TempHP             int `bson:"tempHp"             json:"tempHp"`
	AC                 int `bson:"ac"                 json:"ac"`
	Speed              int `bson:"speed"              json:"speed"`
	InitiativeBonus    int `bson:"initiativeBonus"    json:"initiativeBonus"`
	ProficiencyBonus   int `bson:"proficiencyBonus"   json:"proficiencyBonus"`
	DeathSaveSuccesses int `bson:"deathSaveSuccesses" json:"deathSaveSuccesses"`
	DeathSaveFailures  int `bson:"deathSaveFailures"  json:"deathSaveFailures"`

	// Ability scores — keys: STR, DEX, CON, INT, WIS, CHA
	AbilityScores             map[string]int `bson:"abilityScores"             json:"abilityScores"`
	AbilityTemporaryModifiers map[string]int `bson:"abilityTemporaryModifiers" json:"abilityTemporaryModifiers"`
	SkillTemporaryModifiers   map[string]int `bson:"skillTemporaryModifiers"   json:"skillTemporaryModifiers"`

	// Proficiencies
	ProficientSavingThrows []string `bson:"proficientSavingThrows" json:"proficientSavingThrows"`
	ProficientSkills       []string `bson:"proficientSkills"       json:"proficientSkills"`
	ExpertiseSkills        []string `bson:"expertiseSkills"        json:"expertiseSkills"`
	FeaturesAndFeats       []string `bson:"featuresAndFeats"       json:"featuresAndFeats"`

	// Spell slots — keys "1"–"9"
	SpellSlots map[string]SpellSlot `bson:"spellSlots" json:"spellSlots"`

	// Conditions — e.g. {"poisoned": true}
	Conditions map[string]bool `bson:"conditions" json:"conditions"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// DefaultPlayer returns a new Player with sensible D&D 5e defaults.
// All maps and slices are initialized (never nil) to ensure clean JSON serialisation.
func DefaultPlayer(id, campaignID, linkedUserID, name string, level int) *Player {
	return &Player{
		ID:           id,
		CampaignID:   campaignID,
		LinkedUserID: linkedUserID,
		Name:         name,
		Level:        level,
		Size:         "Medium",
		CurrentHP:    10,
		MaxHP:        10,
		TempHP:       0,
		AC:           10,
		Speed:        30,
		ProficiencyBonus: 2,
		AbilityScores: map[string]int{
			"STR": 10, "DEX": 10, "CON": 10,
			"INT": 10, "WIS": 10, "CHA": 10,
		},
		AbilityTemporaryModifiers: make(map[string]int),
		SkillTemporaryModifiers:   make(map[string]int),
		ProficientSavingThrows:    []string{},
		ProficientSkills:          []string{},
		ExpertiseSkills:           []string{},
		FeaturesAndFeats:          []string{},
		// All 9 spell slot levels pre-populated so client never sees null
		SpellSlots: map[string]SpellSlot{
			"1": {}, "2": {}, "3": {}, "4": {}, "5": {},
			"6": {}, "7": {}, "8": {}, "9": {},
		},
		Conditions: make(map[string]bool),
		UpdatedAt:  time.Now().UTC(),
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/player.go
git commit -m "feat: add player model with defaults"
```

---

### Task 18: Player store

**Files:**
- Create: `internal/store/player.go`

- [ ] **Step 1: Create `internal/store/player.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// PlayerStore handles persistence for player characters.
type PlayerStore struct {
	col      *mongo.Collection
	invStore *InventoryStore // set after inventory store is created (wired in routes)
	spStore  *SpellStore     // set after spell store is created (wired in routes)
}

// NewPlayerStore creates a PlayerStore and ensures required indexes.
func NewPlayerStore(db *DB) *PlayerStore {
	col := db.Collection("players")
	col.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
		{Keys: bson.D{{Key: "linkedUserId", Value: 1}}},
	})
	return &PlayerStore{col: col}
}

// SetInventoryStore links the inventory store for cascade deletes.
func (s *PlayerStore) SetInventoryStore(inv *InventoryStore) { s.invStore = inv }

// SetSpellStore links the spell store for cascade deletes.
func (s *PlayerStore) SetSpellStore(sp *SpellStore) { s.spStore = sp }

// Create inserts a new player.
func (s *PlayerStore) Create(ctx context.Context, p *model.Player) error {
	_, err := s.col.InsertOne(ctx, p)
	return err
}

// Get returns a player by ID for the given role/userID.
// Admin: no ownership filter. Player: filters by linkedUserId.
func (s *PlayerStore) Get(ctx context.Context, id, userID string, isAdmin bool) (*model.Player, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	var p model.Player
	err := s.col.FindOne(ctx, filter).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListForCampaign returns players in a campaign.
// Admin: all players. Player: only the player whose linkedUserId matches.
func (s *PlayerStore) ListForCampaign(ctx context.Context, campaignID, userID string, isAdmin bool) ([]model.Player, error) {
	filter := bson.M{"campaignId": campaignID}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var players []model.Player
	if err := cursor.All(ctx, &players); err != nil {
		return nil, err
	}
	if players == nil {
		players = []model.Player{}
	}
	return players, nil
}

// Update applies a partial $set update and returns the updated player.
// Protected fields (campaignId, linkedUserId) must be stripped by the handler before calling.
// Admin: no ownership filter. Player: filters by linkedUserId.
func (s *PlayerStore) Update(ctx context.Context, id, userID string, isAdmin bool, fields bson.M) (*model.Player, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var p model.Player
	if err := res.Decode(&p); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Delete removes a player and cascades to their inventory and spells.
func (s *PlayerStore) Delete(ctx context.Context, id, userID string, isAdmin bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	if err != nil || res.DeletedCount == 0 {
		return false, err
	}
	// Cascade
	if s.invStore != nil {
		_ = s.invStore.DeleteByPlayerID(ctx, id)
	}
	if s.spStore != nil {
		_ = s.spStore.DeleteByPlayerID(ctx, id)
	}
	return true, nil
}

// DeleteByIDs deletes multiple players (used by campaign cascade delete).
func (s *PlayerStore) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if s.invStore != nil {
			_ = s.invStore.DeleteByPlayerID(ctx, id)
		}
		if s.spStore != nil {
			_ = s.spStore.DeleteByPlayerID(ctx, id)
		}
	}
	_, err := s.col.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}

// IDsForCampaign returns all player IDs in a campaign (used by campaign cascade delete).
func (s *PlayerStore) IDsForCampaign(ctx context.Context, campaignID string) ([]string, error) {
	cursor, err := s.col.Find(ctx, bson.M{"campaignId": campaignID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var results []struct {
		ID string `bson:"_id"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	return ids, nil
}

// CampaignIDsForUser returns all campaignIds for players linked to the given user.
func (s *PlayerStore) CampaignIDsForUser(ctx context.Context, userID string) ([]string, error) {
	cursor, err := s.col.Find(
		ctx,
		bson.M{"linkedUserId": userID},
		options.Find().SetProjection(bson.M{"campaignId": 1}),
	)
	if err != nil {
		return nil, err
	}
	var results []struct {
		CampaignID string `bson:"campaignId"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.CampaignID
	}
	return ids, nil
}

// UserHasPlayerInCampaign returns true if the user has a character in the given campaign.
func (s *PlayerStore) UserHasPlayerInCampaign(ctx context.Context, userID, campaignID string) (bool, error) {
	count, err := s.col.CountDocuments(ctx, bson.M{"campaignId": campaignID, "linkedUserId": userID})
	return count > 0, err
}
```

> **Note:** `InventoryStore` and `SpellStore` are referenced here but defined in later tasks. The file will not compile until those types exist — wire them together in the routes file in Task 21.

- [ ] **Step 2: Commit**

```bash
git add internal/store/player.go
git commit -m "feat: add player store"
```

---

### Task 19: Player handler

**Files:**
- Create: `internal/handler/player.go`

- [ ] **Step 1: Create `internal/handler/player.go`**

```go
package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// PlayerHandler handles all player CRUD endpoints.
// campaigns store is used only for campaign-existence validation on player Create.
type PlayerHandler struct {
	players   *store.PlayerStore
	campaigns *store.CampaignStore
}

// NewPlayerHandler creates a PlayerHandler.
func NewPlayerHandler(players *store.PlayerStore, campaigns *store.CampaignStore) *PlayerHandler {
	return &PlayerHandler{players: players, campaigns: campaigns}
}

// ListForCampaign handles GET /api/campaigns/:campaignId/players.
func (h *PlayerHandler) ListForCampaign(w http.ResponseWriter, r *http.Request) {
	campaignID := chi.URLParam(r, "campaignId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	players, err := h.players.ListForCampaign(r.Context(), campaignID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

// Get handles GET /api/players/:playerId.
func (h *PlayerHandler) Get(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	player, err := h.players.Get(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, player)
}

// Create handles POST /api/players (admin only — enforced by middleware).
func (h *PlayerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CampaignID   string `json:"campaignId"`
		Name         string `json:"name"`
		ClassName    string `json:"className"`
		Level        int    `json:"level"`
		Race         string `json:"race"`
		LinkedUserID string `json:"linkedUserId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.CampaignID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "campaignId and name are required", "BAD_REQUEST")
		return
	}
	// Verify campaign exists before creating the player
	campaign, err := h.campaigns.GetByID(r.Context(), req.CampaignID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "campaign not found", "NOT_FOUND")
		return
	}
	if req.Level <= 0 {
		req.Level = 1
	}

	player := model.DefaultPlayer(uuid.NewString(), req.CampaignID, req.LinkedUserID, req.Name, req.Level)
	if req.ClassName != "" {
		player.ClassName = &req.ClassName
	}
	if req.Race != "" {
		player.Race = req.Race
	}
	player.UpdatedAt = time.Now().UTC()

	if err := h.players.Create(r.Context(), player); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, player)
}

// Update handles PATCH /api/players/:playerId.
// Players can only update their own character; admins can update any.
// Protected fields (campaignId, linkedUserId) are stripped before applying.
func (h *PlayerHandler) Update(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	// Strip protected fields
	delete(req, "campaignId")
	delete(req, "linkedUserId")
	delete(req, "_id")
	delete(req, "id")

	// Validate death save bounds
	if v, ok := req["deathSaveSuccesses"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveSuccesses must be 0-3", "BAD_REQUEST")
			return
		}
	}
	if v, ok := req["deathSaveFailures"]; ok {
		if n, ok := toInt(v); !ok || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "deathSaveFailures must be 0-3", "BAD_REQUEST")
			return
		}
	}

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, isAdmin, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/players/:playerId (admin only — enforced by middleware).
func (h *PlayerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	found, err := h.players.Delete(r.Context(), playerID, "", true) // admin: isAdmin=true
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// toInt is defined in internal/handler/util.go
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/player.go
git commit -m "feat: add player handler"
```

---

## Chunk 5: Inventory

### Task 20: Shared handler utilities

**Files:**
- Create: `internal/handler/util.go`

- [ ] **Step 1: Create `internal/handler/util.go`**

```go
package handler

// toInt converts a JSON-decoded number (float64) or int to int.
// Returns (0, false) if the value is neither type.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/util.go
git commit -m "feat: add shared handler utilities"
```

---

### Task 21: Inventory model

**Files:**
- Create: `internal/model/inventory.go`

- [ ] **Step 1: Create `internal/model/inventory.go`**

```go
package model

import "time"

// InventoryItem is stored in the "inventory" collection.
type InventoryItem struct {
	ID           string    `bson:"_id"          json:"id"`
	PlayerID     string    `bson:"playerId"     json:"playerId"`
	LinkedUserID string    `bson:"linkedUserId" json:"linkedUserId"` // denormalised from player

	Name     string   `bson:"name"     json:"name"`
	Quantity int      `bson:"quantity" json:"quantity"`
	Category string   `bson:"category" json:"category"`
	Tags     []string `bson:"tags"     json:"tags"`
	Notes    string   `bson:"notes,omitempty"    json:"notes,omitempty"`
	ImageURI string   `bson:"imageUri,omitempty" json:"imageUri,omitempty"`

	// Weapon fields
	Damage     string   `bson:"damage,omitempty"     json:"damage,omitempty"`
	DamageType string   `bson:"damageType,omitempty" json:"damageType,omitempty"`
	WeaponType string   `bson:"weaponType,omitempty" json:"weaponType,omitempty"`
	Properties []string `bson:"properties,omitempty" json:"properties,omitempty"`

	// Armor fields
	ArmorClass          *int    `bson:"armorClass,omitempty"          json:"armorClass,omitempty"`
	ArmorBonus          *int    `bson:"armorBonus,omitempty"          json:"armorBonus,omitempty"`
	ShieldBonus         *int    `bson:"shieldBonus,omitempty"         json:"shieldBonus,omitempty"`
	ArmorType           string  `bson:"armorType,omitempty"           json:"armorType,omitempty"`
	StrengthRequirement *int    `bson:"strengthRequirement,omitempty" json:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool   `bson:"stealthDisadvantage,omitempty" json:"stealthDisadvantage,omitempty"`

	// Magic item fields
	CompatibleWith *string  `bson:"compatibleWith,omitempty" json:"compatibleWith,omitempty"`
	EffectSummary  string   `bson:"effectSummary,omitempty"  json:"effectSummary,omitempty"`
	Value          *float64 `bson:"value,omitempty"          json:"value,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/inventory.go
git commit -m "feat: add inventory item model"
```

---

### Task 21: Inventory store

**Files:**
- Create: `internal/store/inventory.go`

- [ ] **Step 1: Create `internal/store/inventory.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// InventoryStore handles persistence for inventory items.
type InventoryStore struct {
	col *mongo.Collection
}

// NewInventoryStore creates an InventoryStore and ensures the playerId index.
func NewInventoryStore(db *DB) *InventoryStore {
	col := db.Collection("inventory")
	col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "playerId", Value: 1}},
	})
	return &InventoryStore{col: col}
}

// ListForPlayer returns all inventory items for a player.
// Admin: no linkedUserId filter. Player: requires linkedUserId match.
func (s *InventoryStore) ListForPlayer(ctx context.Context, playerID, userID string, isAdmin bool) ([]model.InventoryItem, error) {
	filter := bson.M{"playerId": playerID}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var items []model.InventoryItem
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.InventoryItem{}
	}
	return items, nil
}

// Create inserts a new inventory item.
func (s *InventoryStore) Create(ctx context.Context, item *model.InventoryItem) error {
	_, err := s.col.InsertOne(ctx, item)
	return err
}

// Update applies a partial $set update. Admin: no linkedUserId filter. Player: requires match.
func (s *InventoryStore) Update(ctx context.Context, id, userID string, isAdmin bool, fields bson.M) (*model.InventoryItem, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var item model.InventoryItem
	if err := res.Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// Delete removes an inventory item by ID.
func (s *InventoryStore) Delete(ctx context.Context, id, userID string, isAdmin bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	return res.DeletedCount > 0, err
}

// DeleteByPlayerID removes all inventory items for a player (cascade delete).
func (s *InventoryStore) DeleteByPlayerID(ctx context.Context, playerID string) error {
	_, err := s.col.DeleteMany(ctx, bson.M{"playerId": playerID})
	return err
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/inventory.go
git commit -m "feat: add inventory store"
```

---

### Task 22: Inventory handler

**Files:**
- Create: `internal/handler/inventory.go`

- [ ] **Step 1: Create `internal/handler/inventory.go`**

```go
package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// InventoryHandler handles inventory item endpoints.
// Note: toInt helper lives in internal/handler/util.go
type InventoryHandler struct {
	inventory *store.InventoryStore
	players   *store.PlayerStore
}

// NewInventoryHandler creates an InventoryHandler.
func NewInventoryHandler(inventory *store.InventoryStore, players *store.PlayerStore) *InventoryHandler {
	return &InventoryHandler{inventory: inventory, players: players}
}

// List handles GET /api/players/:playerId/inventory.
func (h *InventoryHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	items, err := h.inventory.ListForPlayer(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// Create handles POST /api/players/:playerId/inventory.
func (h *InventoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	// Resolve the player to get linkedUserId for denormalization
	player, err := h.players.Get(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Quantity    int      `json:"quantity"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
		Notes       string   `json:"notes"`
		Damage      string   `json:"damage"`
		DamageType  string   `json:"damageType"`
		Properties  []string `json:"properties"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	// linkedUserId is denormalised from the player for ownership checks.
	// It may be empty if the player was created without a linkedUserId (admin-only character).
	// An empty linkedUserId means no player-role user can access this item — admin-only.
	item := &model.InventoryItem{
		ID:           uuid.NewString(),
		PlayerID:     playerID,
		LinkedUserID: player.LinkedUserID,
		Name:         req.Name,
		Quantity:     req.Quantity,
		Category:     req.Category,
		Tags:         req.Tags,
		Notes:        req.Notes,
		Damage:       req.Damage,
		DamageType:   req.DamageType,
		Properties:   req.Properties,
		UpdatedAt:    time.Now().UTC(),
	}
	if err := h.inventory.Create(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// Update handles PATCH /api/inventory/:itemId.
func (h *InventoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	// Strip protected fields
	delete(req, "_id")
	delete(req, "id")
	delete(req, "playerId")
	delete(req, "linkedUserId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.inventory.Update(r.Context(), itemID, userID, isAdmin, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/inventory/:itemId.
// Admin-only access is enforced by RequireRole("admin") middleware in routes.go.
// The store receives isAdmin=true because this handler is only reachable by admins.
func (h *InventoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemId")
	found, err := h.inventory.Delete(r.Context(), itemID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "inventory item not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/inventory.go
git commit -m "feat: add inventory handler"
```

---

## Chunk 6: Spells

### Task 23: Spell model

**Files:**
- Create: `internal/model/spell.go`

- [ ] **Step 1: Create `internal/model/spell.go`**

```go
package model

import "time"

// Spell is a known spell stored in the "spells" collection.
type Spell struct {
	ID           string    `bson:"_id"          json:"id"`
	PlayerID     string    `bson:"playerId"     json:"playerId"`
	LinkedUserID string    `bson:"linkedUserId" json:"linkedUserId"` // denormalised from player

	Name        string   `bson:"name"               json:"name"`
	Level       int      `bson:"level"              json:"level"` // 0 = cantrip
	School      string   `bson:"school,omitempty"   json:"school,omitempty"`
	CastingTime string   `bson:"castingTime,omitempty" json:"castingTime,omitempty"`
	Range       string   `bson:"range,omitempty"    json:"range,omitempty"`
	Components  []string `bson:"components,omitempty" json:"components,omitempty"`
	Material    string   `bson:"material,omitempty" json:"material,omitempty"`
	Duration    string   `bson:"duration,omitempty" json:"duration,omitempty"`
	Description string   `bson:"description,omitempty" json:"description,omitempty"`
	IsPrepared  bool     `bson:"isPrepared"         json:"isPrepared"`
	IsRitual    bool     `bson:"isRitual,omitempty" json:"isRitual,omitempty"`
	Source      string   `bson:"source,omitempty"   json:"source,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/spell.go
git commit -m "feat: add spell model"
```

---

### Task 24: Spell store

**Files:**
- Create: `internal/store/spell.go`

- [ ] **Step 1: Create `internal/store/spell.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// SpellStore handles persistence for player spells.
type SpellStore struct {
	col *mongo.Collection
}

// NewSpellStore creates a SpellStore and ensures the playerId index.
func NewSpellStore(db *DB) *SpellStore {
	col := db.Collection("spells")
	col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "playerId", Value: 1}},
	})
	return &SpellStore{col: col}
}

// ListForPlayer returns all spells for a player.
func (s *SpellStore) ListForPlayer(ctx context.Context, playerID, userID string, isAdmin bool) ([]model.Spell, error) {
	filter := bson.M{"playerId": playerID}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	cursor, err := s.col.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var spells []model.Spell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.Spell{}
	}
	return spells, nil
}

// Create inserts a new spell.
func (s *SpellStore) Create(ctx context.Context, spell *model.Spell) error {
	_, err := s.col.InsertOne(ctx, spell)
	return err
}

// Update applies a partial $set update to a spell.
func (s *SpellStore) Update(ctx context.Context, id, userID string, isAdmin bool, fields bson.M) (*model.Spell, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	fields["updatedAt"] = time.Now().UTC()
	res := s.col.FindOneAndUpdate(
		ctx,
		filter,
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var spell model.Spell
	if err := res.Decode(&spell); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &spell, nil
}

// Delete removes a spell by ID.
func (s *SpellStore) Delete(ctx context.Context, id, userID string, isAdmin bool) (bool, error) {
	filter := bson.M{"_id": id}
	if !isAdmin {
		filter["linkedUserId"] = userID
	}
	res, err := s.col.DeleteOne(ctx, filter)
	return res.DeletedCount > 0, err
}

// DeleteByPlayerID removes all spells for a player (cascade delete).
func (s *SpellStore) DeleteByPlayerID(ctx context.Context, playerID string) error {
	_, err := s.col.DeleteMany(ctx, bson.M{"playerId": playerID})
	return err
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/spell.go
git commit -m "feat: add spell store"
```

---

### Task 25: Spell handler

**Files:**
- Create: `internal/handler/spell.go`

- [ ] **Step 1: Create `internal/handler/spell.go`**

```go
package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// SpellHandler handles spell and spell-slot endpoints.
type SpellHandler struct {
	spells  *store.SpellStore
	players *store.PlayerStore
}

// NewSpellHandler creates a SpellHandler.
func NewSpellHandler(spells *store.SpellStore, players *store.PlayerStore) *SpellHandler {
	return &SpellHandler{spells: spells, players: players}
}

// List handles GET /api/players/:playerId/spells.
func (h *SpellHandler) List(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	spells, err := h.spells.ListForPlayer(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, spells)
}

// Create handles POST /api/players/:playerId/spells.
func (h *SpellHandler) Create(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	player, err := h.players.Get(r.Context(), playerID, userID, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if player == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Level       int      `json:"level"`
		School      string   `json:"school"`
		CastingTime string   `json:"castingTime"`
		Range       string   `json:"range"`
		Components  []string `json:"components"`
		Material    string   `json:"material"`
		Duration    string   `json:"duration"`
		Description string   `json:"description"`
		IsPrepared  bool     `json:"isPrepared"`
		IsRitual    bool     `json:"isRitual"`
		Source      string   `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}

	spell := &model.Spell{
		ID:           uuid.NewString(),
		PlayerID:     playerID,
		LinkedUserID: player.LinkedUserID,
		Name:         req.Name,
		Level:        req.Level,
		School:       req.School,
		CastingTime:  req.CastingTime,
		Range:        req.Range,
		Components:   req.Components,
		Material:     req.Material,
		Duration:     req.Duration,
		Description:  req.Description,
		IsPrepared:   req.IsPrepared,
		IsRitual:     req.IsRitual,
		Source:       req.Source,
		UpdatedAt:    time.Now().UTC(),
	}
	if err := h.spells.Create(r.Context(), spell); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, spell)
}

// Update handles PATCH /api/spells/:spellId.
func (h *SpellHandler) Update(w http.ResponseWriter, r *http.Request) {
	spellID := chi.URLParam(r, "spellId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	delete(req, "playerId")
	delete(req, "linkedUserId")

	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}

	updated, err := h.spells.Update(r.Context(), spellID, userID, isAdmin, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/spells/:spellId (admin only — enforced by middleware).
func (h *SpellHandler) Delete(w http.ResponseWriter, r *http.Request) {
	spellID := chi.URLParam(r, "spellId")
	found, err := h.spells.Delete(r.Context(), spellID, "", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateSpellSlots handles PUT /api/players/:playerId/spell-slots.
// Replaces the entire spell slot map atomically. Returns the full updated player object.
func (h *SpellHandler) UpdateSpellSlots(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerId")
	userID := middleware.UserIDFromContext(r.Context())
	isAdmin := middleware.RoleFromContext(r.Context()) == model.RoleAdmin

	var slots map[string]model.SpellSlot
	if err := decodeJSON(r, &slots); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}

	updated, err := h.players.Update(r.Context(), playerID, userID, isAdmin, bson.M{"spellSlots": slots})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "player not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/spell.go
git commit -m "feat: add spell handler and spell-slot PUT endpoint"
```

---

## Chunk 7: Arsenal

### Task 26: Arsenal model

**Files:**
- Create: `internal/model/arsenal.go`

- [ ] **Step 1: Create `internal/model/arsenal.go`**

```go
package model

import "time"

// ArsenalSpell is a reference spell in the global catalog ("arsenal_spells" collection).
// This is an independent struct — NOT an embedding of model.Spell.
// It does not have PlayerID or LinkedUserID fields to avoid BSON field leakage.
type ArsenalSpell struct {
	ID          string    `bson:"_id"               json:"id"`
	Name        string    `bson:"name"              json:"name"`
	Level       int       `bson:"level"             json:"level"`
	School      string    `bson:"school,omitempty"  json:"school,omitempty"`
	CastingTime string    `bson:"castingTime,omitempty" json:"castingTime,omitempty"`
	Range       string    `bson:"range,omitempty"   json:"range,omitempty"`
	Components  []string  `bson:"components,omitempty" json:"components,omitempty"`
	Material    string    `bson:"material,omitempty" json:"material,omitempty"`
	Duration    string    `bson:"duration,omitempty" json:"duration,omitempty"`
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	IsRitual    bool      `bson:"isRitual,omitempty" json:"isRitual,omitempty"`
	Source      string    `bson:"source,omitempty"  json:"source,omitempty"`
	UpdatedAt   time.Time `bson:"updatedAt"         json:"updatedAt"`
}

// ArsenalEquipment is a reference item in the global catalog ("arsenal_equipment" collection).
// This is an independent struct — NOT an embedding of model.InventoryItem.
// It does not have PlayerID or LinkedUserID fields to avoid BSON field leakage.
type ArsenalEquipment struct {
	ID       string   `bson:"_id"          json:"id"`
	Name     string   `bson:"name"         json:"name"`
	Category string   `bson:"category"     json:"category"`
	Tags     []string `bson:"tags"         json:"tags"`
	Notes    string   `bson:"notes,omitempty"    json:"notes,omitempty"`
	ImageURI string   `bson:"imageUri,omitempty" json:"imageUri,omitempty"`

	Damage     string   `bson:"damage,omitempty"     json:"damage,omitempty"`
	DamageType string   `bson:"damageType,omitempty" json:"damageType,omitempty"`
	WeaponType string   `bson:"weaponType,omitempty" json:"weaponType,omitempty"`
	Properties []string `bson:"properties,omitempty" json:"properties,omitempty"`

	ArmorClass          *int    `bson:"armorClass,omitempty"          json:"armorClass,omitempty"`
	ArmorBonus          *int    `bson:"armorBonus,omitempty"          json:"armorBonus,omitempty"`
	ShieldBonus         *int    `bson:"shieldBonus,omitempty"         json:"shieldBonus,omitempty"`
	ArmorType           string  `bson:"armorType,omitempty"           json:"armorType,omitempty"`
	StrengthRequirement *int    `bson:"strengthRequirement,omitempty" json:"strengthRequirement,omitempty"`
	StealthDisadvantage *bool   `bson:"stealthDisadvantage,omitempty" json:"stealthDisadvantage,omitempty"`

	CompatibleWith *string  `bson:"compatibleWith,omitempty" json:"compatibleWith,omitempty"`
	EffectSummary  string   `bson:"effectSummary,omitempty"  json:"effectSummary,omitempty"`
	Value          *float64 `bson:"value,omitempty"          json:"value,omitempty"`

	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/arsenal.go
git commit -m "feat: add arsenal models"
```

---

### Task 27: Arsenal store

**Files:**
- Create: `internal/store/arsenal.go`

- [ ] **Step 1: Create `internal/store/arsenal.go`**

```go
package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/elad/rolebook-backend/internal/model"
)

// ArsenalStore handles the global spell and equipment catalog.
type ArsenalStore struct {
	spells    *mongo.Collection
	equipment *mongo.Collection
}

// NewArsenalStore creates an ArsenalStore for both catalog collections.
func NewArsenalStore(db *DB) *ArsenalStore {
	return &ArsenalStore{
		spells:    db.Collection("arsenal_spells"),
		equipment: db.Collection("arsenal_equipment"),
	}
}

// --- Spell catalog ---

func (s *ArsenalStore) ListSpells(ctx context.Context) ([]model.ArsenalSpell, error) {
	cursor, err := s.spells.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var spells []model.ArsenalSpell
	if err := cursor.All(ctx, &spells); err != nil {
		return nil, err
	}
	if spells == nil {
		spells = []model.ArsenalSpell{}
	}
	return spells, nil
}

func (s *ArsenalStore) CreateSpell(ctx context.Context, spell *model.ArsenalSpell) error {
	_, err := s.spells.InsertOne(ctx, spell)
	return err
}

func (s *ArsenalStore) UpdateSpell(ctx context.Context, id string, fields bson.M) (*model.ArsenalSpell, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.spells.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var spell model.ArsenalSpell
	if err := res.Decode(&spell); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &spell, nil
}

func (s *ArsenalStore) DeleteSpell(ctx context.Context, id string) (bool, error) {
	res, err := s.spells.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}

// --- Equipment catalog ---

func (s *ArsenalStore) ListEquipment(ctx context.Context) ([]model.ArsenalEquipment, error) {
	cursor, err := s.equipment.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var items []model.ArsenalEquipment
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.ArsenalEquipment{}
	}
	return items, nil
}

func (s *ArsenalStore) CreateEquipment(ctx context.Context, item *model.ArsenalEquipment) error {
	_, err := s.equipment.InsertOne(ctx, item)
	return err
}

func (s *ArsenalStore) UpdateEquipment(ctx context.Context, id string, fields bson.M) (*model.ArsenalEquipment, error) {
	fields["updatedAt"] = time.Now().UTC()
	res := s.equipment.FindOneAndUpdate(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var item model.ArsenalEquipment
	if err := res.Decode(&item); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (s *ArsenalStore) DeleteEquipment(ctx context.Context, id string) (bool, error) {
	res, err := s.equipment.DeleteOne(ctx, bson.M{"_id": id})
	return res.DeletedCount > 0, err
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/arsenal.go
git commit -m "feat: add arsenal store"
```

---

### Task 28: Arsenal handler

**Files:**
- Create: `internal/handler/arsenal.go`

- [ ] **Step 1: Create `internal/handler/arsenal.go`**

```go
package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

// ArsenalHandler handles the global spell and equipment catalog endpoints.
type ArsenalHandler struct {
	arsenal *store.ArsenalStore
}

// NewArsenalHandler creates an ArsenalHandler.
func NewArsenalHandler(arsenal *store.ArsenalStore) *ArsenalHandler {
	return &ArsenalHandler{arsenal: arsenal}
}

// ListSpells handles GET /api/arsenal/spells.
func (h *ArsenalHandler) ListSpells(w http.ResponseWriter, r *http.Request) {
	spells, err := h.arsenal.ListSpells(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, spells)
}

// CreateSpell handles POST /api/arsenal/spells (admin only).
func (h *ArsenalHandler) CreateSpell(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Level       int      `json:"level"`
		School      string   `json:"school"`
		CastingTime string   `json:"castingTime"`
		Range       string   `json:"range"`
		Components  []string `json:"components"`
		Material    string   `json:"material"`
		Duration    string   `json:"duration"`
		Description string   `json:"description"`
		IsRitual    bool     `json:"isRitual"`
		Source      string   `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	spell := &model.ArsenalSpell{
		ID: uuid.NewString(), Name: req.Name, Level: req.Level,
		School: req.School, CastingTime: req.CastingTime, Range: req.Range,
		Components: req.Components, Material: req.Material, Duration: req.Duration,
		Description: req.Description, IsRitual: req.IsRitual, Source: req.Source,
		UpdatedAt: time.Now().UTC(),
	}
	if err := h.arsenal.CreateSpell(r.Context(), spell); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, spell)
}

// UpdateSpell handles PATCH /api/arsenal/spells/:id (admin only).
func (h *ArsenalHandler) UpdateSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.arsenal.UpdateSpell(r.Context(), id, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteSpell handles DELETE /api/arsenal/spells/:id (admin only).
func (h *ArsenalHandler) DeleteSpell(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	found, err := h.arsenal.DeleteSpell(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "spell not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListEquipment handles GET /api/arsenal/equipment.
func (h *ArsenalHandler) ListEquipment(w http.ResponseWriter, r *http.Request) {
	items, err := h.arsenal.ListEquipment(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// CreateEquipment handles POST /api/arsenal/equipment (admin only).
func (h *ArsenalHandler) CreateEquipment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
		Notes    string   `json:"notes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "BAD_REQUEST")
		return
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	item := &model.ArsenalEquipment{
		ID: uuid.NewString(), Name: req.Name, Category: req.Category,
		Tags: req.Tags, Notes: req.Notes, UpdatedAt: time.Now().UTC(),
	}
	if err := h.arsenal.CreateEquipment(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// UpdateEquipment handles PATCH /api/arsenal/equipment/:id (admin only).
func (h *ArsenalHandler) UpdateEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req map[string]any
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST")
		return
	}
	delete(req, "_id")
	delete(req, "id")
	if len(req) == 0 {
		writeError(w, http.StatusBadRequest, "no valid fields to update", "BAD_REQUEST")
		return
	}
	updated, err := h.arsenal.UpdateEquipment(r.Context(), id, bson.M(req))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if updated == nil {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteEquipment handles DELETE /api/arsenal/equipment/:id (admin only).
func (h *ArsenalHandler) DeleteEquipment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	found, err := h.arsenal.DeleteEquipment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "equipment not found", "NOT_FOUND")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/arsenal.go
git commit -m "feat: add arsenal handler"
```

---

### Task 29: Wire all remaining routes (complete routes.go)

**Files:**
- Modify: `cmd/server/routes.go`

- [ ] **Step 1: Replace `cmd/server/routes.go` with the complete version**

```go
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elad/rolebook-backend/config"
	"github.com/elad/rolebook-backend/internal/handler"
	"github.com/elad/rolebook-backend/internal/middleware"
	"github.com/elad/rolebook-backend/internal/model"
	"github.com/elad/rolebook-backend/internal/store"
)

func registerRoutes(r *chi.Mux, cfg config.Config, db *store.DB) {
	// Stores
	userStore := store.NewUserStore(db)
	campaignStore := store.NewCampaignStore(db)
	playerStore := store.NewPlayerStore(db)
	inventoryStore := store.NewInventoryStore(db)
	spellStore := store.NewSpellStore(db)
	arsenalStore := store.NewArsenalStore(db)

	// Wire cascade dependencies
	playerStore.SetInventoryStore(inventoryStore)
	playerStore.SetSpellStore(spellStore)

	// Handlers
	authHandler := handler.NewAuthHandler(userStore, cfg.JWTSecret, cfg.AdminEmail)
	campaignHandler := handler.NewCampaignHandler(campaignStore, playerStore)
	sessionHandler := handler.NewSessionHandler(campaignStore)
	playerHandler := handler.NewPlayerHandler(playerStore, campaignStore)
	inventoryHandler := handler.NewInventoryHandler(inventoryStore, playerStore)
	spellHandler := handler.NewSpellHandler(spellStore, playerStore)
	arsenalHandler := handler.NewArsenalHandler(arsenalStore)

	adminOnly := middleware.RequireRole(model.RoleAdmin)

	r.Route("/api", func(r chi.Router) {
		// Public
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// Protected (JWT required)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Campaigns
			r.Get("/campaigns", campaignHandler.List)
			r.With(adminOnly).Post("/campaigns", campaignHandler.Create)
			r.Route("/campaigns/{id}", func(r chi.Router) {
				r.Get("/", campaignHandler.Get)
				r.With(adminOnly).Patch("/", campaignHandler.Update)
				r.With(adminOnly).Delete("/", campaignHandler.Delete)
			})

			// Sessions (admin only)
			r.Route("/campaigns/{campaignId}/sessions", func(r chi.Router) {
				r.With(adminOnly).Post("/", sessionHandler.Create)
				r.With(adminOnly).Patch("/{sessionId}", sessionHandler.Update)
				r.With(adminOnly).Delete("/{sessionId}", sessionHandler.Delete)
			})

			// Players
			r.Get("/campaigns/{campaignId}/players", playerHandler.ListForCampaign)
			r.With(adminOnly).Post("/players", playerHandler.Create)
			r.Route("/players/{playerId}", func(r chi.Router) {
				r.Get("/", playerHandler.Get)
				r.Patch("/", playerHandler.Update)
				r.With(adminOnly).Delete("/", playerHandler.Delete)
				// Inventory sub-resource
				r.Get("/inventory", inventoryHandler.List)
				r.Post("/inventory", inventoryHandler.Create)
				// Spells sub-resource
				r.Get("/spells", spellHandler.List)
				r.Post("/spells", spellHandler.Create)
				// Spell slots
				r.Put("/spell-slots", spellHandler.UpdateSpellSlots)
			})

			// Flat inventory routes
			r.Patch("/inventory/{itemId}", inventoryHandler.Update)
			r.With(adminOnly).Delete("/inventory/{itemId}", inventoryHandler.Delete)

			// Flat spell routes
			r.Patch("/spells/{spellId}", spellHandler.Update)
			r.With(adminOnly).Delete("/spells/{spellId}", spellHandler.Delete)

			// Arsenal
			r.Route("/arsenal/spells", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListSpells)
				r.With(adminOnly).Post("/", arsenalHandler.CreateSpell)
				r.With(adminOnly).Patch("/{id}", arsenalHandler.UpdateSpell)
				r.With(adminOnly).Delete("/{id}", arsenalHandler.DeleteSpell)
			})
			r.Route("/arsenal/equipment", func(r chi.Router) {
				r.Get("/", arsenalHandler.ListEquipment)
				r.With(adminOnly).Post("/", arsenalHandler.CreateEquipment)
				r.With(adminOnly).Patch("/{id}", arsenalHandler.UpdateEquipment)
				r.With(adminOnly).Delete("/{id}", arsenalHandler.DeleteEquipment)
			})
		})
	})

	// Health check (Railway uses this)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}
```

- [ ] **Step 2: Verify the full project compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/routes.go
git commit -m "feat: wire all routes — server is fully wired"
```

---

### Task 30: Final build and smoke check

- [ ] **Step 1: Run `go vet`**

```bash
go vet ./...
```

Expected: no output (no issues).

- [ ] **Step 2: Build the Docker image locally**

```bash
docker build -t rolebook-backend .
```

Expected: image builds successfully.

- [ ] **Step 3: Smoke-test the binary locally** (requires a MongoDB connection string)

```bash
MONGO_URI="<your-atlas-uri>" JWT_SECRET="dev-secret" ADMIN_EMAIL="admin@example.com" PORT=3000 ./server &
curl -s http://localhost:3000/health
# Expected: ok

curl -s -X POST http://localhost:3000/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"password123"}' | jq .
# Expected: { "token": "...", "userId": "...", "role": "admin" }
```

- [ ] **Step 4: Commit final state**

```bash
git add .
git commit -m "chore: verified build and smoke test pass"
```

---

## Campaign Store Import Fix

> **Important note for implementer:** In `internal/store/campaign.go`, the helper functions `mongoOptions()`, `mongoUpdateOptions()`, and `mongoFindOneAndUpdateOptions()` reference the `options` package from `go.mongodb.org/mongo-driver/v2/mongo/options`. Ensure this import is present. If the helper functions cause confusion, inline the `options.FindOneAndUpdate()` calls directly.

The `UpdateSession` method uses `mongo.ArrayFilters` — verify the exact API in the mongo-driver v2 docs if the compiler rejects it; the array filter syntax may differ slightly between minor versions.
