---
name: deleting-orphan-users
description: Use when asked to find, clean up, or delete rolebook users who belong to no campaign — "orphan", "unused", "empty", or "inactive" accounts, accounts with no DM or player membership, or pruning the users collection.
---

# Deleting Orphan Users

## Overview

An **orphan user** is a rolebook account tied to no campaign. Deleting them is a sharp tool: there is no `createdAt` on users, no soft-delete, and a single mistyped filter can wipe real accounts. The danger is bundling *detection* and *deletion* into one unattended pass (e.g. `DeleteMany({_id: {$nin: keep}})`). This skill keeps them apart.

**Core principle:** detect read-only → a human approves each user in chat → back up → delete an explicit, re-verified ID list. You are the gate; never automate the gate away.

There is **no committed tool** for this. `mongosh` is not installed. You run an ad-hoc Go program (reference: `orphan-tool.go.txt`) and delete it afterward — the capability must not live in the repo.

## When to use

- "Delete users that aren't in any campaign", "remove unused/empty accounts", "prune the users collection", "clean up orphaned users".

**When NOT to use:** deleting a *specific named* user (just delete that one `_id` after the same backup + admin check); anything touching campaigns, players, or other collections.

## Definition of orphan (exact)

A user `_id` is an orphan iff it appears in **none** of:

1. `campaigns.members[].userId` — campaign membership (DM or player; `Distinct` flattens the array).
2. `players.linkedUserId` — a character record tied to the user. **A user who still owns a player is NOT empty — never delete them.**
3. `ADMIN_USER_IDS` (env) — admins legitimately have zero campaigns. **Never delete an admin.**

`campaign_locations/npcs/map_pins.ownerUserId` owners are always members, so they add nobody — ignore them.

## Procedure

1. **Detect (read-only).** Copy `orphan-tool.go.txt` to `cmd/orphan-scratch/main.go`. Run detect mode (see that file's header). It prints `ORPHAN  <id>  <email>` and counts. It writes nothing.
2. **Present + per-user approval.** Show the candidates to the human and get approval **one user at a time** in chat. Do not batch-approve. Do not proceed on silence. The human's explicit yes per user is the only thing that moves an ID onto the delete list.
3. **Cap.** Confirm the count is within the per-run cap (`-max`, default 25). More than that → stop and ask the human to confirm a higher cap deliberately; never raise it silently.
4. **Back up, then delete.** Run delete mode with the **explicit approved IDs only** and a timestamped `-backup` path. The tool backs up the full docs first (aborting if it can't), re-verifies each ID is *still* an orphan (rejecting any now-member/player/admin), then deletes by exact `_id`, one at a time.
5. **Verify + clean up.** Re-run detect (approved users gone, count dropped by exactly that many). Then `rm -rf cmd/orphan-scratch`, confirm `git status` shows it gone, and tell the human where the backup file is (it contains password hashes — sensitive; keep it safe / delete once recovery is no longer needed).

## Hard rules

- **Never delete by negative filter.** No `DeleteMany({_id: {$nin: ...}})`, no re-deriving the orphan set at delete time. Delete only an explicit ID list captured from a reviewed detect run. (A live `$nin` deletes accounts created *after* you reviewed.)
- **Never skip the backup.** No backup file written → no deletes.
- **Never skip per-user human approval.** No "approve all", no acting on silence.
- **Never delete an admin or a user with a linked player**, even if asked to "just delete them all".
- **Never commit the scratch tool**, a backup file, or leave `cmd/orphan-scratch` behind.

## Red flags — STOP

| Thought | Reality |
|---------|---------|
| "I'll just `DeleteMany` the `$nin` set" | Races with new signups; deletes unreviewed accounts. Explicit IDs only. |
| "The list is short, I'll approve it all at once" | Per-user approval is the gate. One at a time. |
| "Backup is overkill for a few users" | No `createdAt`, no undo. Backup or don't delete. |
| "This admin clearly has no campaign, delete it" | Admins are intentionally campaign-less. Never delete. |
| "I'll add a `cmd/` tool so it's reusable" | A committed mass-delete tool is exactly what we removed. Scratch only. |

## Reference

`orphan-tool.go.txt` — the detect/delete program. Its header documents both invocations. Copy it into `cmd/orphan-scratch/main.go` to run; delete the directory when done.
