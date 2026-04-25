# DM/GC Calling Design

**Date:** 2026-04-25
**Status:** Approved for plan-writing
**Goal:** Add audio/video calling to 1:1 DMs and group DMs, reusing the existing voice-channel infrastructure (LiveKit room, presence, hub events, `useVoiceConnection`, `VoiceChannel` view) wherever possible. Add a Discord-style floating call window. Bundle a long-standing notification bug fix (don't notify on your own messages).

---

## 1. Goals & Non-Goals

**Goals**
- 1:1 DM calls with explicit ring/accept/decline lifecycle.
- Group DM calls without ring — open-room model with in-channel banner + system message.
- Reuse the existing voice-channel stack (LiveKit token, Redis presence, WS events, hooks, view components).
- Add user-on-user **per-listener volume control** (0–200%, with mute = 0) — applies to DM/GC and existing server VC alike.
- Add a draggable in-app **floating call window** (Discord-style) — applies to any active call.
- Native OS-level **incoming-call window** for the desktop app when the main window is unfocused.
- Add a connection-quality indicator on each `ParticipantTile`.
- Plumb a stub harness for **VC activities** so games / watch-together / similar can be dropped in later (no actual activities ship in this iteration).
- Fix bug: DM/GC participants should not receive notification sounds for their own messages.

**Non-Goals**
- No new database schema. All call lifecycle artifacts go in the existing `dm_messages.system_event` JSONB (introduced in Phase B).
- No call recording, transcription, or in-call chat overlay.
- No mobile build considerations (no Parley mobile yet).
- No horizontal scaling of the ring service. Single-instance backend is the existing deployment; documented as a known limitation if Parley ever scales out.
- No cross-device, cross-tab call rehydration on page reload. Current behavior (disconnect on unload) stands.
- No true OS-level secondary window for the *floating call window* (the in-call PiP). That's deferred. Only the *incoming-call window* is a real Tauri secondary window.

---

## 2. Architecture summary

**One key abstraction:** *virtual channel ID* — a prefixed string identifying any voice room.

| Form | Meaning |
|---|---|
| `s:{channelID}` | Server voice channel `channelID` |
| `dm:{dmChannelID}` | DM or GC channel (group flag determined at parse time via `dm_channels.is_group`) |

The LiveKit room name, the Redis presence key (`voice:{vc}`), and the WS broadcast topic all use this string verbatim. One parser, one auth helper, one set of routes — server VC becomes a special case of "voice in any virtual channel."

**Ringing is explicit state**, not implicit on LiveKit join. The backend ring service owns the timer, emits the timeout authoritatively, and emits the missed/declined system message — necessary because "Alice rage-quit her own call" and "Bob never picked up" must be distinguishable, and the only authoritative source is the server.

**Call artifacts** live in `dm_messages.system_event` (existing JSONB column). The chat timeline doubles as the call log. No new tables, no new migrations.

**Per-listener volume** is purely client-side: `localStorage.parley.localVolumes` is a `Map<userID, 0–200>` (where 0 == muted, 100 == default unity gain, up to 200% boost). `ParticipantTile` reads the value, applies via `participant.setVolume(value / 100)`, and renders a strikethrough-speaker icon when `value === 0`. Persists across all calls in any context.

**VC Activities (stub)**: a registry pattern — `register({type, render: Component})` — and a Redis-tracked active-activity-per-virtual-channel state. The harness ships in this iteration with an empty registry; future activities (e.g., watch-together) plug in by calling `register(...)` and don't require backend changes.

**Floating call window** is a `position: fixed` draggable React overlay rendering `<VoiceChannel layout="compact">`. Position persists to localStorage. Toggleable from the existing `VoiceControls` widget.

**Incoming-call window** (desktop only): a real Tauri secondary window — borderless, transparent, always-on-top, skip-taskbar, anchored bottom-right. Spawned only when the main window is unfocused at ring time. If main gains focus while a ring is active, the secondary closes and the in-app `IncomingCallModal` takes over. Web build always uses the in-app modal. **Mobile Tauri builds (iOS/Android) also use the in-app modal**, never the secondary window — multi-window doesn't apply to mobile webviews. Detection uses `@tauri-apps/plugin-os` `platform()`; the Rust commands are gated with `#[cfg(not(any(target_os = "ios", target_os = "android")))]`.

---

## 3. Backend

### 3.1 Virtual channel namespace

New file `internal/voice/virtual_channel.go`:

```go
package voice

import (
    "errors"
    "strconv"
    "strings"
)

type Kind int
const (
    KindServer Kind = iota
    KindDM
)

type VirtualChannel struct {
    Kind Kind
    ID   int64 // server channel ID for KindServer; dm_channel ID for KindDM
}

func (v VirtualChannel) String() string {
    switch v.Kind {
    case KindServer: return "s:" + strconv.FormatInt(v.ID, 10)
    case KindDM:     return "dm:" + strconv.FormatInt(v.ID, 10)
    }
    return ""
}

func ParseVirtualChannel(s string) (VirtualChannel, error) {
    var prefix string
    var rest string
    if r, ok := strings.CutPrefix(s, "s:"); ok {
        prefix, rest = "s", r
    } else if r, ok := strings.CutPrefix(s, "dm:"); ok {
        prefix, rest = "dm", r
    } else {
        return VirtualChannel{}, errors.New("invalid virtual channel id")
    }
    id, err := strconv.ParseInt(rest, 10, 64)
    if err != nil { return VirtualChannel{}, err }
    switch prefix {
    case "s":  return VirtualChannel{Kind: KindServer, ID: id}, nil
    case "dm": return VirtualChannel{Kind: KindDM, ID: id}, nil
    }
    return VirtualChannel{}, errors.New("unreachable")
}
```

### 3.2 Authorization

New file `internal/voice/auth.go` consolidates voice-related permission checks. Three exported functions:

```go
type Authorizer struct { repo Repo /* db.DmRepository, db.ChannelRepository */ }

func (a *Authorizer) AuthorizeJoin(ctx, vc VirtualChannel, userID int64) (bool, error)
// KindServer: existing GetMember(serverID, userID) + role checks
// KindDM:     IsDmMember(vc.ID, userID)

func (a *Authorizer) AuthorizeMute(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error)
// KindServer: PermMuteMembers role check (existing)
// KindDM (1:1): always false — no in-call moderation in 1:1 (just leave)
// KindDM (GC): owner-only — actorID == channel.OwnerUserID

func (a *Authorizer) AuthorizeKick(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error)
// KindServer: PermMoveMembers role check (existing)
// KindDM (1:1): always false
// KindDM (GC): owner-only
```

The frontend must also know the actor's own privilege to decide whether to render the "Force mute" / "Disconnect" context-menu items at all. Privilege is computed client-side from data already in scope: for `KindServer`, check the user's role bitmask; for `KindDM` (GC), check `dmChannel.ownerUserID === currentUserID`. The server is still authoritative — these endpoints reject unauthorized requests — but the UI is consistent without an extra round-trip.

Existing `internal/voice/handler.go` calls into these instead of inlining the server-only checks at lines 56–61.

### 3.3 Routes

The existing `/api/channels/{channelID}/voice/*` route tree is generalized to accept a virtual channel ID. Two options:

**Option A (preferred):** add new route tree `/api/voice/{vc}/{token,join,leave,participants,heartbeat,participants/{userID}/mute,participants/{userID}/kick}` that takes a virtual channel ID directly. The existing `/api/channels/{id}/voice/*` becomes a thin wrapper that constructs `s:{id}` and forwards.

**Option B:** repurpose the existing `{channelID}` path-segment to accept `s:N` / `dm:N` strings. Less clean (URL with `:` in a path segment is ugly).

Plan will go with **A**.

New routes for ring lifecycle on DMs:

```
POST   /api/dm/{id}/call/ring     {} -> {ring_id}            // 1:1 only — initiates ring
POST   /api/dm/{id}/call/start    {} -> {}                   // GC only — emits call_started, no ring
POST   /api/dm/{id}/call/accept   {ring_id} -> {}
POST   /api/dm/{id}/call/decline  {ring_id} -> {}
POST   /api/dm/{id}/call/cancel   {ring_id} -> {}
GET    /api/calls/active          -> {rings: [...], in_call: [...]}  // boot/reload rehydration
```

Authorization on all DM call routes: `IsDmMember(dmID, userID)`.

New routes for the activities stub (work for any virtual channel):

```
POST   /api/voice/{vc}/activity/start  {type, params?} -> {}
POST   /api/voice/{vc}/activity/end    {} -> {}
GET    /api/voice/{vc}/activity        -> 200 + {type, started_by, started_at_ms, params?}, or 204 if no activity
```

Authorization: caller must currently be a participant in `voice:{vc}` (HEXISTS check). The stub does not gate by activity type — backend doesn't know about specific activity types.

### 3.4 Ring service

The ring service handles **1:1 DMs only**. GC calls have no ring layer — they emit `call_started` directly via the open-room path in 3.5.

New file `internal/voice/ring.go`:

```go
type Ring struct {
    ID           string  // ULID
    DmChannelID  int64
    CallerID     int64
    TargetID     int64
    StartedAt    time.Time
    cancel       func()  // stops the timer
}

type RingService struct {
    rings map[string]*Ring  // by ID
    byDM  map[int64]string  // dmChannelID -> active ring ID (1:1 has at most one)
    mu    sync.Mutex
    hub   WSHub
    dm    DmService          // emits system messages
    repo  DmRepo
}

func (r *RingService) Initiate(ctx, dmChannelID, callerID, targetID) (string, error)
func (r *RingService) Accept(ctx, ringID, accepterID) error
func (r *RingService) Decline(ctx, ringID, declinerID) error
func (r *RingService) Cancel(ctx, ringID, callerID) error
// 30s timer fires -> emits timeout
```

State machine (one ring instance):

```
created -> { accepted | declined | canceled | timeout }
```

On any terminal transition, the ring is removed from the maps and the timer is canceled. WS events are emitted to caller and target. A system message is written to the DM via `dm.Service`:

| Transition | WS to caller | WS to target | system_event |
|---|---|---|---|
| accepted   | `CALL_ACCEPT` | `CALL_ACCEPT`* | `call_started` |
| declined   | `CALL_DECLINE` | `CALL_DECLINE`* | `call_declined` |
| canceled   | (close ringback) | `CALL_CANCEL` | `call_missed` |
| timeout    | `CALL_TIMEOUT` | `CALL_TIMEOUT` | `call_missed` |

\* `CALL_ACCEPT` and `CALL_DECLINE` are sent to BOTH the caller and the receiver so other devices/sessions of either party can dismiss their modal/ringback. `CALL_CANCEL` only goes to the receiver — the caller's other sessions never had outgoing-call state to dismiss. `CALL_ACCEPT` payload includes `accepter_user_id`; `CALL_DECLINE` includes `decliner_user_id`. Frontends compare against `currentUserID` to distinguish "I responded from another tab" from "the other user responded."

**`call_started` actor is the caller, not the accepter** — system message reads "Alice started a call" for an Alice→Bob call regardless of who hits Accept (matches conventional UX).

**Race / concurrency:** `RingService.mu` guards all mutations. Glare guard: `Initiate` checks `byDM[dmID]` and returns the existing ring's ID (no error) if one is in flight — both parties end up resolving the same single ring.

**Sentinel errors:** `voice.ErrRingNotFound` and `voice.ErrCancelByNonCaller` are exported. HTTP handlers use `errors.Is` to map them to 404 / 403 respectively.

**Ring timer:** `time.AfterFunc(30s, ...)`. Single-process implementation. If Parley scales out, ring service must move to a Redis-backed scheduled queue.

### 3.5 Call lifecycle (no ring layer — open-room model)

The voice handler's existing join/leave already drives the room. Two changes:

1. Extend **Join** to record call start time on first joiner via `SET voice:{vc}:started_at <now> NX EX 21600` (6h TTL fallback in case leave is missed).
2. Extend **Leave** to detect "I was the last participant" and emit `call_ended`:

```go
func (h *Handler) Leave(...) {
    // ... existing HDEL
    remaining, _ := redis.HLen(ctx, "voice:" + vc.String())
    if remaining == 0 {
        // SETNX with TTL prevents duplicate emission
        startedAt, _ := redis.GetDel(ctx, "voice:" + vc.String() + ":started_at")
        lockKey := fmt.Sprintf("call_ended:%s:%s", vc, startedAt)
        ok, _ := redis.SetNX(ctx, lockKey, "1", 60*time.Second)
        if ok && vc.Kind == KindDM {
            durationMs := time.Now().UnixMilli() - parseInt64(startedAt)
            h.dm.EmitCallEnded(ctx, vc.ID, lastLeaverUserID, durationMs, startedAtMs)
        }
    }
}
```

`voice:{vc}:started_at` is set with `SET NX` on first join. `GetDel` on last leave reads + atomically deletes.

### 3.6 System events

Reuses the existing `dm_messages.system_event` JSONB pipeline introduced in Phase B. Four new event types:

```jsonc
// Ring accepted (1:1) OR first joiner of a GC call
{ "type": "call_started",  "actor_user_id": "1", "actor_display_name": "Alice", "started_at_ms": 1714000000000 }

// Last participant leaves
{ "type": "call_ended",    "duration_ms": 145000, "started_at_ms": 1714000000000 }

// Ring timeout OR caller cancel
{ "type": "call_missed",   "caller_user_id": "1", "caller_display_name": "Alice" }

// Receiver declined
{ "type": "call_declined", "caller_user_id": "1", "caller_display_name": "Alice", "decliner_user_id": "2" }
```

Rendering in `frontend/src/components/chat/SystemMessage.tsx` follows the same display-name fallback pattern from Phase B (prefer embedded names; fall back to `resolveUser`; else "someone").

### 3.7 WS events

New event type strings in `internal/websocket/events.go`:

```
CALL_RING       -> sent to target on Initiate.   payload: {ring_id, vc, caller: {user_id, username, display_name, avatar_url}}
CALL_ACCEPT     -> sent to caller AND target.    payload: {ring_id, accepter_user_id}
CALL_DECLINE    -> sent to caller AND decliner.  payload: {ring_id, decliner_user_id}    (decliner echo dismisses other sessions of the receiver)
CALL_CANCEL     -> sent to target.               payload: {ring_id}
CALL_TIMEOUT    -> sent to caller AND target.    payload: {ring_id}
ACTIVITY_START  -> broadcast to vc.String().     payload: {vc, type, started_by (string), started_at_ms, params?}
ACTIVITY_END    -> broadcast to vc.String().     payload: {vc}
```

Existing `VOICE_STATE_UPDATE`, `VOICE_FORCE_MUTE`, `VOICE_FORCE_DISCONNECT` are reused. Broadcast target depends on `vc.Kind`:

- **KindServer**: `"server:" + serverID` (existing behavior — unchanged so all server members continue to see green dots in the channel list without per-channel WS subscription).
- **KindDM**: `vc.String()` (i.e., `"dm:" + dmChannelID`). All DM/GC members already subscribe to this topic for `DM_MESSAGE_NEW` events from Phase B.

`VOICE_FORCE_MUTE` / `VOICE_FORCE_DISCONNECT` continue to use `SendToUser(targetID, ...)` — unchanged.

**Verification step in the implementation plan:** confirm `cmd/api/main.go`'s WS subscribe-on-auth logic puts DM members on the `dm:{id}` topic for *all* events (not just `DM_MESSAGE_NEW`). If voice-state events are filtered out, the filter must be lifted.

### 3.8 Heartbeat

Backend has heartbeat keys (`voice:heartbeat:{vc}:{userID}` TTL 30s) for stale-presence cleanup. Add a 15s heartbeat loop in `useVoiceConnection` that calls `POST /api/voice/{vc}/heartbeat` while connected. If a loop already exists from prior work, the change is a no-op; if not, this adds it. No verification branch — the plan unconditionally writes the loop.

### 3.9 Activities (stub)

Backend keeps the active activity per virtual channel in Redis: key `voice:{vc}:activity` is a JSON-encoded `{type, started_by (string-encoded int64), started_at_ms, params?}` or absent (no activity). Lifetime is bound to the call: when the last participant leaves, the activity entry is deleted alongside `voice:{vc}:started_at`. The `started_by` field uses `,string` JSON tag so wire form is `"7"` not `7` (matches the rest of the codebase's int64-as-string convention for IDs).

`POST /activity/start` emits `ACTIVITY_START` to `vc.String()`. `POST /activity/end` deletes the key and emits `ACTIVITY_END`. The backend stores `params` opaquely — interpretation is purely a frontend concern. No registry or validation of `type` strings server-side; the frontend's registry is the source of truth for renderable types.

Backend does not write `activity_*` system messages to DM history. (Activities are ephemeral; chat history doesn't need a record of every "Alice started watch-together.")

---

## 4. Frontend

### 4.1 New files

| File | Purpose |
|---|---|
| `frontend/src/api/calls.ts` | Ring lifecycle API client (initiate / accept / decline / cancel / activeRings). |
| `frontend/src/api/activities.ts` | Activities stub API client (start / end / get). |
| `frontend/src/context/CallContext.tsx` | Global call state — incoming-ring queue, outbound ringing state. Subscribes to WS call events. Handles Tauri vs web platform branching for the ring surface. |
| `frontend/src/hooks/useLocalVolumes.ts` | localStorage-backed `Map<userID, 0–200>` with subscribe/get/set/toggle. Mute = 0. |
| `frontend/src/activities/registry.ts` | Activity registry: `register({type, render})`, `lookup(type)`. Exported singleton. Empty in this iteration. |
| `frontend/src/components/calls/IncomingCallModal.tsx` | Web fallback (and Tauri fallback when main is focused). Accept / Decline UI. |
| `frontend/src/components/calls/CallBanner.tsx` | In-channel "📞 N in call · Join" strip in the DM/GC chat header. |
| `frontend/src/components/calls/FloatingCallWindow.tsx` | Draggable in-app overlay rendering `<VoiceChannel layout="compact">`. Position persisted to localStorage. |
| `frontend/src/components/voice/ActivitySlot.tsx` | Pane inside `VoiceChannel` that subscribes to active-activity state and renders the registered renderer (or a placeholder if no renderer registered for that type, or nothing if no activity). |
| `frontend/src/components/voice/ActivitiesModal.tsx` | Modal listing activities from the registry. With empty registry, shows "Activities coming soon" placeholder. |
| `frontend/src/components/voice/ConnectionQualityDot.tsx` | Tiny corner-of-tile indicator backed by `participant.connectionQuality`. |
| `frontend/src/components/voice/VolumeSlider.tsx` | Slider control (0–200%) used inside `VoiceContextMenu`. |
| `frontend/src/ring/main.tsx` | Standalone React entry for the Tauri secondary ring window. |
| `frontend/src/ring/RingApp.tsx` | UI for the ring window: avatar on top, "Incoming call from <name>" text, green Answer / red Decline buttons. |
| `frontend/ring.html` | HTML entry for the ring webview. |
| `frontend/public/ringtone.mp3` | Ringtone audio asset. |

### 4.2 Modified files

| File | Change |
|---|---|
| `frontend/src/api/voice.ts` | All endpoints accept a virtual channel ID string (`s:N` / `dm:N`) instead of bare numeric channel ID. |
| `frontend/src/hooks/useVoiceConnection.ts` | Accept `virtualChannelId: string`. Unconditionally include a 15s heartbeat loop. Wire per-listener volume gating via `useLocalVolumes`. Subscribe to `ACTIVITY_START` / `ACTIVITY_END` and expose active-activity state. |
| `frontend/src/components/voice/VoiceChannel.tsx` | Add `layout: 'full' \| 'compact'` prop. Compact tightens tile sizing for the floating window. Render `<ActivitySlot>` between header and tile grid. |
| `frontend/src/components/voice/VoiceControls.tsx` | Add Pop-Out button (toggles floating mode) and Activities button (opens `ActivitiesModal`). |
| `frontend/src/components/voice/VoiceContextMenu.tsx` | Three controls: per-user `<VolumeSlider>`, "Mute for me" toggle (sets volume to 0), and the privilege-gated "Force mute" / "Disconnect" entries. |
| `frontend/src/components/voice/ParticipantTile.tsx` | Apply per-listener volume; render strikethrough-speaker icon when volume is 0; render `<ConnectionQualityDot>` in a corner. |
| `frontend/src/components/chat/ChatWindow.tsx` | Add 📞 Start/Join Call button to the DM/GC chat header. Render `<CallBanner>` above message list when an active call exists in this channel. |
| `frontend/src/components/chat/SystemMessage.tsx` | Render `call_started` / `call_ended` / `call_missed` / `call_declined` events. |
| `frontend/src/components/layout/DmPanel.tsx` | Phone icon next to DMs/GCs with active calls. |
| `frontend/src/notifications/shouldNotify.ts` | Bug fix: `if (event.actor_user_id === currentUserID) return false;` short-circuit. |
| `frontend/src/App.tsx` | Mount `<CallProvider>` around the existing tree. Render `<IncomingCallModal>` and `<FloatingCallWindow>` from the provider. Existing voice-state plumbing now reads virtual channel IDs throughout. |

### 4.3 Tauri

| File | Change |
|---|---|
| `desktop/src-tauri/src/lib.rs` (or `src/ring_window.rs`) | New commands: `spawn_ring_window(args)`, `dismiss_ring_window(ring_id)`. |
| `desktop/src-tauri/tauri.conf.json` | Build config to include `ring.html` as a second entry. |
| `desktop/src-tauri/capabilities/default.json` | Allow `core:webview:create-webview-window`, `core:window:set-always-on-top`, `core:window:set-skip-taskbar`, `core:window:close`, `core:window:set-position`. |

### 4.4 CallContext state model

```typescript
type CallState =
  | { kind: 'idle' }
  | { kind: 'outgoing'; vc: string; ring_id: string; target: User }
  | { kind: 'connecting'; vc: string }
  | { kind: 'connected'; vc: string };

type IncomingRing = { vc: string; ring_id: string; caller: User };

type CallContextValue = {
  state: CallState;
  incomingQueue: IncomingRing[];   // FIFO, oldest first
  initiate: (target: User, dmChannelID: bigint) => Promise<void>;
  accept: (ring_id: string) => Promise<void>;
  decline: (ring_id: string) => Promise<void>;
  cancel: () => Promise<void>;
  // floating window controls
  floatingMode: boolean;
  setFloatingMode: (on: boolean) => void;
};
```

`useVoiceConnection` continues to own `connecting` / `connected` state; CallContext watches it via the existing `voiceState` plumbing in App.tsx and treats those as the source of truth for `state.kind === 'connected'`.

### 4.5 Ring surface decision

`CallContext` tracks `mainFocused: boolean` via `getCurrentWindow().onFocusChanged(...)` (Tauri only). Recompute the surface on every focus change while a ring is active:

```
isTauri && !mainFocused → spawn or keep one secondary window per queued ring (visually stacked toward the top); hide all modals
otherwise               → render IncomingCallModal for incomingQueue[0] (others remain queued); close all secondary windows
```

**Multiple concurrent rings:** in the secondary-window case, each queued ring gets its own bottom-right window (Tauri stacks them naturally by spawn order). In the modal case, only the head of the queue is shown; resolving it reveals the next.

**Audio:** ringtone always plays from the main webview, regardless of which surface is showing. This avoids audio-context churn on focus flip and keeps the ring audible even while spawning the secondary window.

### 4.6 Floating call window

Default mode: full takeover when the user is on the call's channel; docked widget (`VoiceControls`) when navigated away. Clicking the new Pop-Out button on the docked widget enters floating mode:

```
floatingMode=true && state.kind==='connected' → render <FloatingCallWindow>
                                                hide <VoiceControls> dock widget
                                                hide <VoiceChannel> if user is on call's channel
```

`<FloatingCallWindow>` renders `<VoiceChannel layout="compact">` inside a `position: fixed` div with:
- Drag handle at top (mousedown → tracks delta → updates `transform: translate(...)`)
- Position persisted to `localStorage.parley.floatingPosition`
- "Expand" button → exits floating mode, returns to full takeover (navigates to call's channel if not already there)
- "Dock" button → exits floating mode, returns to docked widget

### 4.7 Per-listener volume persistence

`useLocalVolumes` exposes:

```typescript
function useLocalVolumes(): {
  getVolume: (userID: bigint) => number;     // returns 100 (unity) if no entry
  setVolume: (userID: bigint, value: number) => void;  // 0–200
  toggleMute: (userID: bigint) => void;      // 0 ↔ last non-zero value (default 100)
};
```

Backed by `localStorage.parley.localVolumes` (JSON-encoded `Record<string, number>` keyed by stringified user ID). Cross-tab sync via the `storage` event. `ParticipantTile` reads `getVolume(participant.userID)`, applies via `participant.setVolume(value / 100)`, and renders the strikethrough-speaker icon when value is 0. `VoiceContextMenu` exposes a `<VolumeSlider>` (0–200%) plus a "Mute for me" toggle.

Persists across all calls (server VC, GC call, DM call). Visual indicator on tiles for muted participants makes "why am I getting silence" self-explanatory.

### 4.8 Activities registry

`frontend/src/activities/registry.ts`:

```typescript
type ActivityRenderer = React.FC<{ vc: string; params: unknown }>;
type ActivityDefinition = {
  type: string;          // unique identifier — matches backend type strings
  displayName: string;
  icon?: React.ReactNode;
  render: ActivityRenderer;
  controls?: React.FC<{ vc: string }>;  // optional buttons to start with custom params
};

let registry = new Map<string, ActivityDefinition>();
export const register = (a: ActivityDefinition) => registry.set(a.type, a);
export const lookup = (type: string) => registry.get(type) ?? null;
export const list = () => Array.from(registry.values());
```

`<ActivitySlot vc={...}>` subscribes to `useVoiceConnection`'s active-activity state (driven by `ACTIVITY_START` / `ACTIVITY_END` WS events) and renders:
- nothing if no active activity,
- `lookup(type).render` if the type is registered,
- `<UnknownActivity type={type} />` placeholder if active but no renderer registered (forward-compat: a future activity is running, but this client doesn't have its renderer).

`<ActivitiesModal>` opens from the Activities button in `VoiceControls`; lists registered activities (or "Activities coming soon" if empty). Clicking one calls `POST /api/voice/{vc}/activity/start`.

In this iteration, no activities are registered. The harness exists; future enhancements add `register({...})` calls — no spec changes required.

---

## 5. Data flows

### 5.1 1:1 happy path

```
Alice clicks 📞 in DM → POST /api/dm/{id}/call/ring
  └─ ring service: create ring, start 30s timer, emit:
       • CALL_RING → Bob (with ring_id, vc="dm:{id}", caller info)
       • CallContext (Alice): outgoing state { ring_id, target: Bob }; ringback tone
Bob receives CALL_RING:
  • mainFocused → IncomingCallModal opens; ringtone plays
  • !mainFocused → Tauri: spawn_ring_window(args); ringtone plays from main webview
Bob clicks Accept → POST /api/dm/{id}/call/accept {ring_id}
  └─ ring service: terminate, emit:
       • CALL_ACCEPT → Alice + Bob (accepter_user_id: Bob)
       • call_started system_event into the DM
  └─ Both clients: useVoiceConnection.connect("dm:{id}")
  └─ Existing LiveKit token + Redis presence flow runs unchanged
Last participant leaves:
  └─ Existing leaveVoiceChannel → handler detects empty room → SETNX lock → call_ended system_event
```

### 5.2 GC happy path (no ring)

```
Alice clicks 📞 in GC → POST /api/dm/{id}/call/start
  └─ Service: emits call_started system_event (with started_at_ms = now)
  └─ All GC members receive DM_MESSAGE_NEW with the system_event
  └─ CallBanner appears in the GC chat; phone icon on DM list item
Alice's client: useVoiceConnection.connect("dm:{id}")
Other members: see banner, click Join → useVoiceConnection.connect("dm:{id}")
Last leaves → call_ended (same logic as 1:1)
```

### 5.3 Decline (1:1)

```
Bob clicks Decline → POST /api/dm/{id}/call/decline {ring_id}
  └─ ring service: terminate, emit:
       • CALL_DECLINE → Alice (toast: "Bob declined")
       • call_declined system_event
  └─ Bob's surface (modal or secondary window) closes
  └─ Alice's outgoing state resolves
```

### 5.4 Timeout (1:1)

```
30s timer fires → ring service:
  • CALL_TIMEOUT → Alice + Bob
  • call_missed system_event
Both surfaces close; Alice toast "No answer."
```

### 5.5 Cancel (1:1)

```
Alice clicks Cancel → POST /api/dm/{id}/call/cancel {ring_id}
  └─ ring service: terminate, emit:
       • CALL_CANCEL → Bob (close surface)
       • call_missed system_event (caller cancelled — Bob sees "Missed call from Alice")
```

### 5.6 Glare (Alice and Bob ring each other simultaneously)

```
Alice POST /ring → ring R1 created. Bob is the target.
Bob POST /ring → service checks byDM map: if R1 already exists for this DM,
                 return error 409 "ring already active" with the existing ring_id.
Bob's frontend treats this as "incoming ring already in flight from the other party";
               displays the existing CALL_RING from Alice instead.
```

The `byDM` map keyed by `dmChannelID` gives at-most-one ring per DM, eliminating glare at the source.

### 5.7 Concurrent / busy

```
Bob is in another call when CALL_RING arrives.
Frontend detects state.kind === 'connected' && state.vc !== ring.vc.
IncomingCallModal renders the extra "End current & Accept" button.
Click → useVoiceConnection.disconnect() (existing) → POST /accept (existing) → connect to new vc.
```

No backend `CALL_BUSY` event — handled fully client-side via state inspection.

### 5.8 Ring on a target with multiple sessions

```
Bob is signed in on desktop and web (two WS connections).
CALL_RING is sent via SendToUser(bobID) → fans out to all of Bob's WS connections.
Both surfaces show a ring (modal or secondary window per platform).
First Accept/Decline wins; ring service terminates the ring.
CALL_ACCEPT (carrying accepter_user_id: Bob) goes back via SendToUser(bobID) →
  fans out to all Bob's sessions. Each frontend, on receiving CALL_ACCEPT where
  accepter_user_id === currentUserID, dismisses the modal/window without joining.
The session that clicked Accept proceeds to connect.
```

Existing `BroadcastChannel` mutual-exclusion handles the same-tab case.

### 5.9 Rehydration on page reload

```
On boot, frontend hits GET /api/calls/active.
Response includes:
  • rings: any active rings targeting the current user (ring_id, caller, vc, expires_at)
  • in_call: virtual channels where Redis presence shows the current user as joined
For each ring → CallContext shows incoming surface with remaining time
For each in_call → ignore (current behavior is disconnect-on-unload; user simply isn't in those calls anymore)
```

In-call rehydration is out of scope. The `in_call` list is returned but only used for diagnostics / future enhancement.

---

## 6. Error handling

| Scenario | Handling |
|---|---|
| WS reconnect during ring | On reconnect, frontend hits `GET /api/calls/active` → restores ring state. |
| WS reconnect during call | Existing `useVoiceConnection` LiveKit reconnect handles it. |
| Mic permission denied at Accept | Surface toast "Mic access denied — call cannot start." Trigger immediate disconnect; emits `call_started` then `call_ended {duration_ms: 0}`. Better than blocking the accept. |
| LiveKit token issuance fails | Toast "Couldn't connect to call service." If during Initiate, no ring is created. If during Accept, behave like mic-denied. |
| GC member kicked from channel during call | `KickMember` flow also dispatches `VOICE_FORCE_DISCONNECT` to the kicked user. Existing client handler runs `doDisconnect()`. |
| Stale Redis presence | Existing 30s heartbeat TTL. Frontend pings every 15s — added unconditionally to `useVoiceConnection`. |
| Backend restart with active rings | Ring service is in-memory. On restart, all in-flight rings vanish. Targets' WS reconnect → `GET /api/calls/active` returns empty → ring modals/windows close client-side after a short grace period (5s no-event timeout). Callers observe their outgoing toast eventually time out client-side. Acceptable; documented limitation. |
| Backend restart with active calls | LiveKit room and Redis presence outlive the backend (Redis is separate; LiveKit is external). On restart, in-call clients keep talking. New joins fail until backend is back. |
| Caller's WS dropped before Bob accepts | Server emits `CALL_ACCEPT` to Alice via `SendToUser`; if her WS is offline, the event is queued and delivered on reconnect. If her browser is gone for good, Bob ends up alone in the room → he leaves → `call_ended {duration_ms ≈ 0}`. |
| Browser autoplay policy blocks ringtone | App-load warm-up: play a 1ms silent buffer on first user gesture in the page session to keep the audio context alive. Standard pattern. |
| Notification setting is "muted" for the DM | Modal/secondary window still appears (visual ring) but ringtone is silent and no OS notification fires. Caller hears ringback as normal. Mirrors phone DND. |
| Single-call-per-channel | Frontend rule: if `voice:{vc}` has any participants (cached from `VOICE_STATE_UPDATE`), the 📞 button reads "Join" and routes to `connect()` directly — no ring. |
| Duplicate `call_ended` from concurrent leaves | `SET NX EX 60` lock keyed by `vc:started_at` ensures single emission. |

---

## 7. Accessibility

`IncomingCallModal`:
- Focus trap inside modal on open; restore on close.
- `Escape` declines. `Enter` accepts.
- `aria-live="assertive"` announcement: "Incoming call from Alice."
- Buttons have explicit `aria-label`: "Accept call", "Decline call".

`FloatingCallWindow`:
- Drag handle is a `<button>` with `aria-label="Drag floating call window"`; supports keyboard arrow-key positioning when focused.
- All control buttons inherit `VoiceControls` accessibility.

`SystemMessage` (call_*): each event renders with semantic text; screen reader reads "Missed call from Alice" naturally.

---

## 8. Testing

**Backend (Go)**
- `internal/voice/virtual_channel_test.go` — parse/format roundtrip, error cases.
- `internal/voice/auth_test.go` — table-driven tests for KindServer / KindDM 1:1 / KindDM GC permission matrices.
- `internal/voice/ring_test.go` — ring state machine: initiate → accept, decline, cancel, timeout. Glare rejection. Multi-session target.
- `internal/voice/handler_test.go` — last-leaver detects empty room, emits `call_ended` once even with concurrent leaves.

**Frontend (TS)**
- `useLocalVolumes.test.ts` — localStorage roundtrip, `storage` event sync, mute-via-zero semantics.
- `CallContext.test.tsx` — state transitions for outgoing, incoming, glare, dual-session.
- `activities/registry.test.ts` — register/lookup roundtrip; `lookup` of unregistered type returns null.
- `shouldNotify.test.ts` — own-message short-circuit covered.

**Integration**
- Manual smoke test in production after deploy: 1:1 happy, decline, timeout, GC start/join, multi-session ring, force-mute in GC, local mute persistence, floating window drag/expand/dock, Tauri ring window appears when main unfocused.

---

## 9. Migration & rollout

- **No DB migrations.** All artifacts use the existing `dm_messages.system_event` JSONB column (Phase B).
- **Backend first**, then frontend. New WS event types are additive — old clients ignore unknown event types. Old `/api/channels/{id}/voice/*` routes continue to work (wrap to `s:{id}`).
- **Feature flag:** none. The 📞 button being absent until frontend ships is sufficient gating.
- **Release version:** target `v0.5.0` (minor — new feature surface).
- **CI release:** standard tag-push triggers GitHub Actions; do not build desktop locally (per memory).
- **Server-VC room-name change at deploy:** the prefixed virtual-channel ID changes the LiveKit room name from `123` to `s:123` and the Redis presence key from `voice:123` to `voice:s:123`. Anyone in a server VC at the moment of deploy will be disconnected and need to rejoin once. Briefly stale Redis entries (the un-prefixed keys) age out via the existing 30s heartbeat TTL. This is a one-time cost; recommend deploying during low-traffic hours rather than building a backwards-compat bridge.

---

## 10. Open implementation-time decisions (non-blocking)

These are deferred to build:
- Ringtone audio file selection (royalty-free; ~6s loop).
- Floating window default position (likely top-right of viewport, 320×400).
- Whether GC mute setting (Phase A `notification_setting`) silences the **system message** notification too — likely yes (it's already in scope of the existing `shouldNotify` gate).
- Exact placement of the 📞 button in `ChatWindow.tsx` header (left of message-search vs right).

---

## 11. Out-of-scope follow-ups

- True OS-level secondary window for the floating call window (not the ring window — that's in scope). Adds Tauri IPC complexity for cross-window LiveKit state sync.
- In-call rehydration on page reload (currently disconnect-on-unload).
- Per-DM custom ringtones.
- Call recording / transcription.
- Mobile builds.
- Horizontal-scale-safe ring scheduler.

---

## 12. Implementation deviations from initial spec (Tasks 1–15, as-built)

These are corrections to type/wire shapes the spec referenced before implementation; future tasks consume the as-built shapes.

- **Type names:** `db.ServerMember` (not `db.Member`); `db.DmChannelMember` with field `DmChannelID` (not `db.DmMember.ChannelID`).
- **Permission API:** no `permissions.Repo` / `permissions.Perm` typed wrappers; `voice.Authorizer` uses `*db.Repository` directly and `int64` for the perm bitmask arg passed to `permissions.HasChannelPermission`.
- **`EmitCallEnded` signature:** 5 params — `(ctx, channelID, lastLeaverUserID, durationMs, startedAtMs)`. The `lastLeaverUserID` is the leaving user's ID, used as the system message's `author_id` (FK to `users(id)`). JSON payload still excludes the actor; only `duration_ms` and `started_at_ms` ride.
- **`dm.Handler.Service() *dm.Service` accessor:** added to expose the service for `cmd/api` wiring. `*dm.Service` satisfies `voice.dmServiceLike` structurally.
- **Activity wire shape:** `{type, started_by, started_at_ms, params?}`. `started_by` uses `,string` JSON tag (string-encoded int64 — matches the codebase's int64-as-string convention for IDs). `started_at_ms` is a JSON number.
- **Activity GET semantics:** returns 200 + JSON when present, 204 No Content when absent (not 200 + null). Get is intentionally unauthenticated (other endpoints under `/voice/{vc}/activity/*` are member-gated); locked in by `TestActivityGet_NoAuthRequired`.
- **`/calls/active` `in_call`:** always emits `[]` (empty array, not null). The wire shape is locked for forward-compat; field unused in v0.5.0.
- **GC `/call/start` behavior on unwired starter:** logs and returns 500 (not 503 — the unwired state is permanent until restart, not transient retry).
- **Voice handler `dmCallEnder` unwired branch:** logs `"voice handler: dmCallEnder not wired; skipping call_ended for dm:%d"` so missed wiring shows up in production traces.
- **`broadcastTarget` for `KindServer`:** returns `"server:" + serverID` (preserves existing server-VC sidebar behavior — DM members see `dm:{id}` topic). Uses `errors.Is(err, db.ErrNotFound)` to silent-deny missing channels; logs other errors.
- **`ActivityHandler.Start` race fallback:** if `GetActivity` read-back after `StartActivity` returns nil/err (race with concurrent `EndActivity`), Start falls back to local-state Activity built from request inputs and broadcasts `ACTIVITY_START` anyway. Logs the race for forensics.
- **Test harness:** `internal/voice/auth_test.go`'s `authRepoFake` is the shared fake repo for voice handler tests (extended in Tasks 12 with `GetDmMembers` / `GetUserByID`). `internal/voice/ring_test.go`'s `fakeHub` tracks both `toUser` (SendToUser) and `broadcasts` (BroadcastToChannel) with a buffered `gotMsg` channel for deterministic async assertions. `newRedisForTest` uses `MaxRetries: -1, DialTimeout: 200ms` so SKIP resolves quickly when no local Redis.
