# Group DMs — design spec

**Status:** Design approved 2026-04-24. Implementation pending.
**Sub-project:** A of two. Sub-project B (call infra for DMs + group DMs) follows in a separate spec.
**Owner:** Dylan Hart.
**Repo branch (planned):** `feat/group-dms` (worktree-based, per recent project pattern).

---

## 1. Summary

Extend Parley's existing 1:1 direct messages to support multi-party group DMs (max 100 members), with cross-cutting per-channel read-state and notification settings introduced as part of the same project (applies to server channels too). Skype/Discord-style: anyone in the group can add anyone, only the owner can kick, anyone can leave, themed generated default names, composite mosaic avatars, transferable ownership.

## 2. Product decisions

| Topic | Decision |
|---|---|
| Member cap | 100 |
| Add members | Anyone in group can add anyone (no friend constraint) |
| Kick | Owner-only |
| Leave | Anyone, anytime |
| 1:1 → group promotion | Spawns brand-new group; original 1:1 untouched (privacy: 1:1 history doesn't leak into the group) |
| Default name | Generator picks at create-time from size-bucketed lists (3 / 4–10 / 11+); locked once set, not regenerated on threshold crossings |
| Default avatar | Composite mosaic of first 4 member avatars, computed client-side; uploaded avatar overrides |
| Rename / avatar change | Anyone in group; emits chat system message |
| Owner leaves | Modal forces successor pick before leaving; escape hatch "leave without transferring → kick power evaporates" |
| Owner transfer (without leaving) | Standalone owner-only context-menu action; emits chat system message |
| Read-state | Cross-cutting: `last_read_message_id` per `(user, channel)` for **both** server channels and DMs |
| Mute / notification setting | 3-state per `(user, channel)`: `ALL` / `MENTIONS_ONLY` / `MUTED`; cross-cutting same as read-state |

## 3. Architecture

### 3.1 Backend code shape

```
cmd/api/                                       (existing, extended)
  routes.go                                    + new routes (Section 5)
internal/dm/
  handler.go                                   (extended; one code path for 1:1 + group)
  service.go                                   NEW — business logic moved out of handler
  names.go                                     NEW — themed default-name generator
internal/db/
  models.go                                    (extended — Section 4)
  dm_repository.go                             (extended)
  user_channel_state_repository.go             NEW — cross-cutting read/mute repo
  migrations.go                                + migration #64 (Section 8)
internal/notification/service.go               (extended — group fan-out, mute filter)
internal/websocket/                            (existing — new event types fan out via ws.Hub)
```

The new `internal/dm/service.go` exists because today's `internal/dm/handler.go` is ~500 LOC of mixed HTTP-shape and business logic. Adding group-DM management, system-message generation, ownership-transfer, and themed-name picking pushes it past comfortable readability. Pattern matches `internal/server/service.go` and `internal/message/service.go` — extracted service layer with handler kept thin.

### 3.2 Frontend code shape

New components:
- `frontend/src/components/dm/CreateGroupModal.tsx`
- `frontend/src/components/dm/GroupMembersPanel.tsx`
- `frontend/src/components/dm/AddPeopleModal.tsx`
- `frontend/src/components/dm/TransferOwnershipModal.tsx`
- `frontend/src/components/dm/LeaveGroupModal.tsx`
- `frontend/src/components/dm/MosaicAvatar.tsx`
- `frontend/src/components/chat/SystemMessage.tsx`

Modified components:
- `frontend/src/components/chat/ChatWindow.tsx` (group-aware membership + header)
- `frontend/src/components/chat/MessageList.tsx` (system message rendering, last-read marker)
- `frontend/src/components/layout/DmPanel.tsx` (mosaic avatar, member count, mute icon, unread dot)
- `frontend/src/components/layout/Sidebar.tsx` ("+ New Group" entry point)
- `frontend/src/components/layout/ChannelList.tsx` (server-channel notification settings + unread badge)

New hooks:
- `useChannelReadState(channelKind, channelId)` — fetches + tracks `last_read_message_id`; exposes `markRead(messageId)`.
- `useChannelNotificationSetting(channelKind, channelId)` — read/write `ALL`/`MENTIONS_ONLY`/`MUTED`.
- `useGroupMembers(dmChannelId)` — fetches + caches member list; subscribes to `DM_MEMBER_ADD/REMOVE` WS events.

## 4. Schema changes

Single migration #64 (next available — #63 was D3 scopes), all in one transaction.

```sql
-- 4.1: Generalize dm_channels
ALTER TABLE dm_channels
    ADD COLUMN is_group           BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN name               TEXT,                                    -- nullable; only meaningful when is_group
    ADD COLUMN avatar_url         TEXT,                                    -- nullable; null = composite mosaic
    ADD COLUMN created_by_user_id BIGINT      REFERENCES users(id),
    ADD COLUMN owner_user_id      BIGINT      REFERENCES users(id);        -- mutable; transferable

-- 4.2: Source-of-truth membership join table (covers 1:1 + group)
CREATE TABLE dm_channel_members (
    dm_channel_id BIGINT      NOT NULL REFERENCES dm_channels(id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dm_channel_id, user_id)
);
CREATE INDEX dm_channel_members_user_idx ON dm_channel_members(user_id);

-- 4.3: Backfill existing 1:1 channels
INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
SELECT id, user1_id, created_at FROM dm_channels
UNION ALL
SELECT id, user2_id, created_at FROM dm_channels
ON CONFLICT DO NOTHING;
-- user1_id/user2_id columns kept as fast-lookup hint for is_group=false; join table is authoritative.

-- 4.4: System events on dm_messages
ALTER TABLE dm_messages
    ADD COLUMN system_event JSONB;   -- nullable; null = normal user message; non-null = synthetic event

-- 4.5: Cross-cutting read-state + notification settings (server channels too)
CREATE TABLE user_channel_state (
    user_id              BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_kind         SMALLINT    NOT NULL CHECK (channel_kind IN (1, 2)),  -- 1=server, 2=dm
    channel_id           BIGINT      NOT NULL,
    last_read_message_id BIGINT,                                                -- nullable; null = never read
    notification_setting SMALLINT    NOT NULL DEFAULT 0,                         -- 0=ALL, 1=MENTIONS_ONLY, 2=MUTED
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, channel_kind, channel_id)
);
CREATE INDEX user_channel_state_user_idx ON user_channel_state(user_id);
```

**Notes:**
- `system_event` is `JSONB` because per-event payloads vary (`name_changed` carries old/new name; `member_kicked` carries target_user_id). Single column, one switch in the renderer.
- `user1_id`/`user2_id` on `dm_channels` are kept as a fast-lookup hint for 1:1 (`WHERE NOT is_group AND user1_id = ? AND user2_id = ?`), used by the existing `GetOrCreateDmChannel(userA, userB)` query.
- `user_channel_state` is INSERT-on-first-write — no row preallocated. Default state (no row) = `ALL` notification, never-read.
- `channel_kind` discriminator avoids polymorphic FK; integrity at app layer.
- `notification_setting` is enum-via-int (room to add cases later without migration).

The Go `DmChannel` and `DmMessage` structs (in `internal/db/models.go`) gain the matching fields. All int64 IDs continue to use `,string` json tags (per the recent `9a85c91` / `afaa2ac` fixes).

## 5. API surface

### 5.1 Channel lifecycle

| Method | Path | Purpose | Auth |
|---|---|---|---|
| POST | `/api/dms` | **Extended.** Body `{user_ids: [N]}`. `[1]` → 1:1 (existing path), `[2+]` → group. Optional `{name}`. Server picks themed default if `name` absent. Old `{user_id: X}` body still accepted as synonym for `{user_ids: [X]}`. | authed user |
| GET | `/api/dms` | List user's DM channels (existing; now returns mixed 1:1 + group). | authed user |
| GET | `/api/dms/{id}/messages` | Existing. Same shape for groups. System messages interleaved by `created_at`. | member of channel |

### 5.2 Group membership

| Method | Path | Purpose | Auth |
|---|---|---|---|
| POST | `/api/dms/{id}/members` | Add member(s). Body `{user_ids: [...]}`. Returns 400 on a 1:1 channel (use `POST /api/dms` to spawn a new group from a 1:1). | any member |
| DELETE | `/api/dms/{id}/members/{userId}` | Kick. 403 for non-owner trying to kick anyone. | owner |
| POST | `/api/dms/{id}/leave` | Leave. Optional body `{transfer_to: userId}` — when current user is owner, atomically transfers before removing self. (Body-not-query for consistency with the rest of POST endpoints.) | self only |

### 5.3 Group metadata

| Method | Path | Purpose | Auth |
|---|---|---|---|
| PATCH | `/api/dms/{id}` | Update name. Body `{name: "..."}`. Emits `name_changed` system message. | any member |
| POST | `/api/dms/{id}/avatar` | Upload custom avatar (multipart, reuses `internal/spaces` upload). Emits `avatar_changed` system message. | any member |
| DELETE | `/api/dms/{id}/avatar` | Revert to mosaic (clears `avatar_url`). Emits `avatar_changed` system message with `removed: true`. | any member |
| POST | `/api/dms/{id}/owner` | Transfer ownership. Body `{user_id}`. Emits `owner_transferred` system message. | current owner |

### 5.4 Cross-cutting read-state and notifications

| Method | Path | Purpose | Auth |
|---|---|---|---|
| POST | `/api/channels/{id}/read` | Update `last_read_message_id` for a server channel. Body `{message_id}`. UPSERT on `user_channel_state`. | member of server |
| POST | `/api/dms/{id}/read` | Same, for DM. | member of channel |
| PATCH | `/api/channels/{id}/notifications` | Body `{setting: "ALL"\|"MENTIONS_ONLY"\|"MUTED"}`. UPSERT on `user_channel_state`. | member of server |
| PATCH | `/api/dms/{id}/notifications` | Same. | member of channel |

### 5.5 WebSocket events

Broadcast through existing `ws.Hub` over `dm:<channelID>` per-channel topic.

- `DM_CHANNEL_CREATE` — group creation, fan out to all initial members
- `DM_CHANNEL_UPDATE` — name / avatar / owner changes
- `DM_MEMBER_ADD` — new member, fan out to existing members + the new member's connected sessions
- `DM_MEMBER_REMOVE` — leave or kick, fan out to remaining members + the removed user
- `DM_MESSAGE_CREATE` (existing, reused for system messages — frontend distinguishes by `system_event != null`)
- `DM_MESSAGE_UPDATE` / `DM_MESSAGE_DELETE` (existing)
- `CHANNEL_READ_STATE_UPDATE` — only fan out to the user's own sessions (multi-tab sync)
- `CHANNEL_NOTIFICATION_UPDATE` — same single-user fan-out

### 5.6 Bot scopes

D3 scope mapping:
- All DM message endpoints: `messages:read` / `messages:write` per existing convention.
- Group-membership endpoints (`add`, `kick`, `transfer`, name/avatar changes): `profile:write` (mutates account-state-adjacent data).
- Read-state and notification-settings endpoints: `profile:write`.

## 6. Frontend behavior

### 6.1 New components

| Component | Behavior |
|---|---|
| `CreateGroupModal` | Multi-select user picker (typeahead by display_name + username, unfiltered by friend status), live preview of generated default name, optional name override, Create button (disabled until ≥2 picks). On submit: `POST /api/dms` with `user_ids`. |
| `GroupMembersPanel` | Slide-out right panel. Member list: avatar + display name + owner crown + (per-row context menu: kick (owner only), DM, view profile). Header buttons: Add People, Transfer Ownership (owner only), Leave Group, Mute toggle (3-way ALL / Mentions / Muted). |
| `AddPeopleModal` | Same picker as Create modal. Two contexts: (a) from a 1:1 → composes call to `POST /api/dms` with old + new users → opens new group; (b) from a group's panel → calls `POST /api/dms/{id}/members`. UI distinguishes the two: "This will create a new group with X, Y, Z" vs. "These users will be added to this group." |
| `TransferOwnershipModal` | Member picker (excludes self), confirmation step. |
| `LeaveGroupModal` | Two-step for owners: step 1 picker for successor (with "Leave without transferring — kick power evaporates" escape hatch), step 2 confirm. One-step for non-owners. |
| `MosaicAvatar` | 1/2/3/4-tile composite. Inputs: list of `{avatarUrl, displayName}` (used for initials fallback when avatarUrl empty). Reused in DmPanel rows, ChatWindow header, modal previews. |
| `SystemMessage` | Compact rendering of `system_event` payloads. Small italic gray, centered, no avatar, no hover actions. Switch on `event.type` for the localized text. |

### 6.2 Modified components

- `ChatWindow.tsx` — read membership from new `members` field on the channel object instead of inferring from `user1_id/user2_id`. Group-aware header: shows mosaic avatar + group name + member count + Members button. `canManageMessages = false` and `canPin = false` continue to apply (DMs/groups don't get mod tools or pins).
- `MessageList.tsx` — render `<SystemMessage>` when `message.system_event != null`. Update last-read-marker on scroll-to-bottom + window-focus events. Render a "New" divider above the first unread message.
- `DmPanel.tsx` — group rows show `MosaicAvatar` + member count ("Cool Crew · 7"); muted-icon (🔕) per row; unread dot from new `last_read_message_id` data. Sort: most-recent-message first (existing behavior).
- `Sidebar.tsx` — "+ New Group" entry point. Exact placement is an open question deferred to build time; see §13.
- `ChannelList.tsx` — per-channel context menu gets a Notification settings submenu (ALL / Mentions only / Muted); unread-badge rendering for server channels in the sidebar.

### 6.3 Notification client logic

Single gate, applied to every WS message receipt:

```ts
function shouldNotify(msg, setting, currentUserId) {
  if (setting === 'MUTED') return false;
  if (msg.author_id === currentUserId) return false;
  if (setting === 'MENTIONS_ONLY') return msg.mentions.includes(currentUserId);
  return true;  // ALL
}
```

Wraps existing `useNotifications` hook's toast/sound dispatch. Backend always fans out the message; muting is purely a client-side concern (lets the user unmute and see history without server replay).

## 7. System messages

### 7.1 Event types

| `type` | Payload (JSONB) | Rendered as |
|---|---|---|
| `group_created` | `{actor_user_id}` | "Alice created the group" |
| `member_added` | `{actor_user_id, target_user_id}` | "Alice added Bob" |
| `member_left` | `{actor_user_id}` (== target) | "Bob left the group" |
| `member_kicked` | `{actor_user_id, target_user_id}` | "Alice removed Bob" |
| `name_changed` | `{actor_user_id, old_name, new_name}` | "Alice renamed the group to 'Cool Crew'" |
| `avatar_changed` | `{actor_user_id, removed: bool}` | `removed=false`: "Alice changed the group avatar" / `removed=true`: "Alice reset the group avatar" |
| `owner_transferred` | `{actor_user_id, new_owner_user_id}` | "Alice transferred ownership to Bob" |

### 7.2 Storage + flow

System messages are stored as `dm_messages` rows with `system_event` populated and `content` empty. They flow through the same `INSERT` + `DM_MESSAGE_CREATE` WS broadcast as user messages — same pagination, same scrollback. Frontend distinguishes by `system_event != null` and renders via `<SystemMessage>` instead of `<Message>`.

### 7.3 Atomic emission

System events are emitted in the **same DB transaction** as the underlying state change:

```go
// Pseudocode for service.AddMember
tx := db.Begin()
defer tx.Rollback()
tx.Exec("INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at) VALUES (?, ?, NOW())", channelID, target)
tx.Exec("INSERT INTO dm_messages (dm_channel_id, author_id, content, system_event, created_at, updated_at) VALUES (?, ?, '', ?, NOW(), NOW())",
    channelID, actor, fmt.Sprintf(`{"type":"member_added","actor_user_id":%d,"target_user_id":%d}`, actor, target))
tx.Commit()
hub.Broadcast(channelID, "DM_MEMBER_ADD", ...)
hub.Broadcast(channelID, "DM_MESSAGE_CREATE", systemMsg)
```

Same atomic pattern for kick / leave / rename / avatar / transfer.

### 7.4 Name resolution

Payload stores user IDs only (no snapshot usernames). Frontend resolves to current `display_name` at render time. If a user is hard-deleted, render falls back to "[deleted user]" — same convention as message-author rendering today. `old_name` and `new_name` are stored as literals (they ARE the data, not references).

## 8. Read-state, mute, and notifications (cross-cutting)

### 8.1 Read-marker semantics

- `user_channel_state` row exists only when the user has either marked something read or changed their notification setting. Default (no row) = `last_read_message_id = NULL`, `notification_setting = ALL`.
- Client POSTs `/{kind}/{id}/read` on (a) scroll-to-bottom + window-focused, or (b) explicit "mark as read" action.
- Optimistic local update; server is lazy source of truth. WS `CHANNEL_READ_STATE_UPDATE` fans out to user's other sessions for multi-tab sync.
- Unread badge: client compares `latest_message_id_in_channel > last_read_message_id`. NULL `last_read_message_id` + any messages = unread.
- Mention badge: separate, client-side only, computed by indexing message contents on receive.

### 8.2 Notification setting semantics

- `ALL` (0, default) — every new message → notification + sound + unread badge.
- `MENTIONS_ONLY` (1) — only @mentions → notification + sound. Unread badge still increments.
- `MUTED` (2) — no notification, no sound. Unread badge **still increments** (see Discord-style "muted but visible").

Backend always fans out the message; muting is purely client-side. (Lets users unmute and see full history; backend stays simple.)

### 8.3 Implicit read on send

When a user sends a message, that message is implicitly read by them. Server-side: on `INSERT dm_messages`, also UPSERT `user_channel_state(user_id=author, channel_kind=2, channel_id=dm_channel_id, last_read_message_id=new_id)`. Same for `INSERT messages` for server channels. Saves a round-trip.

### 8.4 Per-server mute (deferred)

A "mute the entire server" toggle is **out of scope**. Data model supports adding it later as a UI rollup over per-channel settings — just iterate channels. Per-channel mute is the only granularity in v1.

## 9. Migration and rollout

### 9.1 Single migration #64

All DDL + backfill in one transaction at api startup. Additive only — no destructive DDL. Fast (sub-second) at any realistic Parley scale.

### 9.2 Deploy ordering

```
[1] Apply migration (api restart triggers it)
[2] Roll new api binary (new endpoints, generalized 1:1+group, new WS events)
[3] Roll new frontend (uses new endpoints, renders groups, system messages, read-state)
```

Compat during rollout:
- Old api after migration: works (additive DDL).
- Old frontend with new api: works (new endpoints unused; existing 1:1 endpoints behave identically; no unread badges, matches today).
- New frontend with old api: 404 on group endpoints — avoid this ordering (always deploy api before frontend).

### 9.3 Rollback

Reverse migration:
```sql
DROP TABLE user_channel_state;
ALTER TABLE dm_messages DROP COLUMN system_event;
DROP TABLE dm_channel_members;
ALTER TABLE dm_channels DROP COLUMN owner_user_id, DROP COLUMN created_by_user_id, DROP COLUMN avatar_url, DROP COLUMN name, DROP COLUMN is_group;
```

Plus `git revert` of the api commit. Keep this SQL in `docs/security/runbooks/group-dms-rollback.md` for ops convenience.

## 10. Risk areas

| Risk | Mitigation |
|---|---|
| Migration backfill slow on large `dm_channels` | Sub-second at Parley scale (~hundreds of rows). Documented in runbook. |
| WS fan-out at 100-member groups | Existing hub handles server channels of similar scale. Smoke-test with a 100-member group + sustained chat. |
| UPSERT on `user_channel_state` per message-send | Cheap (PK lookup). Batch later if it ever shows up in profiling. |
| Owner-leave successor race | Atomic transaction in handler; reject + retry-prompt if successor gone. |
| `is_otheronline = onlineUserIds?.has(channel.other_user_id)` only valid for 1:1 | For groups: drop single-online-dot, use member count instead. Online presence in groups is a deferred enhancement (Section 11). |

## 11. Non-goals and deferrals

- Group calls / DM calls — sub-project B, separate spec.
- Per-server-level mute rollup — data model supports, no UI in v1.
- Online presence inside groups — defer (member count only in v1).
- Roles / permissions — flat model; owner-only-kick is the only role distinction.
- Group invite links — friend-list-style add only.
- Mention badges separate from message badges — combined badge in v1.
- Group-DM-aware bot APIs — bots are users; no special handling.
- Group-DM search — existing search treats groups same as 1:1.

## 12. Testing scope

- **Unit (`internal/dm/`)** — name generator picks from correct list per size bucket; `service.AddMember` / `KickMember` / `LeaveGroup` / `Rename` / `TransferOwnership` enforce auth rules; system-message payload shape per type; atomic transaction property.
- **Integration (`cmd/api/`)** — full flow create/add/kick/transfer/leave; WS event emission per flow; backfill migration against seeded 1:1 dataset.
- **Frontend (where harness reaches)** — `MosaicAvatar` 1/2/3/4 tiles; `SystemMessage` per event type; `CreateGroupModal` validates ≥2 picks; `LeaveGroupModal` owner flow forces successor.
- **Manual smoke** — create group, rename, add 3 more, kick one, transfer ownership, owner leaves, mute, unmute, verify unread badges and system messages.

## 13. Open questions resolved during implementation

(Not blocking — to be resolved during build, listed for visibility.)

- Exact "+ New Group" entry-point placement in sidebar.
- Concrete themed-name word banks: ~30 entries per bucket (3 / 4–10 / 11+) — seed during build, owner curates before merge.
- Mosaic layout for 100-person group: show first 4 avatars (joined-earliest? alphabetical?). Pick during build.

## 14. Spec change log

- 2026-04-24 — initial draft, design approved by user across sections 1–8.
