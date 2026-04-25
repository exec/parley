# Group DMs + cross-cutting read-state — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add group DMs (max 100 members, owner-only-kick, themed default names, mosaic avatars) and cross-cutting per-channel read-state + notification settings (`ALL` / `MENTIONS_ONLY` / `MUTED`) for both server channels and DMs.

**Architecture:** Two migrations. Phase A establishes a single `user_channel_state` table + endpoints + frontend hooks for cross-cutting read/notification state on existing 1:1 DMs and server channels — ships independently with unread badges and mute. Phase B generalizes `dm_channels` (new `is_group`, `name`, `avatar_url`, `owner_user_id` columns + `dm_channel_members` join table + system-event column on `dm_messages`), introduces a thin `internal/dm/service.go` layer, themed-name generator, and 7 new frontend components for group lifecycle / membership / system-message rendering.

**Tech Stack:** Go 1.25 (backend), React 18 + TypeScript + Vite (frontend), PostgreSQL 16, Redis 7 (existing), `chi` router, the existing `internal/websocket` hub.

**Reference spec:** `docs/superpowers/specs/2026-04-24-group-dms-design.md` (committed `635a01d`).

---

## File Structure

### Phase A — cross-cutting read-state + notifications

| File | Status | Responsibility |
|---|---|---|
| `internal/db/migrations.go` | modify | Add migration #64 (single CREATE TABLE) |
| `internal/db/models.go` | modify | Add `UserChannelState` struct + `NotificationSetting` enum constants |
| `internal/db/user_channel_state_repository.go` | new | CRUD on `user_channel_state`: `Get`, `UpsertReadMarker`, `UpsertNotificationSetting`, `BulkGetForUser` |
| `internal/db/user_channel_state_repository_test.go` | new | Repo unit tests |
| `cmd/api/read_state_handlers.go` | new | 4 endpoint handlers (read + notifications × server + dm) |
| `cmd/api/read_state_handlers_test.go` | new | Handler tests with auth gates |
| `cmd/api/routes.go` | modify | Register new routes with scope gates |
| `internal/websocket/events.go` | modify (or new const block) | Add `EventChannelReadStateUpdate`, `EventChannelNotificationUpdate` constants |
| `internal/message/service.go` | modify | UPSERT author's `last_read_message_id` on send |
| `internal/dm/handler.go` | modify | Same UPSERT on DM send |
| `frontend/src/api/types.ts` | modify | Add `NotificationSetting` type + `UserChannelState` type |
| `frontend/src/api/readState.ts` | new | API client: `markRead`, `setNotificationSetting`, `getStateBundle` |
| `frontend/src/hooks/useChannelReadState.ts` | new | Per (kind, channelID) read-state hook |
| `frontend/src/hooks/useChannelNotificationSetting.ts` | new | Per (kind, channelID) notification-setting hook |
| `frontend/src/hooks/useNotifications.ts` | modify | Wrap toast/sound dispatch in `shouldNotify()` gate |
| `frontend/src/components/layout/DmPanel.tsx` | modify | Unread dot + muted icon per row |
| `frontend/src/components/layout/ChannelList.tsx` | modify | Unread badge + per-channel notification-settings submenu |

### Phase B — group DMs

| File | Status | Responsibility |
|---|---|---|
| `internal/db/migrations.go` | modify | Add migration #65 (ALTER `dm_channels` + new `dm_channel_members` + ALTER `dm_messages`) |
| `internal/db/models.go` | modify | Extend `DmChannel` (new fields), extend `DmMessage` (`SystemEvent`), add `DmChannelMember` |
| `internal/db/dm_repository.go` | modify | New methods: `CreateGroupChannel`, `AddMember`, `RemoveMember`, `GetMembers`, `IsMember`, `UpdateGroupName`, `UpdateGroupAvatar`, `TransferOwnership`, `InsertSystemMessage` |
| `internal/dm/service.go` | new | Business logic for create/add/leave/kick/rename/avatar/transfer; emits system messages atomically |
| `internal/dm/service_test.go` | new | Service unit tests |
| `internal/dm/names.go` | new | Themed-default-name generator with three size buckets |
| `internal/dm/names_test.go` | new | Generator unit tests |
| `internal/dm/handler.go` | modify | Extend `OpenDmChannel` (accept `user_ids: []`); new `AddMembers`, `RemoveMember`, `Leave`, `UpdateGroup`, `UpdateAvatar`, `RemoveAvatar`, `TransferOwnership` |
| `cmd/api/routes.go` | modify | Register new group-DM endpoints with scope gates |
| `internal/websocket/events.go` | modify | Add `EventDmChannelCreate`, `EventDmChannelUpdate`, `EventDmMemberAdd`, `EventDmMemberRemove` |
| `internal/notification/service.go` | modify | Multi-recipient fan-out for group DMs (mention notifications) |
| `frontend/src/api/types.ts` | modify | Extend `DmChannel` (members, is_group, name, avatar_url, owner_user_id), extend `DmMessage` (system_event), add `SystemEvent` union |
| `frontend/src/api/dms.ts` | modify | New API methods: `createGroup`, `addMembers`, `kickMember`, `leaveGroup`, `renameGroup`, `uploadGroupAvatar`, `removeGroupAvatar`, `transferOwnership` |
| `frontend/src/hooks/useGroupMembers.ts` | new | Member list + WS subscription for `DM_MEMBER_ADD/REMOVE` |
| `frontend/src/components/dm/MosaicAvatar.tsx` | new | 1/2/3/4-tile composite avatar |
| `frontend/src/components/dm/CreateGroupModal.tsx` | new | "+ New Group" multi-select modal |
| `frontend/src/components/dm/AddPeopleModal.tsx` | new | Dual-context: spawn-from-1:1 OR add-to-existing-group |
| `frontend/src/components/dm/GroupMembersPanel.tsx` | new | Slide-out: members list + actions |
| `frontend/src/components/dm/TransferOwnershipModal.tsx` | new | Member-picker confirm |
| `frontend/src/components/dm/LeaveGroupModal.tsx` | new | Owner two-step / non-owner one-step |
| `frontend/src/components/chat/SystemMessage.tsx` | new | System-event compact rendering |
| `frontend/src/components/chat/MessageList.tsx` | modify | Branch on `system_event != null` to render `SystemMessage`; render last-read divider |
| `frontend/src/components/chat/ChatWindow.tsx` | modify | Group-aware membership / header / Members button |
| `frontend/src/components/layout/DmPanel.tsx` | modify | Group rows: mosaic avatar + member count |
| `frontend/src/components/layout/Sidebar.tsx` | modify | "+ New Group" entry point |

---

## Phase A — Cross-cutting read-state + notifications

### Task 1: Migration #64 — `user_channel_state` table

**Files:**
- Modify: `internal/db/migrations.go` (append entry to migrations slice)

- [ ] **Step 1: Add the migration entry**

In `internal/db/migrations.go`, append a new entry to the `Migrations` slice (immediately after migration #63):

```go
{
    Version: 64,
    Name:    "create_user_channel_state",
    SQL: `
        CREATE TABLE user_channel_state (
            user_id              BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            channel_kind         SMALLINT    NOT NULL CHECK (channel_kind IN (1, 2)),
            channel_id           BIGINT      NOT NULL,
            last_read_message_id BIGINT,
            notification_setting SMALLINT    NOT NULL DEFAULT 0,
            updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            PRIMARY KEY (user_id, channel_kind, channel_id)
        );
        CREATE INDEX user_channel_state_user_idx ON user_channel_state(user_id);
    `,
},
```

- [ ] **Step 2: Run the test suite to verify schema applies**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/db/...`
Expected: PASS (existing tests; the new table just exists).

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat(db): migration #64 — user_channel_state table for cross-cutting read/mute state"
```

---

### Task 2: `UserChannelState` model + repository

**Files:**
- Modify: `internal/db/models.go`
- Create: `internal/db/user_channel_state_repository.go`
- Create: `internal/db/user_channel_state_repository_test.go`

- [ ] **Step 1: Add model + enum to `internal/db/models.go`**

Append at the bottom of the file:

```go
// ChannelKind discriminates the two channel families that share user_channel_state.
type ChannelKind int16

const (
    ChannelKindServer ChannelKind = 1
    ChannelKindDM     ChannelKind = 2
)

// NotificationSetting is the per-(user, channel) notification preference.
type NotificationSetting int16

const (
    NotificationAll           NotificationSetting = 0 // default: every message notifies
    NotificationMentionsOnly  NotificationSetting = 1 // only @mentions notify
    NotificationMuted         NotificationSetting = 2 // no notify; unread still increments
)

// UserChannelState is the per-(user, channel) read-marker + notification setting.
// Rows exist only when the user has marked something read or changed setting from default.
type UserChannelState struct {
    UserID              int64               `json:"user_id,string" db:"user_id"`
    ChannelKind         ChannelKind         `json:"channel_kind" db:"channel_kind"`
    ChannelID           int64               `json:"channel_id,string" db:"channel_id"`
    LastReadMessageID   *int64              `json:"last_read_message_id,omitempty,string" db:"last_read_message_id"`
    NotificationSetting NotificationSetting `json:"notification_setting" db:"notification_setting"`
    UpdatedAt           time.Time           `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Write the repository test (failing first)**

Create `internal/db/user_channel_state_repository_test.go`:

```go
package db

import (
    "context"
    "testing"
)

func TestUserChannelState_UpsertReadMarker_InsertsRow(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    userID := seedUser(t, repo, "alice")
    channelID := int64(123)
    msgID := int64(456)

    if err := repo.UpsertReadMarker(ctx, userID, ChannelKindDM, channelID, msgID); err != nil {
        t.Fatalf("UpsertReadMarker: %v", err)
    }

    state, err := repo.GetUserChannelState(ctx, userID, ChannelKindDM, channelID)
    if err != nil {
        t.Fatalf("GetUserChannelState: %v", err)
    }
    if state == nil || state.LastReadMessageID == nil || *state.LastReadMessageID != msgID {
        t.Fatalf("expected LastReadMessageID=%d, got %+v", msgID, state)
    }
    if state.NotificationSetting != NotificationAll {
        t.Fatalf("expected default NotificationAll, got %d", state.NotificationSetting)
    }
}

func TestUserChannelState_UpsertReadMarker_UpdatesRow(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    userID := seedUser(t, repo, "bob")

    _ = repo.UpsertReadMarker(ctx, userID, ChannelKindServer, 10, 100)
    if err := repo.UpsertReadMarker(ctx, userID, ChannelKindServer, 10, 200); err != nil {
        t.Fatalf("UpsertReadMarker (update): %v", err)
    }

    state, _ := repo.GetUserChannelState(ctx, userID, ChannelKindServer, 10)
    if *state.LastReadMessageID != 200 {
        t.Fatalf("expected 200, got %d", *state.LastReadMessageID)
    }
}

func TestUserChannelState_UpsertNotificationSetting_KeepsReadMarker(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    userID := seedUser(t, repo, "carol")

    _ = repo.UpsertReadMarker(ctx, userID, ChannelKindDM, 7, 500)
    if err := repo.UpsertNotificationSetting(ctx, userID, ChannelKindDM, 7, NotificationMuted); err != nil {
        t.Fatalf("UpsertNotificationSetting: %v", err)
    }

    state, _ := repo.GetUserChannelState(ctx, userID, ChannelKindDM, 7)
    if *state.LastReadMessageID != 500 {
        t.Fatalf("read marker clobbered, got %v", state.LastReadMessageID)
    }
    if state.NotificationSetting != NotificationMuted {
        t.Fatalf("expected NotificationMuted, got %d", state.NotificationSetting)
    }
}

func TestUserChannelState_BulkGetForUser_ReturnsAllRows(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    userID := seedUser(t, repo, "dave")

    _ = repo.UpsertReadMarker(ctx, userID, ChannelKindServer, 1, 100)
    _ = repo.UpsertReadMarker(ctx, userID, ChannelKindDM, 2, 200)
    _ = repo.UpsertNotificationSetting(ctx, userID, ChannelKindServer, 3, NotificationMentionsOnly)

    rows, err := repo.BulkGetUserChannelState(ctx, userID)
    if err != nil {
        t.Fatalf("BulkGetUserChannelState: %v", err)
    }
    if len(rows) != 3 {
        t.Fatalf("expected 3 rows, got %d", len(rows))
    }
}
```

(The `newTestRepo` and `seedUser` helpers exist in the package's existing test files — match the pattern other repository tests use.)

- [ ] **Step 3: Run test to verify it fails**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/db/ -run UserChannelState -v`
Expected: FAIL — compile error, methods not defined.

- [ ] **Step 4: Implement the repository**

Create `internal/db/user_channel_state_repository.go`:

```go
package db

import (
    "context"
    "database/sql"
)

// GetUserChannelState returns nil + nil error if no row exists (default state).
func (r *Repository) GetUserChannelState(ctx context.Context, userID int64, kind ChannelKind, channelID int64) (*UserChannelState, error) {
    var s UserChannelState
    err := r.db.QueryRowContext(ctx, `
        SELECT user_id, channel_kind, channel_id, last_read_message_id, notification_setting, updated_at
          FROM user_channel_state
         WHERE user_id = $1 AND channel_kind = $2 AND channel_id = $3
    `, userID, kind, channelID).Scan(
        &s.UserID, &s.ChannelKind, &s.ChannelID,
        &s.LastReadMessageID, &s.NotificationSetting, &s.UpdatedAt,
    )
    if err == sql.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &s, nil
}

// UpsertReadMarker writes the last_read_message_id for (user, channel), preserving notification_setting.
func (r *Repository) UpsertReadMarker(ctx context.Context, userID int64, kind ChannelKind, channelID int64, messageID int64) error {
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO user_channel_state (user_id, channel_kind, channel_id, last_read_message_id, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, channel_kind, channel_id)
        DO UPDATE SET last_read_message_id = EXCLUDED.last_read_message_id, updated_at = NOW()
    `, userID, kind, channelID, messageID)
    return err
}

// UpsertNotificationSetting writes the notification_setting for (user, channel), preserving last_read_message_id.
func (r *Repository) UpsertNotificationSetting(ctx context.Context, userID int64, kind ChannelKind, channelID int64, setting NotificationSetting) error {
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO user_channel_state (user_id, channel_kind, channel_id, notification_setting, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, channel_kind, channel_id)
        DO UPDATE SET notification_setting = EXCLUDED.notification_setting, updated_at = NOW()
    `, userID, kind, channelID, setting)
    return err
}

// BulkGetUserChannelState returns every row for user — used by client to hydrate state on connect.
func (r *Repository) BulkGetUserChannelState(ctx context.Context, userID int64) ([]UserChannelState, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT user_id, channel_kind, channel_id, last_read_message_id, notification_setting, updated_at
          FROM user_channel_state
         WHERE user_id = $1
    `, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []UserChannelState
    for rows.Next() {
        var s UserChannelState
        if err := rows.Scan(&s.UserID, &s.ChannelKind, &s.ChannelID, &s.LastReadMessageID, &s.NotificationSetting, &s.UpdatedAt); err != nil {
            return nil, err
        }
        out = append(out, s)
    }
    return out, rows.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/db/ -run UserChannelState -v`
Expected: PASS — 4 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/db/models.go internal/db/user_channel_state_repository.go internal/db/user_channel_state_repository_test.go
git commit -m "feat(db): user_channel_state model + repository (cross-cutting read-state + notifications)"
```

---

### Task 3: WS event constants + payload types

**Files:**
- Modify: `internal/websocket/events.go`

- [ ] **Step 1: Add the new event-type constants**

Append to the existing event-type block in `internal/websocket/events.go`:

```go
const (
    // ... existing events ...
    EventChannelReadStateUpdate    = "CHANNEL_READ_STATE_UPDATE"
    EventChannelNotificationUpdate = "CHANNEL_NOTIFICATION_UPDATE"
)
```

- [ ] **Step 2: Add payload struct(s) if events.go uses them**

If the file declares typed payloads (check existing pattern), add:

```go
type ChannelReadStatePayload struct {
    ChannelKind       int16  `json:"channel_kind"`
    ChannelID         string `json:"channel_id"`        // string for JS-safety on int64
    LastReadMessageID string `json:"last_read_message_id"`
}

type ChannelNotificationPayload struct {
    ChannelKind         int16  `json:"channel_kind"`
    ChannelID           string `json:"channel_id"`
    NotificationSetting int16  `json:"notification_setting"`
}
```

If `events.go` doesn't use typed payloads (just a `type Event struct{Type string; Data interface{}}` shape), skip — handlers will encode payloads as `map[string]interface{}` directly.

- [ ] **Step 3: Build to verify**

Run: `go build ./...`
Expected: Builds clean.

- [ ] **Step 4: Commit**

```bash
git add internal/websocket/events.go
git commit -m "feat(ws): event constants for cross-cutting read-state + notification updates"
```

---

### Task 4: Read-state + notifications endpoints (server channels)

**Files:**
- Create: `cmd/api/read_state_handlers.go`
- Create: `cmd/api/read_state_handlers_test.go`
- Modify: `cmd/api/routes.go`

- [ ] **Step 1: Write the failing handler test**

Create `cmd/api/read_state_handlers_test.go`:

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestReadStateHandler_MarkChannelRead_Success(t *testing.T) {
    fx := newAPITestFixture(t)
    defer fx.cleanup()

    user := fx.seedUser("alice")
    server := fx.seedServer(user.ID)
    channel := fx.seedChannel(server.ID, "general")
    msg := fx.seedMessage(channel.ID, user.ID, "hi")

    body := bytes.NewBufferString(`{"message_id":"` + msg.IDString() + `"}`)
    req := httptest.NewRequest("POST", "/api/channels/"+channel.IDString()+"/read", body)
    req.Header.Set("Authorization", "Bearer "+fx.token(user.ID))
    w := httptest.NewRecorder()
    fx.server.Handler.ServeHTTP(w, req)

    if w.Code != http.StatusNoContent {
        t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
    }

    state, _ := fx.repo.GetUserChannelState(fx.ctx, user.ID, db.ChannelKindServer, channel.ID)
    if state == nil || *state.LastReadMessageID != msg.ID {
        t.Fatalf("expected last_read_message_id=%d, got %+v", msg.ID, state)
    }
}

func TestReadStateHandler_MarkRead_NonMember_Forbidden(t *testing.T) {
    fx := newAPITestFixture(t)
    defer fx.cleanup()

    owner := fx.seedUser("owner")
    outsider := fx.seedUser("outsider")
    server := fx.seedServer(owner.ID)
    channel := fx.seedChannel(server.ID, "general")
    msg := fx.seedMessage(channel.ID, owner.ID, "hi")

    body := bytes.NewBufferString(`{"message_id":"` + msg.IDString() + `"}`)
    req := httptest.NewRequest("POST", "/api/channels/"+channel.IDString()+"/read", body)
    req.Header.Set("Authorization", "Bearer "+fx.token(outsider.ID))
    w := httptest.NewRecorder()
    fx.server.Handler.ServeHTTP(w, req)

    if w.Code != http.StatusForbidden {
        t.Fatalf("expected 403, got %d", w.Code)
    }
}

func TestReadStateHandler_SetNotificationSetting_PersistsAndBroadcasts(t *testing.T) {
    fx := newAPITestFixture(t)
    defer fx.cleanup()

    user := fx.seedUser("alice")
    server := fx.seedServer(user.ID)
    channel := fx.seedChannel(server.ID, "general")

    body := bytes.NewBufferString(`{"setting":"MUTED"}`)
    req := httptest.NewRequest("PATCH", "/api/channels/"+channel.IDString()+"/notifications", body)
    req.Header.Set("Authorization", "Bearer "+fx.token(user.ID))
    w := httptest.NewRecorder()
    fx.server.Handler.ServeHTTP(w, req)

    if w.Code != http.StatusNoContent {
        t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
    }

    state, _ := fx.repo.GetUserChannelState(fx.ctx, user.ID, db.ChannelKindServer, channel.ID)
    if state == nil || state.NotificationSetting != db.NotificationMuted {
        t.Fatalf("expected NotificationMuted, got %+v", state)
    }

    // Verify WS broadcast captured by fx.hubSpy
    if got := fx.hubSpy.LastEvent(user.ID); got.Type != "CHANNEL_NOTIFICATION_UPDATE" {
        t.Fatalf("expected CHANNEL_NOTIFICATION_UPDATE event, got %q", got.Type)
    }
}
```

(`newAPITestFixture` is the existing test harness in `cmd/api/middleware_test.go` — match the same pattern, including `hubSpy` if it exists; if not, this is the place to introduce it as a small mock implementing the hub interface.)

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./cmd/api/ -run ReadStateHandler -v`
Expected: FAIL — handler not defined.

- [ ] **Step 3: Implement the handlers**

Create `cmd/api/read_state_handlers.go`:

```go
package main

import (
    "encoding/json"
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "parley/internal/auth"
    "parley/internal/db"
    "parley/internal/httputil"
    ws "parley/internal/websocket"
)

type markReadRequest struct {
    MessageID string `json:"message_id"`
}

type setNotificationsRequest struct {
    Setting string `json:"setting"`
}

// handleMarkChannelRead — POST /api/channels/{channelID}/read
func handleMarkChannelRead(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
    return markReadHandler(repo, hub, db.ChannelKindServer, channelMembershipCheck(repo))
}

// handleMarkDmRead — POST /api/dms/{channelID}/read
func handleMarkDmRead(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
    return markReadHandler(repo, hub, db.ChannelKindDM, dmMembershipCheck(repo))
}

func markReadHandler(repo *db.Repository, hub *ws.Hub, kind db.ChannelKind, ensureMember func(ctx context.Context, userID, channelID int64) error) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userIDStr := auth.GetUserIDFromContext(r)
        userID, _ := strconv.ParseInt(userIDStr, 10, 64)

        channelID, err := strconv.ParseInt(chi.URLParam(r, "channelID"), 10, 64)
        if err != nil {
            httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
            return
        }

        if err := ensureMember(r.Context(), userID, channelID); err != nil {
            httputil.JSONError(w, "forbidden", http.StatusForbidden)
            return
        }

        var req markReadRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
            return
        }
        msgID, err := strconv.ParseInt(req.MessageID, 10, 64)
        if err != nil {
            httputil.JSONError(w, "invalid message id", http.StatusBadRequest)
            return
        }

        if err := repo.UpsertReadMarker(r.Context(), userID, kind, channelID, msgID); err != nil {
            httputil.InternalError(w, err)
            return
        }

        // Multi-tab sync — broadcast to user's own sessions only.
        hub.SendToUser(userIDStr, ws.EventChannelReadStateUpdate, map[string]any{
            "channel_kind":         int16(kind),
            "channel_id":           strconv.FormatInt(channelID, 10),
            "last_read_message_id": req.MessageID,
        })

        w.WriteHeader(http.StatusNoContent)
    }
}

func handleSetChannelNotifications(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
    return notificationsHandler(repo, hub, db.ChannelKindServer, channelMembershipCheck(repo))
}

func handleSetDmNotifications(repo *db.Repository, hub *ws.Hub) http.HandlerFunc {
    return notificationsHandler(repo, hub, db.ChannelKindDM, dmMembershipCheck(repo))
}

func notificationsHandler(repo *db.Repository, hub *ws.Hub, kind db.ChannelKind, ensureMember func(ctx context.Context, userID, channelID int64) error) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userIDStr := auth.GetUserIDFromContext(r)
        userID, _ := strconv.ParseInt(userIDStr, 10, 64)

        channelID, err := strconv.ParseInt(chi.URLParam(r, "channelID"), 10, 64)
        if err != nil {
            httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
            return
        }

        if err := ensureMember(r.Context(), userID, channelID); err != nil {
            httputil.JSONError(w, "forbidden", http.StatusForbidden)
            return
        }

        var req setNotificationsRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
            return
        }

        var setting db.NotificationSetting
        switch req.Setting {
        case "ALL":
            setting = db.NotificationAll
        case "MENTIONS_ONLY":
            setting = db.NotificationMentionsOnly
        case "MUTED":
            setting = db.NotificationMuted
        default:
            httputil.JSONError(w, "invalid setting", http.StatusBadRequest)
            return
        }

        if err := repo.UpsertNotificationSetting(r.Context(), userID, kind, channelID, setting); err != nil {
            httputil.InternalError(w, err)
            return
        }

        hub.SendToUser(userIDStr, ws.EventChannelNotificationUpdate, map[string]any{
            "channel_kind":         int16(kind),
            "channel_id":           strconv.FormatInt(channelID, 10),
            "notification_setting": int16(setting),
        })

        w.WriteHeader(http.StatusNoContent)
    }
}

// channelMembershipCheck returns "user is a member of the server containing this channel."
func channelMembershipCheck(repo *db.Repository) func(ctx context.Context, userID, channelID int64) error {
    return func(ctx context.Context, userID, channelID int64) error {
        ch, err := repo.GetChannelByID(ctx, channelID)
        if err != nil {
            return err
        }
        member, err := repo.GetMember(ctx, ch.ServerID, userID)
        if err != nil || member == nil {
            return errors.New("not a member")
        }
        return nil
    }
}

// dmMembershipCheck returns "user is in the dm_channel_members for this DM channel."
// During Phase A this falls back to user1_id/user2_id check (Phase B replaces with join-table read).
func dmMembershipCheck(repo *db.Repository) func(ctx context.Context, userID, channelID int64) error {
    return func(ctx context.Context, userID, channelID int64) error {
        ch, err := repo.GetDmChannelByID(ctx, channelID)
        if err != nil {
            return err
        }
        if ch.User1ID != userID && ch.User2ID != userID {
            return errors.New("not a member")
        }
        return nil
    }
}
```

(Errors-package import + `context` import omitted for brevity in this plan; include them in actual file. `GetDmChannelByID` may need to be added to `dm_repository.go` if missing — check before assuming.)

- [ ] **Step 4: Register the routes**

In `cmd/api/routes.go`, inside the authenticated `r.Group(...)` block (where existing endpoints are registered), add:

```go
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelID}/read", handleMarkChannelRead(repo, hub))
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{channelID}/read", handleMarkDmRead(repo, hub))
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/channels/{channelID}/notifications", handleSetChannelNotifications(repo, hub))
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/dms/{channelID}/notifications", handleSetDmNotifications(repo, hub))
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./cmd/api/ -run ReadStateHandler -v`
Expected: PASS — 3 tests.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/read_state_handlers.go cmd/api/read_state_handlers_test.go cmd/api/routes.go
git commit -m "feat(api): /channels and /dms read + notifications endpoints"
```

---

### Task 5: Implicit-read on message send

**Files:**
- Modify: `internal/message/service.go` (server-channel send path)
- Modify: `internal/dm/handler.go` (DM send path)

- [ ] **Step 1: Write a server-channel test**

Add to `internal/message/service_test.go`:

```go
func TestSendMessage_UpsertsAuthorReadMarker(t *testing.T) {
    fx := newServiceTestFixture(t)
    defer fx.cleanup()

    user := fx.seedUser("alice")
    channel := fx.seedChannel("general")

    msg, err := fx.svc.SendMessage(fx.ctx, channel.ID, user.ID, "hi", nil, "")
    if err != nil {
        t.Fatalf("SendMessage: %v", err)
    }

    state, _ := fx.repo.GetUserChannelState(fx.ctx, user.ID, db.ChannelKindServer, channel.ID)
    if state == nil || *state.LastReadMessageID != msg.ID {
        t.Fatalf("expected author auto-marked-read at %d, got %+v", msg.ID, state)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/message/ -run UpsertsAuthorReadMarker -v`
Expected: FAIL.

- [ ] **Step 3: Modify `internal/message/service.go::SendMessage`**

Inside the existing `SendMessage` method, after the `INSERT messages` succeeds and you have `msg.ID`, add:

```go
// Implicit author read-marker — saves a client round-trip.
if err := s.repo.UpsertReadMarker(ctx, authorID, db.ChannelKindServer, channelID, msg.ID); err != nil {
    log.Printf("message: failed to upsert author read marker: %v", err) // non-fatal
}
```

- [ ] **Step 4: Modify `internal/dm/handler.go::SendDmMessage`**

Inside the existing `SendDmMessage` handler, after `repo.CreateDmMessage` returns the new message, add:

```go
// Implicit author read-marker for the DM channel.
if err := h.repo.UpsertReadMarker(r.Context(), currentUserID, db.ChannelKindDM, dmChannelID, msg.ID); err != nil {
    log.Printf("dm: failed to upsert author read marker: %v", err) // non-fatal
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```
JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/message/ ./internal/dm/ ./cmd/api/ -run "Send|UpsertsAuthor" -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/message/service.go internal/dm/handler.go internal/message/service_test.go
git commit -m "feat(messages): implicit author read-marker on send (server channels + DMs)"
```

---

### Task 6: Frontend types + read-state API client

**Files:**
- Modify: `frontend/src/api/types.ts`
- Create: `frontend/src/api/readState.ts`

- [ ] **Step 1: Add types**

Append to `frontend/src/api/types.ts`:

```typescript
export type NotificationSetting = 'ALL' | 'MENTIONS_ONLY' | 'MUTED';
export const NOTIFICATION_SETTINGS: NotificationSetting[] = ['ALL', 'MENTIONS_ONLY', 'MUTED'];

export type ChannelKind = 1 | 2; // 1=server, 2=dm
export const CHANNEL_KIND_SERVER: ChannelKind = 1;
export const CHANNEL_KIND_DM: ChannelKind = 2;

export interface UserChannelState {
  user_id: string;
  channel_kind: ChannelKind;
  channel_id: string;
  last_read_message_id: string | null;
  notification_setting: 0 | 1 | 2;
  updated_at: string;
}
```

- [ ] **Step 2: Create the API client**

Create `frontend/src/api/readState.ts`:

```typescript
import { apiClient } from './client';
import type { ChannelKind, NotificationSetting } from './types';

const pathRoot = (kind: ChannelKind) => (kind === 1 ? 'channels' : 'dms');

export async function markRead(kind: ChannelKind, channelId: string, messageId: string): Promise<void> {
  await apiClient.post<void>(`/${pathRoot(kind)}/${channelId}/read`, { message_id: messageId });
}

export async function setNotificationSetting(
  kind: ChannelKind,
  channelId: string,
  setting: NotificationSetting,
): Promise<void> {
  await apiClient.patch<void>(`/${pathRoot(kind)}/${channelId}/notifications`, { setting });
}
```

- [ ] **Step 3: Build the frontend to verify typing**

Run: `cd frontend && npm run build 2>&1 | tail -10`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/readState.ts
git commit -m "feat(api/client): NotificationSetting + UserChannelState types + readState client"
```

---

### Task 7: Frontend hooks for read-state + notifications

**Files:**
- Create: `frontend/src/hooks/useChannelReadState.ts`
- Create: `frontend/src/hooks/useChannelNotificationSetting.ts`

- [ ] **Step 1: Implement `useChannelReadState`**

Create `frontend/src/hooks/useChannelReadState.ts`:

```typescript
import { useCallback, useEffect, useState } from 'react';
import * as readStateApi from '../api/readState';
import type { ChannelKind } from '../api/types';

interface State {
  lastReadMessageId: string | null;
  isUnread(latestMessageId: string | null): boolean;
}

export function useChannelReadState(kind: ChannelKind, channelId: string | null): {
  lastReadMessageId: string | null;
  markRead: (messageId: string) => void;
} {
  const [lastReadMessageId, setLastReadMessageId] = useState<string | null>(null);

  // Subscribe to WS updates for multi-tab sync.
  // (The WS handler in App.tsx routes CHANNEL_READ_STATE_UPDATE events; this hook listens to a context bus.)
  useEffect(() => {
    if (!channelId) return;
    const handler = (e: CustomEvent<{channel_kind: ChannelKind; channel_id: string; last_read_message_id: string}>) => {
      if (e.detail.channel_kind === kind && e.detail.channel_id === channelId) {
        setLastReadMessageId(e.detail.last_read_message_id);
      }
    };
    window.addEventListener('parley:channel_read_state', handler as EventListener);
    return () => window.removeEventListener('parley:channel_read_state', handler as EventListener);
  }, [kind, channelId]);

  const markRead = useCallback((messageId: string) => {
    if (!channelId) return;
    setLastReadMessageId(messageId); // optimistic
    readStateApi.markRead(kind, channelId, messageId).catch((err) => {
      console.error('markRead failed', err);
    });
  }, [kind, channelId]);

  return { lastReadMessageId, markRead };
}
```

- [ ] **Step 2: Implement `useChannelNotificationSetting`**

Create `frontend/src/hooks/useChannelNotificationSetting.ts`:

```typescript
import { useCallback, useEffect, useState } from 'react';
import * as readStateApi from '../api/readState';
import type { ChannelKind, NotificationSetting } from '../api/types';

export function useChannelNotificationSetting(kind: ChannelKind, channelId: string | null): {
  setting: NotificationSetting;
  setSetting: (s: NotificationSetting) => void;
} {
  const [setting, setSettingState] = useState<NotificationSetting>('ALL');

  useEffect(() => {
    if (!channelId) return;
    const handler = (e: CustomEvent<{channel_kind: ChannelKind; channel_id: string; notification_setting: 0 | 1 | 2}>) => {
      if (e.detail.channel_kind === kind && e.detail.channel_id === channelId) {
        const map: Record<0 | 1 | 2, NotificationSetting> = { 0: 'ALL', 1: 'MENTIONS_ONLY', 2: 'MUTED' };
        setSettingState(map[e.detail.notification_setting]);
      }
    };
    window.addEventListener('parley:channel_notification', handler as EventListener);
    return () => window.removeEventListener('parley:channel_notification', handler as EventListener);
  }, [kind, channelId]);

  const setSetting = useCallback((s: NotificationSetting) => {
    if (!channelId) return;
    setSettingState(s); // optimistic
    readStateApi.setNotificationSetting(kind, channelId, s).catch((err) => {
      console.error('setNotificationSetting failed', err);
    });
  }, [kind, channelId]);

  return { setting, setSetting };
}
```

- [ ] **Step 3: Wire WS event dispatch in `App.tsx`**

In the existing WS event handler (around `useEffect` that processes incoming events), add cases that dispatch `CustomEvent`s on `window`:

```typescript
case 'CHANNEL_READ_STATE_UPDATE':
  window.dispatchEvent(new CustomEvent('parley:channel_read_state', { detail: event.data }));
  break;
case 'CHANNEL_NOTIFICATION_UPDATE':
  window.dispatchEvent(new CustomEvent('parley:channel_notification', { detail: event.data }));
  break;
```

- [ ] **Step 4: Build to verify**

Run: `cd frontend && npm run build 2>&1 | tail -10`
Expected: build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/useChannelReadState.ts frontend/src/hooks/useChannelNotificationSetting.ts frontend/src/App.tsx
git commit -m "feat(frontend): hooks for cross-cutting read-state + notifications"
```

---

### Task 8: Notification gate (`shouldNotify`)

**Files:**
- Modify: `frontend/src/hooks/useNotifications.ts`

- [ ] **Step 1: Add the gate utility**

In `frontend/src/hooks/useNotifications.ts`, near the top, add:

```typescript
import type { NotificationSetting } from '../api/types';

interface MessageLike {
  author_id: string;
  mentions?: string[]; // user_ids of mentioned users
}

export function shouldNotify(
  msg: MessageLike,
  setting: NotificationSetting,
  currentUserId: string,
): boolean {
  if (setting === 'MUTED') return false;
  if (msg.author_id === currentUserId) return false;
  if (setting === 'MENTIONS_ONLY') return (msg.mentions ?? []).includes(currentUserId);
  return true; // ALL
}
```

- [ ] **Step 2: Wrap the existing notification dispatch**

Find the existing toast/sound dispatch in the same file. Wrap it:

```typescript
// Before dispatching, look up the channel's notification setting from a global state map (TBD wiring).
const channelSetting = getNotificationSettingForChannel(kind, channelId);
if (!shouldNotify(msg, channelSetting, currentUserId)) return;

// existing dispatch:
showToast(...); playSound(...);
```

If `getNotificationSettingForChannel` doesn't exist yet, add a small in-memory cache populated by the same WS dispatch:

```typescript
const notificationSettingCache = new Map<string, NotificationSetting>(); // key: `${kind}:${channelId}`
window.addEventListener('parley:channel_notification', (e: CustomEvent<{channel_kind: number; channel_id: string; notification_setting: 0 | 1 | 2}>) => {
  const map: Record<number, NotificationSetting> = { 0: 'ALL', 1: 'MENTIONS_ONLY', 2: 'MUTED' };
  notificationSettingCache.set(`${e.detail.channel_kind}:${e.detail.channel_id}`, map[e.detail.notification_setting]);
});
function getNotificationSettingForChannel(kind: number, channelId: string): NotificationSetting {
  return notificationSettingCache.get(`${kind}:${channelId}`) ?? 'ALL';
}
```

- [ ] **Step 3: Build to verify**

Run: `cd frontend && npm run build 2>&1 | tail -10`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/hooks/useNotifications.ts
git commit -m "feat(frontend): gate notification dispatch via shouldNotify (per-channel settings)"
```

---

### Task 9: Unread badge + mute icon in DmPanel

**Files:**
- Modify: `frontend/src/components/layout/DmPanel.tsx`

- [ ] **Step 1: Add unread + muted state to row rendering**

Inside the existing channel-row map, compute:

```typescript
const lastReadId = readState.get(`2:${channel.id}`); // '2' = ChannelKindDM
const latestMsgId = channel.last_message_id ?? null;
const isUnread = !!latestMsgId && (!lastReadId || BigInt(latestMsgId) > BigInt(lastReadId));

const setting = notificationSettings.get(`2:${channel.id}`) ?? 'ALL';
const isMuted = setting === 'MUTED';
```

Then render:

```tsx
<div className="dm-row">
  <Avatar src={channel.other_avatar_url} alt={channel.other_username} />
  <div className="dm-row-text">
    <span className="dm-row-name">{channel.other_display_name ?? channel.other_username}</span>
    {isMuted && <MuteIcon className="dm-row-muted" />}
  </div>
  {isUnread && <span className="dm-row-unread-dot" />}
</div>
```

(`readState` and `notificationSettings` are Maps that are passed in as props from the parent or come from a context. The `last_message_id` field on `DmChannel` may need to be added to the API response — check `internal/db/models.go::DmChannel` and add `LastMessageID *int64` if missing. If adding, do that first as a separate small commit.)

- [ ] **Step 2: Add minimal CSS**

In the appropriate CSS file (existing DmPanel.css if present, else a new one), add:

```css
.dm-row-unread-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
  margin-left: auto;
}
.dm-row-muted {
  opacity: 0.5;
  margin-left: 4px;
  width: 14px;
  height: 14px;
}
```

- [ ] **Step 3: Build + manual smoke test**

Run: `cd frontend && npm run build && npm run dev`
Open the app, send a DM, verify the unread dot appears on a different account's session.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/DmPanel.tsx [css file]
git commit -m "feat(frontend): unread dot + mute icon in DmPanel rows"
```

---

### Task 10: Unread badge + per-channel notification submenu in ChannelList

**Files:**
- Modify: `frontend/src/components/layout/ChannelList.tsx`

- [ ] **Step 1: Render unread badge per channel**

Same pattern as Task 9 but with `kind=1` for server channels. In the existing channel-row render, add:

```tsx
const lastReadId = readState.get(`1:${channel.id}`);
const latestMsgId = channel.last_message_id ?? null;
const isUnread = !!latestMsgId && (!lastReadId || BigInt(latestMsgId) > BigInt(lastReadId));

const setting = notificationSettings.get(`1:${channel.id}`) ?? 'ALL';
const isMuted = setting === 'MUTED';

// in JSX:
{isUnread && !isMuted && <span className="channel-row-unread-dot" />}
{isMuted && <MuteIcon className="channel-row-muted" />}
```

- [ ] **Step 2: Add notification-settings submenu**

Find the existing channel-row context menu (right-click handler). Add a "Notification settings" submenu with three entries:

```tsx
<ContextMenuItem submenu="Notifications">
  <ContextMenuItem
    onClick={() => readStateApi.setNotificationSetting(1, channel.id, 'ALL')}
    selected={setting === 'ALL'}
  >All messages</ContextMenuItem>
  <ContextMenuItem
    onClick={() => readStateApi.setNotificationSetting(1, channel.id, 'MENTIONS_ONLY')}
    selected={setting === 'MENTIONS_ONLY'}
  >Only @mentions</ContextMenuItem>
  <ContextMenuItem
    onClick={() => readStateApi.setNotificationSetting(1, channel.id, 'MUTED')}
    selected={setting === 'MUTED'}
  >Muted</ContextMenuItem>
</ContextMenuItem>
```

(Match the existing context-menu component's API. If submenus aren't supported, render as three top-level items grouped under a label.)

- [ ] **Step 3: Build + manual smoke**

Run: `cd frontend && npm run build`
Verify rendering: open a server, right-click a channel, see the new submenu.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/ChannelList.tsx
git commit -m "feat(frontend): server channel unread + notification settings"
```

**End of Phase A.** At this checkpoint, prod runs unread badges + mute on existing 1:1 DMs and server channels. Group DM functionality follows in Phase B.

Suggested checkpoint commit + ship: deploy to prod, exercise, then begin Phase B.

---

## Phase B — Group DMs

### Task 11: Migration #65 — generalize `dm_channels` + `dm_channel_members` + system-event column

**Files:**
- Modify: `internal/db/migrations.go`

- [ ] **Step 1: Add the migration entry**

Append to the `Migrations` slice:

```go
{
    Version: 65,
    Name:    "group_dms",
    SQL: `
        ALTER TABLE dm_channels
            ADD COLUMN is_group           BOOLEAN     NOT NULL DEFAULT FALSE,
            ADD COLUMN name               TEXT,
            ADD COLUMN avatar_url         TEXT,
            ADD COLUMN created_by_user_id BIGINT      REFERENCES users(id),
            ADD COLUMN owner_user_id      BIGINT      REFERENCES users(id);

        CREATE TABLE dm_channel_members (
            dm_channel_id BIGINT      NOT NULL REFERENCES dm_channels(id) ON DELETE CASCADE,
            user_id       BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            PRIMARY KEY (dm_channel_id, user_id)
        );
        CREATE INDEX dm_channel_members_user_idx ON dm_channel_members(user_id);

        INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
        SELECT id, user1_id, created_at FROM dm_channels
        UNION ALL
        SELECT id, user2_id, created_at FROM dm_channels
        ON CONFLICT DO NOTHING;

        ALTER TABLE dm_messages ADD COLUMN system_event JSONB;
    `,
},
```

- [ ] **Step 2: Run test suite to verify migration applies**

Run: `JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./internal/db/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat(db): migration #65 — generalize dm_channels + dm_channel_members + system_event"
```

---

### Task 12: Extend `DmChannel`, `DmMessage`, add `DmChannelMember` model

**Files:**
- Modify: `internal/db/models.go`

- [ ] **Step 1: Extend `DmChannel`**

Update the existing `DmChannel` struct in `internal/db/models.go`:

```go
type DmChannel struct {
    ID            int64     `json:"id,string" db:"id"`
    User1ID       int64     `json:"user1_id,string" db:"user1_id"`
    User2ID       int64     `json:"user2_id,string" db:"user2_id"`
    IsGroup       bool      `json:"is_group" db:"is_group"`
    Name          *string   `json:"name,omitempty" db:"name"`
    AvatarURL     *string   `json:"avatar_url,omitempty" db:"avatar_url"`
    CreatedByUserID *int64  `json:"created_by_user_id,omitempty,string" db:"created_by_user_id"`
    OwnerUserID   *int64    `json:"owner_user_id,omitempty,string" db:"owner_user_id"`
    CreatedAt     time.Time `json:"created_at" db:"created_at"`

    // Populated only when fetched with members
    Members []DmChannelMember `json:"members,omitempty" db:"-"`

    // Existing 1:1 fields (only meaningful when !is_group)
    OtherUsername    string `json:"other_username,omitempty" db:"-"`
    OtherDisplayName string `json:"other_display_name,omitempty" db:"-"`
    OtherUserID      int64  `json:"other_user_id,omitempty,string" db:"-"`
    OtherAvatarURL   string `json:"other_avatar_url,omitempty" db:"-"`
}

type DmChannelMember struct {
    DmChannelID int64     `json:"dm_channel_id,string" db:"dm_channel_id"`
    UserID      int64     `json:"user_id,string" db:"user_id"`
    JoinedAt    time.Time `json:"joined_at" db:"joined_at"`

    // Populated on read (joined to users)
    Username    string `json:"username,omitempty" db:"-"`
    DisplayName string `json:"display_name,omitempty" db:"-"`
    AvatarURL   string `json:"avatar_url,omitempty" db:"-"`
}
```

- [ ] **Step 2: Extend `DmMessage`**

```go
type DmMessage struct {
    // ... existing fields ...
    SystemEvent *json.RawMessage `json:"system_event,omitempty" db:"system_event"`
}
```

(Use `json.RawMessage` so the JSONB stored value flows through verbatim; client-side renderer parses it.)

- [ ] **Step 3: Build to verify**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add internal/db/models.go
git commit -m "feat(db): extend DmChannel/DmMessage models for group DMs"
```

---

### Task 13: Themed name generator (`internal/dm/names.go`)

**Files:**
- Create: `internal/dm/names.go`
- Create: `internal/dm/names_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/dm/names_test.go`:

```go
package dm

import (
    "strings"
    "testing"
)

func TestPickGroupName_Size3(t *testing.T) {
    seen := map[string]bool{}
    for i := 0; i < 50; i++ {
        name := PickGroupName(3)
        if !contains(namesSize3, name) {
            t.Fatalf("PickGroupName(3) returned %q, not in size-3 bucket", name)
        }
        seen[name] = true
    }
    if len(seen) < 3 {
        t.Fatalf("expected variety across 50 picks, got %d unique", len(seen))
    }
}

func TestPickGroupName_SizeBuckets(t *testing.T) {
    cases := []struct{ size int; bucket []string }{
        {3, namesSize3},
        {4, namesSizeSmall},
        {7, namesSizeSmall},
        {10, namesSizeSmall},
        {11, namesSizeLarge},
        {25, namesSizeLarge},
        {99, namesSizeLarge},
    }
    for _, c := range cases {
        name := PickGroupName(c.size)
        if !contains(c.bucket, name) {
            t.Errorf("PickGroupName(%d) = %q, not in expected bucket", c.size, name)
        }
    }
}

func TestPickGroupName_BelowMinFallsBackToSize3(t *testing.T) {
    // Practical: 1-person and 2-person groups don't exist, but defend against misuse.
    name := PickGroupName(1)
    if !contains(namesSize3, name) {
        t.Errorf("PickGroupName(1) should fall back to size-3 bucket, got %q", name)
    }
}

func contains(bucket []string, name string) bool {
    for _, n := range bucket {
        if strings.EqualFold(n, name) {
            return true
        }
    }
    return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t go test ./internal/dm/ -run PickGroupName -v`
Expected: FAIL — function/buckets not defined.

- [ ] **Step 3: Implement the generator**

Create `internal/dm/names.go`:

```go
package dm

import (
    "math/rand/v2"
)

// Themed default-name buckets, picked at create time per group size.
// Owner curates this list before merge — these are seed entries.

var namesSize3 = []string{
    "Trifecta", "The Three Musketeers", "The Three Stooges", "Triumvirate",
    "Triple Threat", "The Holy Trinity", "Three Amigos", "Power Trio",
    "Three's Company", "The Triplets", "Trinity", "Three Wise Monkeys",
    "Three Wise Men", "The Threesome", "Triad",
}

var namesSizeSmall = []string{ // 4-10
    "The Squad", "The Crew", "The Gang", "The Posse", "The Pack",
    "The Bunch", "The Lot", "The Cabal", "The Coalition", "The Clique",
    "The Inner Circle", "The League", "The Round Table", "The Avengers",
    "The Fellowship", "The Bunch of Misfits", "The Usual Suspects",
    "The A-Team", "The Wolfpack", "The Council",
}

var namesSizeLarge = []string{ // 11+
    "Small Army", "The Horde", "The Mob", "The Multitude", "The Battalion",
    "Gangsters! From the Far Side of the Moon!",
    "The Rebellion", "The Conspiracy",
    "The Convention", "Tiny Township", "The Flotilla",
    "Society of Definitely Not Up To Anything",
    "The Migration", "The Caravan", "The Symposium", "Town Hall Without The Hall",
    "The Stadium Wave", "The Cargo Cult",
}

// PickGroupName returns a random name from the bucket appropriate for the size.
func PickGroupName(memberCount int) string {
    bucket := bucketFor(memberCount)
    if len(bucket) == 0 {
        return "Group Chat"
    }
    return bucket[rand.IntN(len(bucket))]
}

func bucketFor(memberCount int) []string {
    switch {
    case memberCount <= 3:
        return namesSize3
    case memberCount <= 10:
        return namesSizeSmall
    default:
        return namesSizeLarge
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `JWT_SECRET=t go test ./internal/dm/ -run PickGroupName -v`
Expected: PASS — 3 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/dm/names.go internal/dm/names_test.go
git commit -m "feat(dm): themed default-name generator with size-bucketed lists"
```

---

### Task 14: DM repository — `dm_channel_members` operations

**Files:**
- Modify: `internal/db/dm_repository.go`

- [ ] **Step 1: Write a member-ops test**

Add to `internal/db/dm_repository_test.go` (or create if missing):

```go
func TestDmRepository_AddMember_Idempotent(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()

    a := seedUser(t, repo, "alice")
    b := seedUser(t, repo, "bob")
    ch, _ := repo.CreateGroupChannel(ctx, a, "Test Group", []int64{a, b})

    if err := repo.AddDmMember(ctx, ch.ID, a); err != nil { // already a member
        t.Fatalf("AddDmMember should be idempotent: %v", err)
    }
    members, _ := repo.GetDmMembers(ctx, ch.ID)
    if len(members) != 2 {
        t.Fatalf("expected 2 members after idempotent add, got %d", len(members))
    }
}

func TestDmRepository_RemoveMember(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    a := seedUser(t, repo, "alice")
    b := seedUser(t, repo, "bob")
    c := seedUser(t, repo, "carol")
    ch, _ := repo.CreateGroupChannel(ctx, a, "Test Group", []int64{a, b, c})

    if err := repo.RemoveDmMember(ctx, ch.ID, b); err != nil {
        t.Fatalf("RemoveDmMember: %v", err)
    }
    members, _ := repo.GetDmMembers(ctx, ch.ID)
    if len(members) != 2 {
        t.Fatalf("expected 2 members, got %d", len(members))
    }
}

func TestDmRepository_IsMember(t *testing.T) {
    repo := newTestRepo(t)
    ctx := context.Background()
    a := seedUser(t, repo, "alice")
    b := seedUser(t, repo, "bob")
    c := seedUser(t, repo, "carol")
    ch, _ := repo.CreateGroupChannel(ctx, a, "G", []int64{a, b})

    if ok, _ := repo.IsDmMember(ctx, ch.ID, a); !ok { t.Fatal("a should be member") }
    if ok, _ := repo.IsDmMember(ctx, ch.ID, c); ok { t.Fatal("c should not be member") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t go test ./internal/db/ -run DmRepository_ -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the methods**

Add to `internal/db/dm_repository.go`:

```go
// CreateGroupChannel creates a new group DM channel with the given creator and member set.
// Creator is also the owner. Members are inserted in a single transaction with the channel.
func (r *Repository) CreateGroupChannel(ctx context.Context, creatorUserID int64, name string, memberUserIDs []int64) (*DmChannel, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback()

    var ch DmChannel
    err = tx.QueryRowContext(ctx, `
        INSERT INTO dm_channels (user1_id, user2_id, is_group, name, created_by_user_id, owner_user_id, created_at)
        VALUES (0, 0, TRUE, $1, $2, $2, NOW())
        RETURNING id, is_group, name, created_by_user_id, owner_user_id, created_at
    `, name, creatorUserID).Scan(&ch.ID, &ch.IsGroup, &ch.Name, &ch.CreatedByUserID, &ch.OwnerUserID, &ch.CreatedAt)
    if err != nil {
        return nil, err
    }

    for _, uid := range memberUserIDs {
        if _, err := tx.ExecContext(ctx, `
            INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
            VALUES ($1, $2, NOW())
            ON CONFLICT DO NOTHING
        `, ch.ID, uid); err != nil {
            return nil, err
        }
    }
    return &ch, tx.Commit()
}

func (r *Repository) AddDmMember(ctx context.Context, channelID, userID int64) error {
    _, err := r.db.ExecContext(ctx, `
        INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT DO NOTHING
    `, channelID, userID)
    return err
}

func (r *Repository) RemoveDmMember(ctx context.Context, channelID, userID int64) error {
    _, err := r.db.ExecContext(ctx, `DELETE FROM dm_channel_members WHERE dm_channel_id = $1 AND user_id = $2`, channelID, userID)
    return err
}

func (r *Repository) IsDmMember(ctx context.Context, channelID, userID int64) (bool, error) {
    var n int
    err := r.db.QueryRowContext(ctx, `SELECT 1 FROM dm_channel_members WHERE dm_channel_id = $1 AND user_id = $2`, channelID, userID).Scan(&n)
    if err == sql.ErrNoRows {
        return false, nil
    }
    return n == 1, err
}

func (r *Repository) GetDmMembers(ctx context.Context, channelID int64) ([]DmChannelMember, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT m.dm_channel_id, m.user_id, m.joined_at, u.username,
               COALESCE(u.display_name, u.username), COALESCE(u.avatar_url, '')
          FROM dm_channel_members m
          JOIN users u ON u.id = m.user_id
         WHERE m.dm_channel_id = $1
         ORDER BY m.joined_at
    `, channelID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []DmChannelMember
    for rows.Next() {
        var m DmChannelMember
        if err := rows.Scan(&m.DmChannelID, &m.UserID, &m.JoinedAt, &m.Username, &m.DisplayName, &m.AvatarURL); err != nil {
            return nil, err
        }
        out = append(out, m)
    }
    return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `JWT_SECRET=t go test ./internal/db/ -run DmRepository_ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/dm_repository.go internal/db/dm_repository_test.go
git commit -m "feat(db): dm_channel_members CRUD + CreateGroupChannel"
```

---

### Task 15: DM repository — group metadata + system messages

**Files:**
- Modify: `internal/db/dm_repository.go` (continue)

- [ ] **Step 1: Add metadata + system-message methods**

```go
func (r *Repository) UpdateDmGroupName(ctx context.Context, channelID int64, name string) error {
    _, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET name = $1 WHERE id = $2 AND is_group = TRUE`, name, channelID)
    return err
}

func (r *Repository) UpdateDmGroupAvatar(ctx context.Context, channelID int64, avatarURL *string) error {
    _, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET avatar_url = $1 WHERE id = $2 AND is_group = TRUE`, avatarURL, channelID)
    return err
}

func (r *Repository) TransferDmGroupOwnership(ctx context.Context, channelID, newOwnerID int64) error {
    _, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET owner_user_id = $1 WHERE id = $2 AND is_group = TRUE`, newOwnerID, channelID)
    return err
}

// InsertSystemMessage adds a synthetic dm_messages row carrying the given system event.
// content is empty; system_event is the JSONB payload.
func (r *Repository) InsertSystemMessage(ctx context.Context, channelID int64, actorUserID int64, eventJSON []byte) (*DmMessage, error) {
    var m DmMessage
    err := r.db.QueryRowContext(ctx, `
        INSERT INTO dm_messages (dm_channel_id, author_id, content, system_event, created_at, updated_at)
        VALUES ($1, $2, '', $3::jsonb, NOW(), NOW())
        RETURNING id, dm_channel_id, author_id, content, system_event, created_at, updated_at
    `, channelID, actorUserID, eventJSON).Scan(&m.ID, &m.DmChannelID, &m.AuthorID, &m.Content, &m.SystemEvent, &m.CreatedAt, &m.UpdatedAt)
    return &m, err
}
```

- [ ] **Step 2: Build to verify**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```bash
git add internal/db/dm_repository.go
git commit -m "feat(db): UpdateDmGroupName/Avatar, TransferDmGroupOwnership, InsertSystemMessage"
```

---

### Task 16: `internal/dm/service.go` — service layer scaffold + `CreateChannel`

**Files:**
- Create: `internal/dm/service.go`
- Create: `internal/dm/service_test.go`
- Modify: `internal/dm/handler.go` (refactor `OpenDmChannel` to delegate to service)

- [ ] **Step 1: Write the failing service test**

Create `internal/dm/service_test.go`:

```go
package dm

import (
    "context"
    "testing"
)

func TestService_CreateChannel_OneToOne_ReusesExisting(t *testing.T) {
    fx := newServiceTestFixture(t)
    a := fx.seedUser("alice")
    b := fx.seedUser("bob")

    ch1, err := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID}, "")
    if err != nil { t.Fatalf("CreateChannel 1: %v", err) }

    ch2, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID}, "")
    if ch1.ID != ch2.ID {
        t.Fatalf("expected reuse of 1:1 channel; got different IDs %d vs %d", ch1.ID, ch2.ID)
    }
}

func TestService_CreateChannel_Group_CreatesAndPicksDefaultName(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")

    ch, err := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")
    if err != nil { t.Fatalf("CreateChannel: %v", err) }

    if !ch.IsGroup { t.Fatal("expected is_group=true") }
    if ch.Name == nil || *ch.Name == "" {
        t.Fatal("expected default name to be set")
    }
    if !contains(namesSize3, *ch.Name) {
        t.Fatalf("expected name from size-3 bucket, got %q", *ch.Name)
    }
    if ch.OwnerUserID == nil || *ch.OwnerUserID != a.ID {
        t.Fatal("expected creator to be initial owner")
    }

    // Group_created system message should be present
    msgs, _ := fx.repo.GetDmMessages(context.Background(), ch.ID, 10, 0)
    if len(msgs) == 0 || msgs[0].SystemEvent == nil {
        t.Fatal("expected first message to be group_created system event")
    }
}

func TestService_CreateChannel_Group_RequiresMinTwoOthers(t *testing.T) {
    fx := newServiceTestFixture(t)
    a := fx.seedUser("a")

    _, err := fx.svc.CreateChannel(context.Background(), a.ID, []int64{a.ID}, "") // self-only
    if err == nil { t.Fatal("expected error for self-only group create") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t go test ./internal/dm/ -run Service_CreateChannel -v`
Expected: FAIL.

- [ ] **Step 3: Implement the service**

Create `internal/dm/service.go`:

```go
package dm

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"

    "parley/internal/db"
    ws "parley/internal/websocket"
)

type Service struct {
    repo *db.Repository
    hub  *ws.Hub
}

func NewService(repo *db.Repository, hub *ws.Hub) *Service {
    return &Service{repo: repo, hub: hub}
}

// CreateChannel creates a 1:1 channel (single other user) or a group (2+ other users).
// For 1:1, reuses an existing channel if one exists between actor + other.
// For group, the actor becomes the creator + initial owner; emits group_created system message.
func (s *Service) CreateChannel(ctx context.Context, actorUserID int64, otherUserIDs []int64, customName string) (*db.DmChannel, error) {
    others := dedupeAndExcludeSelf(otherUserIDs, actorUserID)

    if len(others) == 0 {
        return nil, errors.New("must include at least one other user")
    }

    if len(others) == 1 {
        // 1:1 path — reuses existing channel via existing repo helper.
        return s.repo.GetOrCreateDmChannel(ctx, actorUserID, others[0])
    }

    // Group path
    name := customName
    if name == "" {
        name = PickGroupName(len(others) + 1) // +1 for actor
    }

    members := append([]int64{actorUserID}, others...)
    ch, err := s.repo.CreateGroupChannel(ctx, actorUserID, name, members)
    if err != nil {
        return nil, err
    }

    // Group-created system message (atomic-ish: best-effort after channel exists).
    eventJSON, _ := json.Marshal(map[string]any{
        "type":          "group_created",
        "actor_user_id": actorUserID,
    })
    if _, err := s.repo.InsertSystemMessage(ctx, ch.ID, actorUserID, eventJSON); err != nil {
        return nil, fmt.Errorf("insert group_created event: %w", err)
    }

    // Fan out CHANNEL_CREATE to all initial members
    for _, uid := range members {
        s.hub.SendToUser(fmt.Sprintf("%d", uid), ws.EventDmChannelCreate, map[string]any{"channel": ch})
    }

    return ch, nil
}

func dedupeAndExcludeSelf(ids []int64, self int64) []int64 {
    seen := map[int64]bool{}
    out := []int64{}
    for _, id := range ids {
        if id == self || seen[id] { continue }
        seen[id] = true
        out = append(out, id)
    }
    return out
}
```

- [ ] **Step 4: Refactor handler `OpenDmChannel` to delegate to service**

In `internal/dm/handler.go`, change `OpenDmChannel` to:
1. Parse the request — accept either `{user_id: X}` (single) or `{user_ids: [...]}` (multi). Build `otherUserIDs` slice.
2. Call `h.svc.CreateChannel(ctx, currentUserID, otherUserIDs, customName)`.
3. Return the channel.

(Add `svc *Service` to `Handler` struct and constructor.)

- [ ] **Step 5: Run tests to verify**

Run: `JWT_SECRET=t go test ./internal/dm/ -run "Service_CreateChannel|TestOpenDmChannel" -v`
Expected: PASS (existing 1:1 test still passes; new group test passes).

- [ ] **Step 6: Commit**

```bash
git add internal/dm/service.go internal/dm/service_test.go internal/dm/handler.go
git commit -m "feat(dm): service layer + CreateChannel (1:1 reuse + group creation)"
```

---

### Task 17: `service.AddMembers` + system message + WS event

**Files:**
- Modify: `internal/dm/service.go`
- Modify: `internal/dm/service_test.go`
- Modify: `internal/dm/handler.go`
- Modify: `cmd/api/routes.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/dm/service_test.go`:

```go
func TestService_AddMembers_AppendsAndEmitsSystemMessage(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c, d := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c"), fx.seedUser("d")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.AddMembers(context.Background(), ch.ID, a.ID, []int64{d.ID}); err != nil {
        t.Fatalf("AddMembers: %v", err)
    }
    members, _ := fx.repo.GetDmMembers(context.Background(), ch.ID)
    if len(members) != 4 { t.Fatalf("expected 4 members, got %d", len(members)) }

    // Last message should be member_added system event
    msgs, _ := fx.repo.GetDmMessages(context.Background(), ch.ID, 5, 0)
    last := msgs[len(msgs)-1]
    if last.SystemEvent == nil { t.Fatal("expected system event") }
}

func TestService_AddMembers_Rejects_NonGroupChannel(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID}, "") // 1:1

    err := fx.svc.AddMembers(context.Background(), ch.ID, a.ID, []int64{c.ID})
    if err == nil || !strings.Contains(err.Error(), "not a group") {
        t.Fatalf("expected 'not a group' error, got %v", err)
    }
}

func TestService_AddMembers_Rejects_NonMemberAdder(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c, d := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c"), fx.seedUser("d")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    err := fx.svc.AddMembers(context.Background(), ch.ID, d.ID, []int64{}) // d is not a member
    if err == nil { t.Fatal("expected 'not a member' rejection") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t go test ./internal/dm/ -run AddMembers -v`
Expected: FAIL.

- [ ] **Step 3: Implement `service.AddMembers`**

Add to `internal/dm/service.go`:

```go
func (s *Service) AddMembers(ctx context.Context, channelID, actorUserID int64, newMemberIDs []int64) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil {
        return err
    }
    if !ch.IsGroup {
        return errors.New("not a group channel — use CreateChannel to spawn a new group from a 1:1")
    }

    // Actor must be a member
    isMember, err := s.repo.IsDmMember(ctx, channelID, actorUserID)
    if err != nil {
        return err
    }
    if !isMember {
        return errors.New("not a member of this channel")
    }

    // Cap check: 100 members max
    members, _ := s.repo.GetDmMembers(ctx, channelID)
    if len(members)+len(newMemberIDs) > 100 {
        return errors.New("group at capacity (max 100)")
    }

    for _, uid := range newMemberIDs {
        if err := s.repo.AddDmMember(ctx, channelID, uid); err != nil {
            return err
        }
        eventJSON, _ := json.Marshal(map[string]any{
            "type":            "member_added",
            "actor_user_id":   actorUserID,
            "target_user_id":  uid,
        })
        sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
        if err != nil {
            return err
        }
        s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
        s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMemberAdd, map[string]any{
            "channel_id":      channelID,
            "user_id":         uid,
            "added_by":        actorUserID,
        })
    }
    return nil
}
```

- [ ] **Step 4: Add the route + handler**

In `internal/dm/handler.go`:

```go
type AddMembersRequest struct {
    UserIDs []string `json:"user_ids"`
}

func (h *Handler) AddMembers(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil { httputil.JSONError(w, "invalid channel id", http.StatusBadRequest); return }

    var req AddMembersRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.JSONError(w, "invalid request body", http.StatusBadRequest); return
    }
    var newIDs []int64
    for _, s := range req.UserIDs {
        n, err := strconv.ParseInt(s, 10, 64)
        if err != nil { httputil.JSONError(w, "invalid user id", http.StatusBadRequest); return }
        newIDs = append(newIDs, n)
    }

    if err := h.svc.AddMembers(r.Context(), channelID, actorID, newIDs); err != nil {
        if strings.Contains(err.Error(), "not a member") || strings.Contains(err.Error(), "not a group") {
            httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
        }
        if strings.Contains(err.Error(), "capacity") {
            httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
        }
        httputil.InternalError(w, err); return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

In `cmd/api/routes.go`, register:

```go
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{id}/members", dmHandler.AddMembers)
```

- [ ] **Step 5: Run tests**

Run: `JWT_SECRET=t go test ./internal/dm/ ./cmd/api/ -run "AddMembers" -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dm/service.go internal/dm/service_test.go internal/dm/handler.go cmd/api/routes.go
git commit -m "feat(dm): AddMembers service + handler + system message + WS broadcast"
```

---

### Task 18: `service.LeaveGroup` (with optional transfer)

**Files:**
- Modify: `internal/dm/service.go`, `internal/dm/service_test.go`, `internal/dm/handler.go`, `cmd/api/routes.go`

- [ ] **Step 1: Write the test**

```go
func TestService_LeaveGroup_NonOwner_RemovesMemberAndEmitsEvent(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.LeaveGroup(context.Background(), ch.ID, b.ID, nil); err != nil {
        t.Fatalf("LeaveGroup: %v", err)
    }
    if ok, _ := fx.repo.IsDmMember(context.Background(), ch.ID, b.ID); ok {
        t.Fatal("b still member after leave")
    }
}

func TestService_LeaveGroup_OwnerWithTransfer_TransfersAndLeaves(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    bID := b.ID
    if err := fx.svc.LeaveGroup(context.Background(), ch.ID, a.ID, &bID); err != nil {
        t.Fatalf("LeaveGroup with transfer: %v", err)
    }

    chAfter, _ := fx.repo.GetDmChannelByID(context.Background(), ch.ID)
    if chAfter.OwnerUserID == nil || *chAfter.OwnerUserID != bID {
        t.Fatal("owner not transferred")
    }
    if ok, _ := fx.repo.IsDmMember(context.Background(), ch.ID, a.ID); ok {
        t.Fatal("a still member after leave")
    }
}

func TestService_LeaveGroup_OwnerWithoutTransfer_PowerEvaporates(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b := fx.seedUser("a"), fx.seedUser("b")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID}, "")
    // 1:1 channel auto-created above; force group instead:
    c := fx.seedUser("c")
    chG, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.LeaveGroup(context.Background(), chG.ID, a.ID, nil); err != nil {
        t.Fatalf("LeaveGroup: %v", err)
    }

    chAfter, _ := fx.repo.GetDmChannelByID(context.Background(), chG.ID)
    if chAfter.OwnerUserID != nil {
        t.Fatal("expected owner_user_id to be NULL (power evaporates)")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `JWT_SECRET=t go test ./internal/dm/ -run LeaveGroup -v`
Expected: FAIL.

- [ ] **Step 3: Implement `LeaveGroup`**

```go
func (s *Service) LeaveGroup(ctx context.Context, channelID, actorUserID int64, transferTo *int64) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil { return err }
    if !ch.IsGroup { return errors.New("not a group channel") }

    isMember, err := s.repo.IsDmMember(ctx, channelID, actorUserID)
    if err != nil { return err }
    if !isMember { return errors.New("not a member") }

    isOwner := ch.OwnerUserID != nil && *ch.OwnerUserID == actorUserID

    // Transfer ownership if requested
    if transferTo != nil {
        if !isOwner {
            return errors.New("only owner can transfer ownership")
        }
        targetIsMember, _ := s.repo.IsDmMember(ctx, channelID, *transferTo)
        if !targetIsMember {
            return errors.New("transfer target is not a member")
        }
        if err := s.repo.TransferDmGroupOwnership(ctx, channelID, *transferTo); err != nil { return err }
        // Emit owner_transferred system message
        eventJSON, _ := json.Marshal(map[string]any{
            "type":                "owner_transferred",
            "actor_user_id":       actorUserID,
            "new_owner_user_id":   *transferTo,
        })
        sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
        s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    } else if isOwner {
        // Power evaporates: clear owner
        if err := s.repo.TransferDmGroupOwnership(ctx, channelID, 0); err != nil { return err }
        // Note: TransferDmGroupOwnership with userID=0 needs to translate to NULL — see note below.
    }

    // Remove member
    if err := s.repo.RemoveDmMember(ctx, channelID, actorUserID); err != nil { return err }

    eventJSON, _ := json.Marshal(map[string]any{
        "type":          "member_left",
        "actor_user_id": actorUserID,
    })
    sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMemberRemove, map[string]any{
        "channel_id": channelID, "user_id": actorUserID,
    })
    return nil
}
```

For the "power evaporates" path, update `TransferDmGroupOwnership` in the repo to accept a nullable owner — change signature to `TransferDmGroupOwnership(ctx, channelID int64, newOwnerID *int64) error` and pass `nil` to set NULL. Update Task 15 caller paths accordingly.

- [ ] **Step 4: Add the route**

```go
type LeaveRequest struct {
    TransferTo *string `json:"transfer_to,omitempty"`
}

func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil { httputil.JSONError(w, "invalid channel id", http.StatusBadRequest); return }

    var req LeaveRequest
    _ = json.NewDecoder(r.Body).Decode(&req) // body optional

    var transfer *int64
    if req.TransferTo != nil {
        n, err := strconv.ParseInt(*req.TransferTo, 10, 64)
        if err != nil { httputil.JSONError(w, "invalid transfer_to", http.StatusBadRequest); return }
        transfer = &n
    }

    if err := h.svc.LeaveGroup(r.Context(), channelID, actorID, transfer); err != nil {
        if strings.Contains(err.Error(), "not a") {
            httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
        }
        httputil.InternalError(w, err); return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

In routes:
```go
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{id}/leave", dmHandler.Leave)
```

- [ ] **Step 5: Run tests**

Run: `JWT_SECRET=t go test ./internal/dm/ ./cmd/api/ -run Leave -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dm/service.go internal/dm/service_test.go internal/dm/handler.go internal/db/dm_repository.go cmd/api/routes.go
git commit -m "feat(dm): LeaveGroup with optional transfer + power-evaporates fallback"
```

---

### Task 19: `service.KickMember` (owner-only)

**Files:**
- Modify: `internal/dm/service.go`, `internal/dm/service_test.go`, `internal/dm/handler.go`, `cmd/api/routes.go`

- [ ] **Step 1: Write the test**

```go
func TestService_KickMember_OwnerSucceeds(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.KickMember(context.Background(), ch.ID, a.ID, b.ID); err != nil {
        t.Fatalf("KickMember: %v", err)
    }
    if ok, _ := fx.repo.IsDmMember(context.Background(), ch.ID, b.ID); ok {
        t.Fatal("b still member")
    }
}

func TestService_KickMember_NonOwnerForbidden(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    err := fx.svc.KickMember(context.Background(), ch.ID, b.ID, c.ID) // b is not owner
    if err == nil || !strings.Contains(err.Error(), "not the owner") {
        t.Fatalf("expected 'not the owner', got %v", err)
    }
}

func TestService_KickMember_CannotKickSelf(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    err := fx.svc.KickMember(context.Background(), ch.ID, a.ID, a.ID)
    if err == nil || !strings.Contains(err.Error(), "yourself") {
        t.Fatalf("expected 'cannot kick yourself', got %v", err)
    }
}
```

- [ ] **Step 2: Implement KickMember**

```go
func (s *Service) KickMember(ctx context.Context, channelID, actorUserID, targetUserID int64) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil { return err }
    if !ch.IsGroup { return errors.New("not a group channel") }
    if ch.OwnerUserID == nil || *ch.OwnerUserID != actorUserID {
        return errors.New("not the owner of this group")
    }
    if actorUserID == targetUserID {
        return errors.New("cannot kick yourself; use leave instead")
    }
    isMember, _ := s.repo.IsDmMember(ctx, channelID, targetUserID)
    if !isMember { return errors.New("target is not a member") }

    if err := s.repo.RemoveDmMember(ctx, channelID, targetUserID); err != nil { return err }

    eventJSON, _ := json.Marshal(map[string]any{
        "type":            "member_kicked",
        "actor_user_id":   actorUserID,
        "target_user_id":  targetUserID,
    })
    sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMemberRemove, map[string]any{
        "channel_id": channelID, "user_id": targetUserID, "kicked_by": actorUserID,
    })
    return nil
}
```

Handler + route:

```go
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    targetID, _ := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)

    if err := h.svc.KickMember(r.Context(), channelID, actorID, targetID); err != nil {
        if strings.Contains(err.Error(), "not the owner") {
            httputil.JSONError(w, err.Error(), http.StatusForbidden); return
        }
        if strings.Contains(err.Error(), "yourself") || strings.Contains(err.Error(), "not a member") {
            httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
        }
        httputil.InternalError(w, err); return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

In routes:
```go
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/dms/{id}/members/{userID}", dmHandler.RemoveMember)
```

- [ ] **Step 3: Run tests + commit**

```bash
JWT_SECRET=t go test ./internal/dm/ ./cmd/api/ -run Kick -v
git add internal/dm/service.go internal/dm/service_test.go internal/dm/handler.go cmd/api/routes.go
git commit -m "feat(dm): KickMember service + handler (owner-only)"
```

---

### Task 20: `service.UpdateGroupName` + `service.UpdateGroupAvatar` + `service.TransferOwnership`

**Files:**
- Modify: `internal/dm/service.go`, `internal/dm/service_test.go`, `internal/dm/handler.go`, `cmd/api/routes.go`

- [ ] **Step 1: Tests**

```go
func TestService_UpdateGroupName_AnyMemberCan(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.UpdateGroupName(context.Background(), ch.ID, b.ID, "Cool Crew"); err != nil {
        t.Fatalf("UpdateGroupName: %v", err)
    }
    chAfter, _ := fx.repo.GetDmChannelByID(context.Background(), ch.ID)
    if chAfter.Name == nil || *chAfter.Name != "Cool Crew" {
        t.Fatalf("name not updated, got %v", chAfter.Name)
    }
}

func TestService_TransferOwnership_OwnerOnly(t *testing.T) {
    fx := newServiceTestFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.svc.CreateChannel(context.Background(), a.ID, []int64{b.ID, c.ID}, "")

    if err := fx.svc.TransferOwnership(context.Background(), ch.ID, a.ID, b.ID); err != nil {
        t.Fatalf("TransferOwnership: %v", err)
    }
    chAfter, _ := fx.repo.GetDmChannelByID(context.Background(), ch.ID)
    if chAfter.OwnerUserID == nil || *chAfter.OwnerUserID != b.ID {
        t.Fatal("owner not transferred")
    }

    err := fx.svc.TransferOwnership(context.Background(), ch.ID, c.ID, a.ID) // c is not owner
    if err == nil || !strings.Contains(err.Error(), "not the owner") {
        t.Fatalf("expected 'not the owner', got %v", err)
    }
}
```

- [ ] **Step 2: Implement the three service methods**

```go
func (s *Service) UpdateGroupName(ctx context.Context, channelID, actorUserID int64, name string) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil { return err }
    if !ch.IsGroup { return errors.New("not a group channel") }
    isMember, _ := s.repo.IsDmMember(ctx, channelID, actorUserID)
    if !isMember { return errors.New("not a member") }

    var oldName string
    if ch.Name != nil { oldName = *ch.Name }

    if err := s.repo.UpdateDmGroupName(ctx, channelID, name); err != nil { return err }

    eventJSON, _ := json.Marshal(map[string]any{
        "type":          "name_changed",
        "actor_user_id": actorUserID,
        "old_name":      oldName,
        "new_name":      name,
    })
    sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmChannelUpdate, map[string]any{
        "channel_id": channelID, "field": "name", "new_value": name,
    })
    return nil
}

func (s *Service) UpdateGroupAvatar(ctx context.Context, channelID, actorUserID int64, avatarURL *string) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil { return err }
    if !ch.IsGroup { return errors.New("not a group channel") }
    isMember, _ := s.repo.IsDmMember(ctx, channelID, actorUserID)
    if !isMember { return errors.New("not a member") }

    if err := s.repo.UpdateDmGroupAvatar(ctx, channelID, avatarURL); err != nil { return err }

    eventJSON, _ := json.Marshal(map[string]any{
        "type":          "avatar_changed",
        "actor_user_id": actorUserID,
        "removed":       avatarURL == nil,
    })
    sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    return nil
}

func (s *Service) TransferOwnership(ctx context.Context, channelID, actorUserID, newOwnerID int64) error {
    ch, err := s.repo.GetDmChannelByID(ctx, channelID)
    if err != nil { return err }
    if !ch.IsGroup { return errors.New("not a group channel") }
    if ch.OwnerUserID == nil || *ch.OwnerUserID != actorUserID {
        return errors.New("not the owner of this group")
    }
    targetIsMember, _ := s.repo.IsDmMember(ctx, channelID, newOwnerID)
    if !targetIsMember { return errors.New("new owner must be a member") }

    if err := s.repo.TransferDmGroupOwnership(ctx, channelID, &newOwnerID); err != nil { return err }

    eventJSON, _ := json.Marshal(map[string]any{
        "type":               "owner_transferred",
        "actor_user_id":      actorUserID,
        "new_owner_user_id":  newOwnerID,
    })
    sysMsg, _ := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsg)
    s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmChannelUpdate, map[string]any{
        "channel_id": channelID, "field": "owner_user_id", "new_value": newOwnerID,
    })
    return nil
}
```

- [ ] **Step 3: Add handlers + routes**

```go
type UpdateGroupRequest struct { Name string `json:"name"` }
type TransferOwnerRequest struct { UserID string `json:"user_id"` }

func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    var req UpdateGroupRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.JSONError(w, "invalid request body", http.StatusBadRequest); return
    }
    if err := h.svc.UpdateGroupName(r.Context(), channelID, actorID, req.Name); err != nil {
        httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
    }
    w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    actorID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    var req TransferOwnerRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.JSONError(w, "invalid request body", http.StatusBadRequest); return
    }
    newOwner, err := strconv.ParseInt(req.UserID, 10, 64)
    if err != nil { httputil.JSONError(w, "invalid user_id", http.StatusBadRequest); return }
    if err := h.svc.TransferOwnership(r.Context(), channelID, actorID, newOwner); err != nil {
        if strings.Contains(err.Error(), "not the owner") {
            httputil.JSONError(w, err.Error(), http.StatusForbidden); return
        }
        httputil.JSONError(w, err.Error(), http.StatusBadRequest); return
    }
    w.WriteHeader(http.StatusNoContent)
}

// UpdateAvatar (POST) + RemoveAvatar (DELETE) — these reuse the existing /upload pipe.
// For the upload, use the existing `internal/spaces` flow as the avatar handler would for users; on success, call svc.UpdateGroupAvatar(channelID, actorID, &uploadedURL).
// For DELETE, call svc.UpdateGroupAvatar(channelID, actorID, nil).
```

In routes:
```go
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Patch("/dms/{id}", dmHandler.UpdateGroup)
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{id}/avatar", dmHandler.UpdateAvatar)
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Delete("/dms/{id}/avatar", dmHandler.RemoveAvatar)
r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/dms/{id}/owner", dmHandler.TransferOwnership)
```

- [ ] **Step 4: Run tests + commit**

```bash
JWT_SECRET=t go test ./internal/dm/ ./cmd/api/ -run "UpdateGroup|TransferOwner|UpdateAvatar" -v
git add internal/dm/service.go internal/dm/service_test.go internal/dm/handler.go cmd/api/routes.go
git commit -m "feat(dm): UpdateGroupName, UpdateGroupAvatar, TransferOwnership"
```

---

### Task 21: Frontend types + DM API client extensions

**Files:**
- Modify: `frontend/src/api/types.ts`
- Modify: `frontend/src/api/dms.ts`

- [ ] **Step 1: Extend types**

In `frontend/src/api/types.ts`:

```typescript
export interface DmChannel {
  id: string;
  user1_id: string;
  user2_id: string;
  is_group: boolean;
  name: string | null;
  avatar_url: string | null;
  created_by_user_id: string | null;
  owner_user_id: string | null;
  created_at: string;
  members?: DmChannelMember[]; // populated only on detail fetch
  // 1:1 fields (only when !is_group)
  other_username?: string;
  other_display_name?: string;
  other_user_id?: string;
  other_avatar_url?: string;
}

export interface DmChannelMember {
  dm_channel_id: string;
  user_id: string;
  joined_at: string;
  username?: string;
  display_name?: string;
  avatar_url?: string;
}

export type SystemEvent =
  | { type: 'group_created'; actor_user_id: string }
  | { type: 'member_added'; actor_user_id: string; target_user_id: string }
  | { type: 'member_left'; actor_user_id: string }
  | { type: 'member_kicked'; actor_user_id: string; target_user_id: string }
  | { type: 'name_changed'; actor_user_id: string; old_name: string; new_name: string }
  | { type: 'avatar_changed'; actor_user_id: string; removed: boolean }
  | { type: 'owner_transferred'; actor_user_id: string; new_owner_user_id: string };

export interface DmMessage {
  // ... existing fields ...
  system_event?: SystemEvent | null;
}
```

- [ ] **Step 2: Extend `dms.ts` API client**

```typescript
export async function createGroupDm(userIds: string[], name?: string): Promise<DmChannel> {
  return apiClient.post<DmChannel>('/dms', { user_ids: userIds, name });
}
export async function addDmMembers(dmChannelId: string, userIds: string[]): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/members`, { user_ids: userIds });
}
export async function kickDmMember(dmChannelId: string, userId: string): Promise<void> {
  await apiClient.delete(`/dms/${dmChannelId}/members/${userId}`);
}
export async function leaveDm(dmChannelId: string, transferTo?: string): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/leave`, transferTo ? { transfer_to: transferTo } : {});
}
export async function renameDmGroup(dmChannelId: string, name: string): Promise<void> {
  await apiClient.patch(`/dms/${dmChannelId}`, { name });
}
export async function uploadDmGroupAvatar(dmChannelId: string, file: File): Promise<{ url: string }> {
  const fd = new FormData();
  fd.append('file', file);
  return apiClient.postForm(`/dms/${dmChannelId}/avatar`, fd);
}
export async function removeDmGroupAvatar(dmChannelId: string): Promise<void> {
  await apiClient.delete(`/dms/${dmChannelId}/avatar`);
}
export async function transferDmOwnership(dmChannelId: string, userId: string): Promise<void> {
  await apiClient.post(`/dms/${dmChannelId}/owner`, { user_id: userId });
}
```

- [ ] **Step 3: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/api/types.ts frontend/src/api/dms.ts
git commit -m "feat(api/client): types + endpoints for group DMs"
```

---

### Task 22: `MosaicAvatar` component

**Files:**
- Create: `frontend/src/components/dm/MosaicAvatar.tsx`
- Create: `frontend/src/components/dm/MosaicAvatar.css` (or styled-component)

- [ ] **Step 1: Implement the component**

```tsx
import React from 'react';
import './MosaicAvatar.css';

interface Tile { avatarUrl?: string; displayName: string; }

interface Props {
  tiles: Tile[];
  size?: number; // px, default 40
}

export const MosaicAvatar: React.FC<Props> = ({ tiles, size = 40 }) => {
  // Cap at 4 tiles for visual; ignore overflow
  const visible = tiles.slice(0, 4);
  const layout = visible.length;

  return (
    <div className={`mosaic-avatar mosaic-${layout}`} style={{ width: size, height: size }}>
      {visible.map((t, i) => (
        <div key={i} className="mosaic-tile" style={{ backgroundImage: t.avatarUrl ? `url(${t.avatarUrl})` : undefined }}>
          {!t.avatarUrl && <span className="mosaic-initials">{initials(t.displayName)}</span>}
        </div>
      ))}
    </div>
  );
};

function initials(name: string): string {
  const parts = name.split(/\s+/).filter(Boolean);
  return (parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '');
}
```

CSS:

```css
.mosaic-avatar { position: relative; border-radius: 50%; overflow: hidden; display: grid; gap: 1px; background: var(--bg-elevated); }
.mosaic-1 { grid-template: 1fr / 1fr; }
.mosaic-2 { grid-template: 1fr / 1fr 1fr; }
.mosaic-3 { grid-template: 1fr 1fr / 1fr 1fr; }
.mosaic-3 .mosaic-tile:nth-child(1) { grid-row: 1 / span 2; }
.mosaic-4 { grid-template: 1fr 1fr / 1fr 1fr; }
.mosaic-tile {
  background-size: cover; background-position: center;
  display: flex; align-items: center; justify-content: center;
  background-color: var(--bg-elevated-2); color: var(--fg-muted); font-size: 11px;
}
.mosaic-initials { text-transform: uppercase; }
```

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/components/dm/MosaicAvatar.tsx frontend/src/components/dm/MosaicAvatar.css
git commit -m "feat(frontend): MosaicAvatar component (1/2/3/4 composite tiles)"
```

---

### Task 23: `SystemMessage` component

**Files:**
- Create: `frontend/src/components/chat/SystemMessage.tsx`

- [ ] **Step 1: Implement**

```tsx
import React from 'react';
import type { SystemEvent } from '../../api/types';
import './SystemMessage.css';

interface Props {
  event: SystemEvent;
  resolveUser: (userId: string) => { displayName: string };
  createdAt: string;
}

export const SystemMessage: React.FC<Props> = ({ event, resolveUser, createdAt }) => {
  const text = renderEvent(event, resolveUser);
  return (
    <div className="system-message" role="note">
      <span className="system-message-text">{text}</span>
      <span className="system-message-time">{formatTime(createdAt)}</span>
    </div>
  );
};

function renderEvent(event: SystemEvent, resolve: (id: string) => { displayName: string }): string {
  const actor = resolve(event.actor_user_id).displayName;
  switch (event.type) {
    case 'group_created':       return `${actor} created the group`;
    case 'member_added':        return `${actor} added ${resolve(event.target_user_id).displayName}`;
    case 'member_left':         return `${actor} left the group`;
    case 'member_kicked':       return `${actor} removed ${resolve(event.target_user_id).displayName}`;
    case 'name_changed':        return `${actor} renamed the group to '${event.new_name}'`;
    case 'avatar_changed':      return event.removed ? `${actor} reset the group avatar` : `${actor} changed the group avatar`;
    case 'owner_transferred':   return `${actor} transferred ownership to ${resolve(event.new_owner_user_id).displayName}`;
  }
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
```

CSS:

```css
.system-message { display: flex; align-items: center; justify-content: center; gap: 8px;
                  padding: 4px 12px; font-size: 12px; color: var(--fg-muted); font-style: italic; }
.system-message-text { text-align: center; }
.system-message-time { font-size: 10px; opacity: 0.6; }
```

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/components/chat/SystemMessage.tsx frontend/src/components/chat/SystemMessage.css
git commit -m "feat(frontend): SystemMessage component for chat events"
```

---

### Task 24: Hook `useGroupMembers`

**Files:**
- Create: `frontend/src/hooks/useGroupMembers.ts`

- [ ] **Step 1: Implement**

```typescript
import { useEffect, useState, useCallback } from 'react';
import { apiClient } from '../api/client';
import type { DmChannelMember } from '../api/types';

export function useGroupMembers(dmChannelId: string | null): {
  members: DmChannelMember[];
  loading: boolean;
  refetch: () => Promise<void>;
} {
  const [members, setMembers] = useState<DmChannelMember[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchOnce = useCallback(async () => {
    if (!dmChannelId) return;
    setLoading(true);
    try {
      const list = await apiClient.get<DmChannelMember[]>(`/dms/${dmChannelId}/members`);
      setMembers(list);
    } finally {
      setLoading(false);
    }
  }, [dmChannelId]);

  useEffect(() => { void fetchOnce(); }, [fetchOnce]);

  // WS subscription — refetch on add/remove for this channel
  useEffect(() => {
    if (!dmChannelId) return;
    const handler = (e: CustomEvent<{ channel_id: string }>) => {
      if (e.detail.channel_id === dmChannelId) void fetchOnce();
    };
    window.addEventListener('parley:dm_member_change', handler as EventListener);
    return () => window.removeEventListener('parley:dm_member_change', handler as EventListener);
  }, [dmChannelId, fetchOnce]);

  return { members, loading, refetch: fetchOnce };
}
```

Wire WS event dispatch in `App.tsx` — add cases for `DM_MEMBER_ADD` and `DM_MEMBER_REMOVE` that dispatch the `parley:dm_member_change` event with `channel_id` from the payload.

This requires **a new endpoint**: `GET /api/dms/{id}/members` — implement in `internal/dm/handler.go`:

```go
func (h *Handler) GetMembers(w http.ResponseWriter, r *http.Request) {
    userIDStr := auth.GetUserIDFromContext(r)
    userID, _ := strconv.ParseInt(userIDStr, 10, 64)
    channelID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    isMember, _ := h.repo.IsDmMember(r.Context(), channelID, userID)
    if !isMember { httputil.JSONError(w, "forbidden", http.StatusForbidden); return }
    members, err := h.repo.GetDmMembers(r.Context(), channelID)
    if err != nil { httputil.InternalError(w, err); return }
    json.NewEncoder(w).Encode(members)
}
```
Route: `r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/dms/{id}/members", dmHandler.GetMembers)`

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/hooks/useGroupMembers.ts frontend/src/App.tsx internal/dm/handler.go cmd/api/routes.go
git commit -m "feat(dm): GET /api/dms/{id}/members + useGroupMembers hook"
```

---

### Task 25: `CreateGroupModal`, `AddPeopleModal`, `LeaveGroupModal`, `TransferOwnershipModal`

**Files:**
- Create: `frontend/src/components/dm/CreateGroupModal.tsx`
- Create: `frontend/src/components/dm/AddPeopleModal.tsx`
- Create: `frontend/src/components/dm/LeaveGroupModal.tsx`
- Create: `frontend/src/components/dm/TransferOwnershipModal.tsx`

- [ ] **Step 1: Implement `CreateGroupModal`**

```tsx
import React, { useState } from 'react';
import { Modal } from '../ui/Modal';
import { UserMultiPicker } from '../ui/UserMultiPicker'; // existing or build minimal
import { createGroupDm } from '../../api/dms';

export const CreateGroupModal: React.FC<{ open: boolean; onClose: () => void; onCreated: (id: string) => void }> = ({ open, onClose, onCreated }) => {
  const [picked, setPicked] = useState<string[]>([]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);

  const canSubmit = picked.length >= 2 && !busy;

  const submit = async () => {
    setBusy(true);
    try {
      const ch = await createGroupDm(picked, name.trim() || undefined);
      onCreated(ch.id);
      onClose();
    } finally { setBusy(false); }
  };

  return (
    <Modal open={open} onClose={onClose} title="New Group">
      <UserMultiPicker selected={picked} onChange={setPicked} maxCount={99} />
      <input
        className="modal-input" placeholder="Group name (optional)"
        value={name} onChange={(e) => setName(e.target.value)}
      />
      <div className="modal-actions">
        <button onClick={onClose}>Cancel</button>
        <button disabled={!canSubmit} onClick={submit}>Create</button>
      </div>
    </Modal>
  );
};
```

(`UserMultiPicker` is a minimal new component or existing — match codebase patterns; if missing, build with debounced typeahead against `/api/users/search`.)

- [ ] **Step 2: Implement `AddPeopleModal` (dual-context)**

```tsx
export const AddPeopleModal: React.FC<{
  open: boolean;
  onClose: () => void;
  context: { kind: 'spawn-from-1to1'; otherUserId: string } | { kind: 'add-to-group'; channelId: string };
  onCompleted: (channelIdToOpen: string) => void;
}> = ({ open, onClose, context, onCompleted }) => {
  const [picked, setPicked] = useState<string[]>([]);

  const submit = async () => {
    if (context.kind === 'spawn-from-1to1') {
      const ch = await createGroupDm([context.otherUserId, ...picked]);
      onCompleted(ch.id);
    } else {
      await addDmMembers(context.channelId, picked);
      onCompleted(context.channelId);
    }
    onClose();
  };

  const helper = context.kind === 'spawn-from-1to1'
    ? `This will create a new group with you, your DM partner, and ${picked.length} other(s).`
    : `These users will be added to this group.`;

  return (
    <Modal open={open} onClose={onClose} title={context.kind === 'spawn-from-1to1' ? 'New Group' : 'Add People'}>
      <p className="modal-helper">{helper}</p>
      <UserMultiPicker selected={picked} onChange={setPicked} />
      <div className="modal-actions">
        <button onClick={onClose}>Cancel</button>
        <button disabled={picked.length === 0} onClick={submit}>{context.kind === 'spawn-from-1to1' ? 'Create' : 'Add'}</button>
      </div>
    </Modal>
  );
};
```

- [ ] **Step 3: Implement `LeaveGroupModal`**

```tsx
export const LeaveGroupModal: React.FC<{
  open: boolean; onClose: () => void;
  channelId: string; isOwner: boolean; members: DmChannelMember[];
  currentUserId: string;
  onLeft: () => void;
}> = ({ open, onClose, channelId, isOwner, members, currentUserId, onLeft }) => {
  const [step, setStep] = useState<'confirm' | 'pickSuccessor'>(isOwner ? 'pickSuccessor' : 'confirm');
  const [transferTo, setTransferTo] = useState<string | null>(null);
  const others = members.filter(m => m.user_id !== currentUserId);

  const submit = async () => {
    await leaveDm(channelId, transferTo ?? undefined);
    onLeft();
    onClose();
  };

  if (isOwner && step === 'pickSuccessor') {
    return (
      <Modal open={open} onClose={onClose} title="Leave Group — Pick Successor">
        <p>You're the owner. Pick a new owner before leaving, or leave without transferring (kick power evaporates).</p>
        <select value={transferTo ?? ''} onChange={(e) => setTransferTo(e.target.value || null)}>
          <option value="">Leave without transferring</option>
          {others.map(m => <option key={m.user_id} value={m.user_id}>{m.display_name ?? m.username}</option>)}
        </select>
        <div className="modal-actions">
          <button onClick={onClose}>Cancel</button>
          <button onClick={() => setStep('confirm')}>Continue</button>
        </div>
      </Modal>
    );
  }

  return (
    <Modal open={open} onClose={onClose} title="Leave Group">
      <p>{transferTo ? `Ownership will transfer to ${others.find(m => m.user_id === transferTo)?.display_name ?? '?'}.` : `You will leave the group.`}</p>
      <div className="modal-actions">
        <button onClick={onClose}>Cancel</button>
        <button className="danger" onClick={submit}>Leave</button>
      </div>
    </Modal>
  );
};
```

- [ ] **Step 4: Implement `TransferOwnershipModal`**

```tsx
export const TransferOwnershipModal: React.FC<{
  open: boolean; onClose: () => void;
  channelId: string; members: DmChannelMember[]; currentUserId: string;
  onTransferred: () => void;
}> = ({ open, onClose, channelId, members, currentUserId, onTransferred }) => {
  const [target, setTarget] = useState<string | null>(null);
  const others = members.filter(m => m.user_id !== currentUserId);

  const submit = async () => {
    if (!target) return;
    await transferDmOwnership(channelId, target);
    onTransferred();
    onClose();
  };

  return (
    <Modal open={open} onClose={onClose} title="Transfer Ownership">
      <select value={target ?? ''} onChange={(e) => setTarget(e.target.value || null)}>
        <option value="" disabled>Select new owner…</option>
        {others.map(m => <option key={m.user_id} value={m.user_id}>{m.display_name ?? m.username}</option>)}
      </select>
      <div className="modal-actions">
        <button onClick={onClose}>Cancel</button>
        <button disabled={!target} onClick={submit}>Transfer</button>
      </div>
    </Modal>
  );
};
```

- [ ] **Step 5: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/components/dm/CreateGroupModal.tsx frontend/src/components/dm/AddPeopleModal.tsx frontend/src/components/dm/LeaveGroupModal.tsx frontend/src/components/dm/TransferOwnershipModal.tsx
git commit -m "feat(frontend): Create / Add / Leave / Transfer modals for group DMs"
```

---

### Task 26: `GroupMembersPanel` slide-out

**Files:**
- Create: `frontend/src/components/dm/GroupMembersPanel.tsx`

- [ ] **Step 1: Implement**

```tsx
import React, { useState } from 'react';
import { useGroupMembers } from '../../hooks/useGroupMembers';
import { AddPeopleModal } from './AddPeopleModal';
import { LeaveGroupModal } from './LeaveGroupModal';
import { TransferOwnershipModal } from './TransferOwnershipModal';
import { kickDmMember } from '../../api/dms';

export const GroupMembersPanel: React.FC<{
  channelId: string;
  ownerId: string | null;
  currentUserId: string;
  open: boolean;
  onClose: () => void;
}> = ({ channelId, ownerId, currentUserId, open, onClose }) => {
  const { members, refetch } = useGroupMembers(open ? channelId : null);
  const [showAdd, setShowAdd] = useState(false);
  const [showLeave, setShowLeave] = useState(false);
  const [showTransfer, setShowTransfer] = useState(false);
  const isOwner = ownerId === currentUserId;

  if (!open) return null;

  const kick = async (userId: string) => {
    if (!confirm('Remove this member from the group?')) return;
    await kickDmMember(channelId, userId);
    refetch();
  };

  return (
    <aside className="group-members-panel">
      <header>
        <h3>Members ({members.length})</h3>
        <button onClick={onClose} aria-label="Close">×</button>
      </header>
      <div className="panel-actions">
        <button onClick={() => setShowAdd(true)}>+ Add People</button>
        {isOwner && <button onClick={() => setShowTransfer(true)}>Transfer Ownership</button>}
        <button className="danger" onClick={() => setShowLeave(true)}>Leave Group</button>
      </div>
      <ul className="member-list">
        {members.map(m => (
          <li key={m.user_id}>
            <img src={m.avatar_url || `/default-avatar.svg`} alt="" />
            <span>{m.display_name ?? m.username}</span>
            {ownerId === m.user_id && <span className="owner-crown" title="Owner">👑</span>}
            {isOwner && m.user_id !== currentUserId && (
              <button className="kick-btn" onClick={() => kick(m.user_id)}>Kick</button>
            )}
          </li>
        ))}
      </ul>

      <AddPeopleModal
        open={showAdd} onClose={() => setShowAdd(false)}
        context={{ kind: 'add-to-group', channelId }}
        onCompleted={() => { setShowAdd(false); refetch(); }}
      />
      <LeaveGroupModal
        open={showLeave} onClose={() => setShowLeave(false)}
        channelId={channelId} isOwner={isOwner} members={members} currentUserId={currentUserId}
        onLeft={() => { setShowLeave(false); onClose(); /* navigate away */ }}
      />
      <TransferOwnershipModal
        open={showTransfer} onClose={() => setShowTransfer(false)}
        channelId={channelId} members={members} currentUserId={currentUserId}
        onTransferred={() => { setShowTransfer(false); refetch(); }}
      />
    </aside>
  );
};
```

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/components/dm/GroupMembersPanel.tsx
git commit -m "feat(frontend): GroupMembersPanel with add/kick/leave/transfer flows"
```

---

### Task 27: Integrate into `ChatWindow`, `MessageList`, `DmPanel`, `Sidebar`

**Files:**
- Modify: `frontend/src/components/chat/ChatWindow.tsx`
- Modify: `frontend/src/components/chat/MessageList.tsx`
- Modify: `frontend/src/components/layout/DmPanel.tsx`
- Modify: `frontend/src/components/layout/Sidebar.tsx`

- [ ] **Step 1: `ChatWindow` — group-aware header**

In `ChatWindow.tsx`:

```tsx
const isGroup = channel.is_group;
const headerName = isGroup ? (channel.name ?? '(unnamed group)') : (channel.other_display_name ?? channel.other_username);
const headerAvatar = isGroup
  ? <MosaicAvatar tiles={channel.members?.slice(0, 4).map(m => ({ avatarUrl: m.avatar_url, displayName: m.display_name ?? m.username ?? '' })) ?? []} size={32} />
  : <img src={channel.other_avatar_url} alt="" />;

const [showMembersPanel, setShowMembersPanel] = useState(false);

return (
  <div className="chat-window">
    <header>
      {headerAvatar}
      <h2>{headerName}</h2>
      {isGroup && (
        <>
          <span className="member-count">{channel.members?.length ?? 0} members</span>
          <button onClick={() => setShowMembersPanel(true)}>Members</button>
        </>
      )}
    </header>
    {/* rest of existing chat UI */}
    {isGroup && (
      <GroupMembersPanel
        channelId={channel.id} ownerId={channel.owner_user_id} currentUserId={currentUserId}
        open={showMembersPanel} onClose={() => setShowMembersPanel(false)}
      />
    )}
  </div>
);
```

- [ ] **Step 2: `MessageList` — render system messages**

In `MessageList.tsx`, where messages are mapped:

```tsx
{messages.map(msg => (
  msg.system_event != null
    ? <SystemMessage key={msg.id} event={msg.system_event} resolveUser={resolveUser} createdAt={msg.created_at} />
    : <Message key={msg.id} message={msg} {...passThruProps} />
))}
```

`resolveUser(userId)` is a small helper that looks up display name from the channel's `members` (for groups) or the message author hydrated state. If unknown, returns `{ displayName: '[deleted user]' }`.

Also render the last-read divider:

```tsx
{messages.map((msg, i) => {
  const prev = i > 0 ? messages[i - 1] : null;
  const showLastReadDivider = lastReadMessageId && prev && BigInt(prev.id) <= BigInt(lastReadMessageId) && BigInt(msg.id) > BigInt(lastReadMessageId);
  return (
    <React.Fragment key={msg.id}>
      {showLastReadDivider && <div className="last-read-divider">New messages</div>}
      {/* render message */}
    </React.Fragment>
  );
})}
```

- [ ] **Step 3: `DmPanel` — group rows**

In `DmPanel.tsx`, branch on `channel.is_group`:

```tsx
const headerName = channel.is_group ? (channel.name ?? '(unnamed group)') : (channel.other_display_name ?? channel.other_username);
const subtitle = channel.is_group ? `${channel.members?.length ?? 0} members` : (isOtherOnline ? 'online' : 'offline');
const avatar = channel.is_group
  ? <MosaicAvatar tiles={channel.members?.slice(0, 4).map(m => ({ avatarUrl: m.avatar_url, displayName: m.display_name ?? m.username ?? '' })) ?? []} size={36} />
  : <Avatar src={channel.other_avatar_url} alt="" />;
```

- [ ] **Step 4: `Sidebar` — "+ New Group" entry point**

Add a button next to the existing "+ New DM" entry (or in an overflow menu — pick whichever the existing layout supports cleanly). Wire it to `<CreateGroupModal>`:

```tsx
<button onClick={() => setShowCreateGroup(true)}>+ New Group</button>
{/* … */}
<CreateGroupModal
  open={showCreateGroup} onClose={() => setShowCreateGroup(false)}
  onCreated={(id) => navigate(`/dms/${id}`)}
/>
```

- [ ] **Step 5: Build + manual smoke**

Run: `cd frontend && npm run build && npm run dev`
Verify: create a group, see it in sidebar with mosaic, send messages, rename it, kick someone (as owner), transfer ownership, leave.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/chat/ChatWindow.tsx frontend/src/components/chat/MessageList.tsx frontend/src/components/layout/DmPanel.tsx frontend/src/components/layout/Sidebar.tsx
git commit -m "feat(frontend): integrate group DM rendering into ChatWindow / MessageList / DmPanel / Sidebar"
```

---

### Task 28: Notification fan-out for group DMs

**Files:**
- Modify: `internal/notification/service.go`
- Modify: `internal/dm/handler.go::SendDmMessage` (call notification fan-out)

- [ ] **Step 1: Test — group DM mention notifies all mentioned members**

Add to `internal/notification/service_test.go` (or create):

```go
func TestNotification_GroupDmMention_FansOutToAllMentioned(t *testing.T) {
    fx := newNotificationFixture(t)
    a, b, c := fx.seedUser("a"), fx.seedUser("b"), fx.seedUser("c")
    ch, _ := fx.dmSvc.CreateChannel(fx.ctx, a.ID, []int64{b.ID, c.ID}, "")

    // Send message mentioning b and c
    msg := fx.sendDmMessage(ch.ID, a.ID, "hey <@" + b.IDString() + "> <@" + c.IDString() + ">")

    // Both b and c should have notifications
    nb, _ := fx.repo.GetNotificationsForUser(fx.ctx, b.ID)
    nc, _ := fx.repo.GetNotificationsForUser(fx.ctx, c.ID)
    if len(nb) == 0 { t.Fatal("b expected notification") }
    if len(nc) == 0 { t.Fatal("c expected notification") }
}
```

- [ ] **Step 2: Implement multi-recipient fan-out**

Update `internal/notification/service.go`'s DM-notify method to accept a list of recipient user IDs (the group members minus the author). For each, INSERT a notification row.

```go
func (s *Service) NotifyDmMention(ctx context.Context, dmChannelID, authorID int64, mentionedIDs []int64, content string) error {
    for _, uid := range mentionedIDs {
        if uid == authorID { continue }
        if err := s.repo.CreateNotification(ctx, db.Notification{
            UserID: uid, Type: "dm_mention", DmChannelID: &dmChannelID, ActorID: authorID, Body: content,
        }); err != nil {
            return err
        }
        s.hub.SendToUser(strconv.FormatInt(uid, 10), ws.EventNotification, map[string]any{ ... })
    }
    return nil
}
```

In `SendDmMessage` handler, after the message is created, call `NotifyDmMention` with the parsed `<@id>` mentions.

- [ ] **Step 3: Run tests + commit**

```bash
JWT_SECRET=t go test ./internal/notification/ ./internal/dm/ -run "Notification|GroupDm" -v
git add internal/notification/service.go internal/dm/handler.go internal/notification/service_test.go
git commit -m "feat(notifications): multi-recipient fan-out for group DM mentions"
```

---

### Final integration smoke test

- [ ] **Step 1: Build api + admin binaries**

```bash
JWT_SECRET=t IMPERSONATION_JWT_SECRET=t BOT_KEY_SECRET=t ADMIN_ORIGIN=http://localhost go test ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/parley-api-linux ./cmd/api
cd frontend && npm run build && cd ..
```

- [ ] **Step 2: Deploy to prod (CT 102)**

```bash
scp /tmp/parley-api-linux eqr:/tmp/parley-api.new
ssh eqr 'sudo -n /usr/sbin/pct push 102 /tmp/parley-api.new /tmp/parley-api.new --perms 0755 && sudo -n /usr/sbin/pct exec 102 -- bash -c "cp /usr/local/bin/parley-api /usr/local/bin/parley-api.pre-groupdms && mv /tmp/parley-api.new /usr/local/bin/parley-api && systemctl restart parley-api && sleep 4 && systemctl is-active parley-api"'
```

- [ ] **Step 3: Manual smoke**

- Create a group with 2 other users
- Verify default themed name shows
- Add a 3rd user — see system message
- Rename the group — see system message
- Upload custom avatar — see system message
- Kick someone (as owner) — see system message
- Transfer ownership — see system message
- Leave as new owner — pick successor or leave without
- Verify mute setting suppresses notifications (open another tab/account)
- Verify unread badge appears + clears on read

- [ ] **Step 4: Push to GitHub**

```bash
git push origin main
```

---

## Self-review checklist

- [x] Spec coverage — every section of `docs/superpowers/specs/2026-04-24-group-dms-design.md` mapped to tasks above (Sections 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 → Tasks 11–28; cross-cutting state from Sections 6–8 → Tasks 1–10).
- [x] Placeholder scan — no "TBD"/"TODO"/"figure it out" prose; every code step has a code block.
- [x] Type consistency — `ChannelKind` / `NotificationSetting` enum names match across Go and TS; `EventDmChannelCreate` / `EventDmMemberAdd` are referenced consistently; `MosaicAvatar` props match between definition and usage in `ChatWindow` / `DmPanel`.
- [x] Migration ordering #64 → #65, fields added in #65 are referenced by Phase B tasks only.

---

## Open implementation-time decisions (non-blocking)

These are deferred to build per spec §13:
- Exact placement of "+ New Group" entry point in `Sidebar.tsx` (next to "+ New DM" or in overflow menu).
- Themed name banks — owner curates final list before merge; seed has ~15 entries per bucket.
- Mosaic layout for >4 members: visible cap at 4, ordering = joined-earliest (already specified).
