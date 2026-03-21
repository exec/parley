# Server Audit Log — Design Spec

**Date:** 2026-03-21
**Status:** Approved

---

## Goal

Add a server audit log that records privileged actions (kicks, bans, role changes, channel changes, invite changes, server setting changes) and exposes them to users with `PermViewAuditLog` via a new tab in ServerSettings.

---

## Architecture

**Approach:** Inline logging — each service method calls `auditSvc.Log(...)` after the primary action succeeds. Audit write failures are logged but do not bubble up (a failed audit write must never break the primary action).

**New package:** `internal/audit/` owns the `AuditService` and `Entry` struct. Repository methods live in `internal/db/repository.go` alongside all other DB access.

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
CREATE INDEX idx_sal_actor       ON server_audit_logs(server_id, actor_id);
CREATE INDEX idx_sal_action      ON server_audit_logs(server_id, action);
```

**Field semantics:**
- `action` — dot-namespaced string (see Action Strings below)
- `target_id` — TEXT so it can hold any ID type or invite code
- `target_type` — `'user'` | `'role'` | `'channel'` | `'invite'` | `'server'`
- `target_name` — display name snapshotted at action time (survives deletion)
- `changes` — only populated for update actions; shape: `{"before": {...}, "after": {...}}`
- `actor_id` — nullable; SET NULL on user delete so log rows survive

---

## Action Strings

| Action | Trigger |
|--------|---------|
| `member.kick` | KickMember |
| `member.ban` | BanMember |
| `member.unban` | UnbanMember |
| `member.role_add` | AssignRoleToMember |
| `member.role_remove` | RemoveRoleFromMember |
| `role.create` | CreateServerRole |
| `role.update` | UpdateServerRole |
| `role.delete` | DeleteServerRole |
| `channel.create` | CreateChannel |
| `channel.update` | UpdateChannel |
| `channel.delete` | DeleteChannel |
| `invite.create` | CreateInvite |
| `invite.revoke` | RevokeInvite |
| `server.update` | UpdateServer |
| `server.vanity_update` | SetVanityURL |

---

## Backend

### `internal/audit/service.go`

```go
type Entry struct {
    ServerID      int64
    ActorID       int64
    ActorUsername string
    Action        string
    TargetID      string   // optional
    TargetType    string   // optional
    TargetName    string   // optional
    Changes       any      // marshalled to JSONB; nil if not applicable
    Reason        string   // optional
}

type AuditService struct { repo AuditRepository }

func (s *AuditService) Log(ctx context.Context, e Entry) {
    if err := s.repo.CreateAuditLog(ctx, e); err != nil {
        log.Printf("audit log write failed: %v", err)
    }
}
```

### `internal/db/repository.go` — new methods

```go
CreateAuditLog(ctx, Entry) error
ListAuditLogs(ctx, serverID int64, actorID *int64, action string, limit, offset int) ([]AuditLog, int, error)
```

`ListAuditLogs` filters by `server_id` always; optionally by `actor_id` and/or `action` prefix (e.g. `action LIKE 'member.%'`). Returns total count for pagination.

### Instrumentation call sites (~15 total)

- `internal/server/server_members.go` — KickMember, BanMember, UnbanMember, AssignRoleToMember, RemoveRoleFromMember
- `internal/server/server_roles.go` — CreateServerRole, UpdateServerRole (captures before/after), DeleteServerRole
- `internal/server/server_invites.go` — CreateInvite, RevokeInvite
- `internal/server/server_crud.go` — UpdateServer, SetVanityURL
- `internal/channel/handler.go` — CreateChannel, UpdateChannel, DeleteChannel (handler level; channel service doesn't carry actor context)

`AuditService` is constructed in `cmd/api/main.go` and injected into `ServerService` and `ChannelHandler`.

### New endpoint

```
GET /api/servers/{id}/audit-log
  ?limit=50&offset=0&action=member.kick&actor_id=123456
```

- Permission: `PermViewAuditLog` or owner
- Response: `{ "logs": [...], "total": 1042 }`
- `limit` clamped to max 100, `offset` min 0
- Handler added to `internal/server/handler.go`
- Route registered in `cmd/api/routes.go` inside the existing auth+membership middleware group

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
- Props: `{ server: Server; myPerms: bigint }`
- Filter bar: action `<select>` grouped by category (All / Members / Roles / Channels / Invites / Server) + actor username text input (client-side filter on loaded results, or passed as query param on load)
- Log list: each row — action icon, human-readable description, relative timestamp (full ISO on hover)
- `changes` rendered as a compact before→after diff for update actions
- Load More button (offset += 50), disabled when `offset + logs.length >= total`
- Empty state: "No audit log entries."

### `ServerSettings.tsx` changes

- Add `'auditlog'` to `Tab` type
- Nav button visible only when `hasPermission(myPerms, PermViewAuditLog) || isOwner`
- `{activeTab === 'auditlog' && <AuditLogTab server={server} myPerms={myPerms} />}`

---

## Error Handling

- Audit write failures: log + ignore (never fail the primary action)
- Endpoint 403 if caller lacks `PermViewAuditLog` and is not owner
- Malformed query params: silently use defaults (same pattern as other paginated endpoints)

---

## Files Touched

| File | Change |
|------|--------|
| `internal/db/migrations.go` | Add `server_audit_logs` migration |
| `internal/db/repository.go` | Add `CreateAuditLog`, `ListAuditLogs` |
| `internal/audit/service.go` | New — `AuditService`, `Entry` |
| `internal/server/server_members.go` | 5 Log() calls |
| `internal/server/server_roles.go` | 3 Log() calls + before/after capture |
| `internal/server/server_invites.go` | 2 Log() calls |
| `internal/server/server_crud.go` | 2 Log() calls |
| `internal/channel/handler.go` | 3 Log() calls |
| `internal/server/handler.go` | Add `GetAuditLog` handler |
| `cmd/api/main.go` | Construct + inject `AuditService` |
| `cmd/api/routes.go` | Register `GET /servers/{id}/audit-log` |
| `frontend/src/api/audit.ts` | New — API client |
| `frontend/src/components/settings/AuditLogTab.tsx` | New — tab component |
| `frontend/src/components/settings/ServerSettings.tsx` | Add tab + gate |
