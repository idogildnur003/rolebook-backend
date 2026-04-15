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

- **Campaign DM**: The user who created the campaign is its DM. Write operations on campaigns, sessions, and players check that the caller is the DM of the *specific* campaign. This is enforced in handlers, not middleware.
- **Linked user**: A player's linked user can read and update their own character, spells, and inventory.

---

## Auth

No Bearer token required. Test scripts auto-set `token` and `userId`.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/auth/register` | Register a new user | 201 |
| POST | `/auth/login` | Login and get JWT | 200 |

**Body:**
```json
{ "email": "dm@example.com", "password": "secret123" }
```

---

## Campaigns

Requires Bearer `{{token}}`. The user who creates a campaign becomes its DM.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns` | Any | List campaigns (slim: id, role, name, themeImage, sessions, players¹) | 200 |
| GET | `/campaigns/{{campaignId}}` | DM or player | Get single campaign (full) | 200 |
| POST | `/campaigns` | Any | Create campaign (caller becomes DM) → sets `campaignId` | 201 |
| PATCH | `/campaigns/{{campaignId}}` | Campaign DM | Update campaign fields | 200 |
| DELETE | `/campaigns/{{campaignId}}` | Campaign DM | Delete campaign + all sub-resources | 204 |

**GET `/campaigns` response:**
```json
[
  {
    "id": "abc-123",
    "role": "dm",
    "name": "Lost Mine of Phandelver",
    "themeImage": "forest",
    "sessions": [
      { "id": "sess-1", "name": "Session 1 — The Cave" }
    ],
    "players": [
      { "playerId": "player-1", "isActive": true }
    ]
  }
]
```

¹ `players` is only included when `role` is `"dm"`. Omitted for player-role campaigns.

**POST body:**
```json
{ "name": "Lost Mine of Phandelver", "themeImage": "forest" }
```

**PATCH body (all mutable fields):**
```json
{
  "name": "Updated Name",
  "themeImage": "desert",
  "disabledSpells": ["spell-id-1"],
  "disabledEquipment": ["equip-id-1"]
}
```

---

## Sessions

Requires Bearer `{{token}}`. All operations require campaign DM.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/campaigns/{{campaignId}}/sessions` | Create session → sets `sessionId` | 201 |
| PATCH | `/campaigns/{{campaignId}}/sessions/{{sessionId}}` | Update session | 200 |
| DELETE | `/campaigns/{{campaignId}}/sessions/{{sessionId}}` | Delete session | 204 |

**POST body:**
```json
{ "name": "Session 1 — The Cave", "description": "The party entered the goblin cave." }
```

---

## Players

Requires Bearer `{{token}}`.

| Method | Path | Access | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/player` | Any campaign member | Get caller's own player in campaign → sets `playerId` | 200 |
| GET | `/campaigns/{{campaignId}}/players` | Campaign DM | List all players in campaign | 200 |
| GET | `/players/{{playerId}}` | Campaign DM or linked user | Get single player | 200 |
| POST | `/players` | Campaign DM | Create player → sets `playerId` | 201 |
| PATCH | `/players/{{playerId}}` | Campaign DM or linked user | Update player fields | 200 |
| DELETE | `/players/{{playerId}}` | Campaign DM | Delete player (spells/inventory embedded, deleted with player) | 204 |

**POST body (DM provides campaign + user email only; player fills in details later):**
```json
{ "campaignId": "{{campaignId}}", "userEmail": "player@example.com" }
```

**PATCH body (all editable fields):**
```json
{
  "name": "Thorn Ironbark",
  "className": "Ranger",
  "level": 5,
  "race": "Wood Elf",
  "notes": "Prefers ranged combat",
  "avatarUri": "https://example.com/avatar.png",
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
  "deathSaveSuccesses": 0,
  "deathSaveFailures": 0,
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
