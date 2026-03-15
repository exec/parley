# Permissions Overhaul Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 6-bit server-wide permission system with a full Discord-style model: 42 permission bits, @everyone baseline role, positional role hierarchy, tri-state overwrites at category/channel/member level, category sync, and View Channel visibility filtering.

**Architecture:** Permission constants and computation engine in `internal/permissions/`, overwrite CRUD in `internal/db/`, enforcement distributed across service layers. Frontend mirrors the computation for client-side gating. Migration remaps existing bit values and seeds @everyone roles. All 28 existing permission check points updated to use the new constants and channel-aware computation.

**Tech Stack:** Go 1.25, PostgreSQL, chi router, React 18, TypeScript.

**Spec:** `docs/specs/2026-03-15-permissions-overhaul-design.md`

---

## Chunk 1: Permission Engine and Database Migration

### Task 1: New Permission Constants

**Files:**
- Modify: `internal/permissions/permissions.go`

- [ ] **Step 1: Replace the permission constants**

Replace the existing 6 constants with the full 42-bit set. Organize by scope:

```go
package permissions

// Server-only permissions (bits 0-13) — not overridable per-channel
const (
    PermAdministrator      int64 = 1 << 0
    PermManageServer       int64 = 1 << 1
    PermManageRoles        int64 = 1 << 2
    PermManageChannels     int64 = 1 << 3
    PermKickMembers        int64 = 1 << 4
    PermBanMembers         int64 = 1 << 5
    PermManageNicknames    int64 = 1 << 6
    PermChangeNickname     int64 = 1 << 7
    PermCreateInvite       int64 = 1 << 8
    PermViewAuditLog       int64 = 1 << 9
    PermManageWebhooks     int64 = 1 << 10
    PermManageExpressions  int64 = 1 << 11
    PermManageEvents       int64 = 1 << 12
    PermModerateMember     int64 = 1 << 13
)

// Channel permissions — Text & Bin (bits 16-31)
const (
    PermViewChannel          int64 = 1 << 16
    PermSendMessages         int64 = 1 << 17
    PermEmbedLinks           int64 = 1 << 18
    PermAttachFiles          int64 = 1 << 19
    PermAddReactions         int64 = 1 << 20
    PermMentionEveryone      int64 = 1 << 21
    PermManageMessages       int64 = 1 << 22
    PermReadMessageHistory   int64 = 1 << 23
    PermUseExternalEmoji     int64 = 1 << 24
    PermPinMessages          int64 = 1 << 25
    PermManageThreads        int64 = 1 << 26
    PermCreatePublicThreads  int64 = 1 << 27
    PermSendMessagesInThreads int64 = 1 << 28
    PermCreatePosts          int64 = 1 << 29
    PermManagePosts          int64 = 1 << 30
    PermManageTags           int64 = 1 << 31
)

// Channel permissions — Voice (bits 32-41)
const (
    PermConnect         int64 = 1 << 32
    PermSpeak           int64 = 1 << 33
    PermMuteMembers     int64 = 1 << 34
    PermDeafenMembers   int64 = 1 << 35
    PermMoveMembers     int64 = 1 << 36
    PermUseVAD          int64 = 1 << 37
    PermPrioritySpeaker int64 = 1 << 38
    PermStream          int64 = 1 << 39
    PermUseSoundboard   int64 = 1 << 40
    PermSendVoiceMessages int64 = 1 << 41
)

// Masks
const (
    PermAllPermissions      int64 = (1 << 42) - 1
    PermChannelMask         int64 = PermAllPermissions &^ ((1 << 16) - 1) // bits 16-41 only
    PermServerOnlyMask      int64 = (1 << 14) - 1                         // bits 0-13 only
)

// Default permissions for @everyone role
const PermDefaultEveryone int64 = PermViewChannel | PermSendMessages | PermReadMessageHistory |
    PermAddReactions | PermEmbedLinks | PermAttachFiles | PermConnect | PermSpeak |
    PermUseVAD | PermChangeNickname | PermCreateInvite | PermCreatePosts
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

Expected: Build will FAIL because all callers reference old constant names. That's fine — we'll fix them in later tasks.

- [ ] **Step 3: Commit**

```bash
git add internal/permissions/permissions.go
git commit -m "feat: expand permission bitfield from 6 to 42 bits with new constant layout"
```

---

### Task 2: Permission Computation Engine

**Files:**
- Modify: `internal/permissions/permissions.go` — replace GetEffectivePermissions and HasPermission

- [ ] **Step 1: Rewrite the computation functions**

Replace `GetEffectivePermissions` and `HasPermission` with the new engine:

```go
// ComputeBasePermissions computes a member's server-wide permissions from their roles.
// Owner gets all permissions. Administrator grants all permissions.
func ComputeBasePermissions(everyonePerms int64, memberRolePerms []int64, isOwner bool) int64 {
    if isOwner {
        return PermAllPermissions
    }

    perms := everyonePerms
    for _, rp := range memberRolePerms {
        perms |= rp
    }

    if perms&PermAdministrator != 0 {
        return PermAllPermissions
    }
    return perms
}

// Overwrite represents a permission overwrite for a channel.
type Overwrite struct {
    TargetType int   // 0 = role, 1 = member
    TargetID   int64
    Allow      int64
    Deny       int64
}

// ComputeChannelPermissions computes a member's effective permissions in a channel.
func ComputeChannelPermissions(basePerms int64, memberID int64, memberRoleIDs []int64, everyoneRoleID int64, overwrites []Overwrite) int64 {
    if basePerms&PermAdministrator != 0 {
        return PermAllPermissions
    }

    perms := basePerms

    // Step 1: @everyone overwrites
    for _, ow := range overwrites {
        if ow.TargetType == 0 && ow.TargetID == everyoneRoleID {
            perms &= ^ow.Deny
            perms |= ow.Allow
            break
        }
    }

    // Step 2: Role overwrites (combined)
    var roleAllow, roleDeny int64
    roleSet := make(map[int64]bool, len(memberRoleIDs))
    for _, rid := range memberRoleIDs {
        roleSet[rid] = true
    }
    for _, ow := range overwrites {
        if ow.TargetType == 0 && ow.TargetID != everyoneRoleID && roleSet[ow.TargetID] {
            roleAllow |= ow.Allow
            roleDeny |= ow.Deny
        }
    }
    perms &= ^roleDeny
    perms |= roleAllow

    // Step 3: Member-specific overwrite
    for _, ow := range overwrites {
        if ow.TargetType == 1 && ow.TargetID == memberID {
            perms &= ^ow.Deny
            perms |= ow.Allow
            break
        }
    }

    // Implicit denials
    if perms&PermViewChannel == 0 {
        perms &= ^PermChannelMask // deny all channel perms
    }
    if perms&PermSendMessages == 0 {
        perms &= ^(PermMentionEveryone | PermAttachFiles | PermEmbedLinks)
    }
    if perms&PermConnect == 0 {
        perms &= ^(PermSpeak | PermMuteMembers | PermDeafenMembers | PermMoveMembers | PermUseVAD | PermPrioritySpeaker | PermStream | PermUseSoundboard | PermSendVoiceMessages)
    }

    return perms
}

// HasPerm checks if a computed permission set includes a specific permission.
func HasPerm(perms int64, perm int64) bool {
    return perms&perm == perm
}
```

Keep the old `HasPermission` and `GetEffectivePermissions` functions temporarily (they'll be updated in Task 6 to use the new engine). Mark them with a `// DEPRECATED` comment.

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/permissions/permissions.go
git commit -m "feat: add Discord-style permission computation engine with overwrites and implicit denials"
```

---

### Task 3: Database Migration — Schema Changes

**Files:**
- Modify: `internal/db/migrations.go` — append new migration
- Modify: `internal/db/models.go` — add `IsEveryone` to ServerRole, `Synced` to Channel

- [ ] **Step 1: Add migration**

Append to the `Migrations` slice:

```go
`-- Add is_everyone flag to server_roles
ALTER TABLE server_roles ADD COLUMN IF NOT EXISTS is_everyone BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_server_roles_everyone ON server_roles(server_id) WHERE is_everyone = TRUE;

-- Add synced flag to channels for category permission sync
ALTER TABLE channels ADD COLUMN IF NOT EXISTS synced BOOLEAN NOT NULL DEFAULT TRUE;

-- Create permission_overwrites table
CREATE TABLE IF NOT EXISTS permission_overwrites (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    target_type SMALLINT NOT NULL,
    target_id BIGINT NOT NULL,
    allow BIGINT NOT NULL DEFAULT 0,
    deny BIGINT NOT NULL DEFAULT 0,
    UNIQUE(channel_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_perm_overwrites_channel ON permission_overwrites(channel_id);
CREATE INDEX IF NOT EXISTS idx_perm_overwrites_target ON permission_overwrites(target_type, target_id);

-- Remap existing permission bits to new layout
-- Old: SendMessages=1, ManageMessages=2, ManageChannels=4, KickMembers=8, ManageServer=16, Administrator=32
-- New: Administrator=1, ManageServer=2, ManageChannels=8, KickMembers=16, SendMessages=131072, ManageMessages=4194304
UPDATE server_roles SET permissions =
    (CASE WHEN permissions & 32 != 0 THEN 1 ELSE 0 END) |          -- Administrator: bit 5 -> bit 0
    (CASE WHEN permissions & 16 != 0 THEN 2 ELSE 0 END) |          -- ManageServer: bit 4 -> bit 1
    (CASE WHEN permissions & 4  != 0 THEN 8 ELSE 0 END) |          -- ManageChannels: bit 2 -> bit 3
    (CASE WHEN permissions & 8  != 0 THEN 16 ELSE 0 END) |         -- KickMembers: bit 3 -> bit 4
    (CASE WHEN permissions & 1  != 0 THEN 131072 ELSE 0 END) |     -- SendMessages: bit 0 -> bit 17
    (CASE WHEN permissions & 2  != 0 THEN 4194304 ELSE 0 END)      -- ManageMessages: bit 1 -> bit 22
WHERE permissions != 0;

-- Rename any existing roles named @everyone to avoid unique constraint collision
UPDATE server_roles SET name = 'everyone (renamed)' WHERE name = '@everyone';

-- Seed @everyone role for every server that doesn't have one
INSERT INTO server_roles (server_id, name, color, permissions, hoist, position, is_everyone, created_at)
SELECT s.id, '@everyone', '#99aab5',
    -- DEFAULT_EVERYONE_PERMISSIONS bits
    (1::BIGINT << 16) | (1::BIGINT << 17) | (1::BIGINT << 23) | (1::BIGINT << 20) |
    (1::BIGINT << 18) | (1::BIGINT << 19) | (1::BIGINT << 32) | (1::BIGINT << 33) |
    (1::BIGINT << 37) | (1::BIGINT << 7) | (1::BIGINT << 8) | (1::BIGINT << 29),
    FALSE, 0, TRUE, NOW()
FROM servers s
WHERE NOT EXISTS (
    SELECT 1 FROM server_roles sr WHERE sr.server_id = s.id AND sr.is_everyone = TRUE
);
`,
```

- [ ] **Step 2: Update models**

Add `IsEveryone` to `ServerRole` in `internal/db/models.go`:
```go
IsEveryone  bool      `json:"is_everyone" db:"is_everyone"`
```

Add `Synced` to `Channel` in `internal/db/models.go`:
```go
Synced      bool      `json:"synced" db:"synced"`
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations.go internal/db/models.go
git commit -m "feat: add permission_overwrites table, is_everyone flag, synced column, and remap permission bits"
```

---

### Task 4: Overwrite Repository

**Files:**
- Create: `internal/db/overwrite_repository.go`

- [ ] **Step 1: Create the repository**

Methods on `*Repository`:

- `GetChannelOverwrites(ctx, channelID int64) ([]permissions.Overwrite, error)` — SELECT all overwrites for a channel, mapped to `permissions.Overwrite` structs.
- `UpsertOverwrite(ctx, channelID int64, targetType int, targetID int64, allow, deny int64) error` — INSERT ON CONFLICT UPDATE. Mask out server-only bits (bits 0-13) from allow and deny before storage.
- `DeleteOverwrite(ctx, overwriteID int64) error` — DELETE by ID.
- `DeleteOverwritesByTarget(ctx, targetType int, targetID int64) error` — DELETE all overwrites for a role or member (used when deleting a role).
- `CopyOverwrites(ctx, fromChannelID, toChannelID int64) error` — DELETE all overwrites on toChannel, then INSERT copies from fromChannel (used for category sync).
- `GetOverwritesByChannels(ctx, channelIDs []int64) (map[int64][]permissions.Overwrite, error)` — bulk fetch for channel list filtering.

Also add a `PermissionOverwrite` DB model:
```go
type PermissionOverwrite struct {
    ID         int64 `json:"id" db:"id"`
    ChannelID  int64 `json:"channel_id" db:"channel_id"`
    TargetType int   `json:"target_type" db:"target_type"`
    TargetID   int64 `json:"target_id" db:"target_id"`
    Allow      int64 `json:"allow" db:"allow"`
    Deny       int64 `json:"deny" db:"deny"`
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/db/overwrite_repository.go
git commit -m "feat: add permission overwrite repository with upsert, delete, copy, and bulk fetch"
```

---

### Task 5: @everyone Role Repository Support

**Files:**
- Modify: `internal/db/role_repository.go` — add GetEveryoneRole method
- Modify: `internal/server/server_crud.go` or wherever CreateServer lives — seed @everyone on server creation

- [ ] **Step 1: Add repository method**

```go
// GetEveryoneRole returns the @everyone role for a server.
func (r *Repository) GetEveryoneRole(ctx context.Context, serverID int64) (*ServerRole, error) {
    var role ServerRole
    err := r.db.QueryRowContext(ctx,
        `SELECT id, server_id, name, color, permissions, hoist, position, is_everyone, created_at
         FROM server_roles WHERE server_id = $1 AND is_everyone = TRUE`, serverID).
        Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt)
    if err != nil {
        return nil, err
    }
    return &role, nil
}
```

- [ ] **Step 2: Add CreateEveryoneRole repository method and seed in CreateServer**

Add a dedicated method to the repository:

```go
func (r *Repository) CreateEveryoneRole(ctx context.Context, serverID int64) error {
    _, err := r.db.ExecContext(ctx,
        `INSERT INTO server_roles (server_id, name, color, permissions, hoist, position, is_everyone, created_at)
         VALUES ($1, '@everyone', '#99aab5', $2, FALSE, 0, TRUE, NOW())`,
        serverID, permissions.PermDefaultEveryone)
    return err
}
```

Find where servers are created (likely `internal/server/server_crud.go` or `service.go`). After the server INSERT, call `repo.CreateEveryoneRole(ctx, serverID)`.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/db/role_repository.go internal/server/
git commit -m "feat: add @everyone role support with auto-seed on server creation"
```

---

## Chunk 2: Update All Permission Check Points

### Task 6: Update Permission Helper Functions

**Files:**
- Modify: `internal/permissions/permissions.go` — update HasPermission and GetEffectivePermissions to use new engine

- [ ] **Step 1: Update the legacy helper functions**

Replace the deprecated functions with versions that use the new engine internally. They need to:
1. Fetch the @everyone role for the server
2. Fetch the member's assigned roles
3. Call `ComputeBasePermissions`
4. Return the result

```go
// GetEffectivePermissions returns the computed base permissions for a user in a server.
func GetEffectivePermissions(ctx context.Context, repo interface {
    GetEveryoneRole(ctx context.Context, serverID int64) (*db.ServerRole, error)
    GetMemberRoles(ctx context.Context, serverID, userID int64) ([]db.ServerRole, error)
}, serverID, userID, ownerID int64) int64 {
    isOwner := userID == ownerID

    everyoneRole, err := repo.GetEveryoneRole(ctx, serverID)
    if err != nil {
        if isOwner { return PermAllPermissions }
        return 0
    }

    memberRoles, err := repo.GetMemberRoles(ctx, serverID, userID)
    if err != nil {
        return ComputeBasePermissions(everyoneRole.Permissions, nil, isOwner)
    }

    rolePerms := make([]int64, len(memberRoles))
    for i, r := range memberRoles {
        rolePerms[i] = r.Permissions
    }

    return ComputeBasePermissions(everyoneRole.Permissions, rolePerms, isOwner)
}

// HasPermission checks if a user has a specific permission in a server.
func HasPermission(ctx context.Context, repo interface {
    GetEveryoneRole(ctx context.Context, serverID int64) (*db.ServerRole, error)
    GetMemberRoles(ctx context.Context, serverID, userID int64) ([]db.ServerRole, error)
}, serverID, userID, ownerID int64, perm int64) (bool, error) {
    perms := GetEffectivePermissions(ctx, repo, serverID, userID, ownerID)
    return HasPerm(perms, perm), nil
}
```

Note: the interface parameter allows the function to accept `*db.Repository` without importing the `db` package directly, avoiding circular imports.

**IMPORTANT**: This changes the function signatures. All 28 existing call sites must be updated in Tasks 7–11 to pass the `repo` parameter. The call sites are:
- `internal/channel/service.go` — 4 calls (CreateChannel, UpdateChannel, DeleteChannel, ReorderChannels)
- `internal/message/service.go` — 1 call (CanManageMessage)
- `internal/server/server_members.go` — 2 calls (CanKickBan, GetMyPermissions)
- `internal/server/handler.go` — 1 call (AddMember/PermAdministrator check)
- `internal/bin/service.go` — calls added in Task 10

Build verification after this task WILL fail until Tasks 7–11 update the callers. That's expected.

- [ ] **Step 2: Add channel-aware permission check**

```go
// HasChannelPermission checks if a user has a permission in a specific channel.
func HasChannelPermission(ctx context.Context, repo interface {
    GetEveryoneRole(ctx context.Context, serverID int64) (*db.ServerRole, error)
    GetMemberRoles(ctx context.Context, serverID, userID int64) ([]db.ServerRole, error)
    GetChannelOverwrites(ctx context.Context, channelID int64) ([]Overwrite, error)
}, serverID, userID, ownerID, channelID int64, perm int64) (bool, error) {
    basePerms := GetEffectivePermissions(ctx, repo, serverID, userID, ownerID)
    if HasPerm(basePerms, PermAdministrator) {
        return true, nil
    }

    everyoneRole, _ := repo.GetEveryoneRole(ctx, serverID)
    memberRoles, _ := repo.GetMemberRoles(ctx, serverID, userID)
    overwrites, err := repo.GetChannelOverwrites(ctx, channelID)
    if err != nil {
        return HasPerm(basePerms, perm), nil
    }

    everyoneID := int64(0)
    if everyoneRole != nil {
        everyoneID = everyoneRole.ID
    }

    roleIDs := make([]int64, len(memberRoles))
    for i, r := range memberRoles {
        roleIDs[i] = r.ID
    }

    channelPerms := ComputeChannelPermissions(basePerms, userID, roleIDs, everyoneID, overwrites)
    return HasPerm(channelPerms, perm), nil
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/permissions/permissions.go
git commit -m "feat: update permission helpers to use new computation engine with channel overwrite support"
```

---

### Task 7: Update Channel Service Permission Checks

**Files:**
- Modify: `internal/channel/service.go` — update CreateChannel, UpdateChannel, DeleteChannel, ReorderChannels

- [ ] **Step 1: Update permission constant references**

In each of the 4 methods, replace:
- `permissions.PermManageChannels` (old value `4`) — now `permissions.PermManageChannels` (new value `1 << 3 = 8`)

The constant name hasn't changed, only the value, so if the code uses the constant name it should just work after rebuilding. Verify this.

- [ ] **Step 2: Add synced column handling**

In `CreateChannel` — when a channel is created inside a category (parent_id is set), copy the category's overwrites to the new channel and set `synced = true`. Read the parent's overwrites and insert copies for the new channel.

In the channel query/response — include the `synced` field in the Channel struct returned by GetServerChannels.

- [ ] **Step 3: Add View Channel filtering to GetServerChannels**

In `GetServerChannels`, after fetching all channels, compute each channel's permissions for the requesting user. Filter out channels where the user lacks `ViewChannel`. Filter out categories where zero children are visible. Skip filtering for server owner and users with Administrator.

This requires the method to accept a `userID` parameter (it may not have one currently — read the existing code).

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add internal/channel/service.go internal/channel/handler.go
git commit -m "feat: add View Channel filtering, synced handling, and channel-aware permission checks"
```

---

### Task 8: Update Message Service Permission Checks

**Files:**
- Modify: `internal/message/service.go` — update CanManageMessage and SendMessage
- Modify: `internal/message/handler.go` — update EditMessage author check

- [ ] **Step 1: Update CanManageMessage**

The existing function checks `PermManageMessages` at the server level. Update it to use `HasChannelPermission` with the message's channel ID so overwrites are respected.

- [ ] **Step 2: Add SendMessage permission check**

Currently `SendMessage` doesn't check permissions — anyone can send. Add a `HasChannelPermission` check for `PermSendMessages` on the target channel. This enforces channel-level send restrictions.

Also add a check for `PermAttachFiles` when the message has an attachment, and `PermMentionEveryone` when the message content contains `@everyone` or `@here`.

- [ ] **Step 3: Add ViewChannel check to GetChannelMessages**

Before returning messages, verify the user has `ViewChannel` on the channel. Return 404 (not 403) if denied — to avoid leaking channel existence.

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add internal/message/service.go internal/message/handler.go
git commit -m "feat: add channel-aware permission checks for messages (send, manage, view)"
```

---

### Task 9: Update Server Handler Permission Checks

**Files:**
- Modify: `internal/server/handler.go` — update role management to use ManageRoles permission and enforce hierarchy
- Modify: `internal/server/server_members.go` — update CanKickBan with hierarchy enforcement

- [ ] **Step 1: Update role management handlers**

Currently role CRUD is owner-only. Change to require `ManageRoles` permission (or owner). Add hierarchy enforcement:

For `CreateServerRole`:
- Check `HasPermission(PermManageRoles)` instead of owner-only
- New role's position must be below actor's highest role position
- New role's permissions can only include permissions the actor has

For `UpdateServerRole`:
- Requires `ManageRoles` (or owner)
- **Special case**: editing the @everyone role requires `ManageServer` (not just ManageRoles), per spec
- Can't edit roles at or above actor's highest position
- Can't grant permissions the actor doesn't have
- Can't edit the @everyone role's name or is_everyone flag

For `DeleteServerRole`:
- Same permission check
- Can't delete roles at or above actor's highest position
- Can't delete the @everyone role

For `AssignRoleToMember` and `RemoveRoleFromMember`:
- Check `ManageRoles` instead of owner-only
- Can only assign/remove roles below actor's highest position

- [ ] **Step 2: Update CanKickBan with hierarchy**

In `server_members.go`, update `CanKickBan` to:
1. Check `PermKickMembers` (for kick) or `PermBanMembers` (for ban) — split these into separate checks
2. Fetch both actor's and target's highest role positions
3. Return false if target's highest role >= actor's highest role
4. Owner always bypasses

Add a helper `GetHighestRolePosition(ctx, serverID, userID int64) (int, error)` to the repository.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/server/handler.go internal/server/server_members.go internal/db/role_repository.go
git commit -m "feat: enforce role hierarchy for kick/ban/role management, use ManageRoles permission"
```

---

### Task 10: Update Bin Service Permission Checks

**Files:**
- Modify: `internal/bin/service.go` — add permission checks for bin operations
- Modify: `internal/bin/line_comments.go`
- Modify: `internal/bin/tags.go`

- [ ] **Step 1: Add permission checks to bin service**

For `CreatePost`:
- Check `HasChannelPermission(PermCreatePosts)` on the bin channel
- Check `HasChannelPermission(PermViewChannel)` — return 404 if denied

For `EditPost` and `DeletePost`:
- Author can always edit/delete own posts
- Others need `HasChannelPermission(PermManagePosts)`

For `CreateLineComment`:
- Check `HasChannelPermission(PermSendMessages)` (comments are like messages)

For `CreateTag`, `DeleteTag`:
- Check `HasChannelPermission(PermManageTags)`

For `GetPost`, `ListPosts`, `GetLineComments`:
- Check `HasChannelPermission(PermViewChannel)` — return 404 if denied

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/bin/service.go internal/bin/line_comments.go internal/bin/tags.go
git commit -m "feat: add channel-aware permission checks to bin operations"
```

---

### Task 11: Overwrite API Endpoints

**Files:**
- Create: `internal/channel/overwrites.go` — overwrite handlers in the channel package
- Modify: `cmd/api/routes.go` — register new routes

- [ ] **Step 1: Create overwrite handlers**

In a new file `internal/channel/overwrites.go` within the existing channel package:

- `GetOverwrites(w, r)` — GET /channels/{id}/overwrites. Requires ViewChannel on the channel. Returns list of `PermissionOverwrite` objects.
- `UpsertOverwrite(w, r)` — PUT /channels/{id}/overwrites. Requires ManageRoles OR ManageChannels. Validates:
  - Masks out server-only bits (bits 0-13) from allow and deny before storage
  - A bit can't be in both allow and deny — if a bit is in both, clear it from deny
  - Actor can only set bits they themselves have
  - If channel has a parent (is in a category), set `synced = false` on the channel and broadcast `CHANNEL_UPDATE`
  - If channel IS a category, propagate overwrites to synced children: within a database transaction, call `repo.CopyOverwrites(ctx, categoryID, childChannelID)` for each child with `synced = true`. Broadcast `CHANNEL_OVERWRITE_UPDATE` for each affected child channel.
- `DeleteOverwrite(w, r)` — DELETE /channels/{id}/overwrites/{overwriteId}. Same permission requirements. Same sync handling.
- `GetMyChannelPermissions(w, r)` — GET /channels/{id}/my-permissions. Returns computed permissions for the requesting user.

- [ ] **Step 2: Register routes**

In `cmd/api/routes.go`, add inside the protected route group:

```go
r.Get("/channels/{id}/overwrites", channelHandler.GetOverwrites)
r.Put("/channels/{id}/overwrites", channelHandler.UpsertOverwrite)
r.Delete("/channels/{id}/overwrites/{overwriteId}", channelHandler.DeleteOverwrite)
r.Get("/channels/{id}/my-permissions", channelHandler.GetMyChannelPermissions)
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/channel/overwrites.go cmd/api/routes.go
git commit -m "feat: add permission overwrite CRUD endpoints and per-channel my-permissions"
```

---

## Chunk 3: Frontend

### Task 12: Frontend Permission Utility

**Files:**
- Create: `frontend/src/lib/permissions.ts`

- [ ] **Step 1: Create the permission utility module**

Mirror the backend constants and computation:

```typescript
// Server-only permissions (bits 0-13)
export const PERM_ADMINISTRATOR       = 1 << 0;
export const PERM_MANAGE_SERVER       = 1 << 1;
export const PERM_MANAGE_ROLES        = 1 << 2;
export const PERM_MANAGE_CHANNELS     = 1 << 3;
export const PERM_KICK_MEMBERS        = 1 << 4;
export const PERM_BAN_MEMBERS         = 1 << 5;
export const PERM_MANAGE_NICKNAMES    = 1 << 6;
export const PERM_CHANGE_NICKNAME     = 1 << 7;
export const PERM_CREATE_INVITE       = 1 << 8;
export const PERM_VIEW_AUDIT_LOG      = 1 << 9;
export const PERM_MANAGE_WEBHOOKS     = 1 << 10;
export const PERM_MANAGE_EXPRESSIONS  = 1 << 11;
export const PERM_MANAGE_EVENTS       = 1 << 12;
export const PERM_MODERATE_MEMBER     = 1 << 13;

// Channel permissions — Text & Bin (bits 16-31)
export const PERM_VIEW_CHANNEL           = 1 << 16;
export const PERM_SEND_MESSAGES          = 1 << 17;
export const PERM_EMBED_LINKS            = 1 << 18;
export const PERM_ATTACH_FILES           = 1 << 19;
export const PERM_ADD_REACTIONS          = 1 << 20;
export const PERM_MENTION_EVERYONE       = 1 << 21;
export const PERM_MANAGE_MESSAGES        = 1 << 22;
export const PERM_READ_MESSAGE_HISTORY   = 1 << 23;
export const PERM_USE_EXTERNAL_EMOJI     = 1 << 24;
export const PERM_PIN_MESSAGES           = 1 << 25;
export const PERM_MANAGE_THREADS         = 1 << 26;
export const PERM_CREATE_PUBLIC_THREADS  = 1 << 27;
export const PERM_SEND_MESSAGES_IN_THREADS = 1 << 28;
export const PERM_CREATE_POSTS           = 1 << 29;
export const PERM_MANAGE_POSTS           = 1 << 30;
export const PERM_MANAGE_TAGS            = 1 << 31;

// Voice (bits 32-41) — use BigInt for bits > 31 in JS, or split into high/low
// For simplicity, we'll use two 32-bit numbers or handle via Number for bits <= 52
export const PERM_CONNECT          = 2 ** 32;
export const PERM_SPEAK            = 2 ** 33;
export const PERM_MUTE_MEMBERS     = 2 ** 34;
export const PERM_DEAFEN_MEMBERS   = 2 ** 35;
export const PERM_MOVE_MEMBERS     = 2 ** 36;
export const PERM_USE_VAD          = 2 ** 37;
export const PERM_PRIORITY_SPEAKER = 2 ** 38;
export const PERM_STREAM           = 2 ** 39;
export const PERM_USE_SOUNDBOARD   = 2 ** 40;
export const PERM_SEND_VOICE_MESSAGES = 2 ** 41;

export const PERM_ALL = (2 ** 42) - 1;

// Permission categories for UI display
export const PERMISSION_CATEGORIES = {
  General: [
    { name: 'Administrator', bit: PERM_ADMINISTRATOR, desc: 'Bypasses all permission checks' },
    { name: 'Manage Server', bit: PERM_MANAGE_SERVER, desc: 'Edit server name, icon, settings' },
    { name: 'Manage Roles', bit: PERM_MANAGE_ROLES, desc: 'Create/edit/delete roles' },
    { name: 'Manage Channels', bit: PERM_MANAGE_CHANNELS, desc: 'Create/edit/delete channels' },
    { name: 'Kick Members', bit: PERM_KICK_MEMBERS, desc: 'Kick members from server' },
    { name: 'Ban Members', bit: PERM_BAN_MEMBERS, desc: 'Ban members from server' },
    { name: 'Manage Nicknames', bit: PERM_MANAGE_NICKNAMES, desc: 'Change other members\' nicknames' },
    { name: 'Change Nickname', bit: PERM_CHANGE_NICKNAME, desc: 'Change own nickname' },
    { name: 'Create Invite', bit: PERM_CREATE_INVITE, desc: 'Create invite links' },
  ],
  Text: [
    { name: 'View Channel', bit: PERM_VIEW_CHANNEL, desc: 'See channels in the sidebar' },
    { name: 'Send Messages', bit: PERM_SEND_MESSAGES, desc: 'Send messages in text channels' },
    { name: 'Embed Links', bit: PERM_EMBED_LINKS, desc: 'Links auto-embed previews' },
    { name: 'Attach Files', bit: PERM_ATTACH_FILES, desc: 'Upload files and images' },
    { name: 'Add Reactions', bit: PERM_ADD_REACTIONS, desc: 'Add reactions to messages' },
    { name: 'Mention @everyone', bit: PERM_MENTION_EVERYONE, desc: 'Use @everyone and @here' },
    { name: 'Manage Messages', bit: PERM_MANAGE_MESSAGES, desc: 'Delete others\' messages, pin' },
    { name: 'Read Message History', bit: PERM_READ_MESSAGE_HISTORY, desc: 'Read messages sent before joining' },
    { name: 'Pin Messages', bit: PERM_PIN_MESSAGES, desc: 'Pin messages in channel' },
  ],
  Bin: [
    { name: 'Create Posts', bit: PERM_CREATE_POSTS, desc: 'Create bin posts' },
    { name: 'Manage Posts', bit: PERM_MANAGE_POSTS, desc: 'Edit/delete others\' posts' },
    { name: 'Manage Tags', bit: PERM_MANAGE_TAGS, desc: 'Create/delete channel tags' },
  ],
  Voice: [
    { name: 'Connect', bit: PERM_CONNECT, desc: 'Connect to voice channels' },
    { name: 'Speak', bit: PERM_SPEAK, desc: 'Speak in voice channels' },
    { name: 'Mute Members', bit: PERM_MUTE_MEMBERS, desc: 'Server-mute others' },
    { name: 'Deafen Members', bit: PERM_DEAFEN_MEMBERS, desc: 'Server-deafen others' },
    { name: 'Move Members', bit: PERM_MOVE_MEMBERS, desc: 'Move members between channels' },
    { name: 'Use Voice Activity', bit: PERM_USE_VAD, desc: 'Use voice activity detection' },
  ],
};

export interface PermOverwrite {
  id: string;
  channel_id: string;
  target_type: number; // 0 = role, 1 = member
  target_id: string;
  allow: number;
  deny: number;
}

export function hasPerm(perms: number, perm: number): boolean {
  return (perms & perm) === perm;
}

export function computeBasePermissions(
  everyonePerms: number,
  memberRolePerms: number[],
  isOwner: boolean
): number {
  if (isOwner) return PERM_ALL;
  let perms = everyonePerms;
  for (const rp of memberRolePerms) perms |= rp;
  if (perms & PERM_ADMINISTRATOR) return PERM_ALL;
  return perms;
}

export function computeChannelPermissions(
  basePerms: number,
  memberID: string,
  memberRoleIDs: string[],
  everyoneRoleID: string,
  overwrites: PermOverwrite[]
): number {
  if (basePerms & PERM_ADMINISTRATOR) return PERM_ALL;
  let perms = basePerms;

  // @everyone overwrites
  const evOw = overwrites.find(o => o.target_type === 0 && o.target_id === everyoneRoleID);
  if (evOw) {
    perms &= ~evOw.deny;
    perms |= evOw.allow;
  }

  // Role overwrites
  const roleSet = new Set(memberRoleIDs);
  let roleAllow = 0, roleDeny = 0;
  for (const ow of overwrites) {
    if (ow.target_type === 0 && ow.target_id !== everyoneRoleID && roleSet.has(ow.target_id)) {
      roleAllow |= ow.allow;
      roleDeny |= ow.deny;
    }
  }
  perms &= ~roleDeny;
  perms |= roleAllow;

  // Member overwrites
  const memOw = overwrites.find(o => o.target_type === 1 && o.target_id === memberID);
  if (memOw) {
    perms &= ~memOw.deny;
    perms |= memOw.allow;
  }

  // Implicit denials — zero channel bits only, not server-only bits (match backend PermChannelMask)
  if (!(perms & PERM_VIEW_CHANNEL)) perms &= (1 << 16) - 1; // keep only bits 0-15 (server-only)
  if (!(perms & PERM_SEND_MESSAGES)) perms &= ~(PERM_MENTION_EVERYONE | PERM_ATTACH_FILES | PERM_EMBED_LINKS);
  if (!(perms & PERM_CONNECT)) perms &= ~(PERM_SPEAK | PERM_MUTE_MEMBERS | PERM_DEAFEN_MEMBERS | PERM_MOVE_MEMBERS | PERM_USE_VAD);

  return perms;
}
```

**IMPORTANT — JavaScript 32-bit limitation**: JavaScript's bitwise operators (`|`, `&`, `~`, `^`) operate on 32-bit signed integers. Permission bits 32+ (voice) will silently produce wrong results with bitwise ops.

**Solution**: All permission computation functions must use **BigInt** internally. Store permission values as `bigint` constants (`const PERM_CONNECT = 1n << 32n`), and convert to/from `number`/`string` at API boundaries. The `hasPerm`, `computeBasePermissions`, and `computeChannelPermissions` functions must all operate on `bigint`. When sending to the API, convert via `Number(bigintValue)` (safe up to 2^53, which covers 42 bits). When receiving from API, convert via `BigInt(numberValue)`.

Update all the constants above to use `bigint` syntax: `export const PERM_ADMINISTRATOR = 1n << 0n;` etc. Update the function signatures to accept and return `bigint`.

- [ ] **Step 2: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/permissions.ts
git commit -m "feat: add frontend permission constants and Discord-style computation engine"
```

---

### Task 13: Update Role Management Modal

**Files:**
- Modify: `frontend/src/components/modals/ManageRolesModal.tsx` (or ServerSettings.tsx — check where role management lives)
- Modify: `frontend/src/api/roles.ts` — if needed

- [ ] **Step 1: Update permission definitions**

Replace the existing 6-item `PERMISSIONS` array with `PERMISSION_CATEGORIES` from `lib/permissions.ts`. Render permissions grouped by category (General, Text, Voice, Bin) with section headers.

- [ ] **Step 2: Add @everyone role handling**

Mark the @everyone role distinctly in the role list (can't be deleted, can't be renamed). Show it at the bottom of the list (position 0).

- [ ] **Step 3: Add hierarchy enforcement in UI**

Disable editing roles at or above the current user's highest role position. Disable granting permissions the current user doesn't have (grey out those checkboxes). This requires knowing the current user's permissions — fetch via `GET /servers/{id}/my-permissions`.

- [ ] **Step 4: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ frontend/src/api/
git commit -m "feat: update role management modal with 42 permissions, categories, and hierarchy enforcement"
```

---

### Task 14: Channel Permission Settings UI

**Files:**
- Create: `frontend/src/components/settings/ChannelPermissions.tsx`
- Create: `frontend/src/components/settings/ChannelPermissions.css`
- Create: `frontend/src/api/overwrites.ts`

- [ ] **Step 1: Create overwrite API client**

`frontend/src/api/overwrites.ts`:

```typescript
export async function getOverwrites(channelId: string): Promise<PermOverwrite[]> { ... }
export async function upsertOverwrite(channelId: string, data: { target_type: number; target_id: string; allow: number; deny: number }): Promise<void> { ... }
export async function deleteOverwrite(channelId: string, overwriteId: string): Promise<void> { ... }
export async function getMyChannelPermissions(channelId: string): Promise<{ permissions: number }> { ... }
```

- [ ] **Step 2: Create ChannelPermissions component**

A component for channel/category settings with:
- **Add role/member dropdown** — search roles and members, add them to the overwrite list
- **Per-target permission grid** — for each overwrite target, show all channel permissions (from PERMISSION_CATEGORIES, excluding General/server-only) as rows
- **Tri-state toggle per permission**: green checkmark (allow) → grey slash (inherit) → red X (deny) → green. Clicking cycles through.
- **Sync indicator** — if channel has a parent category, show "Synced" or "Not synced" with a "Sync Now" button
- **Save button** — calls upsertOverwrite for each modified overwrite

CSS should match the green terminal theme. The tri-state toggles should use distinct colors: green (#32CD32), grey (#555), red (#ff4444).

- [ ] **Step 3: Integrate into channel settings**

Find where channel settings are rendered (likely in a modal or settings panel). Add a "Permissions" tab that renders `ChannelPermissions`.

- [ ] **Step 4: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/settings/ChannelPermissions.tsx frontend/src/components/settings/ChannelPermissions.css frontend/src/api/overwrites.ts
git commit -m "feat: add channel permission settings UI with tri-state toggles and category sync"
```

---

### Task 15: Permission-Gated UI Elements

**Files:**
- Modify: `frontend/src/components/chat/MessageInput.tsx` — disable if no SendMessages
- Modify: `frontend/src/components/chat/Message.tsx` — hide delete button if no ManageMessages
- Modify: `frontend/src/components/layout/ChannelList.tsx` — filter by ViewChannel
- Modify: Various components — add permission gates

- [ ] **Step 1: Create a usePermissions hook**

Create `frontend/src/hooks/usePermissions.ts` that:
- Takes serverID and optional channelID
- Returns computed permissions for the current user
- Uses data from AppContext (user's roles, server owner) and fetched overwrites
- Memoizes to avoid recomputation

- [ ] **Step 2: Gate message input**

In `MessageInput.tsx`, check `hasPerm(channelPerms, PERM_SEND_MESSAGES)`. If false, show a disabled input with placeholder "You do not have permission to send messages in this channel."

Check `hasPerm(channelPerms, PERM_ATTACH_FILES)` to show/hide upload button.

- [ ] **Step 3: Gate message actions**

In `Message.tsx`:
- Delete button for others' messages: only if `hasPerm(channelPerms, PERM_MANAGE_MESSAGES)`
- Pin button: only if `hasPerm(channelPerms, PERM_PIN_MESSAGES)`
- Add reaction: only if `hasPerm(channelPerms, PERM_ADD_REACTIONS)`

- [ ] **Step 4: Filter channel list**

In `ChannelList.tsx`, use the permission computation to filter channels where the user lacks ViewChannel. Hide empty categories. This should work client-side using cached role and overwrite data.

- [ ] **Step 5: Gate other actions**

- Channel create/edit/delete buttons: `hasPerm(serverPerms, PERM_MANAGE_CHANNELS)`
- Kick/ban buttons: `hasPerm(serverPerms, PERM_KICK_MEMBERS)` / `PERM_BAN_MEMBERS`
- Role management: `hasPerm(serverPerms, PERM_MANAGE_ROLES)`
- Bin create post: `hasPerm(channelPerms, PERM_CREATE_POSTS)`

- [ ] **Step 6: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 7: Commit**

```bash
git add frontend/src/hooks/usePermissions.ts frontend/src/components/
git commit -m "feat: add permission-gated UI elements across chat, channels, and server management"
```

---

### Task 16: WebSocket Events and Final Integration

**Files:**
- Modify: `internal/websocket/events.go` — add overwrite and role events
- Modify: `internal/channel/overwrites.go` — broadcast events on changes
- Modify: `frontend/src/hooks/useWebSocket.ts` — handle new events

- [ ] **Step 1: Add event constants**

In `internal/websocket/events.go`:
```go
EventChannelOverwriteUpdate = "CHANNEL_OVERWRITE_UPDATE"
EventRoleUpdate            = "ROLE_UPDATE"
EventRoleDelete            = "ROLE_DELETE"
```

- [ ] **Step 2: Broadcast on overwrite changes**

In the overwrite upsert and delete handlers:
- Broadcast `CHANNEL_OVERWRITE_UPDATE` with `{ channel_id, overwrites[] }` to the channel
- When a channel's `synced` flag changes (desync on direct modification, resync via API), broadcast `CHANNEL_UPDATE` with the updated channel object

In role update/delete handlers (already existing):
- Broadcast `ROLE_UPDATE` / `ROLE_DELETE` to the server
- **On role deletion**: call `repo.DeleteOverwritesByTarget(ctx, 0, roleID)` to clean up all permission overwrites referencing the deleted role (spec requirement). Broadcast `CHANNEL_OVERWRITE_UPDATE` for each affected channel.

- [ ] **Step 3: Handle events in frontend**

In `useWebSocket.ts`, add handlers for:
- `CHANNEL_OVERWRITE_UPDATE` — update cached overwrites for permission computation
- `ROLE_UPDATE` — update cached roles
- `ROLE_DELETE` — remove from cached roles

- [ ] **Step 4: Full backend build**

Run: `go build ./cmd/api && echo "Backend build OK"`

- [ ] **Step 5: Full frontend build**

Run: `cd frontend && npx tsc --noEmit && npm run build && echo "Frontend build OK"`

- [ ] **Step 6: Commit**

```bash
git add internal/websocket/events.go internal/channel/overwrites.go internal/server/ frontend/src/
git commit -m "feat: add WebSocket events for permission overwrites and role changes"
```

---

### Task 17: Final Verification and Smoke Test

- [ ] **Step 1: Full build**

Run: `go build ./cmd/api && cd frontend && npm run build`

- [ ] **Step 2: Manual smoke test**

1. Start the server (migrations run automatically)
2. Check that existing roles have remapped permissions
3. Verify @everyone role was seeded for all servers
4. Create a new server — verify @everyone role is auto-created
5. Edit @everyone role permissions
6. Create a role with ManageChannels, assign to a user
7. Create a channel, add an overwrite denying SendMessages for @everyone
8. Add an overwrite allowing SendMessages for a specific role
9. Verify users without the role can't send, users with it can
10. Create a category with overwrites, verify synced channels inherit
11. Modify a synced channel's overwrites, verify it desyncs
12. Set ViewChannel deny on @everyone for a channel, verify it's hidden
13. Test kick/ban hierarchy enforcement
14. Verify the tri-state permission UI works in channel settings
