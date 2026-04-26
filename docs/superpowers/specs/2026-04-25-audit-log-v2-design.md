# Server Audit Log v2 — Design Spec

**Date:** 2026-04-25
**Status:** Approved (solo design — user delegated)
**Builds on:** `2026-03-21-server-audit-log-design.md` (v1, shipped)

---

## Goal

The v1 audit log shipped but feels thin: only ~15 actions are logged, update entries snapshot 2-3 fields each, and the UI shows flat text rows like "dh updated channel general" with no diff. v2 closes those gaps: cover the privileged actions v1 missed, add reasons + target search, and render entries with avatars and human-readable diffs.

Three concrete weaknesses:

1. **Coverage gaps** — Permission overwrites, soundboard CRUD, bot add/remove, AI config changes, server-VC force-mute/disconnect, and category assignments are all unaudited today. ~10 new actions needed.
2. **Reason support is asymmetric** — Only `member.ban` accepts a reason. Kicks, unbans, role/channel deletes should accept one too, and the UI should prompt for it.
3. **UI is flat** — Actor and target render as plain usernames; permission diffs render as raw bitfield ints (`524288`); booleans show as `true`/`false`. No target search.

---

## Non-Goals

- No new "delete log entries" admin action — entries are append-only and survive resource deletion (already true in v1).
- No diff for `channel.reorder` — too noisy and rarely audited in practice. The list of reordered IDs is uninteresting at a glance.
- No diff for `server.update` icon URL beyond before/after URL — image preview UI is out of scope.
- No bin / paste actions — bin is user-content, not a privileged server resource.
- No retention / pruning. Append-only forever, like v1.
- No real-time push of new audit entries via WS. Refresh-on-open is fine for now.

---

## Architecture

Same skeleton as v1: handler/service writes one `audit.Entry` after the primary action succeeds; failures log + swallow. v2 only adds new call sites and new fields to existing rows — no new tables, no schema changes to `server_audit_logs`.

The two changes that touch shared infrastructure:

1. **`ListAuditLogs` joins users** to return `actor_avatar_url` (and `target_avatar_url` when target_type='user'). Read-only optimization, no schema change.
2. **`ListAuditLogs` accepts a `target` filter** (substring match on `target_name` via `ILIKE`). New optional query param `target=<substr>`.

---

## New Action Strings

| Action | Trigger handler | Target | Changes shape | Reason? |
|--------|-----------------|--------|---------------|---------|
| `channel.overwrite_set` | `UpsertOverwrite` | channel (TargetID/Name = channel) | `{overwrite_target_type, overwrite_target_id, overwrite_target_name, before:{allow,deny}, after:{allow,deny}}` — when no prior overwrite existed, `before:{allow:0, deny:0}` | no |
| `channel.overwrite_delete` | `DeleteOverwrite` | channel | `{overwrite_target_type, overwrite_target_id, overwrite_target_name, before:{allow,deny}}` | no |
| `soundboard.create` | `Upload` | sound (TargetID = sound id, Name = sound name) | nil | no |
| `soundboard.update` | `UpdateSound` | sound | `{before:{name,emoji}, after:{name,emoji}}` | no |
| `soundboard.delete` | `DeleteSound` | sound | nil | no |
| `bot.add` | `AddBot` (developer adds own bot) AND `AcceptInvite` (third party accepts bot invite) — both fire one entry each | user (TargetID = bot user id, Name = bot username) | `{granted_permissions: int64}` for AcceptInvite path only; nil for AddBot path | no |
| `bot.remove` | `RemoveBot` | user (bot) | nil | no |
| `bot.ai_config_update` | `SetAIConfig` | server (TargetType="server") | `{before:{provider,model,preset_verbosity,preset_personality,preset_role,has_api_key}, after:{...}}` | no |
| `voice.force_mute` | `MuteParticipant` | user | nil | no |
| `voice.force_disconnect` | `KickParticipant` | user | nil | no |
| `server.categories_update` | `SetServerCategories` | server | `{before:[id...], after:[id...]}` | no |

`bot.ai_config_update` deliberately stores `has_api_key: bool` rather than the key itself — never log secrets.

For `voice.force_mute` and `voice.force_disconnect`, only fire the audit log when `vc.Kind == KindServer`. DM-VC moderation is between the GC owner and members — no server context, no server audit log.

---

## Reason Field Expansion

v1 has `audit.Entry.Reason` and a column for it; only `member.ban` populates it. v2 lets the actor optionally provide a reason for these additional actions, captured from request body and persisted on the audit row:

| Action | Where reason is captured |
|--------|--------------------------|
| `member.kick` | `KickMember` handler reads `{"reason": "..."}` from body (optional, like ban already does) |
| `member.unban` | `UnbanMember` handler reads body |
| `role.delete` | `DeleteServerRole` handler reads body |
| `channel.delete` | `DeleteChannel` handler reads body |

Empty/missing reason is fine — column allows it, frontend hides the row if empty (already does).

**Method-signature changes:**
```go
KickMember(ctx, serverID, userID, actorID, actorUsername, reason string) error
UnbanMember(ctx, serverID, userID, actorID, actorUsername, reason string) error
DeleteServerRole(ctx, serverID, roleID, actorID, actorUsername, reason string) error
// channel.delete is logged at handler level (Pattern A); pass reason directly into the Log() call.
```

---

## Backend Changes

### `internal/db/repository.go`

`ListAuditLogs` rewritten to:
- Accept `target` (substring) as a new optional filter.
- LEFT JOIN `users actor` on `actor_id` to get `actor_avatar_url` (NULL when actor was deleted).
- LEFT JOIN `users target` on `target_id::bigint` *only when target_type='user'* (use a `CASE` expression — not all target_ids parse as bigints).
- Returned `AuditLog` struct gains `ActorAvatarURL string` and `TargetAvatarURL string`.

```go
func (r *Repository) ListAuditLogs(ctx context.Context,
    serverID int64, actorID *int64, action, target string,
    limit, offset int) ([]AuditLog, int, error)
```

Index `idx_sal_action` and `idx_sal_actor` already cover the existing filters; the new `target ILIKE` is a sequential scan over the page-window result set after WHERE, which is acceptable at audit-log scale.

### Handler updates (Pattern A — handler-level Log calls)

- `internal/channel/overwrites.go` — both `UpsertOverwrite` and `DeleteOverwrite` get audit calls. Need to fetch overwrite-target name (role name from `GetServerRoleByID` or username from `GetUsernameByID` based on `target_type`) and (for upsert) the previous overwrite for the diff.
- `internal/soundboard/handler.go` — `Upload`, `UpdateSound`, `DeleteSound` get audit calls. Constructor takes new `auditSvc *audit.AuditService`.
- `internal/bots/handler.go` — `AddBot`, `RemoveBot`, `AcceptInvite`, `SetAIConfig` get audit calls. Constructor takes new `auditSvc *audit.AuditService`.
- `internal/voice/handler.go` — `MuteParticipant`, `KickParticipant`. For server VCs: lookup `ch.ServerID` via `repo.GetChannelByID(vc.ID)`, then audit. Skip for DM VCs. Constructor takes new `auditSvc *audit.AuditService`.
- `internal/server/handler.go` — `SetServerCategories` and `KickMember`/`UnbanMember`/`DeleteServerRole`/`DeleteChannel` (the last via channel handler) gain a reason in their request body; pass through to service.

### Service updates (Pattern B)

- `KickMember`, `UnbanMember`, `DeleteServerRole` gain a trailing `reason string` param and pass it to `audit.Entry.Reason`.

### Constructor wiring (`cmd/api/main.go`)

`NewSoundboardHandler`, `NewBotsHandler`, `NewVoiceHandler` constructors gain `auditSvc *audit.AuditService` parameter (matching the v1 pattern from `NewChannelHandler`). Update call sites in `main.go`.

---

## Frontend Changes

### `frontend/src/api/audit.ts`

```ts
export interface AuditLogEntry {
  id: string;
  server_id: string;
  actor_id?: string;
  actor_username: string;
  actor_avatar_url?: string;     // NEW
  action: string;
  target_id?: string;
  target_type?: string;
  target_name?: string;
  target_avatar_url?: string;    // NEW (only when target_type='user')
  changes?: { before?: Record<string, unknown>; after?: Record<string, unknown> } | Record<string, unknown>;
  reason?: string;
  created_at: string;
}

export async function getAuditLog(
  serverId: string,
  params: { limit?: number; offset?: number; action?: string; actorId?: string; target?: string }
): Promise<{ logs: AuditLogEntry[]; total: number }>;
```

The `changes` shape becomes a discriminated union: most update entries still use `{before, after}`, but new entries like `channel.overwrite_set` use a flat-keyed shape (`{overwrite_target_name, before, after}`). Renderer must handle both.

### `frontend/src/components/settings/AuditLogTab.tsx`

Add the new rows + visual upgrade — keep filter UI structure but extend it.

**New filter:** target search input. Mirrors actor search (client-side substring match against `target_name`). Cleared when action filter changes.

**New action `<optgroup>`s** in the filter `<select>`:
- Channels: append `channel.overwrite_set`, `channel.overwrite_delete`
- Soundboard (new group): `soundboard.create`, `soundboard.update`, `soundboard.delete`
- Bots (new group): `bot.add`, `bot.remove`, `bot.ai_config_update`
- Voice (new group): `voice.force_mute`, `voice.force_disconnect`
- Server: append `server.categories_update`

**`describe()` extensions** (one switch case per new action; mirror the v1 phrasings).

**Row visual upgrade:**
- Replace the emoji icon with a 28px circular `<img>` of the actor's avatar (fallback emoji when `actor_avatar_url` is empty).
- For target = user (kick/ban/role-add/role-remove/voice-force-*), render the target's avatar inline beside their name.
- Action category color stripe on the left of each row (channels=blue, members=red, roles=purple, server=gray, soundboard=teal, bots=orange, voice=pink). 4px border-left.

**Diff renderer (`ChangeDiff`) overhaul:**
- Support both `{before, after}` shape and flat-keyed shapes (e.g., overwrite entries).
- For keys named `permissions`, `allow`, `deny` (any int64-bitfield), decode bits to permission name list and render added bits with `+` prefix and removed bits with `-` prefix using `frontend/src/lib/permissions.ts` constants.
- For `color`, render the hex code beside a 12×12 swatch.
- For `is_public` and other booleans, render "Yes" / "No" instead of `true` / `false`.
- For `granted_permissions` (bot.add via invite), use the same permission-bitfield decoder.
- For `before:[id...]` / `after:[id...]` arrays in `server.categories_update`, just show counts ("3 categories → 2 categories"); details would need a categories lookup that's not worth it here.

### `frontend/src/components/settings/MembersTab.tsx`, `ChannelPermissions.tsx`, etc.

For the **reason prompt** UX: each destructive-action confirmation modal that today only confirms now includes an optional `<textarea placeholder="Reason (optional)">`. Plumb the value through to the API call.

Affected confirm flows:
- Kick member (MembersTab)
- Unban member (MembersTab — bans subview)
- Delete role (RolesTab — needs a new tab; **currently no UI** for role deletion exists, so this is "later" — just plumb the API arg now and leave UI work for whoever builds the role delete flow)
- Delete channel (channel context menu, wherever that lives — likely `ChannelList.tsx`)

If a confirm flow has no existing modal, don't add one just for the reason — pass `reason: ""` and call it good. The backend accepts empty.

---

## Files Touched

| File | Change |
|------|--------|
| `internal/db/repository.go` | `ListAuditLogs` — new `target` filter, JOIN `users` for actor + target avatars; `AuditLog` struct gains `ActorAvatarURL`, `TargetAvatarURL` |
| `internal/server/handler.go` | `ListAuditLogs` handler reads new `target` query param; `KickMember`/`UnbanMember`/`SetServerCategories` handlers read `reason` from body where applicable, plus audit log for `server.categories_update` |
| `internal/server/server_members.go` | `KickMember`, `UnbanMember` gain trailing `reason string` param; pass to `audit.Entry.Reason` |
| `internal/server/server_roles.go` | `DeleteServerRole` gains trailing `reason string` param; pass to `audit.Entry.Reason` |
| `internal/server/server_crud.go` | `SetServerCategories` returns `(beforeIDs, afterIDs []int64, err)` so the handler can build the diff — service stays free of audit imports |
| `internal/channel/handler.go` | `DeleteChannel` reads `reason` from body, passes to audit log |
| `internal/channel/overwrites.go` | New audit calls in `UpsertOverwrite` + `DeleteOverwrite`; needs auditSvc field; `Handler` struct + `NewHandler` already shared with v1 channel handler |
| `internal/soundboard/handler.go` | `auditSvc` field added to constructor + struct; audit calls in `Upload`/`UpdateSound`/`DeleteSound` |
| `internal/bots/handler.go` | `auditSvc` field added; audit calls in `AddBot`/`RemoveBot`/`AcceptInvite`/`SetAIConfig` (with provider/model/preset diff) |
| `internal/voice/handler.go` | `auditSvc` field added; audit calls in `MuteParticipant`/`KickParticipant` only when `vc.Kind == KindServer` |
| `cmd/api/main.go` | Pass `auditSvc` into 4 constructors above |
| `frontend/src/api/audit.ts` | Extended `AuditLogEntry` (avatar URLs); `getAuditLog` accepts `target` param |
| `frontend/src/components/settings/AuditLogTab.tsx` | Target search input; new action options in select; `describe()` cases for new actions; avatar-based actor/target rendering; permission-bitfield + color-swatch + boolean diff renderer |
| `frontend/src/components/settings/MembersTab.tsx` | Optional reason textarea in kick/unban confirm modals; pass through to API |
| `frontend/src/api/servers.ts` | `kickMember(serverId, userId, reason?)` and `unbanMember(serverId, userId, reason?)` accept optional reason |
| `frontend/src/api/channels.ts` | `deleteChannel(id, reason?)` accepts optional reason |

---

## Error Handling

Same as v1:
- Audit write failures: `log.Printf` + ignore.
- API endpoint: 403 if caller lacks `PermViewAuditLog` and is not owner.
- Malformed query params: silently use defaults.
- New `target` param: empty string = no filter, otherwise treated as substring. Use `escapeLike(query)` from `internal/db/user_repository.go:645` (existing helper) and pass with `ESCAPE '\'` clause — same pattern used by `admin_repository.go:183`, `discovery_repository.go:139`, `message_repository.go:353`.

---

## Test Plan

Backend unit/integration:
- `repository_test.go` — `ListAuditLogs` with `target` filter returns matching subset; with non-matching filter returns empty.
- For each new action, integration test: trigger handler → assert one row in `server_audit_logs` with expected `action`, `target_*`, `changes` JSONB.
- For `voice.force_mute`/`voice.force_disconnect`: trigger on a DM VC → assert no audit row written.
- For `bot.ai_config_update`: snapshot has `has_api_key: true/false` boolean, never the key string.

Frontend smoke:
- Audit Log tab loads after each new action and renders the new row with permission names (not raw bitfield int).
- Target search filters in-memory list.
- Kick member with reason populates `reason` column.

---

## Out-of-Scope Cleanup

- Move the inline `style={{ ... }}` props in `AuditLogTab.tsx` into a real CSS file. Worth doing while we're in there — current row styling is unmaintainable. Add `frontend/src/components/settings/AuditLogTab.css`.
- Migrate the `_isOwner` `void` line at the top of `AuditLogTab.tsx` away — it's a leftover from v1's permission gating that's no longer needed.
