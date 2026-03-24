# Rolebook API — Postman Reference

Base URL: `http://localhost:3000/api` (set via `{{baseUrl}}` environment variable)

## Setup

1. Import `rolebook-environment.json` into Postman (Environments → Import)
2. Import `rolebook-collection.json` into Postman (Collections → Import)
3. Select **Rolebook Local** as the active environment
4. Run **Login** (or **Register**) first — the test script auto-sets `{{token}}`
5. Run **Create Campaign** → **Create Player** to populate IDs for downstream requests

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
| GET | `/campaigns` | Any | List campaigns (slim: id, role, name, themeImage, sessions) | 200 |
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
    ]
  }
]
```

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
| GET | `/campaigns/{{campaignId}}/players` | Campaign DM | List players in campaign | 200 |
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
| 403 | Not the campaign DM |
| 404 | Resource not found |
| 409 | Duplicate entry (e.g. spell already added, email taken) |
| 500 | Unexpected server error |
