# Rolebook API — Postman Reference

Base URL: `http://localhost:3000/api` (set via `{{baseUrl}}` environment variable)

## Setup

The collection and environment are stored as YAML files under `postman/` and are loaded via native Git integration.

1. Open Postman and connect the workspace to this repository (the `.postman/resources.yaml` file maps everything automatically)
2. Select **Rolebook Local** as the active environment
3. Run **Login** (or **Register**) first — the test script auto-sets `{{token}}`
4. Run **Create Campaign** → **Create Player** to populate IDs for downstream requests

> **Legacy files:** `rolebook-environment.json` is kept as a reference backup but is no longer the active source.

## Access Control

The API uses two kinds of authorization:

- **Campaign DM**: The user who created the campaign is its DM. DM identity is recorded as a `role: "dm"` entry in the campaign's `members[]`, alongside a backing Player record (`kind: "dm"`). Write operations on campaigns, sessions, and players check that the caller is the DM of the *specific* campaign. This is enforced in handlers, not middleware.
- **Linked user**: A player's linked user can read and update their own character, spells, and inventory.

---

## Auth

No Bearer token required. Test scripts auto-set `token`, `userId`, and `verifyEmail`.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/auth/register` | Register a new user | 201 |
| POST | `/auth/login` | Login and get JWT | 200 |
| POST | `/auth/verify-email` | Confirm the emailed OTP, get JWT | 200 |
| POST | `/auth/resend-verification` | Re-send the OTP (always 200) | 200 |
| POST | `/auth/change-password` | Change password (Bearer required) | 204 |
| POST | `/auth/forgot-password` | Request a password-reset code (always 200) | 200 |
| POST | `/auth/verify-reset-code` | Exchange the reset code for a single-use token | 200 |
| POST | `/auth/reset-password` | Set a new password with the reset token | 204 |

**Register / Login body:**
```json
{ "email": "dm@example.com", "password": "secret123" }
```

**Forgot / reset password bodies:**
```json
// POST /auth/forgot-password
{ "email": "dm@example.com" }

// POST /auth/verify-reset-code  → { "resetToken": "<hex>" }
{ "email": "dm@example.com", "code": "123456" }

// POST /auth/reset-password  → 204, revokes pre-reset JWTs
{ "email": "dm@example.com", "resetToken": "<hex>", "newPassword": "new-secret123" }
```
Run **Forgot Password → Verify Reset Code** (paste the logged code) **→ Reset Password**; the token auto-carries via `{{resetToken}}`.

### Email verification

Verification is **on** when the server has `RESEND_API_KEY` set (or `EMAIL_VERIFICATION_ENABLED=true`), and **off** otherwise — so local dev with no key skips it entirely.

**Verification off:** `register` returns `{ token, userId, emailVerified: true }` immediately, same as before.

**Verification on:** `register` returns `{ "status": "verification_required", "email": "..." }` and sends a 6-digit code (printed to the server log when no real email provider is configured). The account gets **no token** until the code is confirmed:

- `POST /auth/verify-email` with `{ "email", "code" }` → on success returns `{ token, userId, emailVerified: true }`. Wrong/expired code → `400 INVALID_CODE`; too many attempts → `429 TOO_MANY_ATTEMPTS`; already verified → `400 ALREADY_VERIFIED`.
- `POST /auth/resend-verification` with `{ "email" }` → always `200 { "status": "ok" }` (no account enumeration); a fresh code is sent only when the account is unverified and past the 60s cooldown.
- `login` for an unverified account → `403 EMAIL_NOT_VERIFIED`. Successful login responses also include `emailVerified`.

In Postman, run **Register** first (it stashes `verifyEmail`), read the code from the server log, paste it into **Verify Email**'s `code`, and send.

### Change password

**Change password** — requires Bearer `{{token}}`. `newPassword` must be at least 8 characters. The existing token stays valid (no re-issue).
```json
{ "currentPassword": "secret123", "newPassword": "evenbettersecret" }
```

| Status | Code | Meaning |
|---|---|---|
| 204 | — | Password changed |
| 400 | `BAD_REQUEST` | Missing fields or malformed body |
| 400 | `WEAK_PASSWORD` | `newPassword` shorter than 8 characters |
| 400 | `INVALID_CURRENT_PASSWORD` | `currentPassword` does not match |
| 401 | `UNAUTHORIZED` | Missing or expired token |

### Change email

All three require Bearer `{{token}}`. Email changes use a **pending swap**: the new address is verified by a 6-digit code before the account email changes, and the old address keeps working until then.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/auth/change-email` | Re-auth + start change; code sent to new email | 200 |
| POST | `/auth/verify-email-change` | Confirm the code; swap the email | 200 |
| POST | `/auth/resend-email-change` | Re-send the code to the pending address | 200 |

- `change-email` `{newEmail, currentPassword}` → `200 {status:"verification_required", email}`. Wrong password → `400 INVALID_CURRENT_PASSWORD`; same as current → `400 SAME_EMAIL`; already taken → `409 EMAIL_TAKEN`.
- `verify-email-change` `{code}` → `200 {email}`. Wrong/expired → `400 INVALID_CODE`; too many → `429 TOO_MANY_ATTEMPTS`; none pending → `400 NO_PENDING_CHANGE`. On success the **old** address receives a security notification.
- `resend-email-change` → always `200 {status:"ok"}` when a change is pending (60s cooldown).

---

## Campaigns

Requires Bearer `{{token}}`. The user who creates a campaign becomes its DM.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns` | Any | List campaigns (slim: id, myRole, myPlayerId, name, themeImage, mapImageUri, sessions, members¹) | 200 |
| GET | `/campaigns/{{campaignId}}` | DM or player | Get single campaign (full) | 200 |
| POST | `/campaigns` | Any | Create campaign (caller becomes DM) → sets `campaignId` | 201 |
| PATCH | `/campaigns/{{campaignId}}` | Campaign DM | Update campaign fields | 200 |
| DELETE | `/campaigns/{{campaignId}}` | Campaign DM | Delete campaign + all sub-resources | 204 |
| PATCH | `/campaigns/{{campaignId}}/players/{{playerId}}` | Campaign DM | Archive/restore a player member (`isActive`) | 200 / 400² |

**GET `/campaigns` response:**
```json
[
  {
    "id": "abc-123",
    "myRole": "dm",
    "myPlayerId": "player-dm-1",
    "name": "Lost Mine of Phandelver",
    "themeImage": "forest",
    "mapImageUri": "https://bucket.s3.amazonaws.com/campaigns/abc-123/maps/...?X-Amz=...",
    "sessions": [
      { "id": "sess-1", "name": "Session 1 — The Cave" }
    ],
    "members": [
      { "playerId": "player-dm-1", "role": "dm", "isActive": true },
      { "playerId": "player-1",    "role": "player", "isActive": true }
    ]
  }
]
```

¹ `members` is only included when `myRole` is `"dm"`. Omitted for player-role campaigns. Member entries never include `userId` — identity on the wire is by `playerId`.

**GET `/campaigns/{{campaignId}}` response (full detail):**
```json
{
  "id": "abc-123",
  "myRole": "dm",
  "myPlayerId": "player-dm-1",
  "name": "Lost Mine of Phandelver",
  "themeImage": "forest",
  "mapImageUri": null,
  "mapPins": [],
  "sessions": [{ "id": "sess-1", "name": "Session 1 — The Cave", "description": "" }],
  "members": [
    { "playerId": "player-dm-1", "role": "dm", "isActive": true },
    { "playerId": "player-1",    "role": "player", "isActive": true }
  ],
  "disabledSpells": [],
  "disabledEquipment": [],
  "updatedAt": "2026-04-28T10:00:00Z"
}
```

`members` is included for both DM and player callers on the detail view (visible to all members; still no userIds).

**POST body:**
```json
{ "name": "Lost Mine of Phandelver", "themeImage": "forest" }
```

The created campaign returns `myRole: "dm"` and a `myPlayerId` for a freshly-minted DM stub Player (`kind: "dm"`). The DM is a first-class member with a real Player record — created atomically alongside the campaign — so the campaign always has at least one member.

**PATCH body (all mutable fields):**
```json
{
  "name": "Updated Name",
  "themeImage": "desert",
  "disabledSpells": ["spell-id-1"],
  "disabledEquipment": ["equip-id-1"]
}
```

**PATCH `/campaigns/{{campaignId}}/players/{{playerId}}` body (archive/restore):**
```json
{ "isActive": false }
```

² Returns `400 BAD_REQUEST` with message `"the DM cannot be archived"` if the target member is the campaign DM. The DM is a first-class member but cannot be archived — only `role: "player"` members can be flipped active/inactive.

---

## Sessions

Requires Bearer `{{token}}`. Session CRUD requires the campaign DM; schedule sub-resources are open to active members as noted below.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/campaigns/{{campaignId}}/sessions` | Create session → sets `sessionId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/sessions/{{sessionId}}` | Update session | 200 |
| DELETE | `/campaigns/{{campaignId}}/sessions/{{sessionId}}` | Delete session | 204 |
| PUT    | `/campaigns/{{campaignId}}/sessions/{{sessionId}}/availability`     | Upsert caller's availability grid; X-Timezone required on first write — any active member | 200 |
| DELETE | `/campaigns/{{campaignId}}/sessions/{{sessionId}}/availability`     | Remove caller's availability entry — any active member                                    | 204 |
| PUT    | `/campaigns/{{campaignId}}/sessions/{{sessionId}}/confirmed-slot`   | Set/replace the confirmed slot — DM only                                                  | 200 |
| DELETE | `/campaigns/{{campaignId}}/sessions/{{sessionId}}/confirmed-slot`   | Clear the confirmed slot — DM only                                                        | 204 |

**POST body:**
```json
{ "name": "Session 1 — The Cave", "description": "The party entered the goblin cave." }
```

### Session Schedule

`session.schedule` is an optional sub-document on each session. Identity is by `playerId` — `userId` never appears.

```json
{
  "schedule": {
    "dmTimezone": "America/New_York",
    "participantAvailabilities": [
      {
        "playerId": "player-dm-1",
        "availabilityByDate": {
          "2026-05-04": { "morning": true, "noon": true, "evening": false },
          "2026-05-11": { "morning": false, "noon": true, "evening": true }
        },
        "updatedAt": "2026-04-20T10:30:00Z"
      }
    ],
    "confirmedSlot": {
      "date": "2026-05-11",
      "dayPart": "evening",
      "startAt": "2026-05-11T23:00:00Z",
      "durationMinutes": 240,
      "confirmedAt": "2026-04-20T10:31:00Z"
    },
    "updatedAt": "2026-04-20T10:31:00Z"
  }
}
```

**Bootstrap rule:** the *first* write on a session bootstraps `schedule` with `dmTimezone`. The DM must make this call (with `X-Timezone: <IANA>`). A non-DM caller writing first gets `409 SCHEDULE_NOT_INITIALIZED`.

**Error codes:** `SCHEDULE_NOT_INITIALIZED` (409), `INVALID_TIMEZONE` (400), `INVALID_DAY_PART` (400), `INVALID_DATE_KEY` (400), `INVALID_DURATION` (400).

---

## Session Notes

Per-user, per-session private notes. Each campaign member (DM and players alike) has their own private notes for each session in a campaign. Notes are never visible to other members and are never serialized in any other endpoint's response.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/my-session-notes` | Any member (active or inactive) | Returns the caller's notes for every session in the campaign | 200 |
| PUT | `/campaigns/{{campaignId}}/sessions/{{sessionId}}/my-notes` | Active member | Upsert the caller's note for a single session | 200 |

**GET response:**
```json
{ "notes": { "sess-1": "I rolled a nat 20", "sess-2": "TPK avoided" } }
```

Returns `{ "notes": {} }` when the caller has no notes. Inactive (archived) members retain read access.

**PUT body:**
```json
{ "text": "Today the party explored the cave..." }
```

**PUT response:**
```json
{ "sessionId": "sess-1", "text": "Today the party explored the cave..." }
```

- `text` is trimmed of leading/trailing whitespace before storage.
- Empty / whitespace-only `text` removes the note entry and returns `{ sessionId, text: "" }`. No separate DELETE endpoint.
- `text` length cap: 10,000 characters. Over the cap → `400 BAD_REQUEST`.
- Unknown `sessionId` → `404 NOT_FOUND`.
- Inactive members on PUT → `403 FORBIDDEN`.
- Deleting a session via `DELETE /sessions/{{sessionId}}` strips that sessionId from every member's notes map.

---

## Locations

Per-campaign journal entries for places. Owned per-member: any campaign member can List and Create; only the owner can Update, Delete, or Share. Read visibility is owner-only by default; widened via `visibility.sharedWithAll` or `visibility.sharedPlayerIds`. Sub-locations are supported one level deep (a location with `parentLocationId` cannot itself be a parent).

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/locations` | Any member | List locations visible to caller | 200 |
| POST | `/campaigns/{{campaignId}}/locations` | Any member | Create a location (owner = caller) → sets `locationId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/locations/{{locationId}}` | Owner | Update fields, optionally `visibility`/`shareNote` | 200 |
| DELETE | `/campaigns/{{campaignId}}/locations/{{locationId}}` | Owner | Delete (cascade-deletes pointing pins) | 204 |
| POST | `/campaigns/{{campaignId}}/locations/{{locationId}}/share` | Owner | Clone for recipients, optional NPC + pin cascade | 200 |

**Body (POST):**
```json
{
  "name": "The Sunken Vault",
  "shortNotes": "Old dwarven stronghold",
  "fullDescription": "Buried below the river — guarded by water elementals.",
  "thumbnailUri": "",
  "sessionId": "{{sessionId}}",
  "parentLocationId": "",
  "linkedNpcIds": []
}
```

**Body (PATCH):** Any subset of the create fields, plus optional:
```json
{
  "visibility": { "sharedWithAll": true, "sharedPlayerIds": [] },
  "shareNote": "…"
}
```

**Body (POST /share):**
```json
{
  "recipientPlayerIds": ["{{otherPlayerId}}"],
  "sharedWithAll": false,
  "note": "Sharing because it's relevant to your subplot.",
  "cascade": {
    "npcIds": [],
    "mapPinIds": []
  }
}
```

Returns: array of clones (one per recipient, with thumbnails resolved to short-lived signed URLs).

---

## NPCs

Per-campaign journal entries for non-player characters. Owned per-member: any campaign member can List and Create; only the owner can Update, Delete, or Share. Read visibility is owner-only by default; widened via `visibility.sharedWithAll` or `visibility.sharedPlayerIds`.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/npcs` | Any member | List NPCs visible to caller | 200 |
| POST | `/campaigns/{{campaignId}}/npcs` | Any member | Create an NPC (owner = caller) → sets `npcId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/npcs/{{npcId}}` | Owner | Update fields, optionally `visibility`/`shareNote` | 200 |
| DELETE | `/campaigns/{{campaignId}}/npcs/{{npcId}}` | Owner | Delete (cascade-deletes pointing pins) | 204 |
| POST | `/campaigns/{{campaignId}}/npcs/{{npcId}}/share` | Owner | Clone for recipients, optional location + pin cascade | 200 |

**Body (POST):**
```json
{
  "name": "Sildar Hallwinter",
  "shortNotes": "Lord's Alliance member, traveling companion",
  "fullDescription": "A human warrior who has seen better days…",
  "avatarUri": "",
  "sessionId": "{{sessionId}}",
  "linkedLocationIds": []
}
```

**Body (PATCH):** Any subset of the create fields, plus optional:
```json
{
  "visibility": { "sharedWithAll": true, "sharedPlayerIds": [] },
  "shareNote": "…"
}
```

**Body (POST /share):**
```json
{
  "recipientPlayerIds": ["{{otherPlayerId}}"],
  "sharedWithAll": false,
  "note": "Their backstory is tied to this NPC.",
  "cascade": {
    "locationIds": [],
    "mapPinIds": []
  }
}
```

Returns: array of clones (one per recipient, with avatars resolved to short-lived signed URLs).

---

## Map Pins

Per-campaign map pins. Six types: `location`, `npc`, `item`, `majorFinding`, `travelMarker`, `custom`. Pins of type `location`/`npc` reference an entity via `entityId`; the others carry their own `title` and optional `notes`.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/map-pins` | Any member | List pins visible to caller | 200 |
| POST | `/campaigns/{{campaignId}}/map-pins` | Any member | Create a pin (owner = caller) → sets `mapPinId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/map-pins/{{mapPinId}}` | Owner | Update position/notes (type+entityId are immutable) | 200 |
| DELETE | `/campaigns/{{campaignId}}/map-pins/{{mapPinId}}` | Owner | Delete | 204 |
| POST | `/campaigns/{{campaignId}}/map-pins/{{mapPinId}}/share` | Owner | Clone for recipients, optional entity cascade | 200 |

**Body (POST — location pin example):**
```json
{
  "type": "location",
  "entityId": "{{locationId}}",
  "x": 0.42,
  "y": 0.71,
  "sessionId": "{{sessionId}}"
}
```

**Body (POST — custom pin example):**
```json
{
  "type": "custom",
  "title": "Hidden trapdoor",
  "notes": "Seen during session 4",
  "x": 0.18,
  "y": 0.55,
  "sessionId": "{{sessionId}}"
}
```

**Body (PATCH):**
```json
{
  "x": 0.5,
  "y": 0.5,
  "notes": "Moved by the player"
}
```

**Body (POST /share):**
```json
{
  "recipientPlayerIds": ["{{otherPlayerId}}"],
  "sharedWithAll": false,
  "note": "",
  "cascadeEntity": true
}
```

Returns: array of clones (one per recipient).

---

## Uploads

Requires Bearer `{{token}}`. Issues short-lived presigned URLs for direct-to-S3 uploads.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| POST | `/uploads/url` | Varies by `kind` | Get a presigned PUT URL for upload | 200 / 503¹ |

**Supported `kind` values:**
- `player-avatar` — body: `kind, playerId, contentType`. Caller must be the player's linked user.
- `map` — body: `kind, campaignId, contentType`. Caller must be the campaign DM.
- `location-thumbnail` — body: `kind, campaignId, contentType`. Caller must be a campaign member.
- `npc-avatar` — body: `kind, campaignId, contentType`. Caller must be a campaign member.

`contentType` must be `image/jpeg`, `image/png`, or `image/webp`.

**Response (all kinds):**
```json
{
  "uploadUrl": "https://<bucket>.s3.<region>.amazonaws.com/.../file.png?X-Amz-…",
  "key": "…/file.png",
  "expiresAt": "2026-05-04T12:34:56Z"
}
```

**Client flow:**
1. POST `/uploads/url` → receive `uploadUrl` + `key`.
2. PUT the file bytes directly to `uploadUrl` with the same `Content-Type` you requested. **Do not** attach the API Bearer token to this request — S3 rejects unrecognized headers it didn't sign.
3. Store the returned `key` on the entity (`mapImageUri` for map, `thumbnailUri` for location, `avatarUri` for NPC).

The S3 bucket is private; subsequent reads are served via short-lived presigned GET URLs. Legacy values that already look like URLs are returned unchanged. When AWS is unconfigured (local dev), keys pass through both ways without rewriting.

¹ Returns `503` with `{ "error": "...", "code": "UPLOAD_NOT_CONFIGURED" }` when the server is running without the four AWS env vars (`AWS_REGION`, `AWS_S3_BUCKET`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) — typical for local dev.

---

## Players

Requires Bearer `{{token}}`.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/player` | Any campaign member | Get caller's own player in campaign (any kind, including the DM's stub) → sets `playerId` | 200 |
| GET | `/campaigns/{{campaignId}}/players` | Campaign DM | List all PC players in campaign (`kind:"pc"` only — excludes the DM's stub Player and any future NPC/enemy/boss records) | 200 |
| GET | `/players/{{playerId}}` | Campaign DM or linked user | Get single player | 200 |
| POST | `/players` | Campaign DM | Create a new PC player (`kind:"pc"`) → sets `playerId` | 201 |
| PATCH | `/players/{{playerId}}` | Campaign DM or linked user | Update player fields | 200 |
| DELETE | `/players/{{playerId}}` | Campaign DM | Delete player (spells/inventory embedded, deleted with player) | 204 |

**POST body (DM provides campaign + user email only; player fills in details later):**
```json
{ "campaignId": "{{campaignId}}", "userEmail": "player@example.com" }
```

The created Player has `kind: "pc"`. The DM's stub Player (`kind: "dm"`) is created together with the campaign in `POST /campaigns` and is **not** part of this endpoint — there is no `POST /players` path for creating a DM record. `POST /players` also appends the new member to the campaign's `members[]` with `role: "player"` and `isActive: true`.

**`avatarUri` semantics:** on PATCH, send an S3 object key (no scheme), e.g. `"players/{{playerId}}/avatar/abc.png"`, obtained from `POST /uploads/url` after uploading the file directly to S3 (see **Uploads** below). On read responses, the server rewrites the stored key into a short-lived presigned `https://…` GET URL (TTL 1 minute). Legacy values that already look like URLs are returned unchanged. When AWS is unconfigured (local dev), the field passes through both ways without rewriting.

**PATCH body (all editable fields):**
```json
{
  "name": "Thorn Ironbark",
  "className": "Ranger",
  "level": 5,
  "race": "Wood Elf",
  "notes": "Prefers ranged combat",
  "avatarUri": "players/{{playerId}}/avatar/abc.png",
  "backgroundStory": "Raised in the Emerald Forest by druids.",
  "alignment": "Neutral Good",
  "speciesOrRegion": "Sylvan",
  "subclass": "Gloom Stalker",
  "region": "Emerald Forest",
  "size": "Medium",
  "currentHp": 38,
  "maxHp": 42,
  "tempHp": 5,
  "ac": 15,
  "speed": 35,
  "initiativeBonus": 3,
  "proficiencyBonus": 3,
  "expertiseBonus": 4,
  "deathSaveSuccesses": 0,
  "deathSaveFailures": 0,
  "heroicInspiration": false,
  "abilityScores": { "STR": 12, "DEX": 16, "CON": 14, "INT": 10, "WIS": 14, "CHA": 8 },
  "abilityTemporaryModifiers": {},
  "skillTemporaryModifiers": {},
  "proficientSavingThrows": ["STR", "DEX"],
  "proficientSkills": ["Stealth", "Perception", "Survival"],
  "expertiseSkills": ["Stealth"],
  "featuresAndFeats": ["Favored Enemy", "Natural Explorer", "Dread Ambusher"],
  "conditions": {}
}
```

---

## Player Spells

Requires Bearer `{{token}}`. Spells are embedded in the player document as lightweight references to the arsenal catalog.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/players/{{playerId}}/spells` | Campaign DM or linked user | List known spells (filtered by campaign disabled list) | 200 |
| POST | `/players/{{playerId}}/spells` | Campaign DM or linked user | Add spell from arsenal → validates existence | 201 |
| PATCH | `/players/{{playerId}}/spells/{{spellId}}` | Campaign DM or linked user | Update spell fields | 204 |
| DELETE | `/players/{{playerId}}/spells/{{spellId}}` | Campaign DM or linked user | Remove spell | 204 |
| PUT | `/players/{{playerId}}/spell-slots` | Campaign DM or linked user | Replace all spell slots | 200 |

**GET response:**
```json
[{ "spellId": "abc-123", "name": "Fireball", "isPrepared": true }]
```

**POST body:**
```json
{ "spellId": "{{arsenalSpellId}}", "isPrepared": false }
```

`spellId` is resolved against the SRD arsenal catalog first, then the campaign's custom spells — so a `customSpellId` like `custom-glacial-whisper-4b80ad` is also accepted here.

**PATCH body (any mutable field):**
```json
{ "isPrepared": true }
```

**PUT spell-slots body:**
```json
{ "1": { "max": 4, "used": 0 }, "2": { "max": 3, "used": 1 }, "3": { "max": 3, "used": 0 } }
```

---

## Player Inventory

Requires Bearer `{{token}}`. Inventory items are embedded in the player document as lightweight references to the arsenal catalog.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/players/{{playerId}}/inventory` | Campaign DM or linked user | List inventory (filtered by campaign disabled list) | 200 |
| POST | `/players/{{playerId}}/inventory` | Campaign DM or linked user | Add item from arsenal → validates existence | 201 |
| PATCH | `/players/{{playerId}}/inventory/{{equipmentId}}` | Campaign DM or linked user | Update item fields | 204 |
| DELETE | `/players/{{playerId}}/inventory/{{equipmentId}}` | Campaign DM or linked user | Remove item | 204 |

**GET response:**
```json
[{ "equipmentId": "abc-123", "name": "Longsword", "quantity": 1 }]
```

**POST body:**
```json
{ "equipmentId": "{{arsenalEquipmentId}}", "quantity": 1 }
```

**PATCH body (any mutable field):**
```json
{ "quantity": 2 }
```

---

## Custom Equipment

Requires Bearer `{{token}}`. Per-campaign homebrew equipment catalog, stored in MongoDB alongside the read-only SRD arsenal. IDs are server-issued (`custom-{slug}-{hex}`) on create. Inventory endpoints resolve `equipmentId` against the SRD catalog first, then the campaign custom store.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/custom-equipment` | Any campaign member | List custom equipment for campaign | 200 |
| POST | `/campaigns/{{campaignId}}/custom-equipment` | Any campaign member | Create custom equipment → sets `customEquipmentId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/custom-equipment/{{customEquipmentId}}` | Creator or campaign DM | Update custom equipment fields | 200 |
| DELETE | `/campaigns/{{campaignId}}/custom-equipment/{{customEquipmentId}}` | Campaign DM | Delete and cascade out of every player's inventory in the campaign | 204 |

Server-owned fields (`id`, `campaignId`, `createdBy`, `createdAt`, `updatedAt`) are stamped on create and stripped from PATCH bodies if supplied.

**POST body:**
```json
{
  "name": "Runed Shortblade",
  "category": "weapons",
  "tags": ["melee", "magic"],
  "notes": "A homebrew shortsword etched with faintly glowing runes.",
  "damage": "1d6+1",
  "damageType": "slashing",
  "weaponType": "martial-melee",
  "properties": ["finesse", "light"],
  "cost": 75,
  "currency": "gp"
}
```

**PATCH body (any mutable field):**
```json
{ "notes": "Updated homebrew notes", "cost": 90 }
```

---

## Custom Spells

Requires Bearer `{{token}}`. Per-campaign homebrew spell catalog, stored in MongoDB alongside the read-only SRD arsenal. IDs are server-issued (`custom-{slug}-{hex}`) on create. Player spell endpoints resolve `spellId` against the SRD catalog first, then the campaign custom store.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/custom-spells` | Any campaign member | List custom spells for campaign | 200 |
| POST | `/campaigns/{{campaignId}}/custom-spells` | Any campaign member | Create custom spell → sets `customSpellId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/custom-spells/{{customSpellId}}` | Creator or campaign DM | Update custom spell fields | 200 |
| DELETE | `/campaigns/{{campaignId}}/custom-spells/{{customSpellId}}` | Campaign DM | Delete and cascade out of every player's spell list in the campaign | 204 |

Server-owned fields (`id`, `campaignId`, `createdBy`, `createdAt`, `updatedAt`) are stamped on create and stripped from PATCH bodies if supplied. `level` must be between 0 and 9.

**POST body:**
```json
{
  "name": "Glacial Whisper",
  "level": 2,
  "school": "evocation",
  "castingTime": "1 action",
  "range": "60 feet",
  "components": ["V", "S", "M"],
  "material": "a sliver of ice",
  "duration": "Instantaneous",
  "description": "A shard of supernatural cold pierces a single target.",
  "isRitual": false
}
```

**PATCH body (any mutable field):**
```json
{ "description": "…", "level": 3 }
```

---

## Arsenal

Read-only reference catalog. Data is manually curated in the `arsenal` database. Requires Bearer `{{token}}`, no role restriction.

### Spells

| Method | Path | Description | Status |
|---|---|---|---|
| GET | `/arsenal/spells?page=1&limit=20` | List spells (paginated) | 200 |
| GET | `/arsenal/spells/{{arsenalSpellId}}` | Get full spell details | 200 |

**List response:**
```json
{ "data": [{ "id": "abc-123", "name": "Magic Missile", "level": 1, ... }], "page": 1, "limit": 20, "total": 42 }
```

### Equipment

| Method | Path | Description | Status |
|---|---|---|---|
| GET | `/arsenal/equipment?page=1&limit=20` | List equipment (paginated) | 200 |
| GET | `/arsenal/equipment/{{arsenalEquipmentId}}` | Get full equipment details | 200 |

**List response:**
```json
{ "data": [{ "id": "abc-123", "name": "Chain Mail", "category": "armor", ... }], "page": 1, "limit": 20, "total": 15 }
```

---

## Error Responses

All errors follow this shape:
```json
{ "error": "Human-readable message", "code": "MACHINE_READABLE_CODE" }
```

| Status | When |
|---|---|
| 400 | Invalid body or params |
| 401 | Missing or invalid JWT |
| 403 | Not the campaign DM or not the linked player |
| 404 | Resource not found |
| 409 | Duplicate entry (e.g. spell already added, email taken) |
| 500 | Unexpected server error |

## Notes

- There is no global "role" on users. Whether a user is a DM or player is determined per-campaign (the campaign creator is the DM; linked users are players).
- The JWT contains only the user ID — no role claim.
