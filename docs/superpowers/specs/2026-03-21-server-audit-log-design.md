# Server Audit Log — Design Spec

**Date:** 2026-03-21
**Status:** Approved

---

## Goal

Add a server audit log that records privileged actions (kicks, bans, role changes, channel changes, invite changes, server setting changes) and exposes them to users with `PermViewAuditLog` via a new tab in ServerSettings.

---

## Architecture

**Approach:** Inline logging — each service method (or handler, where actor context is only available there) calls `auditSvc.Log(...)` after the primary action succeeds. Audit write failures are logged but do not bubble up — a failed audit write must never break the primary action.

**New package:** `internal/audit/` owns the `AuditService`, `Entry`, and `AuditRepository` interface.

---

## Data Model

```sql
CREATE TABLE server_audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    server_id       BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    actor_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_username  TEXT NOT NULL DEFAULT '',
    action          VARCHAR(50) NOT NULL,
    target_id       TEXT,
    target_type     VARCHAR(20),
    target_name     TEXT,
    changes         JSONB,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sal_server_time ON server_audit_logs(server_id, created_at DESC);
CREATE INDEX idx_sal_actor       ON server_audit_logs(server_id, actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_sal_action      ON server_audit_logs(server_id, action);
```

**Field semantics:**
- `action` — dot-namespaced string (see Action Strings)
- `target_id` — TEXT to accommodate any ID type or invite code
- `target_type` — `'user'` | `'role'` | `'channel'` | `'invite'` | `'server'`
- `target_name` — display name snapshotted at action time (survives target deletion)
- `changes` — only populated for update actions; shape: `{"before": {...}, "after": {...}}`
- `actor_id` — nullable BIGINT; SET NULL on user delete so rows survive
- `actor_username` — denormalized snapshot of the actor's username at action time

---

## Action Strings

| Action | Trigger | Actor pattern |
|--------|---------|---------------|
| `member.kick` | KickMember | Pattern B (service param) |
| `member.ban` | BanMember | Pattern B (service param) |
| `member.unban` | UnbanMember | Pattern B (service param) |
| `member.role_add` | AssignRoleToMember | Pattern B (service param) |
| `member.role_remove` | RemoveRoleFromMember | Pattern B (service param) |
| `role.create` | CreateServerRole | Pattern B (service param) |
| `role.update` | UpdateServerRole | Pattern B (service param) |
| `role.delete` | DeleteServerRole | Pattern B (service param) |
| `channel.create` | CreateChannel handler | Pattern A (handler) |
| `channel.update` | UpdateChannel handler | Pattern A (handler) |
| `channel.delete` | DeleteChannel handler | Pattern A (handler) |
| `invite.create` | CreateInvite | Pattern B (service param) |
| `invite.revoke` | RevokeInvite | Pattern B (service param) |
| `server.update` | UpdateServer handler | Pattern A (handler) |
| `server.vanity_update` | SetVanityURL | Pattern B (service param) |

---

## Backend

### `internal/audit/service.go`

```go
package audit

import (
    "context"
    "log"
)

// AuditRepository is the persistence interface satisfied by *db.Repository.
type AuditRepository interface {
    CreateAuditLog(ctx context.Context, e Entry) error
}

type Entry struct {
    ServerID      int64
    ActorID       *int64  // nil = unknown/system
    ActorUsername string
    Action        string
    TargetID      string
    TargetType    string
    TargetName    string
    Changes       any    // marshalled to JSONB; nil = no changes field
    Reason        string
}

type AuditService struct {
    repo AuditRepository
}

func NewAuditService(repo AuditRepository) *AuditService {
    return &AuditService{repo: repo}
}

func (s *AuditService) Log(ctx context.Context, e Entry) {
    if err := s.repo.CreateAuditLog(ctx, e); err != nil {
        log.Printf("audit log write failed: %v", err)
    }
}
```

`*db.Repository` satisfies `AuditRepository` by implementing `CreateAuditLog`.

### New repository methods in `internal/db/repository.go`

```go
// Lightweight username lookup for audit call sites
GetUsernameByID(ctx context.Context, userID int64) (string, error)
// SELECT username FROM users WHERE id = $1

// Single-role fetch for role.update before-snapshot
GetServerRoleByID(ctx context.Context, roleID int64) (*ServerRole, error)
// SELECT id, server_id, name, color, permissions, hoist, position, created_at
// FROM server_roles WHERE id = $1

// Audit persistence
CreateAuditLog(ctx context.Context, e audit.Entry) error
ListAuditLogs(ctx context.Context, serverID int64, actorID *int64, action string, limit, offset int) ([]AuditLog, int, error)
```

`ListAuditLogs` always filters by `server_id`. Filters by `actor_id` (when non-nil) and/or `action` (exact match, when non-empty). Returns total row count for pagination.

### Actor context — two patterns

**Pattern A — Handler-level logging** (channel.create/update/delete, server.update):
The handler has `userID string` from `auth.GetUserIDFromContext(r)`. Parse to int64, call `repo.GetUsernameByID(ctx, actorIDInt)` to get `actorUsername`, then call `auditSvc.Log(...)` after the service call returns successfully.

**Pattern B — Service-level logging** (all other actions):
Add `actorID int64, actorUsername string` as new trailing parameters. Callers (handlers) parse actor ID from JWT claims and call `repo.GetUsernameByID` once before the service call.

**Method signatures after changes:**

```go
// server_members.go — actor params added to all five
KickMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername string) error
BanMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername, reason string) error
  // NOTE: removes old `bannedByID string` param. Pass actorID directly to
  // repo.AddServerBan(..., actorID, reason) — AddServerBan already takes int64 for bannedBy.
UnbanMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername string) error
AssignRoleToMember(ctx context.Context, serverID, userID, roleID string, actorID int64, actorUsername string) error
RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID string, actorID int64, actorUsername string) error

// server_roles.go — actor params added to all three
CreateServerRole(ctx context.Context, serverID string, ..., actorID int64, actorUsername string) (*ServerRole, error)
UpdateServerRole(ctx context.Context, serverID string, ..., actorID int64, actorUsername string) (*ServerRole, error)
DeleteServerRole(ctx context.Context, serverID, roleID string, actorID int64, actorUsername string) error

// server_invites.go
CreateInvite(ctx context.Context, serverID, createdBy string, maxUses *int, expiresAt *time.Time, actorUsername string) (*Invite, error)
  // actorUsername added; createdBy string already holds the actor ID
RevokeInvite(ctx context.Context, serverID, code, requestingUserID string, actorUsername string) error
  // actorUsername added; requestingUserID already holds the actor ID
SetVanityURL(ctx context.Context, serverID, vanityURL string, actorID int64, actorUsername string) (*Server, error)
  // actorID replaces the old `userID string` param; use it for BOTH the owner-authorization
  // check (srv.OwnerID != actorID) and the audit log. No security regression.
```

### Before/after snapshots for update actions

**`role.update`:** Call `repo.GetServerRoleByID(ctx, roleIDInt)` immediately before `repo.UpdateServerRole(...)`. Use returned role as `changes.before`. Role returned after update = `changes.after`. Fields: `name`, `color`, `permissions`, `hoist`, `position`.

**`server.update` (Pattern A — handler):** `UpdateServer` already calls `repo.GetServerByID` internally. The handler cannot access this pre-fetch. Instead, the handler calls `svc.GetServer(ctx, serverID)` before calling `svc.UpdateServer(...)` to capture the before-state. Project `*Server` to a plain map for `changes.before` and `changes.after`: `{"name": "...", "icon_url": "...", "description": "...", "is_public": true/false}`. Use the `*Server` API struct (not the raw `*db.Server`) to avoid `sql.NullString` serialization artifacts.

**`channel.update` (Pattern A — handler):** Call `h.service.GetChannel(ctx, id)` before `h.service.UpdateChannel(...)`. Use returned channel for `changes.before` (fields: `name`, `topic`). Updated channel returned by `UpdateChannel` = `changes.after`.

### `internal/channel/handler.go` — constructor change

```go
// Before
func NewHandler(service *ChannelService) *Handler

// After
func NewHandler(service *ChannelService, auditSvc *audit.AuditService) *Handler
```

Add `auditSvc *audit.AuditService` field to `Handler` struct. Update the `NewHandler(...)` call site in `cmd/api/main.go`.

### New endpoint

```
GET /api/servers/{id}/audit-log
  ?limit=50&offset=0&action=member.kick&actor_id=123456
```

- Permission: `permissions.PermViewAuditLog` (already defined as `1 << 9` in `internal/permissions/permissions.go`) or server owner.
  Use the existing `permissions.HasPermission(ctx, h.repo, serverIDInt, actorIDInt, ownerIDInt, permissions.PermViewAuditLog)` pattern — the same pattern used by other permission-gated handlers in `internal/server/handler.go`.
- `limit` clamped to max 100, default 50; `offset` min 0, default 0
- `action` — exact action string; omit/empty = no filter
- `actor_id` — numeric user ID; omit = no filter
- Response: `{ "logs": [...], "total": 1042 }`
- 403 if caller lacks `PermViewAuditLog` and is not owner
- Malformed params: silently use defaults
- Handler in `internal/server/handler.go`
- Route in `cmd/api/routes.go` inside auth+membership middleware group

---

## Frontend

### New files

**`frontend/src/api/audit.ts`**
```ts
export interface AuditLogEntry {
  id: string;
  server_id: string;
  actor_id?: string;
  actor_username: string;
  action: string;
  target_id?: string;
  target_type?: string;
  target_name?: string;
  changes?: { before: Record<string, unknown>; after: Record<string, unknown> };
  reason?: string;
  created_at: string;
}

export async function getAuditLog(
  serverId: string,
  params: { limit?: number; offset?: number; action?: string; actorId?: string }
): Promise<{ logs: AuditLogEntry[]; total: number }>
```

**`frontend/src/components/settings/AuditLogTab.tsx`**

Props: `{ server: Server; currentUserId: string }`

Derives `isOwner = server.owner_id === currentUserId` internally. This tab is only rendered after the permission gate in `ServerSettings`, so no additional permission check is needed inside.

**Action filter select:** `<select>` with "All" (value `""`) followed by individual action string options in `<optgroup>` labels (Members, Roles, Channels, Invites, Server). Each option value is the exact action string (e.g. `member.kick`). Selecting an option re-fetches with `action=<value>`; "All" re-fetches with no action param. Resets to page 0 on change.

**Actor username filter:** text `<input>` that filters the currently loaded entries client-side: `entry.actor_username.toLowerCase().includes(inputValue.toLowerCase())`. Does not trigger a re-fetch. Cleared when the action select changes.

**Log list rows:**
- Icon based on action prefix: `member.*` = 👤, `role.*` = 🔑, `channel.*` = #️⃣, `invite.*` = 🔗, `server.*` = ⚙️
- Human-readable description: "**dylan** kicked **bob**", "**alice** created role **Moderator**"
- Relative timestamp; full ISO date on hover via `title` attribute
- If `entry.changes` is present, render a compact diff below: changed fields only, e.g. `name: "old" → "new"`

**Pagination:** "Load More" button appends next 50 to list. Hidden when `offset + logs.length >= total`.

**Empty state:** "No audit log entries found."

### `ServerSettings.tsx` changes

- Add `'auditlog'` to `Tab` type
- Add `currentUserId: string` to `Props` interface; pass it from `App.tsx` call site
- Nav button gated on `hasPerm(myPerms, PERM_VIEW_AUDIT_LOG) || isOwner`
  - `PERM_VIEW_AUDIT_LOG` is already exported from `frontend/src/lib/permissions.ts`
  - `isOwner` is already computed in `ServerSettings` as `server.owner_id === currentUserId` (add this if not present)
- `{activeTab === 'auditlog' && <AuditLogTab server={server} currentUserId={currentUserId} />}`

---

## Error Handling

- Audit write failures: `log.Printf` + ignore; never fail the primary action
- Endpoint 403 if caller lacks `PermViewAuditLog` and is not owner
- Malformed query params: silently use defaults

---

## Files Touched

| File | Change |
|------|--------|
| `internal/db/migrations.go` | Add `server_audit_logs` migration |
| `internal/db/repository.go` | Add `GetUsernameByID`, `GetServerRoleByID`, `CreateAuditLog`, `ListAuditLogs` |
| `internal/audit/service.go` | New — `AuditService`, `Entry`, `AuditRepository` interface |
| `internal/server/service.go` | Inject `*audit.AuditService` into `ServerService` constructor |
| `internal/server/server_members.go` | Add actorID/actorUsername params to 5 methods; 5 `Log()` calls |
| `internal/server/server_roles.go` | Add actorID/actorUsername params to 3 methods; 3 `Log()` calls; pre-fetch `GetServerRoleByID` for role.update |
| `internal/server/server_invites.go` | Add actorUsername param to CreateInvite, RevokeInvite, SetVanityURL; 3 `Log()` calls |
| `internal/server/server_crud.go` | No new params; server.update logged at handler level |
| `internal/channel/handler.go` | Add `auditSvc` field; update `NewHandler`; 3 `Log()` calls (Pattern A); pre-fetch channel for channel.update |
| `internal/server/handler.go` | Add `GetAuditLog` handler; `Log()` calls for server.update (Pattern A) |
| `cmd/api/main.go` | Construct `AuditService`; update `NewChannelHandler` call site; update `NewServerService` call site |
| `cmd/api/routes.go` | Register `GET /servers/{id}/audit-log` |
| `frontend/src/api/audit.ts` | New — API client |
| `frontend/src/components/settings/AuditLogTab.tsx` | New — tab component |
| `frontend/src/components/settings/ServerSettings.tsx` | Add `'auditlog'` tab, `currentUserId` prop, `hasPerm`/`PERM_VIEW_AUDIT_LOG` gate |
| `frontend/src/App.tsx` | Pass `currentUserId` to `<ServerSettings>` |
