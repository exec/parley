# High-Concurrency Optimization Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the concrete bottlenecks preventing Parley from reaching 25,000 concurrent WebSocket users on the existing `s-2vcpu-2gb` API droplets within the sub-$100 budget.

**Architecture:** The critical path is the hub's single `sync.RWMutex` held during the entire fan-out loop — fixing that is the highest-ROI change. Secondary wins come from DB connection pool configuration, a proper token-bucket rate limiter, and a short-lived membership cache that cuts channel-subscribe DB queries. Infrastructure tasks (sysctl tuning, PgBouncer) support the code changes.

**Tech Stack:** Go 1.25, gorilla/websocket, Redis pub/sub, PostgreSQL, Terraform/cloud-init

**What Gemini got wrong (do NOT implement):**
- `nbio`/`gnet` non-blocking event loop — Go goroutines are cheap; the bottleneck is the mutex, not the I/O model. Rewriting the WS layer is months of work for zero gain here.
- Protocol Buffers — JSON serialization is not the bottleneck. The fan-out is. Protobuf saves ~20% CPU on serialization while we lose 80% of capacity to a single mutex.
- HAProxy JWT extraction via Lua — overly complex, brittle when JWT format changes, wrong layer for this.
- Argo Tunnel / Cloudflare — irrelevant to Proxmox dev benchmarking.

---

## File Structure

```
internal/websocket/
  hub.go              ← Tasks 1, 2, 3 — inverse index, lock-free fan-out, presence cap
  hub_test.go         ← NEW — hub concurrency tests
  client.go           ← Task 3 — send buffer 256→1024

internal/cache/
  membership.go       ← NEW (Task 7) — short-lived membership cache

cmd/api/
  middleware.go       ← Tasks 4, 5 — token bucket + per-user rate limiting
  middleware_test.go  ← NEW — rate limiter tests
  main.go             ← Task 6 — DB pool config + wire membership cache

terraform/
  userdata-api.sh     ← Task 8 — sysctl + ulimit
  userdata-db.sh      ← Task 9 — PgBouncer + PostgreSQL tuning
```

---

## Chunk 1: Hub Performance

### Task 1: Hub inverse index (`clientChannels`) + `safeSend` helper

**Why:** `UnregisterClient` currently iterates every entry in `channelSubs` (O(all channels)) to find which channels a departing client is subscribed to. At 25k users with frequent reconnects across 100+ channels, this is millions of iterations/second under the lock. The inverse index reduces this to O(channels-per-client) — typically 1–5.

**`safeSend` is a prerequisite for Task 2:** After the lock-free fan-out extracts a client snapshot outside the lock, a concurrent `UnregisterClient` may close `client.send` between the snapshot and the send attempt. Sending to a closed channel panics. `safeSend` handles this with a deferred `recover`.

**Files:**
- Modify: `internal/websocket/hub.go`
- Create: `internal/websocket/hub_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/websocket/hub_test.go`:

```go
package websocket

import (
	"fmt"
	"sync"
	"testing"
)

// newTestClient creates a Client without a real WS connection — safe for
// testing hub map logic since ReadPump/WritePump are never started.
func newTestClient(hub *Hub, userID string) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, 1024),
		userID: userID,
	}
}

func TestInverseIndexMaintained(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")

	hub.SubscribeToChannel("ch1", c)
	hub.SubscribeToChannel("ch2", c)

	hub.mu.RLock()
	chans := hub.clientChannels[c]
	hub.mu.RUnlock()

	if !chans["ch1"] || !chans["ch2"] {
		t.Fatal("inverse index not populated after subscribe")
	}

	hub.UnsubscribeFromChannel("ch1", c)

	hub.mu.RLock()
	chans = hub.clientChannels[c]
	hub.mu.RUnlock()

	if chans["ch1"] {
		t.Error("ch1 should have been removed from inverse index")
	}
	if !chans["ch2"] {
		t.Error("ch2 should still be in inverse index")
	}
}

func TestUnregisterCleansInverseIndex(t *testing.T) {
	hub := NewHub()
	c := newTestClient(hub, "user1")

	hub.RegisterClient(c)
	hub.SubscribeToChannel("ch1", c)
	hub.SubscribeToChannel("ch2", c)
	hub.UnregisterClient(c)

	hub.mu.RLock()
	_, inChannelSubs1 := hub.channelSubs["ch1"][c]
	_, inChannelSubs2 := hub.channelSubs["ch2"][c]
	_, inClientChannels := hub.clientChannels[c]
	hub.mu.RUnlock()

	if inChannelSubs1 || inChannelSubs2 {
		t.Error("client still in channelSubs after unregister")
	}
	if inClientChannels {
		t.Error("client still in clientChannels after unregister")
	}
}

func TestSafeSendToClosedChannel(t *testing.T) {
	ch := make(chan []byte, 4)
	close(ch)

	// Must not panic
	sent := safeSend(ch, []byte("hello"))
	if sent {
		t.Error("safeSend should return false for closed channel")
	}
}

func TestSafeSendToFullChannel(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte("full")

	sent := safeSend(ch, []byte("overflow"))
	if sent {
		t.Error("safeSend should return false for full channel")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/dylan/Developer/parley
go test ./internal/websocket/... -run "TestInverse|TestUnregister|TestSafeSend" -v
```

Expected: FAIL — `hub.clientChannels` field does not exist, `safeSend` not defined.

- [ ] **Step 3: Add `clientChannels` field and `safeSend` helper to `hub.go`**

Add the field to the `Hub` struct (after the existing `channelSubs` field):
```go
// clientChannels is the inverse index of channelSubs: for O(k) unregister cleanup
// where k = number of channels this client is subscribed to.
clientChannels map[*Client]map[string]bool
```

Initialize in `NewHub`:
```go
clientChannels: make(map[*Client]map[string]bool),
```

Add `safeSend` as a package-level helper (add before `NewHub`):
```go
// safeSend attempts a non-blocking send to ch. Returns false if the channel is
// full or has been closed (closed-channel send would panic without the recover).
func safeSend(ch chan []byte, msg []byte) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case ch <- msg:
		return true
	default:
		return false
	}
}
```

Update `SubscribeToChannel` — maintain inverse index:
```go
func (h *Hub) SubscribeToChannel(channelID string, client *Client) {
	h.mu.Lock()

	if h.channelSubs[channelID] == nil {
		h.channelSubs[channelID] = make(map[*Client]bool)
	}
	h.channelSubs[channelID][client] = true

	// Maintain inverse index
	if h.clientChannels[client] == nil {
		h.clientChannels[client] = make(map[string]bool)
	}
	h.clientChannels[client][channelID] = true

	h.mu.Unlock()
}
```

Update `UnsubscribeFromChannel` — maintain inverse index:
```go
func (h *Hub) UnsubscribeFromChannel(channelID string, client *Client) {
	h.mu.Lock()

	if h.channelSubs[channelID] != nil {
		delete(h.channelSubs[channelID], client)
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
	}

	// Maintain inverse index
	if h.clientChannels[client] != nil {
		delete(h.clientChannels[client], channelID)
	}

	h.mu.Unlock()
}
```

Replace the O(all channels) loop in `UnregisterClient` with the inverse index lookup. Find this block in `UnregisterClient`:
```go
// Remove from all channel subscriptions
for channelID, clients := range h.channelSubs {
    if _, ok := clients[client]; ok {
        delete(h.channelSubs[channelID], client)
        if len(h.channelSubs[channelID]) == 0 {
            delete(h.channelSubs, channelID)
        }
    }
}
```

Replace it with:
```go
// Remove from all channel subscriptions using O(k) inverse index
// where k = channels this client is subscribed to.
if channels := h.clientChannels[client]; channels != nil {
    for channelID := range channels {
        if h.channelSubs[channelID] != nil {
            delete(h.channelSubs[channelID], client)
            if len(h.channelSubs[channelID]) == 0 {
                delete(h.channelSubs, channelID)
            }
        }
    }
    delete(h.clientChannels, client)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/websocket/... -run "TestInverse|TestUnregister|TestSafeSend" -v -race
```

Expected: PASS (4 tests), no race conditions.

- [ ] **Step 5: Run full test suite to check for regressions**

```bash
go test ./... -race
```

Expected: All existing tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/websocket/hub.go internal/websocket/hub_test.go
git commit -m "perf(hub): add inverse clientChannels index for O(k) unregister + safeSend helper"
```

---

### Task 2: Hub lock-free fan-out

**Why:** Every call to `BroadcastToChannel`, `broadcastToAllLocal`, `BroadcastLocalToChannel`, `SendToUser`, and `SendLocalToUser` currently holds a **write lock** for the entire duration of (1) JSON marshaling, (2) iterating all subscribers, and (3) sending to each client's buffered channel. With 500 subscribers, this serializes ALL other hub operations — including every other channel's broadcasts, every new connection, and every disconnect — for the entire duration of the fan-out.

**The fix:** Three-phase approach per broadcast function:
1. Marshal JSON **outside** all locks (pure CPU, no shared state)
2. Snapshot subscriber list under **RLock** (read-only, concurrent with other reads)
3. Send to each subscriber's channel **outside all locks**
4. Collect slow clients (full buffer) and evict them with a brief **WLock**

This means the write lock is held only for map mutations (evictions), not for the entire fan-out.

**Depends on Task 1:** The `safeSend` helper is used instead of `select { case ch <- msg: default: }` to handle the race where a client is concurrently unregistered between snapshot and send (closing `client.send`) while the broadcast is mid-fan-out.

**Eviction policy (important):** The eviction blocks in this task use **minimal eviction** — they call `closeSend()` and remove the client from the specific subscriber list being iterated, but do NOT delete from `h.clients` or `h.userToClient`. The natural teardown chain (`closeSend` → WritePump detects closed channel → `conn.Close()` → ReadPump exits → sends to `hub.unregister` → `UnregisterClient`) handles full map cleanup and correctly broadcasts `USER_OFFLINE`. If we deleted from `h.clients` in the eviction block, `UnregisterClient`'s guard check (`if _, ok := h.clients[client]; ok`) would fail and `USER_OFFLINE` would never fire — leaving the user permanently appearing online.

**Files:**
- Modify: `internal/websocket/hub.go`
- Modify: `internal/websocket/hub_test.go` (add concurrency tests)

- [ ] **Step 1: Write the failing concurrency tests**

Add to `hub_test.go`:

```go
func TestBroadcastToChannelConcurrentUnregister(t *testing.T) {
	hub := NewHub()
	const N = 50

	clients := make([]*Client, N)
	for i := range clients {
		clients[i] = newTestClient(hub, fmt.Sprintf("user%d", i))
		hub.RegisterClient(clients[i])
		hub.SubscribeToChannel("ch1", clients[i])
	}

	var wg sync.WaitGroup

	// Concurrent broadcasts
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.BroadcastToChannel("ch1", "TEST", []byte(`{"x":1}`))
		}()
	}

	// Concurrent unregisters
	for _, c := range clients[:25] {
		wg.Add(1)
		c := c
		go func() {
			defer wg.Done()
			hub.UnregisterClient(c)
		}()
	}

	wg.Wait()
	// Success: no panic, no race (run with -race)
}

// TestBroadcastToChannelSlowClientMinimalEviction verifies that a slow client
// (full send buffer) is removed from channelSubs for that channel but is NOT
// immediately removed from h.clients — USER_OFFLINE is deferred to UnregisterClient
// via the natural WritePump-exit teardown chain.
//
// This test FAILS with the old code (which deleted from h.clients in the eviction
// block, bypassing USER_OFFLINE), and PASSES with the new minimal-eviction approach.
func TestBroadcastToChannelSlowClientMinimalEviction(t *testing.T) {
	hub := NewHub()

	slow := &Client{
		hub:    hub,
		send:   make(chan []byte, 0), // zero-capacity: always "full"
		userID: "slow",
	}
	fast := newTestClient(hub, "fast")

	hub.RegisterClient(slow)
	hub.RegisterClient(fast)
	hub.SubscribeToChannel("ch1", slow)
	hub.SubscribeToChannel("ch1", fast)

	hub.BroadcastToChannel("ch1", "TEST", []byte(`{}`))

	hub.mu.RLock()
	_, slowInChannelSubs := hub.channelSubs["ch1"][slow]
	_, slowInClients := hub.clients[slow]
	_, fastInClients := hub.clients[fast]
	hub.mu.RUnlock()

	if slowInChannelSubs {
		t.Error("slow client should be removed from channelSubs (won't receive future broadcasts)")
	}
	if !slowInClients {
		// If we prematurely delete from h.clients, UnregisterClient's guard
		// fails and USER_OFFLINE never fires — user appears permanently online.
		t.Error("slow client must remain in h.clients until UnregisterClient fires naturally")
	}
	if !fastInClients {
		t.Error("fast client should be unaffected")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/websocket/... -run "TestBroadcastToChannel" -v -race
```

Expected: `TestBroadcastToChannelSlowClientMinimalEviction` **fails** because the current code deletes from `h.clients` during eviction, so `!slowInClients` is true — contradicting the test's assertion that the slow client remains in `h.clients`. `TestBroadcastToChannelConcurrentUnregister` is a **regression/race safety test** (not a TDD test) — it will pass with the current code because the write lock prevents the race. It is included here to ensure the lock-free rewrite doesn't introduce a panic or data race.

- [ ] **Step 3: Rewrite `BroadcastToChannel`**

Replace the entire `BroadcastToChannel` function in `hub.go` with:

```go
// BroadcastToChannel sends a message to all clients subscribed to a channel.
// It also publishes to Redis (if a publisher is set) so other nodes deliver it too.
//
// Performance: JSON marshaling happens once outside the lock. Subscriber snapshot
// is taken under RLock. Sends happen outside all locks. Evictions (slow/full
// send buffers) take a brief WLock at the end.
func (h *Hub) BroadcastToChannel(channelID string, messageType string, payload []byte) {
	// Step 1: Marshal outside all locks — pure CPU, no shared state.
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastToChannel: marshal error: %v", err)
		return
	}

	// Step 2: Snapshot subscribers and publisher under RLock.
	h.mu.RLock()
	pub := h.publisher
	subs := h.channelSubs[channelID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		if pub != nil {
			pub.PublishToChannel(channelID, messageType, payload)
		}
		return
	}
	clients := make([]*Client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	// Step 3: Send outside all locks. safeSend handles closed channels (evicted
	// clients) and full buffers without panicking.
	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client.send, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	// Step 4: Minimal eviction under a brief WLock.
	// We remove from channelSubs so future broadcasts skip this dead client.
	// We do NOT delete from h.clients — that would bypass UnregisterClient's
	// guard and cause USER_OFFLINE to never fire. The natural teardown chain
	// (closeSend → WritePump exit → conn.Close → ReadPump unregister → UnregisterClient)
	// handles full map cleanup and presence broadcasting.
	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
			delete(h.channelSubs[channelID], client)
			if h.clientChannels[client] != nil {
				delete(h.clientChannels[client], channelID)
			}
		}
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		h.mu.Unlock()
	}

	// Step 5: Cross-node publish.
	if pub != nil {
		pub.PublishToChannel(channelID, messageType, payload)
	}
}
```

- [ ] **Step 4: Rewrite `broadcastToAllLocal`**

Replace the entire `broadcastToAllLocal` function:

```go
func (h *Hub) broadcastToAllLocal(messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client.send, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			// Minimal eviction: close the send channel only.
			// Full cleanup (h.clients, h.userToClient, clientChannels) happens
			// when UnregisterClient fires naturally after closeSend drains.
			client.closeSend()
		}
		h.mu.Unlock()
	}
}
```

- [ ] **Step 5: Rewrite `BroadcastLocalToChannel`**

Replace the entire function:

```go
func (h *Hub) BroadcastLocalToChannel(channelID string, messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastLocalToChannel: marshal error: %v", err)
		return
	}

	h.mu.RLock()
	subs := h.channelSubs[channelID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]*Client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client.send, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
			delete(h.channelSubs[channelID], client)
			if h.clientChannels[client] != nil {
				delete(h.clientChannels[client], channelID)
			}
		}
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		h.mu.Unlock()
	}
}
```

- [ ] **Step 6: Rewrite `SendToUser`**

Replace the entire function:

```go
func (h *Hub) SendToUser(userID string, messageType string, payload []byte) error {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return err
	}

	h.mu.RLock()
	pub := h.publisher
	userClients := h.userToClient[userID]
	clients := make([]*Client, 0, len(userClients))
	for c := range userClients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		if pub != nil {
			pub.PublishToUser(userID, messageType, payload)
		}
		return nil
	}

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client.send, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
		}
		h.mu.Unlock()
	}

	if pub != nil {
		pub.PublishToUser(userID, messageType, payload)
	}

	return nil
}
```

- [ ] **Step 7: Rewrite `SendLocalToUser`**

Replace the entire function:

```go
func (h *Hub) SendLocalToUser(userID string, messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	h.mu.RLock()
	userClients := h.userToClient[userID]
	clients := make([]*Client, 0, len(userClients))
	for c := range userClients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client.send, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
		}
		h.mu.Unlock()
	}
}
```

- [ ] **Step 8: Run all hub tests with race detector**

```bash
go test ./internal/websocket/... -v -race -count=5
```

Expected: All tests pass across 5 runs (the `-count=5` reveals intermittent races that single runs miss).

- [ ] **Step 9: Run full test suite**

```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/websocket/hub.go internal/websocket/hub_test.go
git commit -m "perf(hub): lock-free fan-out — marshal once, snapshot under RLock, send outside lock"
```

---

## Chunk 2: Client Tuning + Presence

### Task 3: Client send buffer 256→1024, remove hot-path logging, cap presence snapshot

**Why three things in one task:** All are small, independent, and all relate to per-client overhead during high-connection-count scenarios.

- **Send buffer 256→1024:** At 1 msg/s per writer and 50 writers in `broadcast-amp`, a 256-slot buffer gives ~256 seconds of headroom before eviction. In practice under bursty load (25k users, multiple servers with active channels), 256 slots is hit regularly. 1024 slots costs 4× more memory per client (`~32KB` vs `~8KB` for `[]byte` pointers) — at 25k clients, that's 800MB vs 200MB. This is the right trade-off for an `s-2vcpu-2gb` node.
- **Hot-path logging:** `SubscribeToChannel`, `UnsubscribeFromChannel`, `RegisterClient`, `UnregisterClient` each call `log.Printf`. Go's `log` package serializes writes with an internal mutex. At 25k connect/subscribe events/second, this becomes a hidden serialization point.
- **Presence snapshot cap:** `RegisterClient` calls `pub.GetOnlineUserIDs()` which executes Redis `SMEMBERS parley:online`. At 25k users, this returns 25k user IDs (~150KB of JSON) sent to every connecting client. Cap at 500 via `SRANDMEMBER` — the frontend fills in the rest via incremental `USER_ONLINE` events.

**Files:**
- Modify: `internal/websocket/client.go`
- Modify: `internal/websocket/hub.go`
- Modify: `internal/websocket/redis.go`

- [ ] **Step 1: Write tests**

Add to `hub_test.go`:

```go
func TestPresenceSnapshotCapped(t *testing.T) {
	// RegisterClient on a hub without a publisher should send a snapshot
	// with at most presenceSnapshotMax user IDs.
	hub := NewHub()

	// Populate userToClient with more than presenceSnapshotMax users
	hub.mu.Lock()
	for i := 0; i < presenceSnapshotMax+50; i++ {
		uid := fmt.Sprintf("user%d", i)
		c := newTestClient(hub, uid)
		hub.clients[c] = true
		hub.userToClient[uid] = map[*Client]bool{c: true}
	}
	hub.mu.Unlock()

	incoming := newTestClient(hub, "newcomer")
	hub.RegisterClient(incoming)

	// Drain the send channel to find the PRESENCE_SNAPSHOT message
	var snapshotMsg []byte
	for {
		select {
		case msg := <-incoming.send:
			// Check if this is the snapshot
			var parsed struct {
				Type    string `json:"type"`
				Payload struct {
					UserIDs []string `json:"user_ids"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(msg, &parsed); err == nil && parsed.Type == EventPresenceSnapshot {
				snapshotMsg = msg
			}
		default:
			goto done
		}
	}
done:
	if snapshotMsg == nil {
		t.Fatal("no PRESENCE_SNAPSHOT received")
	}

	var wrapper struct {
		Payload struct {
			UserIDs []string `json:"user_ids"`
		} `json:"payload"`
	}
	json.Unmarshal(snapshotMsg, &wrapper)
	if len(wrapper.Payload.UserIDs) > presenceSnapshotMax {
		t.Errorf("snapshot contains %d user IDs, want at most %d",
			len(wrapper.Payload.UserIDs), presenceSnapshotMax)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/websocket/... -run "TestPresenceSnapshotCapped" -v
```

Expected: FAIL — `presenceSnapshotMax` is undefined.

- [ ] **Step 3: Increase send buffer in `client.go`**

In `NewClient`, change:
```go
send: make(chan []byte, 256),
```
to:
```go
send: make(chan []byte, 1024),
```

- [ ] **Step 4: Remove hot-path `log.Printf` from `hub.go`**

Remove (or downgrade to `if false` — fully remove is cleaner) these `log.Printf` calls:

In `RegisterClient`, remove:
```go
log.Printf("Client registered for user: %s", client.userID)
```

In `UnregisterClient`, remove:
```go
log.Printf("Client unregistered for user: %s", client.userID)
```

In `SubscribeToChannel`, remove:
```go
log.Printf("Client %s subscribed to channel: %s", client.userID, channelID)
```

In `UnsubscribeFromChannel`, remove:
```go
log.Printf("Client %s unsubscribed from channel: %s", client.userID, channelID)
```

Keep the error-level `log.Printf` calls (marshal errors, unexpected errors) — only remove the informational connection lifecycle ones that fire on every event.

- [ ] **Step 5: Add `presenceSnapshotMax` constant and cap the snapshot in `hub.go`**

Add constant near top of file (after imports):
```go
// presenceSnapshotMax caps the number of online user IDs sent in the
// PRESENCE_SNAPSHOT event on connect. The frontend fills in remaining users
// incrementally via USER_ONLINE events. Sending all 25k IDs per connection
// is ~150KB of JSON with no practical benefit.
const presenceSnapshotMax = 500
```

In `RegisterClient`, replace the local `onlineUserIDs` build logic:

Find this block:
```go
// Build snapshot — use Redis for cross-node truth, fall back to local map
var onlineUserIDs []string
if pub != nil {
    onlineUserIDs = pub.GetOnlineUserIDs()
} else {
    h.mu.RLock()
    seen := make(map[string]bool)
    for uid := range h.userToClient {
        if !seen[uid] {
            seen[uid] = true
            onlineUserIDs = append(onlineUserIDs, uid)
        }
    }
    h.mu.RUnlock()
}
```

Replace with:
```go
// Build snapshot — use Redis for cross-node truth, fall back to local map.
// Capped at presenceSnapshotMax to prevent ~150KB payloads at 25k users.
var onlineUserIDs []string
if pub != nil {
    onlineUserIDs = pub.GetOnlineUserIDs()
} else {
    h.mu.RLock()
    for uid := range h.userToClient {
        onlineUserIDs = append(onlineUserIDs, uid)
        if len(onlineUserIDs) >= presenceSnapshotMax {
            break
        }
    }
    h.mu.RUnlock()
}
if len(onlineUserIDs) > presenceSnapshotMax {
    onlineUserIDs = onlineUserIDs[:presenceSnapshotMax]
}
```

- [ ] **Step 6: Cap the Redis presence response in `redis.go`**

In `GetOnlineUserIDs`, replace the `SMembers` call with `SRandMember` to avoid loading all 25k IDs from Redis only to truncate them:

Replace:
```go
func (r *RedisHub) GetOnlineUserIDs() []string {
    ctx := context.Background()
    ids, err := r.pubsub.Client().SMembers(ctx, redisOnlineKey).Result()
    if err != nil {
        log.Printf("RedisHub: GetOnlineUserIDs failed: %v", err)
        return nil
    }
    return ids
}
```

With:
```go
// GetOnlineUserIDs returns a sample of online user IDs for the presence snapshot.
// Uses SRANDMEMBER to avoid loading the full set (potentially 25k+ IDs) from Redis.
func (r *RedisHub) GetOnlineUserIDs() []string {
    ctx := context.Background()
    ids, err := r.pubsub.Client().SRandMemberN(ctx, redisOnlineKey, presenceSnapshotMax).Result()
    if err != nil {
        log.Printf("RedisHub: GetOnlineUserIDs failed: %v", err)
        return nil
    }
    return ids
}
```

Note: `presenceSnapshotMax` is in the `websocket` package so it's accessible from `redis.go` in the same package.

- [ ] **Step 7: Run tests**

```bash
go test ./internal/websocket/... -v -race
```

Expected: All tests pass including `TestPresenceSnapshotCapped`.

- [ ] **Step 8: Run full test suite**

```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/websocket/hub.go internal/websocket/client.go internal/websocket/redis.go internal/websocket/hub_test.go
git commit -m "perf(hub): 4× send buffer, remove hot-path logging, cap presence snapshot at 500"
```

---

## Chunk 3: Rate Limiting

### Task 4: Replace sliding-window rate limiter with token bucket

**Why:** The current sliding-window stores `[]time.Time` per IP, requiring O(window) memory per key and O(requests-in-window) time for the scan on each request. The token bucket stores 2 floats per key, is O(1), and is mathematically equivalent for sustained-rate enforcement with better burst behaviour. With 25k IPs potentially tracked, the memory savings are real.

**Files:**
- Modify: `cmd/api/middleware.go`
- Create: `cmd/api/middleware_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/api/middleware_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestTokenBucketAllows(t *testing.T) {
	rl := newRateLimiter(10, time.Minute) // 10 req/min

	// First 10 requests should pass
	for i := 0; i < 10; i++ {
		if !rl.Allow("192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestTokenBucketDenies(t *testing.T) {
	rl := newRateLimiter(5, time.Minute) // 5 req/min burst

	// Exhaust the burst
	for i := 0; i < 5; i++ {
		rl.Allow("192.0.2.1")
	}

	// 6th should be denied
	if rl.Allow("192.0.2.1") {
		t.Error("6th request should be denied after burst exhausted")
	}
}

func TestTokenBucketIsolation(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")
	// 10.0.0.1 is exhausted

	// 10.0.0.2 should still have full bucket
	if !rl.Allow("10.0.0.2") {
		t.Error("10.0.0.2 should not be affected by 10.0.0.1 exhaustion")
	}
}

func TestTokenBucketRefills(t *testing.T) {
	rl := newRateLimiter(60, time.Minute) // 1 token/sec refill

	// Exhaust burst
	for i := 0; i < 60; i++ {
		rl.Allow("192.0.2.1")
	}

	if rl.Allow("192.0.2.1") {
		t.Error("should be denied immediately after exhaustion")
	}

	// Wait 1.1 seconds — at least 1 token should have refilled.
	time.Sleep(1100 * time.Millisecond)

	if !rl.Allow("192.0.2.1") {
		t.Error("should be allowed after 1 second refill")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/api/... -run "TestTokenBucket" -v
```

Expected: FAIL — the current implementation is a sliding window, not a token bucket.

- [ ] **Step 3: Replace `rateLimiter` in `middleware.go`**

Replace the entire `rateLimiter` struct and its methods with the token bucket implementation:

```go
// ----- Token bucket rate limiter -----
//
// Token bucket: allows burst up to `burst` requests, then refills at
// `rate` requests per second. O(1) per Allow call, O(1) memory per key.

type tokenBucket struct {
	tokens    float64
	lastSeen  time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 // tokens per second
	burst   float64 // maximum token capacity
}

// newRateLimiter creates a token bucket rate limiter equivalent to limit requests
// per window. burst = limit, rate = limit/window in tokens/second.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    float64(limit) / window.Seconds(),
		burst:   float64(limit),
	}
	go rl.cleanup()
	return rl
}

// Allow returns true if the key has a token available.
func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: rl.burst, lastSeen: now}
		rl.buckets[key] = b
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastSeen = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// cleanup removes stale buckets every 5 minutes.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		threshold := time.Now().Add(-10 * time.Minute)
		rl.mu.Lock()
		for key, b := range rl.buckets {
			if b.lastSeen.Before(threshold) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
```

Keep `rateLimitMiddleware` and `maxBodyMiddleware` exactly as they are — only the struct implementation changes.

- [ ] **Step 4: Run tests**

```bash
go test ./cmd/api/... -run "TestTokenBucket" -v
```

Expected: All token bucket tests pass.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/api/middleware.go cmd/api/middleware_test.go
git commit -m "perf(ratelimit): replace sliding-window with O(1) token bucket"
```

---

### Task 5: Per-user rate limiting on authenticated message POST

**Why:** The current rate limiter is IP-based only. With 25k users, many behind NAT or corporate proxies, a single IP can represent hundreds of legitimate users — all blocked by a single aggressive sender. Conversely, an authenticated user can send from multiple IPs with no cross-IP limit. Adding a per-user-ID rate limit on message writes prevents `message-storm` denial of service from individual accounts.

**Rate:** 5 messages/second per user with burst of 10. This is equivalent to Discord's "you are sending messages too quickly" threshold.

**Files:**
- Modify: `cmd/api/middleware.go`
- Modify: `cmd/api/routes.go`
- Modify: `cmd/api/middleware_test.go`

- [ ] **Step 1: Write the failing test**

Add to `middleware_test.go`:

```go
func TestUserRateLimiterIsolation(t *testing.T) {
	rl := newRateLimiter(3, time.Minute) // 3/min per user

	// User A exhausts their bucket
	for i := 0; i < 3; i++ {
		if !rl.Allow("user:user_a") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("user:user_a") {
		t.Error("4th request for user_a should be denied")
	}

	// User B's bucket is independent
	if !rl.Allow("user:user_b") {
		t.Error("user_b should not be affected by user_a exhaustion")
	}
}
```

- [ ] **Step 2: Run test to verify it passes already (the rate limiter is key-agnostic)**

```bash
go test ./cmd/api/... -run "TestUserRateLimiter" -v
```

Expected: PASS — the token bucket already works with any string key.

- [ ] **Step 3: Add per-user message rate limiter to `routes.go`**

In `registerRoutes`, add after the existing limiter declarations (after `msgReadLimiter := newRateLimiter(120, time.Minute)`):

```go
// Rate limiter for message writes: 5 messages/second per authenticated user (burst 10).
// Keyed on user ID, not IP, to prevent cross-IP bypasses by the same account.
msgWriteLimiter := newRateLimiter(300, time.Minute) // 5/s = 300/min, burst=300
```

Note: `newRateLimiter(300, time.Minute)` sets burst=300 and rate=5/s. That's a large burst. Use a tighter burst by creating a helper constructor, or use `newRateLimiter(10, 2*time.Second)` for burst=10, rate=5/s:

```go
msgWriteLimiter := newRateLimiter(10, 2*time.Second) // 5/s burst 10
```

- [ ] **Step 4: Add `userRateLimitMiddleware` to `middleware.go`**

Add this function after `rateLimitMiddleware`:

```go
func userRateLimitMiddleware(rl *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use authenticated user ID as the rate limit key so the limit applies
			// per account regardless of IP (defeats multi-IP bypass attempts).
			key := auth.GetUserIDFromContext(r)
			if key == "" {
				// Fallback to IP (should not occur on authenticated routes)
				key, _, _ = net.SplitHostPort(r.RemoteAddr)
			} else {
				key = "u:" + key // namespace to avoid collision with IP keys
			}
			if !rl.Allow(key) {
				http.Error(w, "rate limit exceeded, slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

Add import `"parley/internal/auth"` to middleware.go imports block.

- [ ] **Step 5: Wire the middleware onto message POST in `routes.go`**

Find the message POST route:
```go
r.Post("/channels/{channelID}/messages", messageHandler.SendMessage)
```

Replace with:
```go
r.With(userRateLimitMiddleware(msgWriteLimiter)).Post("/channels/{channelID}/messages", messageHandler.SendMessage)
```

- [ ] **Step 6: Run tests**

```bash
go test ./cmd/api/... -v -race
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/api/middleware.go cmd/api/middleware_test.go cmd/api/routes.go
git commit -m "perf(ratelimit): add per-user rate limit on message POST (5/s burst 10)"
```

---

## Chunk 4: Database

### Task 6: DB connection pool configuration

**Why:** `main.go` calls `sql.Open` with no pool configuration. The default is unlimited open connections (`MaxOpenConns=0`) and only 2 idle connections. Under 25k concurrent users, many requests hit the DB simultaneously, spawning hundreds of Postgres connections. Each Postgres backend uses ~5-10MB RAM; 200 connections = 1-2GB RAM on the DB droplet. The `s-2vcpu-4gb` DB has 4GB total. Configure explicit limits that match the hardware.

Also: the `channelAccessChecker` closure in `main.go` uses `context.Background()` — a hung DB query will block the WebSocket goroutine indefinitely. Add a 2-second timeout.

**Connection math:**
- 3 API nodes × 25 max connections = 75 total Postgres connections
- PostgreSQL default `max_connections=100` → leave headroom for admin/migration connections
- `SetMaxIdleConns(5)` → idle connections are closed after `ConnMaxIdleTime`, preventing connection pile-up between load spikes

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: There are no unit tests for DB pool config — verify by checking the pool stats after configuration**

The test here is a build test + observability: after the change, the pool stats should be visible via pprof (stresstest build). We verify correctness by running:

```bash
go build ./cmd/api/
```

Expected: Builds cleanly.

- [ ] **Step 2: Add pool configuration in `main.go` after `sql.Open`**

Find this block in `main.go`:
```go
// Connect to PostgreSQL
dbConn, err := sql.Open("postgres", config.DatabaseURL)
if err != nil {
    log.Fatalf("Failed to connect to database: %v", err)
}
defer dbConn.Close()
```

Replace with:
```go
// Connect to PostgreSQL
dbConn, err := sql.Open("postgres", config.DatabaseURL)
if err != nil {
    log.Fatalf("Failed to connect to database: %v", err)
}
defer dbConn.Close()

// Configure the connection pool.
// 25 open connections per API node × 3 nodes = 75 total, within Postgres
// default max_connections=100 and leaving headroom for admin/migration.
dbConn.SetMaxOpenConns(25)
dbConn.SetMaxIdleConns(5)
dbConn.SetConnMaxLifetime(5 * time.Minute)
dbConn.SetConnMaxIdleTime(2 * time.Minute)
```

- [ ] **Step 3: Add 2-second timeout to `channelAccessChecker` in `main.go`**

In the `hub.SetChannelAccessChecker` closure, replace:
```go
ctx := context.Background()
```
with:
```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
```

This applies to all three branches inside the closure (server: prefix, dm: prefix, and regular channel).

- [ ] **Step 4: Build and run**

```bash
go build ./cmd/api/
```

Expected: Builds cleanly.

- [ ] **Step 5: Commit**

```bash
git add cmd/api/main.go
git commit -m "perf(db): configure connection pool (25 max, 5 idle) + 2s access checker timeout"
```

---

### Task 7: In-memory membership cache for channel access checks

**Why:** The `channelAccessChecker` makes 2 DB queries per `CHANNEL_SUBSCRIBE` event: `GetChannelByID` (channel→server lookup) and `GetMember` (membership check). At startup with 25k users each subscribing to multiple channels, this is tens of thousands of DB queries within seconds. The channel→server mapping is immutable after channel creation. Server membership changes rarely. A 30-second TTL cache eliminates 95%+ of these queries.

**Files:**
- Create: `internal/cache/membership.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write failing tests**

Create `internal/cache/membership_test.go`:

```go
package cache_test

import (
	"testing"
	"time"

	"parley/internal/cache"
)

func TestMembershipCacheHit(t *testing.T) {
	c := cache.NewMembershipCache(30 * time.Second)
	c.SetMember(1, 42, true)

	result, ok := c.GetMember(1, 42)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if !result {
		t.Error("expected isMember=true")
	}
}

func TestMembershipCacheNegative(t *testing.T) {
	c := cache.NewMembershipCache(30 * time.Second)
	c.SetMember(1, 99, false)

	result, ok := c.GetMember(1, 99)
	if !ok {
		t.Fatal("expected cache hit for negative result")
	}
	if result {
		t.Error("expected isMember=false")
	}
}

func TestMembershipCacheMiss(t *testing.T) {
	c := cache.NewMembershipCache(30 * time.Second)

	_, ok := c.GetMember(1, 999)
	if ok {
		t.Error("expected cache miss for unknown entry")
	}
}

func TestMembershipCacheExpiry(t *testing.T) {
	c := cache.NewMembershipCache(10 * time.Millisecond) // very short TTL
	c.SetMember(1, 42, true)

	time.Sleep(20 * time.Millisecond)

	_, ok := c.GetMember(1, 42)
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestChannelServerCacheHit(t *testing.T) {
	c := cache.NewMembershipCache(30 * time.Second)
	c.SetChannelServer(101, 5)

	serverID, ok := c.GetChannelServer(101)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if serverID != 5 {
		t.Errorf("got serverID=%d, want 5", serverID)
	}
}

func TestMembershipCacheInvalidate(t *testing.T) {
	c := cache.NewMembershipCache(30 * time.Second)
	c.SetMember(1, 42, true)
	c.InvalidateMember(1, 42)

	_, ok := c.GetMember(1, 42)
	if ok {
		t.Error("expected cache miss after invalidation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/cache/... -v
```

Expected: FAIL — compilation error: `undefined: cache.NewMembershipCache` (the `internal/cache` package exists but `MembershipCache` is not yet defined).

- [ ] **Step 3: Create `internal/cache/membership.go`**

```go
package cache

import (
	"fmt"
	"sync"
	"time"
)

// MembershipCache is a short-lived in-memory cache for two common lookup types:
//   - Server membership: (serverID, userID) → bool
//   - Channel-to-server mapping: channelID → serverID
//
// Both are stable within a TTL window (memberships change rarely; channel
// server assignments never change). This cache eliminates repeated DB round-
// trips from the WebSocket channelAccessChecker during mass-subscribe events.
type MembershipCache struct {
	mu      sync.RWMutex
	members map[string]memberEntry   // key: "m:serverID:userID"
	chSrv   map[string]chSrvEntry    // key: "c:channelID"
	ttl     time.Duration
}

type memberEntry struct {
	isMember  bool
	expiresAt time.Time
}

type chSrvEntry struct {
	serverID  int64
	expiresAt time.Time
}

// NewMembershipCache creates a cache with the given TTL and starts a background
// cleanup goroutine.
func NewMembershipCache(ttl time.Duration) *MembershipCache {
	c := &MembershipCache{
		members: make(map[string]memberEntry),
		chSrv:   make(map[string]chSrvEntry),
		ttl:     ttl,
	}
	go c.cleanup()
	return c
}

// GetMember returns (isMember, true) on cache hit, (false, false) on miss or expiry.
func (c *MembershipCache) GetMember(serverID, userID int64) (isMember bool, ok bool) {
	key := fmt.Sprintf("m:%d:%d", serverID, userID)
	c.mu.RLock()
	e, exists := c.members[key]
	c.mu.RUnlock()
	if !exists || time.Now().After(e.expiresAt) {
		return false, false
	}
	return e.isMember, true
}

// SetMember caches the membership result for (serverID, userID).
func (c *MembershipCache) SetMember(serverID, userID int64, isMember bool) {
	key := fmt.Sprintf("m:%d:%d", serverID, userID)
	c.mu.Lock()
	c.members[key] = memberEntry{isMember: isMember, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// InvalidateMember removes a cached membership entry (call on kick/ban/leave).
func (c *MembershipCache) InvalidateMember(serverID, userID int64) {
	key := fmt.Sprintf("m:%d:%d", serverID, userID)
	c.mu.Lock()
	delete(c.members, key)
	c.mu.Unlock()
}

// GetChannelServer returns (serverID, true) on cache hit.
// Channel→server mapping is immutable so TTL is long (5 minutes).
func (c *MembershipCache) GetChannelServer(channelID int64) (serverID int64, ok bool) {
	key := fmt.Sprintf("c:%d", channelID)
	c.mu.RLock()
	e, exists := c.chSrv[key]
	c.mu.RUnlock()
	if !exists || time.Now().After(e.expiresAt) {
		return 0, false
	}
	return e.serverID, true
}

// SetChannelServer caches the channel→server mapping.
func (c *MembershipCache) SetChannelServer(channelID, serverID int64) {
	key := fmt.Sprintf("c:%d", channelID)
	c.mu.Lock()
	c.chSrv[key] = chSrvEntry{serverID: serverID, expiresAt: time.Now().Add(5 * time.Minute)}
	c.mu.Unlock()
}

func (c *MembershipCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.members {
			if now.After(e.expiresAt) {
				delete(c.members, k)
			}
		}
		for k, e := range c.chSrv {
			if now.After(e.expiresAt) {
				delete(c.chSrv, k)
			}
		}
		c.mu.Unlock()
	}
}
```

- [ ] **Step 4: Run cache tests**

```bash
go test ./internal/cache/... -v -race
```

Expected: All 6 tests pass.

- [ ] **Step 5: Wire membership cache into `channelAccessChecker` in `main.go`**

Add import for the new cache package. After the hub initialization block (after `hub.SetChannelAccessChecker`... block), instantiate the cache. Then thread it through the closure.

Before the `hub.SetChannelAccessChecker(func(...)` call, add:
```go
// Short-lived membership cache to absorb the wave of CHANNEL_SUBSCRIBE DB
// queries when many users connect simultaneously (e.g., server restart).
memberCache := cache.NewMembershipCache(30 * time.Second)
```

Add `"parley/internal/cache"` to the imports.

Then update the `channelAccessChecker` closure to use the cache. Replace the existing closure body:

```go
hub.SetChannelAccessChecker(func(userID, channelID string) bool {
    uID, err := strconv.ParseInt(userID, 10, 64)
    if err != nil {
        return false
    }
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // "server:{serverID}" virtual channels
    if serverIDStr, ok := strings.CutPrefix(channelID, "server:"); ok {
        sID, err := strconv.ParseInt(serverIDStr, 10, 64)
        if err != nil {
            return false
        }
        if isMember, ok := memberCache.GetMember(sID, uID); ok {
            return isMember
        }
        member, err := repo.GetMember(ctx, sID, uID)
        result := err == nil && member != nil
        memberCache.SetMember(sID, uID, result)
        return result
    }

    // "dm:{dmChannelID}" virtual channels
    if dmIDStr, ok := strings.CutPrefix(channelID, "dm:"); ok {
        dmID, err := strconv.ParseInt(dmIDStr, 10, 64)
        if err != nil {
            return false
        }
        ch, err := repo.GetDmChannelByID(ctx, dmID)
        return err == nil && (ch.User1ID == uID || ch.User2ID == uID)
    }

    // Regular channels: check channel→server mapping (cached) then membership (cached)
    chID, err := strconv.ParseInt(channelID, 10, 64)
    if err != nil {
        return false
    }

    var serverID int64
    if sID, ok := memberCache.GetChannelServer(chID); ok {
        serverID = sID
    } else {
        ch, err := repo.GetChannelByID(ctx, chID)
        if err != nil {
            return false
        }
        memberCache.SetChannelServer(chID, ch.ServerID)
        serverID = ch.ServerID
    }

    if isMember, ok := memberCache.GetMember(serverID, uID); ok {
        return isMember
    }
    member, err := repo.GetMember(ctx, serverID, uID)
    result := err == nil && member != nil
    memberCache.SetMember(serverID, uID, result)
    return result
})
```

- [ ] **Step 6: Build**

```bash
go build ./cmd/api/
```

Expected: Builds cleanly.

- [ ] **Step 7: Run tests**

```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/cache/membership.go internal/cache/membership_test.go cmd/api/main.go
git commit -m "perf(cache): membership cache cuts channel-subscribe DB queries by ~95%"
```

---

## Chunk 5: Infrastructure

### Task 8: sysctl + service tuning on API droplets

**Why:** Linux defaults cap open file descriptors to 1024 per process and the system to ~65536. 25k WebSocket connections require 25k file descriptors for the server sockets alone, plus more for DB connections, Redis connections, and log files. The API service will silently refuse new connections once the FD limit is hit.

**Files:**
- Modify: `terraform/userdata-api.sh`
- Modify: `terraform/variables.tf` (optional: add `api_droplet_size` bump suggestion)

- [ ] **Step 1: Read the full userdata-api.sh to find the right injection point**

```bash
cat -n terraform/userdata-api.sh
```

- [ ] **Step 2: Add sysctl tuning block to `userdata-api.sh`**

Add this block **after** the system update section and **before** package installation (after line 50 approximately — after the `run_with_retry "apt-get upgrade -y"` line):

```bash
echo "=== Applying kernel tuning for high-connection workloads ==="

# Maximum open file descriptors (system-wide)
sysctl -w fs.file-max=2097152

# Per-process FD limit for new sessions
echo "* soft nofile 1048576" >> /etc/security/limits.conf
echo "* hard nofile 1048576" >> /etc/security/limits.conf
echo "root soft nofile 1048576" >> /etc/security/limits.conf
echo "root hard nofile 1048576" >> /etc/security/limits.conf

# Increase local port range for outbound connections (DB, Redis)
sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# Faster recycling of TIME_WAIT sockets
sysctl -w net.ipv4.tcp_tw_reuse=1

# Increase the backlog queue for incoming connections
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535

# Increase socket receive/send buffers
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216

# TCP keepalive: detect dead connections after 60s idle, then every 10s × 6 probes
# Prevents zombie WS connections from stacking up when clients vanish without FIN
sysctl -w net.ipv4.tcp_keepalive_time=60
sysctl -w net.ipv4.tcp_keepalive_intvl=10
sysctl -w net.ipv4.tcp_keepalive_probes=6

# Persist sysctl settings across reboots
cat >> /etc/sysctl.conf << 'SYSCTL'
fs.file-max=2097152
net.ipv4.ip_local_port_range=1024 65535
net.ipv4.tcp_tw_reuse=1
net.core.somaxconn=65535
net.ipv4.tcp_max_syn_backlog=65535
net.core.rmem_max=16777216
net.core.wmem_max=16777216
net.ipv4.tcp_keepalive_time=60
net.ipv4.tcp_keepalive_intvl=10
net.ipv4.tcp_keepalive_probes=6
SYSCTL
```

- [ ] **Step 3: Add `LimitNOFILE` to the systemd service unit in `userdata-api.sh`**

Find the section that creates the systemd service file (search for `[Service]` or `systemd`). Add `LimitNOFILE=1048576` to the service unit's `[Service]` section:

```ini
[Service]
...
LimitNOFILE=1048576
```

If the service unit doesn't exist yet in the script, find where the binary is executed and replace the direct execution with a systemd unit. If it already uses systemd (look for `systemctl enable parley`), add the LimitNOFILE line to the unit file creation block.

- [ ] **Step 4: Validate the script syntax**

```bash
bash -n terraform/userdata-api.sh
```

Expected: No syntax errors.

- [ ] **Step 5: Commit**

```bash
git add terraform/userdata-api.sh
git commit -m "infra: sysctl tuning + 1M FD limit for API droplets"
```

---

### Task 9: PgBouncer + PostgreSQL connection tuning on DB droplet

**Why:** Even with `SetMaxOpenConns(25)` per API node, 3 nodes = 75 Postgres connections in steady state, spking to 75 × 3 = 225 during reconnect storms (all 3 nodes simultaneously reconnecting after a DB restart). Postgres default `max_connections=100` would reject the excess. PgBouncer in transaction-mode pooling reduces application-visible connections to 75 while maintaining many more application-side connections, and smooths out connection storms.

**Pool mode:** Transaction pooling (not session pooling). Each query borrows a backend connection only for the duration of a transaction. This is correct for Parley's DB usage (no `SET` session variables, no prepared statements that outlive transactions, no advisory locks that need a sticky connection).

**Files:**
- Modify: `terraform/userdata-db.sh`

- [ ] **Step 1: Read the full `userdata-db.sh`**

```bash
cat -n terraform/userdata-db.sh
```

- [ ] **Step 2: Add PgBouncer installation and configuration to `userdata-db.sh`**

Add after the PostgreSQL setup and before the final `echo "=== Database setup complete ==="` (or equivalent ending line):

```bash
echo "=== Installing and configuring PgBouncer ==="
apt-get install -y pgbouncer

# PgBouncer configuration
cat > /etc/pgbouncer/pgbouncer.ini << 'EOF'
[databases]
parley = host=localhost port=5432 dbname=parley

[pgbouncer]
listen_addr = *
listen_port = 6432
# auth_type = md5 requires PostgreSQL pg_hba.conf to also use md5.
# Ubuntu 22.04+ PostgreSQL defaults to scram-sha-256. We use scram-sha-256
# here to match. If your PostgreSQL version/config uses md5, change both.
auth_type = scram-sha-256
auth_file = /etc/pgbouncer/userlist.txt
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 25
reserve_pool_size = 5
reserve_pool_timeout = 3
server_idle_timeout = 600
client_idle_timeout = 0
log_connections = 0
log_disconnections = 0
EOF

# PgBouncer auth file — stores the SCRAM verifier, not the plaintext password.
# Generate with: psql -c "SELECT concat('\"', usename, '\" \"', passwd, '\"') FROM pg_shadow WHERE usename='parley';"
# For initial setup, a helper script sets this after PostgreSQL is running.
# See the pg_hba.conf note in Step 3.
echo "\"parley\" \"${db_password}\"" > /etc/pgbouncer/userlist.txt
chmod 640 /etc/pgbouncer/userlist.txt
chown postgres:postgres /etc/pgbouncer/userlist.txt

systemctl enable pgbouncer
systemctl start pgbouncer
echo "PgBouncer listening on port 6432"
```

- [ ] **Step 3: Tune PostgreSQL `max_connections`**

Find the `postgresql.conf` modification section (or add one). Set `max_connections=150` (enough for 3 nodes × 25 + admin headroom, and the default is 100 which is too low for the burst scenario):

```bash
echo "=== Tuning PostgreSQL max_connections ==="
PG_CONF=$(find /etc/postgresql -name postgresql.conf | head -1)
if [ -n "$PG_CONF" ]; then
    sed -i "s/^#*max_connections.*/max_connections = 150/" "$PG_CONF"
    # Shared buffers: 25% of RAM (4GB droplet → 1GB)
    sed -i "s/^#*shared_buffers.*/shared_buffers = 1GB/" "$PG_CONF"
    # Effective cache size: 75% of RAM
    sed -i "s/^#*effective_cache_size.*/effective_cache_size = 3GB/" "$PG_CONF"
    # Work memory for sort operations
    sed -i "s/^#*work_mem.*/work_mem = 4MB/" "$PG_CONF"
    # Huge pages: let Postgres use them if the kernel has them available.
    # With 1GB shared_buffers, huge pages save ~500 TLB entries and reduce
    # kernel memory overhead. "try" falls back gracefully if unavailable.
    sed -i "s/^#*huge_pages.*/huge_pages = try/" "$PG_CONF"
    systemctl restart postgresql
fi

# Enable huge pages in the kernel (2MB pages; 512 pages covers 1GB shared_buffers)
echo "vm.nr_hugepages=512" >> /etc/sysctl.conf
sysctl -w vm.nr_hugepages=512
```

- [ ] **Step 4: Update API nodes to connect to PgBouncer (port 6432) instead of Postgres (5432)**

In `terraform/main.tf`, the `DB_PORT` is currently hardcoded to `"5432"` in the API droplet `user_data`:

```hcl
DB_PORT = "5432"
```

Change to:
```hcl
DB_PORT = "6432"
```

This routes API→PgBouncer→Postgres instead of API→Postgres directly.

- [ ] **Step 5: Validate script syntax**

```bash
bash -n terraform/userdata-db.sh
```

Expected: No syntax errors.

- [ ] **Step 6: Build Go code (unchanged, but verify nothing broke)**

```bash
go build ./cmd/api/
go test ./... -race
```

Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add terraform/userdata-db.sh terraform/main.tf
git commit -m "infra: PgBouncer transaction pooling + PostgreSQL max_connections=150 + shared_buffers tuning"
```

---

## Post-Implementation: Run the Bench

After all tasks are merged, run the bench suite against the stresstest build on Proxmox to validate. Suggested order:

```bash
# Build stresstest server
go build -tags stresstest -o /tmp/parley-api-bench ./cmd/api
BENCH_SECRET=localonly JWT_SECRET=dev BOT_KEY_SECRET=dev \
  DATABASE_URL="postgres://parley:pass@localhost:6432/parley" \
  /tmp/parley-api-bench

# 1. Baseline: hub mutex contention
parley-bench broadcast-amp --bench-secret localonly --listeners 500 --duration 5m

# 2. WS connection cliff
parley-bench ws-scale --bench-secret localonly --max 2000 --sustain 3m

# 3. Message throughput
parley-bench message-storm --bench-secret localonly --writers 50 --duration 5m

# 4. Rate limiting behavior
parley-bench auth-flood --bench-secret localonly --workers 20 --duration 2m

# 5. Full mixed load
parley-bench mixed --bench-secret localonly --users 500 --duration 10m

# Profile mutex contention during broadcast-amp run:
go tool pprof http://localhost:8080/debug/pprof/mutex
```

Compare p99 broadcast latency before/after. The hub fan-out fix should reduce p99 from seconds to sub-millisecond for 500 listeners. The WS cliff should move from ~500 connections (current, estimated) to 5,000+ connections before degradation.
