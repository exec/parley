# Friend System Design

**Date:** 2026-03-17
**Status:** Approved (self-approved — user delegated all decisions)

---

## Goal

Add a Discord-style friend system: send/accept/decline/cancel friend requests by username, maintain a friends list with online status, remove friends, and open DMs from the friends list. Real-time updates via WebSocket.

## Architecture

Single-table data model (`friend_requests`) with a status field tracks the full lifecycle. Accepted friends are queried by filtering status='accepted' where the current user is sender or receiver. A new `internal/friend` package handles the HTTP layer. WebSocket events notify both parties of state changes in real time. Frontend adds a Friends view accessible from the DM sidebar.

**Tech Stack:** PostgreSQL, Go (chi router, httputil), React + TypeScript, existing WebSocket hub.

---

## Database

**Migration 36** — new table:

```sql
CREATE TABLE IF NOT EXISTS friend_requests (
    id         BIGSERIAL PRIMARY KEY,
    sender_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(sender_id, receiver_id)
);
CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver ON friend_requests(receiver_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_sender  ON friend_requests(sender_id);
```

Status values: `pending`, `accepted`. Declining or cancelling **deletes the row** — no `declined` status. This means either party can re-initiate a request after a decline, which is consistent with Discord's behaviour.

**Why one table:** Friends are just accepted friend requests. Querying `WHERE (sender_id=$1 OR receiver_id=$1) AND status='accepted'` is simple and fast with the indexes. A separate friendships table adds a write amplification step for no query benefit at this scale.

**Constraint:** `UNIQUE(sender_id, receiver_id)` prevents duplicate same-direction requests at the DB level. The service enforces bidirectional uniqueness (preventing B→A while A→B is pending) at the application layer with a `SELECT FOR UPDATE` inside a transaction so there's no race window.

---

## Backend

### Package: `internal/friend/`

**handler.go** — HTTP handlers wired to chi router:

| Method | Path | Description |
|--------|------|-------------|
| GET | /friends | List accepted friends with online metadata |
| GET | /friend-requests | Incoming + outgoing pending requests |
| POST | /friend-requests | Send request by `{"username":"..."}` |
| POST | /friend-requests/{id}/accept | Accept incoming request |
| DELETE | /friend-requests/{id} | Decline (incoming) or cancel (outgoing) |
| DELETE | /friends/{userId} | Remove an accepted friend |

All endpoints require auth middleware. All errors return JSON via `httputil.JSONError`.

**service.go** — Business logic + DB queries (embedded, no separate repository file — the feature is self-contained):

- `SendRequest(ctx, senderID, username string)` — resolves username to user ID; within a transaction with `SELECT FOR UPDATE` on the bidirectional pair, checks no existing row in either direction (pending or accepted); inserts pending request; broadcasts `friend_request` WS event to receiver
- `AcceptRequest(ctx, requestID, currentUserID)` — verifies receiver == currentUser, sets status=accepted; broadcasts `friend_accept` to **both** the original sender AND the current user (so all sessions update)
- `DeclineOrCancel(ctx, requestID, currentUserID)` — fetches row, verifies `status='pending'` (returns 404 if accepted or missing), verifies actor is sender or receiver, deletes the row
- `RemoveFriend(ctx, currentUserID, otherUserID)` — finds accepted request between pair, deletes it, broadcasts `friend_remove` to other party
- `GetFriends(ctx, userID)` — returns user records for all accepted friends
- `GetRequests(ctx, userID)` — returns `{incoming: [...], outgoing: [...]}`

**Response types:**

```go
type FriendUser struct {
    ID          string `json:"id"`
    Username    string `json:"username"`
    DisplayName string `json:"display_name,omitempty"`
    AvatarURL   string `json:"avatar_url,omitempty"`
}

type FriendRequest struct {
    ID         string     `json:"id"`
    SenderID   string     `json:"sender_id"`
    ReceiverID string     `json:"receiver_id"`
    Status     string     `json:"status"`
    User       FriendUser `json:"user"` // the other party
    CreatedAt  string     `json:"created_at"`
}

type FriendRequestsResponse struct {
    Incoming []FriendRequest `json:"incoming"`
    Outgoing []FriendRequest `json:"outgoing"`
}
```

### WebSocket Events

Emitted via `hub.SendToUser(targetUserID, eventType, payload)`.

**Event type constants** must be added to `internal/websocket/events.go`:
```go
EventFriendRequest = "FRIEND_REQUEST"
EventFriendAccept  = "FRIEND_ACCEPT"
EventFriendRemove  = "FRIEND_REMOVE"
```

| Event | Payload | Recipient(s) |
|-------|---------|--------------|
| `FRIEND_REQUEST` | `{request: FriendRequest}` | receiver only |
| `FRIEND_ACCEPT` | `{user: FriendUser}` | **both** sender and receiver (so all open tabs refresh) |
| `FRIEND_REMOVE` | `{user_id: string}` | other party |

### Wiring

`internal/friend/handler.go` receives `*db.Repository` and `*websocket.Hub`. Routes registered in `cmd/api/routes.go` under the authenticated route group. Handler initialized in `cmd/api/main.go`.

---

## Frontend

### New Types (`frontend/src/api/types.ts`)

```typescript
export interface FriendUser {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
}

export interface FriendRequest {
  id: string;
  sender_id: string;
  receiver_id: string;
  status: 'pending' | 'accepted';
  user: FriendUser; // the other party
  created_at: string;
}

export interface FriendRequestsResponse {
  incoming: FriendRequest[];
  outgoing: FriendRequest[];
}
```

### API Client (`frontend/src/api/friends.ts`)

```typescript
getFriends(): Promise<FriendUser[]>
getFriendRequests(): Promise<FriendRequestsResponse>
sendFriendRequest(username: string): Promise<FriendRequest>
acceptFriendRequest(requestId: string): Promise<void>
declineOrCancelRequest(requestId: string): Promise<void>
removeFriend(userId: string): Promise<void>
```

### App State (`AppContext.tsx`)

New state fields:
- `friends: FriendUser[]`
- `friendRequests: FriendRequestsResponse` — `{ incoming: [], outgoing: [] }`
- `pendingRequestCount: number` — derived from incoming.length, used for badge

New actions:
- `loadFriends()` — fetch friends + requests, set state
- `sendFriendRequest(username)` — POST, update outgoing state
- `acceptFriendRequest(requestId)` — POST, move from incoming to friends
- `declineOrCancelRequest(requestId)` — DELETE, remove from incoming/outgoing
- `removeFriend(userId)` — DELETE, remove from friends list
- `receiveFriendRequest(req)` — WS handler, add to incoming
- `receiveFriendAccept(user)` — WS handler, add to friends, remove from outgoing (for sender's sessions), remove from incoming (for accepter's other sessions) — both sides receive this event, handler handles both cases
- `receiveFriendRemove(userId)` — WS handler, remove from friends

Friends are loaded on login alongside servers/DMs.

### WebSocket Hook (`useWebSocket.ts`)

Three new callback options: `onFriendRequest`, `onFriendAccept`, `onFriendRemove`. Event type strings match the constants defined in `events.go` (lowercase in WS JSON: `FRIEND_REQUEST`, `FRIEND_ACCEPT`, `FRIEND_REMOVE`).

### UI Components

**`DmPanel.tsx`** — Add "Friends" button in header above the DM list:
```
[ Friends (2) ]   ← badge shows pending count
──────────────
Direct Messages
  • alice
  • bob
```
Clicking "Friends" sets `activeFriendsView: true` in App, deselects server/DM.

**`FriendsView.tsx`** — Full-width main content panel, three tabs:

*All Friends tab:*
- Lists `friends[]` sorted by online status then name
- Each item: avatar + online dot, display_name/username, "Message" button → calls `openDmChannel(friend.id)` from `AppContext` (already exists — DM feature is live)
- Empty state: "No friends yet. Add some!"

*Pending tab (shown with badge if count > 0):*
- "Incoming Requests" section: avatar, username, Accept ✓ / Decline ✗ buttons
- "Outgoing Requests" section: avatar, username, Cancel button
- Empty state per section if none

*Add Friend tab:*
- Text input: "Enter a username"
- Send Friend Request button
- Success/error feedback inline (no modal)

**No new CSS file required** — styles added to `FriendsView.css` (new) and minimal additions to `DmPanel.css`.

---

## Data Flow

1. User types username → POST /friend-requests → server looks up username, creates row, WS pushes `friend_request` to receiver → receiver's incoming list updates live
2. Receiver clicks Accept → POST /friend-requests/{id}/accept → status=accepted → WS pushes `friend_accept` to sender → both parties' friends lists update
3. Either party clicks Remove → DELETE /friends/{userId} → row deleted → WS pushes `friend_remove` to other party → both friends lists update

---

## Error Handling

- Sending request to self: 400 "cannot send friend request to yourself"
- Sending request to existing friend: 400 "already friends"
- Sending duplicate pending request (same or reverse direction): 400 "friend request already pending"
- Accepting request not addressed to you: 403 "not your request"
- Username not found: 404 "user not found"
- Acting on request that doesn't exist or doesn't belong to you: 404 "request not found"

Note: Decline/cancel always deletes the row. After deletion, either party may send a new request.

---

## Out of Scope

- Blocking users
- Friend suggestions
- Mutual friends display
- Friend activity feed
