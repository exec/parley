# Bot Invite Permissions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a permissions bitmask to bot invite tokens so developers can declare requested permissions, admins can edit and grant them at accept time, and the bot automatically gets a role with those permissions.

**Architecture:** DB column `bot_invite_tokens.permissions` stores requested permissions. The accept endpoint accepts `granted_permissions`, runs a single transaction (add bot + create role + assign role). Frontend adds a permissions checklist to `BotInviteEmbed` and a debounced permissions manager to `DeveloperTab`. `Message.tsx` embed extraction is generalized to a pattern array that supports both theme and bot-invite URLs.

**Tech Stack:** Go (database/sql, chi router), PostgreSQL, React + TypeScript

---

## File Map

| File | Change |
|---|---|
| `internal/db/migrations.go` | Add migration: `ALTER TABLE bot_invite_tokens ADD COLUMN IF NOT EXISTS permissions BIGINT NOT NULL DEFAULT 0` |
| `internal/bots/models.go` | Add `Permissions int64` to `BotInviteInfo` and `UserBot` |
| `internal/bots/repository.go` | Update `ResolveInviteToken` to return `(int64, int64, error)` (botUserID + permissions); update `GetUserBots` query to scan `bit.permissions`; add `UpdateBotInvitePermissions`; add `AddBotToServerWithRole` (transactional) |
| `internal/bots/service.go` | Change `AcceptInvite` to accept `grantedPermissions int64`, return `(int64, error)`; call `AddBotToServerWithRole` instead of `AddBotToServer`; add `UpdateInvitePermissions` |
| `internal/bots/handler.go` | Update `AcceptInvite` handler to read `granted_permissions`; add `UpdateInvitePermissions` handler |
| `cmd/api/routes.go` | Register `PATCH /developer/bots/{botId}/invite` |
| `frontend/src/api/bots.ts` | Add `permissions: number` to `BotInviteInfo` + `UserBot`; update `acceptBotInvite` to accept `grantedPermissions: bigint`; add `updateBotInvitePermissions` |
| `frontend/src/components/BotInviteEmbed.tsx` | Add permissions checklist between header and server picker |
| `frontend/src/components/settings/DeveloperTab.tsx` | Add "My Bots" section with permissions picker + invite URL copy |
| `frontend/src/components/chat/Message.tsx` | Replace `THEME_URL_RE` / `extractThemeTokens` / `stripThemeURLs` with `EMBED_PATTERN_DEFS` + `extractEmbeds` + `stripEmbedURLs`; render `BotInviteEmbed` for bot-invite tokens |

---

## Task 1: DB Migration — add `permissions` column

**Files:**
- Modify: `internal/db/migrations.go`

- [ ] **Step 1: Add the migration entry**

At the end of the `Migrations` slice in `internal/db/migrations.go`, append:

```go
`ALTER TABLE bot_invite_tokens
  ADD COLUMN IF NOT EXISTS permissions BIGINT NOT NULL DEFAULT 0;`,
```

Check how `db.RunMigrations` works before adding to confirm it's append-only safe.

- [ ] **Step 2: Build to verify no syntax errors**

```bash
go build ./...
```
Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat: add permissions column to bot_invite_tokens"
```

---

## Task 2: Go models — add `Permissions` field

**Files:**
- Modify: `internal/bots/models.go`

- [ ] **Step 1: Add `Permissions int64 \`json:"permissions"\`` to `BotInviteInfo`**

In `internal/bots/models.go`, add the field after `IsVerified` in `BotInviteInfo`:

```go
Permissions int64  `json:"permissions"`
```

- [ ] **Step 2: Add `Permissions int64 \`json:"permissions"\`` to `UserBot`**

Add the same field after `InviteToken` in `UserBot`.

- [ ] **Step 3: Build**

```bash
go build ./...
```
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/bots/models.go
git commit -m "feat: add Permissions field to BotInviteInfo and UserBot"
```

---

## Task 3: Repository — scan permissions, add helpers

**Files:**
- Modify: `internal/bots/repository.go`

- [ ] **Step 1: Update `ResolveInviteToken` to also return permissions**

Change signature from `(int64, error)` to `(int64, int64, error)`.
Change query from `SELECT bot_user_id FROM bot_invite_tokens WHERE token=$1::uuid`
to `SELECT bot_user_id, permissions FROM bot_invite_tokens WHERE token=$1::uuid`.
Scan into two variables `botUserID, permissions int64`. Return `botUserID, permissions, err`.

- [ ] **Step 2: Update `GetUserBots` query and scan**

Add `bit.permissions` to the SELECT list (after `bit.token::text`).
Add `&b.Permissions` to the `rows.Scan(...)` call (after `&b.InviteToken`).

- [ ] **Step 3: Add `UpdateBotInvitePermissions`**

```go
func (r *Repository) UpdateBotInvitePermissions(ctx context.Context, botUserID, callerID, permissions int64) error {
    result, err := r.db.ExecContext(ctx,
        `UPDATE bot_invite_tokens SET permissions=$1 WHERE bot_user_id=$2 AND created_by=$3`,
        permissions, botUserID, callerID)
    if err != nil {
        return err
    }
    n, _ := result.RowsAffected()
    if n == 0 {
        return ErrNotFound
    }
    return nil
}
```

- [ ] **Step 4: Add `AddBotToServerWithRole` (transactional)**

```go
func (r *Repository) AddBotToServerWithRole(ctx context.Context, serverID, botUserID int64, botUsername string, grantedPermissions int64) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    _, err = tx.ExecContext(ctx,
        `INSERT INTO server_bots (server_id, bot_user_id) VALUES ($1, $2)`,
        serverID, botUserID)
    if isPgUniqueViolation(err) {
        return ErrAlreadyExists
    }
    if err != nil {
        return err
    }

    _, err = tx.ExecContext(ctx,
        `INSERT INTO server_members (server_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
        serverID, botUserID)
    if err != nil {
        return err
    }

    if grantedPermissions > 0 {
        var roleID int64
        err = tx.QueryRowContext(ctx,
            `INSERT INTO server_roles (server_id, name, color, permissions)
             VALUES ($1, $2, $3, $4)
             RETURNING id`,
            serverID, "@"+botUsername, "#99aab5", grantedPermissions,
        ).Scan(&roleID)
        if err != nil {
            return err
        }
        _, err = tx.ExecContext(ctx,
            `INSERT INTO server_member_roles (server_id, user_id, role_id)
             VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
            serverID, botUserID, roleID)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```
Expected: compile errors in `service.go` because `ResolveInviteToken` now returns 3 values — fix in Task 4.

- [ ] **Step 6: Commit**

```bash
git add internal/bots/repository.go
git commit -m "feat: update bot repo — scan permissions, add UpdateBotInvitePermissions and AddBotToServerWithRole"
```

---

## Task 4: Service — wire permissions into AcceptInvite, add UpdateInvitePermissions

**Files:**
- Modify: `internal/bots/service.go`

- [ ] **Step 1: Add `ErrBadRequest` sentinel**

Add to the error vars near the top of `service.go` (alongside `ErrForbidden`):
```go
var ErrBadRequest = errors.New("bad request")
```

- [ ] **Step 2: Fix `AddBot` — update ResolveInviteToken call**

Change `botUserID, err := s.repo.ResolveInviteToken(ctx, inviteToken)` to `botUserID, _, err := s.repo.ResolveInviteToken(ctx, inviteToken)`.

- [ ] **Step 3: Fix `ResolveInvite` — pass permissions through**

Change `botUserID, err := s.repo.ResolveInviteToken(ctx, token)` to `botUserID, permissions, err := s.repo.ResolveInviteToken(ctx, token)`.
After `info, err := s.repo.GetBotInfo(ctx, botUserID)`, add `info.Permissions = permissions`.

- [ ] **Step 4: Update `AcceptInvite` to `(int64, error)` with grantedPermissions**

Full new signature and body:

```go
func (s *Service) AcceptInvite(ctx context.Context, token string, serverID, callerID int64, grantedPermissions int64) (int64, error) {
    const maxPerms = (int64(1) << 42) - 1
    if grantedPermissions < 0 || grantedPermissions > maxPerms {
        return 0, ErrBadRequest
    }
    isMember, err := s.repo.IsServerMember(ctx, serverID, callerID)
    if err != nil {
        return 0, err
    }
    if !isMember {
        return 0, ErrNotFound
    }
    if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
        return 0, err
    }
    botUserID, _, err := s.repo.ResolveInviteToken(ctx, token)
    if errors.Is(err, ErrNotFound) {
        return 0, ErrNotFound
    }
    if err != nil {
        return 0, err
    }
    info, err := s.repo.GetBotInfo(ctx, botUserID)
    if err != nil {
        return 0, err
    }
    if err := s.repo.AddBotToServerWithRole(ctx, serverID, botUserID, info.Username, grantedPermissions); err != nil {
        return 0, err
    }
    return botUserID, nil
}
```

- [ ] **Step 5: Add `UpdateInvitePermissions`**

```go
func (s *Service) UpdateInvitePermissions(ctx context.Context, botUserID, callerID, permissions int64) error {
    const maxPerms = (int64(1) << 42) - 1
    if permissions < 0 || permissions > maxPerms {
        return ErrBadRequest
    }
    err := s.repo.UpdateBotInvitePermissions(ctx, botUserID, callerID, permissions)
    if errors.Is(err, ErrNotFound) {
        return ErrForbidden
    }
    return err
}
```

- [ ] **Step 6: Build**

```bash
go build ./...
```
Expected: compile errors in `handler.go` only — fix in Task 5.

- [ ] **Step 7: Commit**

```bash
git add internal/bots/service.go
git commit -m "feat: update AcceptInvite to accept grantedPermissions and return botUserID; add UpdateInvitePermissions"
```

---

## Task 5: Handler — update AcceptInvite, add UpdateInvitePermissions

**Files:**
- Modify: `internal/bots/handler.go`

- [ ] **Step 1: Update `handleSvcErr` to handle `ErrBadRequest`**

Add inside the switch:
```go
case errors.Is(err, ErrBadRequest):
    httputil.JSONError(w, "invalid permissions value", http.StatusBadRequest)
```

- [ ] **Step 2: Update `AcceptInvite` handler**

Replace the current `AcceptInvite` handler body with:

```go
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
    uid, ok := callerID(r)
    if !ok {
        httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    token := chi.URLParam(r, "token")
    var req struct {
        ServerID           int64 `json:"server_id"`
        GrantedPermissions int64 `json:"granted_permissions"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ServerID == 0 {
        httputil.JSONError(w, "server_id required", http.StatusBadRequest)
        return
    }
    botUserID, err := h.svc.AcceptInvite(r.Context(), token, req.ServerID, uid, req.GrantedPermissions)
    if err != nil {
        handleSvcErr(w, r, err)
        return
    }
    if h.hub != nil {
        payload, _ := json.Marshal(map[string]string{
            "server_id": fmt.Sprintf("%d", req.ServerID),
            "user_id":   fmt.Sprintf("%d", botUserID),
        })
        h.hub.BroadcastToChannel(fmt.Sprintf("server:%d", req.ServerID), ws.EventMemberJoin, payload)
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Add `UpdateInvitePermissions` handler**

```go
func (h *Handler) UpdateInvitePermissions(w http.ResponseWriter, r *http.Request) {
    uid, ok := callerID(r)
    if !ok {
        httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
        return
    }
    botID, err := strconv.ParseInt(chi.URLParam(r, "botId"), 10, 64)
    if err != nil {
        httputil.JSONError(w, "invalid bot id", http.StatusBadRequest)
        return
    }
    var req struct {
        Permissions int64 `json:"permissions"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
        return
    }
    if err := h.svc.UpdateInvitePermissions(r.Context(), botID, uid, req.Permissions); err != nil {
        handleSvcErr(w, r, err)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Build**

```bash
go build ./...
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/bots/handler.go
git commit -m "feat: update AcceptInvite handler; add UpdateInvitePermissions handler"
```

---

## Task 6: Route registration

**Files:**
- Modify: `cmd/api/routes.go`

- [ ] **Step 1: Register new PATCH route**

Find line `r.Patch("/developer/bots/{botId}", handleRenameBotUser(repo))` (line ~283).
Add immediately after it:
```go
r.Patch("/developer/bots/{botId}/invite", botsHandler.UpdateInvitePermissions)
```

- [ ] **Step 2: Build**

```bash
go build ./...
```
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add cmd/api/routes.go
git commit -m "feat: register PATCH /developer/bots/{botId}/invite route"
```

---

## Task 7: Frontend API layer

**Files:**
- Modify: `frontend/src/api/bots.ts`

- [ ] **Step 1: Add `permissions: number` to `BotInviteInfo` interface**

Add `permissions: number;` after `is_verified: boolean;`.

- [ ] **Step 2: Add `permissions: number` to `UserBot` interface**

Add `permissions: number;` after `invite_token: string;`.

- [ ] **Step 3: Update `acceptBotInvite` to pass `grantedPermissions`**

Change:
```typescript
export const acceptBotInvite = (token: string, serverId: number) =>
  apiClient.post<void>(`/bots/invite/${token}/accept`, { server_id: serverId });
```
To:
```typescript
export const acceptBotInvite = (token: string, serverId: number, grantedPermissions: bigint) =>
  apiClient.post<void>(`/bots/invite/${token}/accept`, {
    server_id: serverId,
    granted_permissions: Number(grantedPermissions),
  });
```

`Number()` is safe here because the max permission bit is 41, within JS's 53-bit safe integer range.

- [ ] **Step 4: Add `updateBotInvitePermissions`**

```typescript
export const updateBotInvitePermissions = (botId: number, permissions: bigint) =>
  apiClient.patch<void>(`/developer/bots/${botId}/invite`, {
    permissions: Number(permissions),
  });
```

- [ ] **Step 5: Build TypeScript**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -40
```
Expected: type errors at `acceptBotInvite` call sites (missing 3rd arg) — fixed in Tasks 8+.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/bots.ts
git commit -m "feat: add permissions to BotInviteInfo/UserBot types, update acceptBotInvite, add updateBotInvitePermissions"
```

---

## Task 8: BotInviteEmbed — permissions checklist

**Files:**
- Modify: `frontend/src/components/BotInviteEmbed.tsx`
- Read first: `frontend/src/components/EmbedCard.tsx` (check how children/props work)

- [ ] **Step 1: Read EmbedCard.tsx**

Before coding, read `frontend/src/components/EmbedCard.tsx` to understand how it accepts children.

- [ ] **Step 2: Add import for permissions utilities**

```typescript
import { PERMISSION_CATEGORIES, permFromNumber } from '../lib/permissions';
```

- [ ] **Step 3: Add `grantedPerms` state and initialize from bot data**

Add state: `const [grantedPerms, setGrantedPerms] = useState<bigint>(0n);`

Update the `resolveBotInvite` effect to also set `grantedPerms`:
```typescript
resolveBotInvite(token)
  .then(b => {
    setBot(b);
    setGrantedPerms(permFromNumber(b.permissions));
  })
  .catch(() => setInvalid(true));
```

- [ ] **Step 4: Add toggle helper**

```typescript
const togglePerm = (bit: bigint) => {
  setGrantedPerms(prev => (prev & bit) !== 0n ? prev & ~bit : prev | bit);
};
```

- [ ] **Step 5: Add permissions section JSX**

Between the bot header and the server selector, add:

```tsx
{bot.permissions === 0 ? (
  <p style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)', margin: '0 0 8px' }}>
    No permissions requested
  </p>
) : (
  <div style={{ marginBottom: 10 }}>
    <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--parley-text-muted,#888)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
      Requested Permissions
    </div>
    {PERMISSION_CATEGORIES.map(cat => {
      const relevant = cat.permissions.filter(p => (permFromNumber(bot.permissions) & p.bit) !== 0n);
      if (relevant.length === 0) return null;
      return (
        <div key={cat.label} style={{ marginBottom: 8 }}>
          <div style={{ fontSize: 11, color: 'var(--parley-text-muted,#888)', marginBottom: 4 }}>{cat.label}</div>
          {relevant.map(p => (
            <label key={p.name} title={p.description} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, cursor: 'pointer', marginBottom: 2 }}>
              <input
                type="checkbox"
                checked={(grantedPerms & p.bit) !== 0n}
                onChange={() => togglePerm(p.bit)}
                style={{ cursor: 'pointer' }}
              />
              {p.name}
            </label>
          ))}
        </div>
      );
    })}
  </div>
)}
```

Pass `grantedPerms` to `acceptBotInvite`:
```typescript
await acceptBotInvite(token, selectedServer, grantedPerms);
```

- [ ] **Step 6: Build TypeScript**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -40
```
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/BotInviteEmbed.tsx
git commit -m "feat: add permissions checklist to BotInviteEmbed"
```

---

## Task 9: DeveloperTab — My Bots section with permissions picker

**Files:**
- Modify: `frontend/src/components/settings/DeveloperTab.tsx`

- [ ] **Step 1: Add imports**

```typescript
import { getMyBots, updateBotInvitePermissions, UserBot } from '../../api/bots';
import { PERMISSION_CATEGORIES, permFromNumber } from '../../lib/permissions';
```

- [ ] **Step 2: Add state**

```typescript
const [myBots, setMyBots] = useState<UserBot[]>([]);
const [botPerms, setBotPerms] = useState<Record<number, bigint>>({});
const [botPermsError, setBotPermsError] = useState<Record<number, string>>({});
const saveTimers = React.useRef<Record<number, ReturnType<typeof setTimeout>>>({});
```

- [ ] **Step 3: Load bots on mount**

```typescript
useEffect(() => {
  getMyBots()
    .then(bots => {
      setMyBots(bots);
      const initial: Record<number, bigint> = {};
      for (const b of bots) initial[b.id] = permFromNumber(b.permissions);
      setBotPerms(initial);
    })
    .catch(() => {});
}, []);
```

- [ ] **Step 4: Add debounced toggle handler**

```typescript
const handleTogglePerm = (botId: number, bit: bigint) => {
  setBotPerms(prev => {
    const cur = prev[botId] ?? 0n;
    const next = { ...prev, [botId]: (cur & bit) !== 0n ? cur & ~bit : cur | bit };
    clearTimeout(saveTimers.current[botId]);
    saveTimers.current[botId] = setTimeout(() => {
      updateBotInvitePermissions(botId, next[botId])
        .then(() => setBotPermsError(e => ({ ...e, [botId]: '' })))
        .catch(() => setBotPermsError(e => ({ ...e, [botId]: 'Failed to save — try again' })));
    }, 500);
    return next;
  });
};
```

- [ ] **Step 5: Add My Bots section JSX**

Add after the API Keys section in the return:

```tsx
{myBots.length > 0 && (
  <div className="settings-section">
    <div className="settings-section-title">My Bots</div>
    {myBots.map(bot => {
      const inviteURL = `${window.location.origin}/invite/bot/${bot.invite_token}`;
      return (
        <div key={bot.id} style={{ borderTop: '1px solid var(--border)', paddingTop: 12, marginTop: 12 }}>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>
            {bot.display_name} <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>@{bot.username}</span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
            <code style={{ fontSize: 12, background: 'var(--input)', padding: '3px 6px', borderRadius: 4, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {inviteURL}
            </code>
            <button
              className="settings-btn settings-btn-ghost"
              style={{ padding: '3px 10px', fontSize: 12, flexShrink: 0 }}
              onClick={() => navigator.clipboard.writeText(inviteURL)}
            >
              Copy
            </button>
          </div>
          <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-muted)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Requested Permissions
          </div>
          {PERMISSION_CATEGORIES.map(cat => (
            <div key={cat.label} style={{ marginBottom: 8 }}>
              <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 4 }}>{cat.label}</div>
              {cat.permissions.map(p => (
                <label key={p.name} title={p.description} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, cursor: 'pointer', marginBottom: 2 }}>
                  <input
                    type="checkbox"
                    checked={((botPerms[bot.id] ?? 0n) & p.bit) !== 0n}
                    onChange={() => handleTogglePerm(bot.id, p.bit)}
                    style={{ cursor: 'pointer' }}
                  />
                  {p.name}
                </label>
              ))}
            </div>
          ))}
          {botPermsError[bot.id] && (
            <div style={{ fontSize: 12, color: 'var(--danger)', marginTop: 4 }}>{botPermsError[bot.id]}</div>
          )}
        </div>
      );
    })}
  </div>
)}
```

- [ ] **Step 6: Build TypeScript**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -40
```
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/settings/DeveloperTab.tsx
git commit -m "feat: add My Bots section with permissions picker to DeveloperTab"
```

---

## Task 10: Message.tsx — generalize embed extraction

**Files:**
- Modify: `frontend/src/components/chat/Message.tsx`

**Background:** Replace the module-level stateful `THEME_URL_RE` regex and `extractThemeTokens` / `stripThemeURLs` helpers with a pattern-array approach using fresh `RegExp` instances per call (avoids `lastIndex` statefulness). See the spec at `docs/superpowers/specs/2026-03-19-bot-invite-permissions-design.md` (lines 71-103) for the exact implementation.

- [ ] **Step 1: Add import for BotInviteEmbed**

Alongside the existing `ThemeLinkEmbed` import:
```typescript
import { BotInviteEmbed } from '../BotInviteEmbed';
```

- [ ] **Step 2: Replace module-level regex and helpers (lines 18-31)**

Remove the current `THEME_URL_RE` constant and `extractThemeTokens` / `stripThemeURLs` functions.

Replace with `EmbedType`, `EMBED_PATTERN_DEFS`, `extractEmbeds`, and `stripEmbedURLs` as specified in the design spec (lines 71-103 of `docs/superpowers/specs/2026-03-19-bot-invite-permissions-design.md`).

Key points:
- `type EmbedType = 'theme' | 'bot-invite'`
- `EMBED_PATTERN_DEFS` is a module-level const array with `type` and `source` (string, not RegExp)
- `extractEmbeds` creates `new RegExp(def.source, 'gi')` **inside the loop** — fresh instance per pattern per call, never reused across calls
- `stripEmbedURLs` similarly creates fresh `RegExp` instances inside the loop

- [ ] **Step 3: Update the render section (around line 420)**

Replace:
```typescript
const tokens = extractThemeTokens(message.content);
const cleanContent = tokens.length > 0 ? stripThemeURLs(message.content) : message.content;
```
With:
```typescript
const embeds = extractEmbeds(message.content);
const cleanContent = embeds.length > 0 ? stripEmbedURLs(message.content) : message.content;
```

Replace embed rendering from:
```typescript
{tokens.map(tok => <ThemeLinkEmbed key={tok} token={tok} />)}
```
With:
```typescript
{embeds.map(({ type, token }) =>
  type === 'theme'
    ? <ThemeLinkEmbed key={type + ':' + token} token={token} />
    : <BotInviteEmbed key={type + ':' + token} token={token} />
)}
```

- [ ] **Step 4: Build TypeScript**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | head -40
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/chat/Message.tsx
git commit -m "feat: generalize Message.tsx embed extraction to support bot-invite URLs"
```

---

## Task 11: End-to-end verification

- [ ] **Step 1: Verify permissions migration ran**

```bash
psql $DATABASE_URL -c "\d bot_invite_tokens"
```
Expected: `permissions` column with type `bigint`, default `0`.

- [ ] **Step 2: Test developer flow**

1. Log in, go to Settings → Developer
2. The "My Bots" section shows your bots
3. Toggle permissions for a bot and wait 500ms — no error shown means PATCH /developer/bots/{id}/invite saved
4. Verify: `SELECT permissions FROM bot_invite_tokens WHERE created_by = <your_id>;`

- [ ] **Step 3: Test the embed flow**

1. Copy the bot's invite URL from the Developer tab
2. Paste in any chat message
3. Expected: `BotInviteEmbed` card renders with permissions checklist pre-checked
4. Uncheck a permission, click "Add to Server"
5. Verify role created: `SELECT name, permissions FROM server_roles WHERE name LIKE '@%' ORDER BY created_at DESC LIMIT 1;`
6. The `permissions` value should match what was left checked, not what was originally requested

- [ ] **Step 4: Test no-permissions path**

1. Set bot permissions to 0 (uncheck all in Developer tab)
2. Open invite link → expected: "No permissions requested" shown
3. Add bot to server → no role row created in `server_roles` for that bot

- [ ] **Step 5: Final build**

```bash
cd /home/dylan/Developer/parley && go build ./...
cd frontend && npm run build 2>&1 | tail -5
```
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add -p
git commit -m "chore: final cleanup for bot invite permissions feature"
```
