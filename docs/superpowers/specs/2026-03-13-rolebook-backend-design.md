# Rolebook Backend — Design Spec

**Date:** 2026-03-13
**Stack:** Go 1.26, MongoDB Atlas, Railway

---

## Overview

REST API backend for the D&D 5e Companion mobile app. The client is an offline-first React Native / Expo app that syncs data via JWT-authenticated REST endpoints. This spec covers architecture, data model, authentication, permissions, and deployment.

---

## Project Structure

```
rolebook-backend/
├── cmd/server/main.go           # Entry point: wires everything, starts HTTP server
├── config/config.go             # Loads env vars (PORT, MONGO_URI, JWT_SECRET, ADMIN_EMAIL)
├── internal/
│   ├── handler/
│   │   ├── auth.go              # POST /auth/register, POST /auth/login
│   │   ├── campaign.go          # CRUD /campaigns
│   │   ├── session.go           # CRUD /campaigns/:id/sessions
│   │   ├── player.go            # CRUD /players
│   │   ├── inventory.go         # CRUD /inventory + /players/:id/inventory
│   │   ├── spell.go             # CRUD /spells + spell-slots
│   │   └── arsenal.go           # CRUD /arsenal/spells + /arsenal/equipment
│   ├── middleware/
│   │   └── auth.go              # JWT Bearer validation, role enforcement helpers
│   ├── model/
│   │   ├── user.go
│   │   ├── campaign.go
│   │   ├── player.go
│   │   ├── inventory.go
│   │   ├── spell.go
│   │   └── arsenal.go
│   └── store/
│       ├── mongo.go             # DB client init, collection helpers
│       ├── campaign.go
│       ├── player.go
│       ├── inventory.go
│       ├── spell.go
│       └── arsenal.go
├── go.mod
└── Dockerfile
```

**Module name:** `github.com/elad/rolebook-backend`

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/go-chi/chi/v5` | HTTP router |
| `go.mongodb.org/mongo-driver/v2` | MongoDB driver |
| `github.com/golang-jwt/jwt/v5` | JWT signing/validation |
| `golang.org/x/crypto/bcrypt` | Password hashing |
| `github.com/google/uuid` | ID generation |

---

## Database Layout

**Database name:** `rolebook`

| Collection | Description |
|---|---|
| `users` | User accounts with roles |
| `campaigns` | Campaigns with embedded sessions |
| `players` | Player characters |
| `inventory` | Inventory items per player |
| `spells` | Known spells per player |
| `arsenal_spells` | Global spell catalog (admin-managed) |
| `arsenal_equipment` | Global equipment catalog (admin-managed) |

### Indexes

| Collection | Index |
|---|---|
| `users` | unique on `email` |
| `campaigns` | `userId` |
| `players` | `campaignId`, `userId` |
| `inventory` | `playerId` |
| `spells` | `playerId` |

### Key design decisions

- **Sessions embedded in campaign** — sessions have no independent queries; always accessed through a campaign. Embedding avoids a join and matches the API shape exactly.
- **Players, inventory, spells as separate collections** — queried independently and can be large.
- **`userId` on every document** — every query filters by `userId` to prevent cross-user data leaks. Returns `404` (not `403`) when a resource exists but belongs to another user.
- **IDs:** MongoDB `ObjectID` internally, serialized as hex strings to the client.
- **`updatedAt`:** always set server-side on every write — clients cannot forge it.

---

## Authentication

### Registration — `POST /auth/register`

1. Validate email + password present
2. Check `users` collection — return `409` if email taken
3. Hash password with bcrypt (cost 12)
4. If email matches `ADMIN_EMAIL` env var → role `"admin"`, otherwise role `"player"`
5. Insert user, return `201` with JWT token

### Login — `POST /auth/login`

1. Validate email + password present
2. Look up user by email — `401` if not found
3. `bcrypt.CompareHashAndPassword` — `401` if mismatch
4. Return `200` with JWT token

### JWT

- Algorithm: HS256
- Claims: `sub` (userId), `exp` (now + 7 days)
- Secret: `JWT_SECRET` env var

### Auth response shape

```json
{ "token": "<jwt>", "userId": "<id>", "role": "admin|player" }
```

### JWT middleware

- Extracts `Authorization: Bearer <token>`
- Validates signature and expiry
- Injects `userID` and `role` into request context
- Returns `401` if missing or invalid

---

## Roles & Permissions

Two roles: `admin` and `player`.

| Action | Admin | Player |
|---|---|---|
| GET `/campaigns`, `/campaigns/:id` | ✅ | ✅ |
| POST `/campaigns` | ✅ | ❌ |
| PATCH `/campaigns/:id` | ✅ | ❌ |
| DELETE `/campaigns/:id` | ✅ | ❌ |
| GET sessions | ✅ | ✅ |
| POST/PATCH sessions | ✅ | ❌ |
| DELETE sessions | ✅ | ❌ |
| GET players in campaign | ✅ | ✅ own only |
| POST `/players` | ✅ | ❌ |
| GET/PATCH `/players/:id` | ✅ | ✅ own only |
| DELETE `/players/:id` | ✅ | ❌ |
| GET/POST/PATCH own inventory | ✅ | ✅ own only |
| DELETE inventory | ✅ | ❌ |
| GET/POST/PATCH own spells | ✅ | ✅ own only |
| DELETE spells | ✅ | ❌ |
| PUT `/players/:id/spell-slots` | ✅ | ✅ own only |
| GET `/arsenal/spells`, `/arsenal/equipment` | ✅ | ✅ |
| POST/PATCH/DELETE arsenal | ✅ | ❌ |

**"Own only"** — a player's `userId` is stored on their player document at creation. Players can only access documents where the document's `userId` matches their JWT `sub`.

**Enforcement:**
- `RequireRole("admin")` middleware applied to all DELETE routes and admin-only mutations
- Store layer always filters by `userId`

---

## Request Handling Conventions

**Routing** — all routes under `/api` prefix:

```
/api/auth/register    (public)
/api/auth/login       (public)
/api/...              (JWT middleware required)
```

**Handler pattern:**
1. Extract path params + decode JSON body
2. Validate required fields → `400` if missing
3. Extract `userID` and `role` from context
4. Call store method (always passes `userID` for ownership check)
5. Encode JSON response

**Error response shape:**
```json
{ "error": "Human-readable message", "code": "MACHINE_READABLE_CODE" }
```

**Status codes:**
| Status | When |
|---|---|
| `201 Created` | Successful create |
| `200 OK` | Successful update/read |
| `204 No Content` | Successful delete |
| `400 Bad Request` | Invalid body or params |
| `401 Unauthorized` | Missing or invalid JWT |
| `403 Forbidden` | Insufficient role |
| `404 Not Found` | Resource does not exist (also used when resource belongs to another user) |
| `409 Conflict` | Unique constraint violation |
| `500 Internal Server Error` | Unexpected server error |

**Cascade deletes:**
- `DELETE /campaigns/:id` → deletes campaign + all players in campaign + their inventory + their spells
- `DELETE /players/:id` → deletes player + their inventory + their spells

---

## Arsenal

Global catalog of reference spells and equipment. Not tied to any campaign or user.

- Admins manage the catalog (POST/PATCH/DELETE)
- Players can read it (GET)
- Player workflow: GET arsenal → pick item → POST to own `/players/:id/inventory` or `/players/:id/spells`
- The server does not enforce that player inventory items originate from the arsenal — the arsenal is a reference, not a gate

**Schemas:** same shape as `Spell` and `InventoryItem` from API.md, without `playerId`/`userId` fields.

---

## API Endpoints

### Auth
| Method | Path |
|---|---|
| POST | `/api/auth/register` |
| POST | `/api/auth/login` |

### Campaigns
| Method | Path |
|---|---|
| GET | `/api/campaigns` |
| POST | `/api/campaigns` |
| GET | `/api/campaigns/:id` |
| PATCH | `/api/campaigns/:id` |
| DELETE | `/api/campaigns/:id` |

### Sessions
| Method | Path |
|---|---|
| POST | `/api/campaigns/:campaignId/sessions` |
| PATCH | `/api/campaigns/:campaignId/sessions/:sessionId` |
| DELETE | `/api/campaigns/:campaignId/sessions/:sessionId` |

### Players
| Method | Path |
|---|---|
| GET | `/api/campaigns/:campaignId/players` |
| POST | `/api/players` |
| GET | `/api/players/:playerId` |
| PATCH | `/api/players/:playerId` |
| DELETE | `/api/players/:playerId` |

### Inventory
| Method | Path |
|---|---|
| GET | `/api/players/:playerId/inventory` |
| POST | `/api/players/:playerId/inventory` |
| PATCH | `/api/inventory/:itemId` |
| DELETE | `/api/inventory/:itemId` |

### Spells
| Method | Path |
|---|---|
| GET | `/api/players/:playerId/spells` |
| POST | `/api/players/:playerId/spells` |
| PATCH | `/api/spells/:spellId` |
| DELETE | `/api/spells/:spellId` |
| PUT | `/api/players/:playerId/spell-slots` |

### Arsenal
| Method | Path |
|---|---|
| GET | `/api/arsenal/spells` |
| POST | `/api/arsenal/spells` |
| PATCH | `/api/arsenal/spells/:id` |
| DELETE | `/api/arsenal/spells/:id` |
| GET | `/api/arsenal/equipment` |
| POST | `/api/arsenal/equipment` |
| PATCH | `/api/arsenal/equipment/:id` |
| DELETE | `/api/arsenal/equipment/:id` |

---

## Deployment

**Dockerfile** — two-stage build:
1. `golang:1.26-alpine` builder stage — compiles binary
2. `alpine:latest` runtime stage — copies binary only

**Environment variables:**

| Variable | Description |
|---|---|
| `PORT` | HTTP port (set automatically by Railway) |
| `MONGO_URI` | MongoDB Atlas connection string |
| `JWT_SECRET` | Secret for HS256 JWT signing |
| `ADMIN_EMAIL` | Email that receives admin role on registration |

Railway deployment: push to GitHub → Railway auto-builds from Dockerfile and injects env vars.
