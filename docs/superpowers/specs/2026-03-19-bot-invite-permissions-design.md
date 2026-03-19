# Bot Invite Permissions Flow - Design Spec

## Goal

Replace the current permission-less bot invite flow with one where developers declare requested permissions on their invite link, server admins review and edit those permissions before accepting, and a role is automatically created for the bot on join. Extend the in-chat URL embed system to handle bot invite links using a generalized multi-type embed architecture.

## Background

The existing flow already works end-to-end for adding bots without permissions: `bot_invite_tokens` stores a UUID per bot, `GET /api/bots/invite/{token}` resolves it to bot info, `POST /api/bots/invite/{token}/accept` adds the bot to a server. `BotInvitePage.tsx` and `BotInviteEmbed.tsx` handle the UI. All of this is preserved; this spec extends it.

Discord's flow is all-or-nothing: permissions are baked into the URL and most admins click Authorize without reviewing them. Our improvement: the developer declares *requested* permissions, but the admin sees them pre-checked and editable before confirming. The bot gets a role with whatever the admin actually grants.

## Architecture

### Data model

**Migration** — add one column to `bot_invite_tokens`:

```sql
ALTER TABLE bot_invite_tokens
  ADD COLUMN IF NOT EXISTS permissions BIGINT NOT NULL DEFAULT 0;
```

`permissions` is a bitmask using the same constants as server roles (`internal/permissions/permissions.go`). Default `0` keeps all existing invite tokens working with no behavior change.

No other schema changes. The bot's role is created at accept time using the existing `server_roles` and `server_member_roles` tables.

### Backend

**`BotInviteInfo` struct** (`internal/bots/models.go`) — add `Permissions int64 \`json:"permissions"\``.

**`UserBot` struct** (`internal/bots/models.go`) — add `Permissions int64 \`json:"permissions"\`` so the developer portal can display current requested permissions.

**`GET /api/bots/invite/{token}`** — scan the new `permissions` column and include it in the response. This endpoint is already public and unauthenticated; exposing `permissions` here is intentional — invite tokens are designed to be shared publicly and admins need to see the requested permissions before deciding whether to log in and add the bot.

**`GET /api/bots/mine`** — `GetUserBots` query in `bots/repository.go` already joins `bot_invite_tokens`; extend the `SELECT` to include `bit.permissions` and scan it into `UserBot.Permissions`.

**`PATCH /api/developer/bots/{botId}/invite`** — new endpoint. Use `{botId}` to match the existing rename route parameter name (`PATCH /developer/bots/{botId}`); `/invite` is a distinct sub-path so there is no chi routing conflict. Accepts `{ "permissions": <int64> }`. Rejects with `400` if `permissions < 0` or `permissions > (1<<42)-1`. Ownership check uses a targeted UPDATE: `UPDATE bot_invite_tokens SET permissions=$1 WHERE bot_user_id=$2 AND created_by=$3` — if zero rows affected, return `403`. Returns `204 No Content`. The invite token UUID does not change, so previously shared links automatically pick up the new permissions.

**`Service.AcceptInvite`** (`bots/service.go`) — change return type from `error` to `(int64, error)`, returning `botUserID` alongside any error. This matches the pattern already used by `Service.AddBot` and gives the handler the user ID needed for role assignment.

**`POST /api/bots/invite/{token}/accept`** — body gains `granted_permissions int64`. Rejects with `400` if `granted_permissions < 0` or `granted_permissions > (1<<42)-1`. The entire accept sequence runs in a single database transaction to prevent partial state (bot added without a role):

1. `AddBotToServer(ctx, serverID, botUserID)` — inserts `server_bots` and `server_members` rows
2. If `granted_permissions > 0`: `CreateServerRole(ctx, serverID, "@{botUsername}", "#99aab5", granted_permissions)` using the existing repo method — `hoist` and `position` default to `false`/`0` in the DB schema, which is correct for bot roles, no extra arguments needed
3. If a role was created: `AssignRoleToMember(ctx, serverID, botUserID, roleID)`

All three steps are inside one transaction; if any fails the whole sequence rolls back. `BroadcastToChannel` for `member_join` fires after the transaction commits. If `granted_permissions == 0`, only step 1 runs. The existing 409 conflict on re-add is unchanged.

### Frontend

**`bots.ts`** — type and call updates:
- `BotInviteInfo`: add `permissions: number`
- `UserBot`: add `permissions: number`
- `acceptBotInvite(token: string, serverId: number, grantedPermissions: bigint)` — adds `granted_permissions: Number(grantedPermissions)` to the request body. `Number()` is safe here because the max permission bit is 41, well within JS's 53-bit safe integer range.

**API boundary note**: When converting `permissions: number` from the API to a `bigint` for permission logic, use the existing `permFromNumber()` helper from `permissions.ts` (e.g. `permFromNumber(bot.permissions)`), consistent with the established pattern in the codebase.

**`BotInviteEmbed.tsx`** — add a permissions section between the bot header and server picker. The same component is used on `BotInvitePage` (standalone) and inline in chat — no separate variants, full interactivity in both contexts:
- `grantedPerms: bigint` state, initialized from `permFromNumber(bot.permissions)`
- If `bot.permissions === 0`: show a small muted line "No permissions requested"
- Otherwise: grouped checklist using `PERMISSION_CATEGORIES` from `permissions.ts`, checkbox per permission entry, checked if the bit is set in `grantedPerms`, fully editable by the admin, with description text shown on hover
- `handleAdd` passes current `grantedPerms` to `acceptBotInvite`

**Developer settings** — wherever `getMyBots()` renders bot cards, add a "Requested Permissions" section per bot:
- Same `PERMISSION_CATEGORIES` checklist, pre-populated via `permFromNumber(bot.permissions)`
- 500ms debounced save: any checkbox change schedules a `PATCH /api/developer/bots/{bot.id}/invite`. On error, show an inline error message ("Failed to save — try again") scoped to that bot card.
- Shows the copyable invite URL: `{window.location.origin}/invite/bot/{bot.invite_token}`

**`Message.tsx`** — replace the theme-specific URL extraction with a generic system. Regex literals with the `g`/`gi` flag are stateful (`lastIndex`). To avoid statefulness bugs across renders, create fresh `RegExp` instances inside each function call rather than reusing module-level instances:

```ts
type EmbedType = 'theme' | 'bot-invite';

const EMBED_PATTERN_DEFS: { type: EmbedType; source: string }[] = [
  { type: 'theme',      source: String.raw`https?://[^/\s]+/theme/([0-9a-f-]{36})` },
  { type: 'bot-invite', source: String.raw`https?://[^/\s]+/invite/bot/([0-9a-f-]{36})` },
];

// Returns deduplicated list of { type, token } matched in content.
// Fresh RegExp per call avoids lastIndex statefulness.
function extractEmbeds(content: string): { type: EmbedType; token: string }[] {
  const seen = new Set<string>();
  const results: { type: EmbedType; token: string }[] = [];
  for (const def of EMBED_PATTERN_DEFS) {
    const re = new RegExp(def.source, 'gi');
    let m: RegExpExecArray | null;
    while ((m = re.exec(content)) !== null) {
      const key = def.type + ':' + m[1];
      if (!seen.has(key)) { seen.add(key); results.push({ type: def.type, token: m[1] }); }
    }
  }
  return results;
}

function stripEmbedURLs(content: string): string {
  let out = content;
  for (const def of EMBED_PATTERN_DEFS) {
    out = out.replace(new RegExp(def.source, 'gi'), '');
  }
  return out.trim();
}
```

The render section maps each `{ type, token }` to `<ThemeLinkEmbed token={tok} />` or `<BotInviteEmbed token={tok} />`. Adding a future embed type requires one entry in `EMBED_PATTERN_DEFS` and one `case` branch — nothing else changes.

## Data flow

```
Developer:
  PATCH /api/developer/bots/{id}/invite  { permissions: 196608 }
    → updates bot_invite_tokens.permissions
    → invite URL unchanged: /invite/bot/{uuid}

User pastes URL in chat:
  Message.tsx extracts bot-invite token via EMBED_PATTERNS
  → renders <BotInviteEmbed token={uuid} />
  → GET /api/bots/invite/{uuid}  → { bot_id, username, ..., permissions: 196608 }
  → shows permissions checklist pre-checked

Admin clicks Add to Server:
  POST /api/bots/invite/{uuid}/accept  { server_id: 123, granted_permissions: 131072 }
    → resolves token → botUserID  (AcceptInvite returns (int64, error))
    → BEGIN TRANSACTION
    →   AddBotToServer (server_bots + server_members rows)
    →   CreateServerRole "@BotName" color="#99aab5" permissions=131072
    →   AssignRoleToMember botUserID → roleID
    → COMMIT
    → broadcasts member_join WS event
```

## Error handling

| Condition | HTTP status | UI |
|---|---|---|
| Invalid token | 404 | Embed shows "Bot Not Found" (unchanged) |
| Bot already in server | 409 | Embed shows "Bot is already in that server." (unchanged) |
| `granted_permissions` out of range (`< 0` or `> (1<<42)-1`) | 400 | Embed shows "Invalid permissions value" |
| Transaction failure (role creation / assignment) | 500 | Embed shows "Failed to add bot." (unchanged) |
| `PATCH` permissions by non-owner | 403 | Developer settings shows inline error |
| No servers available | — | Embed shows "You need to be an admin of a server to add bots" |

## What is not changing

- The token UUID format and generation (existing `bot_invite_tokens` rows are unaffected)
- The `server_bots` / `server_members` insertion logic
- `BotInvitePage.tsx` wrapper (just renders `BotInviteEmbed`, no changes needed)
- The official Parley bot (`polly`) seeded invite token — it will get `permissions = 0` which means no permissions requested, consistent with current behavior
- All existing bot management endpoints (`ListBots`, `RemoveBot`, `GetAIConfig`, etc.)
- The existing `PATCH /developer/bots/{botId}` rename endpoint (different sub-path, no routing conflict)
