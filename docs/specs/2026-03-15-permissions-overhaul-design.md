# Permissions Overhaul Design Spec

## Overview

Replace Parley's 6-bit server-wide permission system with a full Discord-style permission model: 42 permission bits, immutable `@everyone` baseline role, strict positional role hierarchy, tri-state (allow/deny/inherit) permission overwrites at category, channel, and member levels, category sync, and View Channel visibility filtering.

## Permission Bitfield

All permissions stored as `int64` bitfields (64 bits available, 42 defined, rest reserved).

### Server-Only Permissions (bits 0–13)

These apply server-wide and cannot be overridden per-channel.

| Bit | Constant | Description |
|-----|----------|-------------|
| 0 | `Administrator` | Bypasses all permission checks. Grants all permissions. |
| 1 | `ManageServer` | Edit server name, icon, settings |
| 2 | `ManageRoles` | Create/edit/delete roles below own highest role |
| 3 | `ManageChannels` | Create/edit/delete/reorder channels and categories |
| 4 | `KickMembers` | Kick members with lower role position |
| 5 | `BanMembers` | Ban members with lower role position |
| 6 | `ManageNicknames` | Change other members' nicknames |
| 7 | `ChangeNickname` | Change own nickname |
| 8 | `CreateInvite` | Create server invite links |
| 9 | `ViewAuditLog` | View the server audit log (future) |
| 10 | `ManageWebhooks` | Create/edit/delete webhooks (future) |
| 11 | `ManageExpressions` | Manage custom emoji/stickers (future) |
| 12 | `ManageEvents` | Create/edit/delete server events (future) |
| 13 | `ModerateMember` | Timeout members (future) |

Bits 14–15 reserved.

### Channel Permissions — Text & Bin (bits 16–31)

These can be overridden per-category, per-channel, or per-member.

| Bit | Constant | Description |
|-----|----------|-------------|
| 16 | `ViewChannel` | See the channel in the sidebar. Denying hides it entirely. |
| 17 | `SendMessages` | Send messages in text channels |
| 18 | `EmbedLinks` | Links auto-embed previews |
| 19 | `AttachFiles` | Upload files and images |
| 20 | `AddReactions` | Add new reactions to messages |
| 21 | `MentionEveryone` | Use `@everyone` and `@here` |
| 22 | `ManageMessages` | Delete others' messages, pin messages |
| 23 | `ReadMessageHistory` | Read messages sent before joining |
| 24 | `UseExternalEmoji` | Use emoji from other servers (future) |
| 25 | `PinMessages` | Pin messages in the channel |
| 26 | `ManageThreads` | Manage threads/forum posts (future) |
| 27 | `CreatePublicThreads` | Create public threads (future) |
| 28 | `SendMessagesInThreads` | Send messages in threads (future) |
| 29 | `CreatePosts` | Create bin posts |
| 30 | `ManagePosts` | Edit/delete others' bin posts |
| 31 | `ManageTags` | Create/delete bin channel tags |

### Channel Permissions — Voice (bits 32–41)

| Bit | Constant | Description |
|-----|----------|-------------|
| 32 | `Connect` | Connect to voice channels (enforced at voice join) |
| 33 | `Speak` | Speak in voice channels (enforced at voice join) |
| 34 | `MuteMembers` | Server-mute others in voice (future enforcement) |
| 35 | `DeafenMembers` | Server-deafen others in voice (future enforcement) |
| 36 | `MoveMembers` | Move members between voice channels (future enforcement) |
| 37 | `UseVAD` | Use voice activity detection vs push-to-talk (future enforcement) |
| 38 | `PrioritySpeaker` | Priority speaker in voice (future) |
| 39 | `Stream` | Share screen / go live (future) |
| 40 | `UseSoundboard` | Use soundboard sounds (future) |
| 41 | `SendVoiceMessages` | Send voice messages (future) |

Bits 42–63 reserved for future use.

### Bitmask Constants

- `ALL_PERMISSIONS` — all 42 bits set
- `CHANNEL_PERMISSION_MASK` — bits 16–41 (the only bits valid in overwrites)
- `DEFAULT_EVERYONE_PERMISSIONS` — `ViewChannel | SendMessages | ReadMessageHistory | AddReactions | EmbedLinks | AttachFiles | Connect | Speak | UseVAD | ChangeNickname | CreateInvite | CreatePosts`

## Role System

### @everyone Role

Every server has an immutable `@everyone` role:
- **Identified by `is_everyone = true`** column on `server_roles` (avoids BIGSERIAL ID conflicts). One per server, enforced by a partial unique index.
- **Position = 0** (always the lowest role)
- Cannot be deleted or renamed
- Not explicitly assigned via `server_member_roles` — every member has it implicitly. The permission computation fetches the `@everyone` role separately via `WHERE server_id = $1 AND is_everyone = true`, then ORs in the member's explicitly assigned roles from the join table.
- Permissions editable by server owner / users with ManageServer
- Created automatically when a server is created
- **Migration**: add `is_everyone BOOLEAN NOT NULL DEFAULT FALSE` column to `server_roles`. Seed an `@everyone` role for every existing server that doesn't have one. Rename any existing roles named `@everyone` to `everyone` before seeding to avoid the UNIQUE(server_id, name) constraint.

### Role Hierarchy

Roles ordered by `position` (higher number = more powerful). Strictly enforced:

- **Role management**: Can only create/edit/delete roles with position below your highest role
- **Permission granting**: Can only grant permissions you yourself have
- **Kick/Ban**: Can only kick/ban members whose highest role position is below yours
- **Role assignment**: Can only assign/remove roles below your highest role
- **Owner bypass**: Server owner bypasses all hierarchy checks, always has all permissions

### Base Permission Computation

```
compute_base_permissions(member, server):
    if server.owner_id == member.user_id:
        return ALL_PERMISSIONS

    perms = everyone_role.permissions

    for role in member.roles:
        perms |= role.permissions

    if perms & Administrator:
        return ALL_PERMISSIONS

    return perms
```

## Permission Overwrites

### Overwrite Model

Each overwrite links a target (role or user) to a channel (or category) with explicit `allow` and `deny` bitfields.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| channel_id | BIGINT FK → channels | The channel or category |
| target_type | SMALLINT NOT NULL | 0 = role, 1 = member |
| target_id | BIGINT NOT NULL | Role ID or User ID |
| allow | BIGINT NOT NULL DEFAULT 0 | Permissions explicitly granted |
| deny | BIGINT NOT NULL DEFAULT 0 | Permissions explicitly denied |
| UNIQUE(channel_id, target_type, target_id) | | One overwrite per target per channel |

Only channel-scoped permissions (bits 16–41) are valid in overwrites. Server-only permission bits (0–13) are masked out before storage.

### Tri-State Logic

For each permission bit in an overwrite:
- **Allow (green)**: Bit set in `allow`, not in `deny` → explicitly granted
- **Deny (red)**: Bit set in `deny`, not in `allow` → explicitly denied
- **Inherit (grey)**: Bit set in neither → falls through to the level above

A bit should never be set in both `allow` and `deny` simultaneously. The API enforces this by clearing a bit from `deny` when setting it in `allow` and vice versa.

### Channel Permission Computation

Matches Discord's algorithm exactly:

```
compute_channel_permissions(member, channel, server):
    base = compute_base_permissions(member, server)

    if base & Administrator:
        return ALL_PERMISSIONS

    perms = base

    # Step 1: @everyone overwrites on this channel
    everyone_ow = get_overwrite(channel_id, role, server.id)
    if everyone_ow:
        perms &= ~everyone_ow.deny
        perms |= everyone_ow.allow

    # Step 2: Role overwrites (combined across all member's roles)
    role_allow = 0
    role_deny = 0
    for role in member.roles:
        ow = get_overwrite(channel_id, role, role.id)
        if ow:
            role_allow |= ow.allow
            role_deny |= ow.deny
    perms &= ~role_deny
    perms |= role_allow

    # Step 3: Member-specific overwrite (highest priority)
    member_ow = get_overwrite(channel_id, member, member.user_id)
    if member_ow:
        perms &= ~member_ow.deny
        perms |= member_ow.allow

    return perms
```

### Implicit Denials

Enforced in computation, not in storage:
- Denying `ViewChannel` → all other channel permissions denied
- Denying `SendMessages` → `MentionEveryone`, `AttachFiles`, `EmbedLinks` denied
- Denying `Connect` → all voice permissions denied

### Category Sync

Channels in a category have a `synced` boolean (default `true`). Sync is **write-time only** — synced channels always have a physical copy of the category's overwrites in the `permission_overwrites` table. The permission computation algorithm does not need category awareness; it only reads the channel's own overwrites.

- **Synced**: Channel's overwrites mirror the parent category's overwrites exactly. When category overwrites change, all synced children have their overwrites replaced with the category's current set (write-time propagation).
- **Desynced**: Channel has independently modified overwrites. Category changes do not propagate.
- A channel desyncs automatically when its overwrites are directly modified.
- A desynced channel can be resynced by setting `synced: true` via the API — this replaces its overwrites with the category's current overwrites.
- New channels created in a category start synced, copying the category's overwrites.

### View Channel Visibility

- Channels where the user lacks `ViewChannel` are hidden from the channel list API response
- Categories with zero visible children are also hidden
- Server owner always sees all channels
- Users with `Administrator` always see all channels
- API endpoints for channels where the user lacks `ViewChannel` must return **404 (not 403)** to avoid leaking channel existence

## Database Changes

### New Table: `permission_overwrites`

```sql
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
```

### Modifications to Existing Tables

**`channels`** — add sync tracking:
```sql
ALTER TABLE channels ADD COLUMN IF NOT EXISTS synced BOOLEAN NOT NULL DEFAULT TRUE;
-- Note: `synced` is only semantically meaningful for non-category channels.
-- Categories always have synced=true but it has no effect (they are the sync source, not target).
```

**`server_roles`** — add `is_everyone` flag:
```sql
ALTER TABLE server_roles ADD COLUMN IF NOT EXISTS is_everyone BOOLEAN NOT NULL DEFAULT FALSE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_server_roles_everyone ON server_roles(server_id) WHERE is_everyone = TRUE;
```
The partial unique index ensures at most one `@everyone` role per server. Permissions column is already `BIGINT`, no change needed.

**`Channel` struct** — add `Synced bool` field to the Go model and include it in API responses for `GET /servers/{serverID}/channels`.

### Migration: Remap Permission Bits

Existing roles use bits 0–5 for 6 permissions. Remap to the new layout:

| Old Bit | Old Permission | New Bit | New Permission |
|---------|---------------|---------|----------------|
| 0 (1) | SendMessages | 17 | SendMessages |
| 1 (2) | ManageMessages | 22 | ManageMessages |
| 2 (4) | ManageChannels | 3 | ManageChannels |
| 3 (8) | KickMembers | 4 | KickMembers |
| 4 (16) | ManageServer | 1 | ManageServer |
| 5 (32) | Administrator | 0 | Administrator |

SQL migration reads each role's `permissions`, remaps bits, writes the new value.

### Migration: Seed @everyone Roles

For every existing server, INSERT a role with `name = '@everyone'`, `position = 0`, `is_everyone = true`, `permissions = DEFAULT_EVERYONE_PERMISSIONS` (the `id` is auto-generated by BIGSERIAL — identification is via the `is_everyone` flag, not the ID). The `CreateServer` flow is updated to do this automatically for new servers.

## API Changes

### Updated Endpoints

**`GET /servers/{id}/my-permissions`** — returns computed base permissions using new bitfield:
```json
{ "permissions": "<int64 bitfield>", "is_owner": false }
```
The `permissions` value reflects the requesting member's computed base permissions (all roles OR'd together). Example: a member with only `@everyone` defaults would have the `DEFAULT_EVERYONE_PERMISSIONS` value.

**`GET /servers/{serverID}/channels`** — filters out channels where user lacks `ViewChannel`. Categories with zero visible children omitted. Owner sees all. Each channel includes `synced` boolean.

**Role CRUD** (existing endpoints, updated enforcement):
- `POST /servers/{id}/roles` — requires `ManageRoles`. Can only set permissions the actor has. Position must be below actor's highest role.
- `PATCH /servers/{id}/roles/{roleId}` — same constraints. Can't edit roles at or above own position. Can't rename or delete `@everyone`.
- `DELETE /servers/{id}/roles/{roleId}` — can't delete roles at or above own position. Can't delete `@everyone`.
- `POST /servers/{id}/members/{userID}/roles` — can only assign roles below own highest position.
- `DELETE /servers/{id}/members/{userID}/roles/{roleId}` — same constraint.

**Kick/Ban** — now enforces role hierarchy. Actor's highest role must be above target's highest role. The `CanKickBan` method in `server_members.go` is updated to fetch both actor's and target's highest role positions and compare them, returning 403 if the target outranks the actor.

### New Endpoints

**Channel permission overwrites:**
```
GET    /channels/{id}/overwrites                — List all overwrites for a channel/category
PUT    /channels/{id}/overwrites                — Upsert an overwrite: { target_type, target_id, allow, deny }
DELETE /channels/{id}/overwrites/{overwriteId}  — Remove an overwrite
```

Requires `ManageRoles` **or** `ManageChannels` permission. Can only set permission bits the actor has. Server-only bits (0–13) are masked out.

**Per-channel permissions:**
```
GET    /channels/{id}/my-permissions            — Computed permissions for requesting user
```

Returns `{ "permissions": "<int64 bitfield>" }` with all overwrites applied.

### Category Sync Behavior

When category overwrites are modified via `PUT /channels/{id}/overwrites`:
1. Update the category's overwrite
2. Find all child channels with `synced = true`
3. Replace their overwrites with the category's current full overwrite set (this bulk delete-then-copy is a service-internal transaction, not a separate API operation)

When a channel's overwrites are directly modified:
1. Set `synced = false` on the channel
2. Apply the overwrite change

When `PUT /channels/{id}` includes `synced: true`:
1. Delete the channel's current overwrites
2. Copy the parent category's overwrites
3. Set `synced = true`

### WebSocket Events

| Event | Payload | When |
|-------|---------|------|
| `CHANNEL_OVERWRITE_UPDATE` | `{ channel_id, overwrites[] }` | Overwrite added/edited/removed |
| `ROLE_UPDATE` | `{ role }` | Role permissions or position changed |
| `ROLE_DELETE` | `{ role_id, server_id }` | Role deleted |

Existing events used: `CHANNEL_UPDATE` (sync state), `MEMBER_ROLE_UPDATE` (role assignment).

### API Documentation

Full endpoint documentation for all permission-related endpoints will be added to `docs/endpoints/` covering request/response formats, error codes, and examples for bot developers.

## Frontend Changes

### Permission Utility Module

`frontend/src/lib/permissions.ts`:
- `computeBasePermissions(member, roles, serverOwnerId)` — OR all role permissions, owner/admin bypass
- `computeChannelPermissions(basePerms, memberRoles, channelOverwrites, memberId)` — full overwrite algorithm
- `hasPermission(perms, bit)` — check a single bit
- All 42 permission bit constants exported
- `CHANNEL_PERMISSION_MASK` — bits valid for overwrites

### Channel List Filtering

`ChannelList.tsx` filters channels through `computeChannelPermissions`. Channels where user lacks `ViewChannel` are hidden. Categories with zero visible children are hidden. Owner sees everything. Client-side filtering handles real-time overwrite updates without refetch.

### Channel Settings — Permissions Tab

New tab in channel/category settings (requires `ManageRoles` or `ManageChannels`):
- **Add role/member**: search dropdown to add targets to the overwrite list
- **Per-target permission grid**: each permission as a row with three toggle states:
  - Green checkmark = allow
  - Grey slash = inherit (neutral)
  - Red X = deny
  - Clicking cycles: inherit → allow → deny → inherit
- **Category header**: organized by permission category (General, Text, Voice, Bin)
- **Sync indicator**: for channels in a category, shows sync status with "Sync Now" button for desynced channels
- Save sends `PUT /channels/{id}/overwrites`

### Role Management Modal Updates

`ManageRolesModal.tsx` — expand from 6 checkboxes to full permission set, organized by category:
- **General**: Administrator, ManageServer, ManageRoles, ManageChannels, KickMembers, BanMembers, etc.
- **Text**: SendMessages, ManageMessages, EmbedLinks, AttachFiles, etc.
- **Voice**: Connect, Speak, MuteMembers, DeafenMembers, etc.
- **Bin**: CreatePosts, ManagePosts, ManageTags

Role permissions remain binary (on/off), not tri-state. Tri-state is only for channel overwrites.

### Permission-Gated UI Elements

Hide or disable actions the user lacks permission for:
- Message input → `SendMessages`
- Delete others' messages → `ManageMessages`
- Channel create/edit/delete → `ManageChannels`
- Kick/ban buttons → `KickMembers` / `BanMembers`
- Role management → `ManageRoles`
- Pin message → `PinMessages`
- Bin create post → `CreatePosts`
- Upload button → `AttachFiles`
- Reaction button → `AddReactions`
- @everyone/@here in message → `MentionEveryone`

## Edge Cases and Conflict Resolution

### Permission Conflicts Between Roles

When a user has multiple roles with overwrites on the same channel:
1. All role denies are OR'd together
2. All role allows are OR'd together
3. Deny is applied first (`perms &= ~deny`)
4. Allow is applied second (`perms |= allow`)

This means **allow wins** when two roles conflict at the same level. This matches Discord.

### Member Overwrite vs Role Overwrite

Member-specific overwrites always win over role overwrites, regardless of direction. A member deny overrides a role allow, and a member allow overrides a role deny.

### Administrator Bypass

Users with `Administrator` permission (via any role) bypass all channel overwrite checks. They always have all permissions in all channels. The only exception: they cannot override the server owner.

### Server Owner

The server owner always has all permissions everywhere. They cannot be kicked, banned, or have permissions restricted. They see all channels regardless of ViewChannel overwrites.

### Deleted Roles

When a role is deleted, all overwrites referencing that role must be cleaned up at the application level (since `target_id` has no FK constraint — it can reference either roles or users). The role deletion service method must `DELETE FROM permission_overwrites WHERE target_type = 0 AND target_id = $role_id` as part of the delete operation. Members who had that role lose any permissions it granted.

### @everyone Overwrite

The `@everyone` role can have channel overwrites like any other role. Since every member implicitly has this role, @everyone overwrites affect all non-administrator members. This is the primary way to restrict a channel (e.g., deny SendMessages on @everyone, then allow it for specific roles).

## Out of Scope

- Audit log implementation (permission bit defined, feature is future)
- Member timeout implementation (permission bit defined, feature is future)
- Webhook management (permission bit defined, feature is future)
- Thread permission enforcement (permission bits defined, threads are future)
- Permission caching layer (optimize later if needed)
