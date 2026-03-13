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

## IDs

All resource IDs are **UUID v4 strings** generated server-side. MongoDB documents use the UUID as the `_id` field (stored as a string). This is consistent with the client's local UUID generation for offline-first pre-sync IDs.

No `createdAt` field is stored — this matches the API.md spec which defines only `updatedAt` on all resources.

---

## Admin Model

**There is exactly one admin**, determined by `ADMIN_EMAIL`. Any user registering with that email gets `role: "admin"`; all others get `role: "player"`. There is no role-escalation endpoint.

**Admin access is global** — admins are not scoped by ownership. An admin can read and mutate any resource regardless of which admin or user created it. The `ownerUserId` concept does not apply to admin queries; the store skips ownership filters entirely when the requester has role `"admin"`.

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
| `campaigns` | `linkedUserId` |
| `players` | `campaignId`, `linkedUserId` |
| `inventory` | `playerId` |
| `spells` | `playerId` |

### Key design decisions

- **Sessions embedded in campaign** — sessions have no independent queries; always accessed through a campaign. Embedding avoids a join and matches the API shape exactly. When a session is created, patched, or deleted, the campaign document's `updatedAt` is also updated — this ensures the client can detect session changes during sync pull via the campaign's `updatedAt`.
- **Players, inventory, spells as separate collections** — queried independently and can be large.
- **`updatedAt`:** always set server-side on every write — clients cannot forge it.
- **Arsenal is never cascade-deleted** — it is a global catalog unrelated to any campaign or user.
- **No MongoDB multi-document transactions** — cascade deletes run as sequential single-collection operations. If the server crashes mid-cascade, orphaned documents may remain. This is an accepted risk for this application tier; no transaction wrapper is required.

---

## Player Ownership Model

Player documents carry a `linkedUserId` field (nullable) — the user account ID of the player-role user who owns this character.

- Set by admin via optional `linkedUserId` field in `POST /players` body
- Can be updated later via `PATCH /players/:id` (admin-only, since only admins can PATCH players they don't own — but any admin can patch any player per the global admin access rule above)
- If `linkedUserId` is null, no player-role user can access the character; only admins can

**Inventory and spell documents** are stamped with both `playerId` and `linkedUserId` (copied from the player document at creation time). If the player's `linkedUserId` is later changed via PATCH, existing inventory/spell documents retain the old `linkedUserId` until explicitly updated — this is an accepted limitation.

**Ownership checks for player-role requests:**
- The store always applies `{ linkedUserId: requester.userId }` as a filter
- Returns `404` if not found or not owned — never `403` (avoids leaking existence of other users' data)
- For `GET /campaigns/:campaignId/players`: filter is `{ campaignId: X, linkedUserId: requester.userId }` — both conditions required

---

## Campaign Visibility

- **Admin**: no ownership filter — sees all campaigns
- **Player**: store performs a lookup: find all player documents where `linkedUserId = requester.userId`, collect their `campaignId`s, then return those campaigns. If no characters exist, returns an empty array.

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
- Claims: `sub` (userId), `role`, `exp` (now + 7 days)
- Secret: `JWT_SECRET` env var
- No refresh mechanism. On `401`, the client must re-authenticate. The client should treat the server response's `updatedAt` as the new local `updatedAt` baseline after every successful sync push.

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

Two roles: `admin` (global access) and `player` (scoped to own character).

| Action | Admin | Player |
|---|---|---|
| GET `/campaigns`, `/campaigns/:id` | ✅ all campaigns | ✅ campaigns where they have a character |
| POST `/campaigns` | ✅ | ❌ |
| PATCH `/campaigns/:id` | ✅ | ❌ |
| DELETE `/campaigns/:id` | ✅ | ❌ |
| GET `/campaigns/:id/players` | ✅ all players | ✅ own character only (`campaignId + linkedUserId` filter) |
| POST/PATCH `/campaigns/:id/sessions` | ✅ | ❌ |
| DELETE `/campaigns/:id/sessions/:id` | ✅ | ❌ |
| POST `/players` | ✅ | ❌ |
| GET/PATCH `/players/:id` | ✅ | ✅ own character only |
| DELETE `/players/:id` | ✅ | ❌ |
| GET/POST/PATCH inventory | ✅ | ✅ own character only |
| DELETE inventory | ✅ | ❌ |
| GET/POST/PATCH spells | ✅ | ✅ own character only |
| DELETE spells | ✅ | ❌ |
| PUT `/players/:id/spell-slots` | ✅ | ✅ own character only |
| GET `/arsenal/spells`, `/arsenal/equipment` | ✅ | ✅ |
| POST/PATCH/DELETE arsenal | ✅ | ❌ |

**Notes:**
- Sessions have no standalone GET — retrieved embedded in `GET /campaigns/:id`.
- Admin has global access; no per-resource ownership filter for admin role.
- For session mutations (`POST/PATCH/DELETE /campaigns/:campaignId/sessions`), the handler validates the campaign exists (404 if not).
- Player PATCH (`PATCH /players/:id`) strips the following fields before applying the update, even if present in the body: `campaignId`, `linkedUserId`. These are protected fields; silently ignored on player-role requests.

**Enforcement:**
- `RequireRole("admin")` middleware on all `DELETE` routes and all admin-only `POST`/`PATCH` routes
- "Own only" for player-accessible routes enforced in store via `{ linkedUserId: requester.userId }` filter
- Admin role is in the JWT claims and available from context — no DB lookup needed per request
- Store methods accept a `requesterRole` param; admin role bypasses ownership filters entirely

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
4. Call store method (always passes `userID` and `role`)
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
| `404 Not Found` | Resource does not exist or belongs to another user |
| `409 Conflict` | Unique constraint violation |
| `500 Internal Server Error` | Unexpected server error |

**Cascade deletes:**
- `DELETE /campaigns/:id` → deletes campaign + all players in campaign + their inventory + their spells. Does NOT touch `arsenal_spells` or `arsenal_equipment`.
- `DELETE /players/:id` → deletes player + their inventory + their spells.
- Cascade runs as sequential single-collection deletes; no transaction. Orphaned documents on crash are an accepted risk.

---

## Spell Slot Updates

Two ways to update spell slots — behavior is explicitly different:

| Method | Endpoint | Behavior |
|---|---|---|
| `PATCH /players/:id` with `spellSlots` field | Merges only the specified slot levels; unspecified levels are unchanged |
| `PUT /players/:id/spell-slots` | **Replaces** the entire spell slot map atomically. Response is the **full updated player object** (not just the slots map), ensuring the client receives a fresh `updatedAt` for its local baseline. |

---

## Map Pins

`PATCH /campaigns/:id` with a `mapPins` array **replaces** the entire pins array. The client manages the full array locally and sends the complete desired state. Partial-merge semantics are not supported for this field.

---

## Player Schema Notes

- `deathSaveSuccesses` and `deathSaveFailures`: server validates 0–3. Returns `400` if out of range.
- `speciesOrRegion` and `region`: both preserved as separate nullable string fields as defined in the original API spec.

---

## Arsenal

Global catalog of reference spells and equipment. Not tied to any campaign or user.

- Admins manage the catalog (POST/PATCH/DELETE)
- Players can read it (GET)
- Player workflow: GET arsenal → pick item → POST to own `/players/:id/inventory` or `/players/:id/spells`
- The server does **not** enforce that player inventory/spells originate from the arsenal — the arsenal is a reference, not a gate
- Arsenal schemas: same shape as `Spell` and `InventoryItem` from API.md, without `playerId`/`linkedUserId` fields
- No pagination. The catalog is expected to remain small enough for a single-response list.

---

## API Endpoints

### Auth (public)
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

### Sessions (embedded in campaign; no standalone GET)
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
