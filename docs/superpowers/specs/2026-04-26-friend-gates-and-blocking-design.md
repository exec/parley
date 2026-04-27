# Friend Gates + User Blocking Design

**Goal:** Close audit items #1 (GC force-add → PII exfiltration), #3 (ring spam → DoS / transcript flood), and #4 (DM-stranger notification spam) by wiring the existing friend system into DM, ring, and notification paths, and by adding a user-block primitive.

**Context:** The friend system (`internal/friend/`, `frontend/src/components/layout/FriendsView.tsx`, `friend_requests` table, WS events) is already built end-to-end but **isn't gating anything**. No blocking exists. This spec covers the gating, blocking, and rate-limit changes only — not the friend system itself.

---

## Architecture

Three integration seams, all wired via setter-injected interfaces in `cmd/api/routes.go` to avoid import cycles (the same pattern used for `dmHandler.SetForwardSourceResolver`):

```
internal/friend.Service
  ├── IsFriend(ctx, a, b) (bool, error)
  └── IsBlocked(ctx, blockerID, blockedID) (bool, error)
       │
       ├──► dm.Service.SetFriendChecker(...)         # gates CreateChannel/AddMembers
       ├──► voice.RingHandler.SetFriendChecker(...)  # gates Ring + suppress quick-cancel
       └──► notification.Service.SetBlockChecker(...) # suppresses notifications to blockers
```

`friend.Service` already exists; we add `IsBlocked`, `Block`, `Unblock`, `GetBlocks`. The other packages get a small interface (typed inside their own package) and a setter — they don't import `friend`.

---

## Schema

Migration #68:

```sql
CREATE TABLE IF NOT EXISTS user_blocks (
    blocker_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id  BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);
CREATE INDEX IF NOT EXISTS idx_user_blocks_blocked ON user_blocks(blocked_id);
```

Asymmetric (A blocking B does not block B from being blocked by A — they're independent rows). Composite PK prevents duplicate-block races.

---

## Friend gates

### 1:1 DM creation (`POST /dms` with one other user)

- Allowed regardless of friendship (DMs are a discovery surface — Discord-style).
- **Reject** if either side has blocked the other.
- Existing `dmCreateLimiter` (30/hr) keeps stranger spam bounded.

### Group DM creation (`POST /dms` with multiple others)

- **Every invitee must be a friend of the actor.**
- **Reject** if actor blocked any invitee, or any invitee blocked actor.
- Rationale: GCs force PII exposure (avatar, status, presence) on everyone in the room. Friends-only is the cheapest defensible boundary.

### GC AddMembers (`POST /dms/{id}/members`)

- Same rule: actor must be friends with each new member.
- Block check both directions.

### Stranger DM message rate (out of scope for v1)

A finer-grained per-pair rate limit on `POST /dms/{id}/messages` for non-friends was considered but cut. The block primitive is sufficient for the audit-blocking case; per-pair stranger rate-limiting is deferred to a future "DM requests inbox" feature.

---

## Ring guards

### Friend gate

`POST /dms/{id}/call/ring`: caller must be friends with target. `403 ring_target_not_friend` otherwise. Block check both directions.

### Rate limiter

```go
ringInitiateLimiter := newRateLimiterFor(rdb, 5, time.Minute) // 5 rings/min per actor
```

Applied via `userRateLimitMiddleware` to both `/call/ring` and `/call/cancel`. Cancel is included because the cancel itself is what spams the transcript via `call_missed`.

### Quick-cancel suppression

Cancels arriving within 3 seconds of ring start: skip `EmitCallMissed`. The receiver hasn't had a chance to dismiss the modal — there's no "missed" event to record. Without this, a bot can ring/cancel/ring/cancel rapidly and flood the DM transcript. Implementation: `RingService` tracks `Ring.StartedAt` (already does); `Cancel()` checks `time.Since(r.StartedAt) < 3*time.Second` and returns a sentinel `ErrCancelTooQuick` (handler still returns 204; just no system message).

---

## Notification suppression

Apply in `notification.Service`:

- `NotifyDM(recipientID, ...)`: skip if recipient blocked sender.
- `NotifyMentions(...)`: skip per-recipient if blocked.
- `NotifyFriendRequest(recipientID, ...)`: skip if blocked.
- `friend.SendRequest`: reject with `ErrBlocked` if either side has blocked the other (don't even create the row).

Block check is best-effort: failures fall through to "deliver" (better to over-notify than silently drop).

---

## Block API

```
POST   /users/{userId}/block      → 204 (idempotent; deletes friendship + pending requests)
DELETE /users/{userId}/block      → 204
GET    /blocks                    → []FriendUser (blocker_id = current user)
```

`Block` semantics in `friend.Service.Block`:
1. Open transaction.
2. Insert `user_blocks` row (`ON CONFLICT DO NOTHING`).
3. Delete any `friend_requests` rows between the two users (any status).
4. Commit.
5. Broadcast `FRIEND_REMOVE` WS event to both parties (existing event reused — UI removes the row from friends list / requests list).

`Unblock`: just delete the row. Doesn't restore friendship; users have to re-friend.

---

## Frontend changes

`frontend/src/components/layout/FriendsView.tsx`:
- Add `'blocked'` tab to `Tab` type.
- New section showing blocked users with "Unblock" action.
- Per existing pattern: avatar + name + button.

`frontend/src/api/friends.ts`:
```ts
export async function blockUser(userId: string): Promise<void>
export async function unblockUser(userId: string): Promise<void>
export async function getBlockedUsers(): Promise<FriendUser[]>
```

`frontend/src/context/AppContext.tsx`:
- Add `blockedUsers: FriendUser[]`, `blockUser`, `unblockUser`.
- Initial load alongside `getFriends` / `getFriendRequests`.

**Block button placement:**
- "Remove Friend" button in FriendsView gets a sibling "Block" button (kicks them out of the friends list and adds to blocks).
- Optional: in 1:1 DM header (deferred — not strictly required for audit).

Toast/feedback: existing `addFeedback` pattern in FriendsView.

No new modal; uses existing patterns.

---

## Error mapping

New sentinels in `friend.Service`:
- `ErrBlocked` — block exists in either direction
- (`ErrSelf`, `ErrAlreadyFriends`, etc. already exist)

New sentinels in `dm.Service`:
- `ErrNotFriend` — for GC creation/AddMembers
- `ErrBlocked` — for 1:1 + GC creation when block exists

New sentinels in `voice` (ring):
- `ErrRingNotFriend`
- `ErrRingBlocked`
- `ErrCancelTooQuick` (internal — never surfaces to caller)

HTTP mapping: 403 with structured JSON `{"error": "not_friend"|"blocked"|...}`.

---

## Testing

Unit tests on the new `friend.Service` methods (Block/Unblock/IsBlocked) using existing test patterns. Integration: a small set of curl-able PoCs against the LXC dev backend, mirroring the audit attack scripts (`/tmp/dm_spam.py`, `/tmp/ring_spam.py`):

- GC AddMembers as non-friend → 403
- 1:1 DM after block → 403
- Ring as non-friend → 403
- Ring spam beyond 5/min → 429
- Quick cancel within 3s → no `call_missed` row in `dm_messages`

---

## Out of scope

- Username search (relies on user typing exact username — same as today).
- Block notifications (no "X blocked you" event — Discord doesn't tell you either).
- Per-pair stranger DM rate limit (deferred).
- "DM requests" inbox (deferred — block + per-user dmCreateLimiter is enough for now).
- Cross-tab sync of block state (FRIEND_REMOVE event already covers the friend-row removal; block list refresh on next mount is acceptable).
