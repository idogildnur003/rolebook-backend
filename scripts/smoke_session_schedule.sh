#!/usr/bin/env bash
# Session-scheduling API smoke test.
# Re-runnable; fails loud on any unexpected status.
#
# Required env: DM_EMAIL, DM_PASSWORD, PLAYER_EMAIL, PLAYER_PASSWORD.
# Optional env: BASE_URL (default http://localhost:3000/api), TZ_HEADER (default America/New_York).
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000/api}"
TZ_HEADER="${TZ_HEADER:-America/New_York}"

: "${DM_EMAIL:?set DM_EMAIL}"
: "${DM_PASSWORD:?set DM_PASSWORD}"
: "${PLAYER_EMAIL:?set PLAYER_EMAIL}"
: "${PLAYER_PASSWORD:?set PLAYER_PASSWORD}"

# expect_status <expected> <actual> <label>
expect_status() {
  if [[ "$2" != "$1" ]]; then
    echo "FAIL: $3 — expected HTTP $1, got $2" >&2
    exit 1
  fi
  echo "OK: $3 (HTTP $2)"
}

# call <method> <path> <token-or-empty> <body-or-empty> [extra-header...]
# echoes "$status\n$body" via global vars STATUS and BODY.
call() {
  local method="$1" path="$2" token="$3" body="$4"; shift 4
  local headers=( -H "Content-Type: application/json" )
  [[ -n "$token" ]] && headers+=( -H "Authorization: Bearer $token" )
  while (( $# )); do headers+=( -H "$1" ); shift; done
  local out
  if [[ -n "$body" ]]; then
    out=$(curl -sS -o /tmp/sched.body -w '%{http_code}' -X "$method" "${headers[@]}" -d "$body" "$BASE_URL$path")
  else
    out=$(curl -sS -o /tmp/sched.body -w '%{http_code}' -X "$method" "${headers[@]}" "$BASE_URL$path")
  fi
  STATUS="$out"
  BODY=$(cat /tmp/sched.body)
}

# ---- 1. Login as DM and player ----
call POST /auth/login "" "{\"email\":\"$DM_EMAIL\",\"password\":\"$DM_PASSWORD\"}"
expect_status 200 "$STATUS" "DM login"
DM_TOKEN=$(echo "$BODY" | jq -r '.token')

call POST /auth/login "" "{\"email\":\"$PLAYER_EMAIL\",\"password\":\"$PLAYER_PASSWORD\"}"
expect_status 200 "$STATUS" "Player login"
PLAYER_TOKEN=$(echo "$BODY" | jq -r '.token')

# ---- 2. DM creates a campaign and a session ----
call POST /campaigns "$DM_TOKEN" "{\"name\":\"Smoke campaign $(date +%s)\",\"themeImage\":\"forest\"}"
expect_status 201 "$STATUS" "Create campaign"
CAMPAIGN_ID=$(echo "$BODY" | jq -r '.id')
DM_PLAYER_ID=$(echo "$BODY" | jq -r '.myPlayerId')

# Verify the wire shape carries no userId on members.
if echo "$BODY" | jq -e '.members[] | has("userId")' >/dev/null 2>&1; then
  echo "FAIL: members[] leaks userId" >&2; exit 1
fi
echo "OK: members[] has no userId"

call POST "/campaigns/$CAMPAIGN_ID/sessions" "$DM_TOKEN" '{"name":"Sess 1","description":"smoke"}'
expect_status 201 "$STATUS" "Create session"
SESSION_ID=$(echo "$BODY" | jq -r '.id')

# ---- 3. Player-before-DM availability → 409 ----
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$PLAYER_TOKEN" \
  '{"availabilityByDate":{"2026-05-04":{"morning":true,"noon":false,"evening":false}}}' \
  "X-Timezone: $TZ_HEADER"
expect_status 409 "$STATUS" "Player writing before DM bootstrap"
test "$(echo "$BODY" | jq -r '.code')" = "SCHEDULE_NOT_INITIALIZED" || { echo "FAIL: expected code SCHEDULE_NOT_INITIALIZED"; exit 1; }
echo "OK: SCHEDULE_NOT_INITIALIZED returned"

# ---- 4. DM bootstrap availability ----
# 4a. Missing X-Timezone on bootstrap → 400 INVALID_TIMEZONE
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$DM_TOKEN" \
  '{"availabilityByDate":{"2026-05-04":{"morning":true,"noon":false,"evening":false}}}'
expect_status 400 "$STATUS" "DM bootstrap missing X-Timezone"
test "$(echo "$BODY" | jq -r '.code')" = "INVALID_TIMEZONE" || { echo "FAIL: expected code INVALID_TIMEZONE"; exit 1; }
echo "OK: INVALID_TIMEZONE returned on bootstrap without header"

# 4b. Bad timezone string → 400 INVALID_TIMEZONE
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$DM_TOKEN" \
  '{"availabilityByDate":{"2026-05-04":{"morning":true,"noon":false,"evening":false}}}' \
  "X-Timezone: Not/A_Zone"
expect_status 400 "$STATUS" "DM bootstrap with bad X-Timezone"

# 4c. Bad date key → 400 INVALID_DATE_KEY
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$DM_TOKEN" \
  '{"availabilityByDate":{"05/04/2026":{"morning":true}}}' \
  "X-Timezone: $TZ_HEADER"
expect_status 400 "$STATUS" "DM bootstrap with bad date key"
test "$(echo "$BODY" | jq -r '.code')" = "INVALID_DATE_KEY" || { echo "FAIL: expected code INVALID_DATE_KEY"; exit 1; }

# 4d. Successful bootstrap.
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$DM_TOKEN" \
  '{"availabilityByDate":{"2026-05-04":{"morning":true,"noon":true,"evening":false}}}' \
  "X-Timezone: $TZ_HEADER"
expect_status 200 "$STATUS" "DM bootstrap availability"
test "$(echo "$BODY" | jq -r '.schedule.dmTimezone')" = "$TZ_HEADER" || { echo "FAIL: dmTimezone not echoed"; exit 1; }
test "$(echo "$BODY" | jq -r '.schedule.participantAvailabilities[0].playerId')" = "$DM_PLAYER_ID" || { echo "FAIL: DM playerId not in availabilities"; exit 1; }

# Critical: response must NOT carry a userId field on availabilities.
if echo "$BODY" | jq -e '.schedule.participantAvailabilities[] | has("userId")' >/dev/null 2>&1; then
  echo "FAIL: schedule entry leaks userId" >&2; exit 1
fi
echo "OK: schedule keyed by playerId only"

# ---- 5. Confirmed slot — DM happy path + DurationValidation ----
# Bad duration → 400 INVALID_DURATION
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/confirmed-slot" "$DM_TOKEN" \
  '{"date":"2026-05-04","dayPart":"morning","durationMinutes":0}' \
  "X-Timezone: $TZ_HEADER"
expect_status 400 "$STATUS" "Confirm slot with duration<=0"
test "$(echo "$BODY" | jq -r '.code')" = "INVALID_DURATION" || { echo "FAIL: expected code INVALID_DURATION"; exit 1; }

# Bad dayPart → 400 INVALID_DAY_PART
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/confirmed-slot" "$DM_TOKEN" \
  '{"date":"2026-05-04","dayPart":"midnight"}' \
  "X-Timezone: $TZ_HEADER"
expect_status 400 "$STATUS" "Confirm slot with bad dayPart"
test "$(echo "$BODY" | jq -r '.code')" = "INVALID_DAY_PART" || { echo "FAIL: expected code INVALID_DAY_PART"; exit 1; }

# Happy path
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/confirmed-slot" "$DM_TOKEN" \
  '{"date":"2026-05-04","dayPart":"morning","startAt":"2026-05-04T13:00:00Z","durationMinutes":180}' \
  "X-Timezone: $TZ_HEADER"
expect_status 200 "$STATUS" "DM confirms a slot"
test "$(echo "$BODY" | jq -r '.schedule.confirmedSlot.dayPart')" = "morning"
test "$(echo "$BODY" | jq -r '.schedule.confirmedSlot.confirmedAt')" != "null"

# ---- 6. Player tries to confirm-slot → 403 ----
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/confirmed-slot" "$PLAYER_TOKEN" \
  '{"date":"2026-05-04","dayPart":"noon"}'
expect_status 403 "$STATUS" "Player attempting confirm-slot"

# ---- 7. Player can now write own availability (schedule already bootstrapped) ----
# First, the player must be added to the campaign as a member. The membership
# story for invites is not part of this plan; if your local data has already
# linked PLAYER_EMAIL to this campaign as a player member, this step proceeds.
# If not, skip this step (echo "SKIP: player not a member") and re-run after
# joining the player.
call PUT "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$PLAYER_TOKEN" \
  '{"availabilityByDate":{"2026-05-04":{"morning":true,"noon":false,"evening":true}}}'
case "$STATUS" in
  200) echo "OK: player wrote availability" ;;
  404) echo "SKIP: player not a member of this campaign — re-run after joining" ;;
  *)   echo "FAIL: player availability write returned $STATUS"; exit 1 ;;
esac

# ---- 8. Cleanup ----
call DELETE "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/confirmed-slot" "$DM_TOKEN" ""
expect_status 204 "$STATUS" "Clear confirmed slot"

call DELETE "/campaigns/$CAMPAIGN_ID/sessions/$SESSION_ID/availability" "$DM_TOKEN" ""
expect_status 204 "$STATUS" "Clear DM availability"

echo
echo "All session-schedule API checks passed."
