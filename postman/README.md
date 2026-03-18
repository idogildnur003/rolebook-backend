# Rolebook API — Postman Reference

Base URL: `http://localhost:3000/api` (set via `{{baseUrl}}` environment variable)

## Setup

1. Import `rolebook-environment.json` into Postman (Environments → Import)
2. Import `rolebook-collection.json` into Postman (Collections → Import)
3. Select **Rolebook Local** as the active environment
4. Run **Login** (or **Register**) first — the test script auto-sets `{{token}}`
5. Run **Create Campaign** → **Create Player** to populate IDs for downstream requests

---

## Auth

No Bearer token required. Test scripts auto-set `token` and `userId`.

| Method | Path | Description | Status |
|---|---|---|---|
| POST | `/auth/register` | Register a new user | 201 |
| POST | `/auth/login` | Login and get JWT | 200 |

**Body:**
```json
{ "email": "admin@example.com", "password": "secret123" }
```

---

## Campaigns

Requires Bearer `{{token}}`. Write operations require admin role.

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns` | No | List all campaigns | 200 |
| GET | `/campaigns/{{campaignId}}` | No | Get single campaign | 200 |
| POST | `/campaigns` | Yes | Create campaign → sets `campaignId` | 201 |
| PATCH | `/campaigns/{{campaignId}}` | Yes | Update campaign fields | 200 |
| DELETE | `/campaigns/{{campaignId}}` | Yes | Delete campaign + all sub-resources | 204 |

**POST body:**
```json
{ "name": "Lost Mine of Phandelver", "themeImage": "forest" }
```

---

## Sessions

Requires Bearer `{{token}}`. All operations require admin role.

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

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/campaigns/{{campaignId}}/players` | No | List players in campaign | 200 |
| GET | `/players/{{playerId}}` | No | Get single player | 200 |
| POST | `/players` | Yes | Create player → sets `playerId` | 201 |
| PATCH | `/players/{{playerId}}` | No | Update player fields | 200 |
| DELETE | `/players/{{playerId}}` | Yes | Delete player + inventory/spells | 204 |

**POST body:**
```json
{ "campaignId": "{{campaignId}}", "name": "Thorn", "className": "Ranger", "level": 1, "race": "Wood Elf" }
```

**PATCH body (HP update example):**
```json
{ "currentHp": 18, "tempHp": 5 }
```

---

## Inventory

Requires Bearer `{{token}}`.

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/players/{{playerId}}/inventory` | No | List inventory | 200 |
| POST | `/players/{{playerId}}/inventory` | No | Add item → sets `itemId` | 201 |
| PATCH | `/inventory/{{itemId}}` | No | Update item | 200 |
| DELETE | `/inventory/{{itemId}}` | Yes | Delete item | 204 |

**POST body:**
```json
{ "name": "Longsword", "quantity": 1, "category": "weapons", "tags": ["melee", "martial"], "damage": "1d8", "damageType": "slashing" }
```

---

## Spells

Requires Bearer `{{token}}`.

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/players/{{playerId}}/spells` | No | List known spells | 200 |
| POST | `/players/{{playerId}}/spells` | No | Add spell → sets `spellId` | 201 |
| PATCH | `/spells/{{spellId}}` | No | Update spell (e.g. toggle prepared) | 200 |
| DELETE | `/spells/{{spellId}}` | Yes | Remove spell | 204 |
| PUT | `/players/{{playerId}}/spell-slots` | No | Replace all spell slots | 200 |

**POST body:**
```json
{ "name": "Fireball", "level": 3, "school": "Evocation", "castingTime": "1 action", "range": "150 feet", "components": ["V","S","M"], "isPrepared": false }
```

**PUT spell-slots body:**
```json
{ "1": { "max": 4, "used": 0 }, "2": { "max": 3, "used": 1 }, "3": { "max": 3, "used": 0 } }
```

---

## Arsenal

Global reference catalog. Requires Bearer `{{token}}`. Write operations require admin role.

### Spells

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/arsenal/spells` | No | List all reference spells | 200 |
| POST | `/arsenal/spells` | Yes | Create reference spell → sets `arsenalSpellId` | 201 |
| PATCH | `/arsenal/spells/{{arsenalSpellId}}` | Yes | Update reference spell | 200 |
| DELETE | `/arsenal/spells/{{arsenalSpellId}}` | Yes | Delete reference spell | 204 |

**POST body:**
```json
{ "name": "Magic Missile", "level": 1, "school": "Evocation", "castingTime": "1 action", "range": "120 feet", "components": ["V","S"], "duration": "Instantaneous" }
```

### Equipment

| Method | Path | Admin | Description | Status |
|---|---|---|---|---|
| GET | `/arsenal/equipment` | No | List all reference equipment | 200 |
| POST | `/arsenal/equipment` | Yes | Create reference item → sets `arsenalEquipmentId` | 201 |
| PATCH | `/arsenal/equipment/{{arsenalEquipmentId}}` | Yes | Update reference item | 200 |
| DELETE | `/arsenal/equipment/{{arsenalEquipmentId}}` | Yes | Delete reference item | 204 |

**POST body:**
```json
{ "name": "Chain Mail", "category": "armor", "tags": ["heavy"], "armorClass": 16, "armorType": "heavy", "stealthDisadvantage": true }
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
| 403 | Authenticated but not admin |
| 404 | Resource not found |
| 409 | Unique constraint (e.g. email taken) |
| 500 | Unexpected server error |
```
