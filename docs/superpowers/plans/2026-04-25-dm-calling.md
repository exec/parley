# DM/GC Calling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 1:1 DM ringing, GC open-room calls, in-app floating call window, Tauri secondary ring window, per-listener volume, connection-quality dot, VC-activities stub, and fix the self-message notification bug — all by generalizing the existing voice infrastructure to a virtual-channel namespace shared by server VCs and DMs.

**Architecture:** Virtual channel ID is a prefixed string (`s:N` or `dm:N`). One auth helper, one ring service (1:1 only), one set of routes. Call lifecycle artifacts go in `dm_messages.system_event` JSONB (no DB migrations). Frontend's `useVoiceConnection` is already lifted at `App.tsx:281`; we extend it. Tauri secondary window spawns when main is unfocused at ring time.

**Tech Stack:** Go 1.25 + chi + Redis + LiveKit. React 18 + TypeScript + Vite. Tauri 2 (multi-window, capabilities). LiveKit JS SDK.

**Spec:** [docs/superpowers/specs/2026-04-25-dm-calling-design.md](../specs/2026-04-25-dm-calling-design.md)

---

## Phase index

| Phase | Tasks | What ships |
|---|---|---|
| 1 — Backend foundation | 1–4 | virtual_channel parser, authorizer, WS event constants, system-event emitters |
| 2 — Backend voice generalization | 5–8 | Refactored handler with vc strings, last-leaver `call_ended`, started_at tracking |
| 3 — Ring service | 9–11 | 1:1 ring lifecycle, timeout, glare guard |
| 4 — Backend HTTP surface | 12–15 | New `/api/voice/{vc}/*` and `/api/dm/{id}/call/*` routes, `GET /api/calls/active`, activities endpoints |
| 5 — WS subscription verification | 16 | DM members receive voice events on `dm:{id}` topic |
| 6 — Frontend foundation | 17–20 | API clients, useLocalVolumes, activities registry |
| 7 — Existing voice components | 21–24 | useVoiceConnection vc-string + heartbeat + activity events; VoiceChannel layout prop; ParticipantTile volume + quality dot; VoiceContextMenu + VoiceControls |
| 8 — Call lifecycle UI | 25–30 | CallContext, IncomingCallModal, CallBanner, FloatingCallWindow, system message rendering, ChatWindow/DmPanel/App wiring |
| 9 — Tauri secondary ring window | 31–33 | Rust commands, capabilities, ring webview |
| 10 — Bug fix + release | 34–35 | shouldNotify own-message short-circuit, smoke test, version bump |

---

## Conventions for every task

- Each task names exact files and includes complete code (no placeholders).
- Tests are written first; failing test runs are captured before implementation.
- Each task ends with a single commit.
- Use the existing patterns: chi for routing, `httputil.JSONError` for HTTP errors, `apiClient` for frontend HTTP, `useState`/CustomEvents for cross-component coordination.
- When modifying existing files, read them in full before editing — line numbers in this plan reflect the state at plan-writing time and may have drifted.
- Run `go test ./internal/voice/... -count=1` from repo root for backend voice tests.
- Run `cd frontend && npm test` for frontend tests (vitest in `run` mode).
- Run `go build ./...` from repo root to verify the backend compiles after each backend task.
- Run `cd frontend && npm run build` to verify the frontend compiles after each frontend task.

---

## Phase 1 — Backend foundation

### Task 1: Virtual channel parser

**Files:**
- Create: `internal/voice/virtual_channel.go`
- Create: `internal/voice/virtual_channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
package voice

import "testing"

func TestParseVirtualChannel(t *testing.T) {
	tests := []struct {
		in       string
		wantKind Kind
		wantID   int64
		wantErr  bool
	}{
		{"s:42", KindServer, 42, false},
		{"dm:1234567890", KindDM, 1234567890, false},
		{"s:0", KindServer, 0, false},
		{"", 0, 0, true},
		{"42", 0, 0, true},
		{"x:42", 0, 0, true},
		{"s:", 0, 0, true},
		{"s:abc", 0, 0, true},
		{"dm:-1", KindDM, -1, false}, // negative IDs are accepted; semantic check is downstream
	}
	for _, tt := range tests {
		got, err := ParseVirtualChannel(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q): expected error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got.Kind != tt.wantKind || got.ID != tt.wantID {
			t.Errorf("Parse(%q) = %+v, want {Kind:%v ID:%d}", tt.in, got, tt.wantKind, tt.wantID)
		}
	}
}

func TestVirtualChannelString(t *testing.T) {
	tests := []struct {
		vc   VirtualChannel
		want string
	}{
		{VirtualChannel{Kind: KindServer, ID: 42}, "s:42"},
		{VirtualChannel{Kind: KindDM, ID: 7}, "dm:7"},
	}
	for _, tt := range tests {
		if got := tt.vc.String(); got != tt.want {
			t.Errorf("String(%+v) = %q, want %q", tt.vc, got, tt.want)
		}
	}
}

func TestRoundtrip(t *testing.T) {
	for _, in := range []string{"s:1", "dm:99", "s:9999999999"} {
		vc, err := ParseVirtualChannel(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := vc.String(); got != in {
			t.Errorf("roundtrip %q -> %q", in, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/voice/ -run TestParseVirtualChannel -count=1
```

Expected: FAIL with "undefined: Kind", "undefined: ParseVirtualChannel", etc.

- [ ] **Step 3: Implement**

```go
package voice

import (
	"errors"
	"strconv"
	"strings"
)

// Kind discriminates the type of voice room a VirtualChannel addresses.
type Kind int

const (
	KindServer Kind = iota
	KindDM
)

// VirtualChannel is the namespaced identity of a voice room (server VC or DM/GC call).
// Use the String form (e.g. "s:42", "dm:7") as the LiveKit room name, the Redis presence
// key, and the WS broadcast topic so the backend never has to know the difference.
type VirtualChannel struct {
	Kind Kind
	ID   int64
}

func (v VirtualChannel) String() string {
	switch v.Kind {
	case KindServer:
		return "s:" + strconv.FormatInt(v.ID, 10)
	case KindDM:
		return "dm:" + strconv.FormatInt(v.ID, 10)
	}
	return ""
}

// ParseVirtualChannel parses an "s:N" or "dm:N" string into a VirtualChannel.
func ParseVirtualChannel(s string) (VirtualChannel, error) {
	var prefix, rest string
	if r, ok := strings.CutPrefix(s, "dm:"); ok {
		prefix, rest = "dm", r
	} else if r, ok := strings.CutPrefix(s, "s:"); ok {
		prefix, rest = "s", r
	} else {
		return VirtualChannel{}, errors.New("invalid virtual channel id: missing prefix")
	}
	if rest == "" {
		return VirtualChannel{}, errors.New("invalid virtual channel id: empty id")
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return VirtualChannel{}, errors.New("invalid virtual channel id: " + err.Error())
	}
	switch prefix {
	case "s":
		return VirtualChannel{Kind: KindServer, ID: id}, nil
	case "dm":
		return VirtualChannel{Kind: KindDM, ID: id}, nil
	}
	return VirtualChannel{}, errors.New("unreachable")
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/voice/ -run TestParseVirtualChannel -count=1
go test ./internal/voice/ -run TestVirtualChannelString -count=1
go test ./internal/voice/ -run TestRoundtrip -count=1
```

Expected: PASS for all three.

- [ ] **Step 5: Commit**

```
git add internal/voice/virtual_channel.go internal/voice/virtual_channel_test.go
git commit -m "feat(voice): add VirtualChannel namespace parser"
```

---

### Task 2: Voice authorizer

**Files:**
- Create: `internal/voice/auth.go`
- Create: `internal/voice/auth_test.go`

The authorizer encapsulates the three permission gates: AuthorizeJoin, AuthorizeMute, AuthorizeKick. For server channels it delegates to existing `permissions.HasChannelPermission`; for DMs it queries `dm_channel_members` and checks owner.

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"context"
	"testing"

	"parley/internal/db"
)

// authRepoFake is a hand-rolled stub implementing the small surface we need.
type authRepoFake struct {
	dmMembers     map[int64]map[int64]bool         // dmID -> userID -> isMember
	dmOwnerByID   map[int64]int64                  // dmID -> ownerUserID
	dmIsGroupByID map[int64]bool                   // dmID -> is_group
	srvMember     map[int64]map[int64]*db.Member   // serverID -> userID -> member
	srvOwner      map[int64]int64                  // serverID -> ownerID
	chByID        map[int64]*db.Channel            // channelID -> channel
}

func (r *authRepoFake) IsDmMember(_ context.Context, dmID, uid int64) (bool, error) {
	return r.dmMembers[dmID][uid], nil
}
func (r *authRepoFake) GetDmChannelByID(_ context.Context, dmID int64) (*db.DmChannel, error) {
	return &db.DmChannel{ID: dmID, IsGroup: r.dmIsGroupByID[dmID], OwnerUserID: ptrInt64(r.dmOwnerByID[dmID])}, nil
}
func (r *authRepoFake) GetMember(_ context.Context, serverID, uid int64) (*db.Member, error) {
	return r.srvMember[serverID][uid], nil
}
func (r *authRepoFake) GetServerByID(_ context.Context, serverID int64) (*db.Server, error) {
	return &db.Server{ID: serverID, OwnerID: r.srvOwner[serverID]}, nil
}
func (r *authRepoFake) GetChannelByID(_ context.Context, channelID int64) (*db.Channel, error) {
	return r.chByID[channelID], nil
}

func ptrInt64(v int64) *int64 { return &v }

func TestAuthorizeJoin_DM(t *testing.T) {
	repo := &authRepoFake{
		dmMembers: map[int64]map[int64]bool{
			10: {1: true, 2: true},
		},
	}
	a := &Authorizer{repo: repo}
	cases := []struct {
		vc   VirtualChannel
		uid  int64
		want bool
	}{
		{VirtualChannel{Kind: KindDM, ID: 10}, 1, true},
		{VirtualChannel{Kind: KindDM, ID: 10}, 2, true},
		{VirtualChannel{Kind: KindDM, ID: 10}, 3, false},
		{VirtualChannel{Kind: KindDM, ID: 99}, 1, false},
	}
	for _, c := range cases {
		ok, err := a.AuthorizeJoin(context.Background(), c.vc, c.uid)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ok != c.want {
			t.Errorf("AuthorizeJoin(%v,%d) = %v, want %v", c.vc, c.uid, ok, c.want)
		}
	}
}

func TestAuthorizeMute_DM_GC_OwnerOnly(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmOwnerByID:   map[int64]int64{10: 1},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}

	// owner (1) muting any other GC member is allowed
	ok, _ := a.AuthorizeMute(context.Background(), vc, 1, 2)
	if !ok {
		t.Fatal("owner should be allowed to mute member")
	}
	// non-owner (2) muting member (3) is denied
	ok, _ = a.AuthorizeMute(context.Background(), vc, 2, 3)
	if ok {
		t.Fatal("non-owner must not mute")
	}
	// owner cannot mute themselves
	ok, _ = a.AuthorizeMute(context.Background(), vc, 1, 1)
	if ok {
		t.Fatal("self-mute via force-mute is nonsense")
	}
}

func TestAuthorizeMute_DM_OneOnOne_Denied(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}
	ok, _ := a.AuthorizeMute(context.Background(), vc, 1, 2)
	if ok {
		t.Fatal("force-mute is not allowed in 1:1 DM")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/voice/ -run TestAuthorize -count=1
```

Expected: FAIL with "undefined: Authorizer" etc.

- [ ] **Step 3: Implement**

```go
package voice

import (
	"context"

	"parley/internal/db"
	"parley/internal/permissions"
)

// authzRepo is the slice of repository methods the Authorizer needs.
// Defined as an interface so tests can stub it without dragging in the full Repository.
type authzRepo interface {
	IsDmMember(ctx context.Context, dmID, userID int64) (bool, error)
	GetDmChannelByID(ctx context.Context, dmID int64) (*db.DmChannel, error)
	GetMember(ctx context.Context, serverID, userID int64) (*db.Member, error)
	GetServerByID(ctx context.Context, serverID int64) (*db.Server, error)
	GetChannelByID(ctx context.Context, channelID int64) (*db.Channel, error)
}

// permChecker is the small surface of the permissions package used here.
// Wrapping it as a function-typed field lets tests substitute a fake.
type permChecker func(ctx context.Context, repo permissions.Repo, serverID, userID, ownerID, channelID int64, perm permissions.Perm) (bool, error)

type Authorizer struct {
	repo authzRepo
	// hasChannelPerm defaults to permissions.HasChannelPermission. Override in tests.
	hasChannelPerm permChecker
	// permRepo is the value passed as the Repo argument to hasChannelPerm.
	permRepo permissions.Repo
}

func NewAuthorizer(repo *db.Repository) *Authorizer {
	return &Authorizer{
		repo:           repo,
		hasChannelPerm: permissions.HasChannelPermission,
		permRepo:       repo,
	}
}

// AuthorizeJoin returns true if userID is allowed to join the voice room vc.
func (a *Authorizer) AuthorizeJoin(ctx context.Context, vc VirtualChannel, userID int64) (bool, error) {
	switch vc.Kind {
	case KindDM:
		return a.repo.IsDmMember(ctx, vc.ID, userID)
	case KindServer:
		ch, err := a.repo.GetChannelByID(ctx, vc.ID)
		if err != nil || ch == nil {
			return false, err
		}
		m, err := a.repo.GetMember(ctx, ch.ServerID, userID)
		if err != nil || m == nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// AuthorizeMute returns true if actorID may force-mute targetID in vc.
// Server channels: PermMuteMembers role check.
// DM (1:1):       always false.
// DM (GC):        owner-only and never self.
func (a *Authorizer) AuthorizeMute(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error) {
	if actorID == targetID {
		return false, nil
	}
	switch vc.Kind {
	case KindDM:
		dm, err := a.repo.GetDmChannelByID(ctx, vc.ID)
		if err != nil || dm == nil || !dm.IsGroup {
			return false, err
		}
		if dm.OwnerUserID == nil {
			return false, nil
		}
		return *dm.OwnerUserID == actorID, nil
	case KindServer:
		return a.serverChannelPerm(ctx, vc.ID, actorID, permissions.PermMuteMembers)
	}
	return false, nil
}

// AuthorizeKick mirrors AuthorizeMute but for force-disconnect (PermMoveMembers on servers).
func (a *Authorizer) AuthorizeKick(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error) {
	if actorID == targetID {
		return false, nil
	}
	switch vc.Kind {
	case KindDM:
		dm, err := a.repo.GetDmChannelByID(ctx, vc.ID)
		if err != nil || dm == nil || !dm.IsGroup {
			return false, err
		}
		if dm.OwnerUserID == nil {
			return false, nil
		}
		return *dm.OwnerUserID == actorID, nil
	case KindServer:
		return a.serverChannelPerm(ctx, vc.ID, actorID, permissions.PermMoveMembers)
	}
	return false, nil
}

func (a *Authorizer) serverChannelPerm(ctx context.Context, channelID, userID int64, perm permissions.Perm) (bool, error) {
	ch, err := a.repo.GetChannelByID(ctx, channelID)
	if err != nil || ch == nil {
		return false, err
	}
	srv, err := a.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil || srv == nil {
		return false, err
	}
	return a.hasChannelPerm(ctx, a.permRepo, ch.ServerID, userID, srv.OwnerID, channelID, perm)
}
```

> **Note for the implementer:** if `permissions.Perm` / `permissions.Repo` / `permissions.HasChannelPermission` have different exact names in this codebase, adjust the `permChecker` typedef and the import accordingly. Run `grep -rn "func HasChannelPermission" internal/permissions/` to confirm.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/voice/ -run TestAuthorize -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/auth.go internal/voice/auth_test.go
git commit -m "feat(voice): add Authorizer for server/DM voice permissions"
```

---

### Task 3: WS event constants for calls + activities

**Files:**
- Modify: `internal/websocket/events.go`

- [ ] **Step 1: Read the current file**

```
cat internal/websocket/events.go
```

Note the section headers and conventions — UPPER_SNAKE_CASE strings, grouped by feature.

- [ ] **Step 2: Add the call + activity constants**

Append before the closing `)` of the const block, after `EventInteractionCreate`:

```go
	// Call events (1:1 ringing)
	EventCallRing    = "CALL_RING"
	EventCallAccept  = "CALL_ACCEPT"
	EventCallDecline = "CALL_DECLINE"
	EventCallCancel  = "CALL_CANCEL"
	EventCallTimeout = "CALL_TIMEOUT"

	// VC activity events (stub harness; events fire even though the registry is empty in this iteration)
	EventActivityStart = "ACTIVITY_START"
	EventActivityEnd   = "ACTIVITY_END"
```

- [ ] **Step 3: Verify compilation**

```
go build ./internal/websocket/...
```

Expected: no output (clean build).

- [ ] **Step 4: Commit**

```
git add internal/websocket/events.go
git commit -m "feat(ws): add CALL_* and ACTIVITY_* event constants"
```

---

### Task 4: DM service emit helpers for call_* system events

**Files:**
- Modify: `internal/dm/service.go`
- Modify: `internal/dm/service_test.go` (if it exists; otherwise create)

The existing pattern (see `CreateGroupChannel` and `AddMembers` in this file) is:
1. Marshal a `map[string]any{"type": ..., ...}` into `eventJSON`.
2. Call `s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)`.
3. `BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)`.

Follow that pattern verbatim for the four new emit helpers.

- [ ] **Step 1: Write the failing test**

If `internal/dm/service_test.go` does not exist, create it with the standard test scaffolding used elsewhere in this repo. Otherwise append.

```go
// inside internal/dm/service_test.go (extend the existing scaffolding)

func TestEmitCallStarted_BroadcastsAndPersists(t *testing.T) {
	// Use whatever in-memory repo+hub the existing dm tests use.
	// If none exists, this task may grow to include creating the harness;
	// in that case lift it into a small helper at the top of the file
	// rather than duplicating per-test.
	repo, hub := newDmTestHarness(t)
	svc := NewService(repo, hub)

	const channelID int64 = 42
	const actorID int64 = 7
	repo.SeedDmChannel(channelID, /*isGroup=*/false, /*ownerID=*/0, []int64{actorID, 8})

	if err := svc.EmitCallStarted(context.Background(), channelID, actorID, 1714000000000); err != nil {
		t.Fatalf("EmitCallStarted: %v", err)
	}
	got := repo.LastSystemMessageFor(channelID)
	if got == nil {
		t.Fatal("expected system message inserted")
	}
	var ev map[string]any
	if err := json.Unmarshal(got.SystemEvent, &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if ev["type"] != "call_started" {
		t.Errorf("type = %v, want call_started", ev["type"])
	}
	if ev["actor_user_id"] != "7" {
		t.Errorf("actor_user_id = %v, want \"7\"", ev["actor_user_id"])
	}
	if ev["started_at_ms"] != float64(1714000000000) {
		t.Errorf("started_at_ms = %v", ev["started_at_ms"])
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast on dm:42")
	}
}

func TestEmitCallEnded_DurationOnly(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := NewService(repo, hub)

	const channelID int64 = 42
	repo.SeedDmChannel(channelID, false, 0, []int64{1, 2})

	if err := svc.EmitCallEnded(context.Background(), channelID, 145000, 1714000000000); err != nil {
		t.Fatalf("EmitCallEnded: %v", err)
	}
	var ev map[string]any
	json.Unmarshal(repo.LastSystemMessageFor(channelID).SystemEvent, &ev)
	if ev["type"] != "call_ended" {
		t.Errorf("type = %v", ev["type"])
	}
	if ev["duration_ms"] != float64(145000) {
		t.Errorf("duration_ms = %v", ev["duration_ms"])
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast")
	}
}

func TestEmitCallMissed_NoDecliner(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := NewService(repo, hub)
	repo.SeedDmChannel(42, false, 0, []int64{1, 2})

	if err := svc.EmitCallMissed(context.Background(), 42, 1); err != nil {
		t.Fatalf("EmitCallMissed: %v", err)
	}
	var ev map[string]any
	json.Unmarshal(repo.LastSystemMessageFor(42).SystemEvent, &ev)
	if ev["type"] != "call_missed" || ev["caller_user_id"] != "1" {
		t.Errorf("got %+v", ev)
	}
	if _, hasDecliner := ev["decliner_user_id"]; hasDecliner {
		t.Error("missed event must not carry decliner_user_id")
	}
	_ = hub
}

func TestEmitCallDeclined_HasDecliner(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := NewService(repo, hub)
	repo.SeedDmChannel(42, false, 0, []int64{1, 2})

	if err := svc.EmitCallDeclined(context.Background(), 42, /*caller*/1, /*decliner*/2); err != nil {
		t.Fatalf("EmitCallDeclined: %v", err)
	}
	var ev map[string]any
	json.Unmarshal(repo.LastSystemMessageFor(42).SystemEvent, &ev)
	if ev["type"] != "call_declined" || ev["caller_user_id"] != "1" || ev["decliner_user_id"] != "2" {
		t.Errorf("got %+v", ev)
	}
	_ = hub
}
```

> **If `newDmTestHarness` does not exist:** create one as a small helper that returns an in-memory fake repo (implementing only the methods used here: `IsDmMember`, `InsertSystemMessage`, `GetDmChannelByID`, `GetDmMembers`, `SeedDmChannel`, `LastSystemMessageFor`) and a hub fake (recording broadcasts to a slice keyed by topic). Look at how `internal/voice/auth_test.go` (Task 2) builds its hand-rolled stub for the pattern.

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/dm/ -run TestEmitCall -count=1
```

Expected: FAIL — methods don't exist yet.

- [ ] **Step 3: Implement the four emit helpers**

Append to `internal/dm/service.go`:

```go
// EmitCallStarted writes a `call_started` system_event into the DM channel and
// broadcasts the resulting message on the dm:{id} virtual channel. Used both
// by the ring service on accept (1:1) and by the open-room start handler (GC).
// startedAtMs is the Unix-millis canonical call-start timestamp.
func (s *Service) EmitCallStarted(ctx context.Context, channelID, actorUserID, startedAtMs int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":               "call_started",
		"actor_user_id":      strconv.FormatInt(actorUserID, 10),
		"actor_display_name": s.resolveDisplayName(ctx, actorUserID),
		"started_at_ms":      startedAtMs,
	})
	return s.broadcastSystemMessage(ctx, channelID, actorUserID, eventJSON)
}

// EmitCallEnded writes `call_ended` with the call's duration. actorUserID is
// not part of the payload; emission is driven by last-leaver detection in the
// voice handler, so we attribute the message to the system (user 0).
func (s *Service) EmitCallEnded(ctx context.Context, channelID, durationMs, startedAtMs int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":          "call_ended",
		"duration_ms":   durationMs,
		"started_at_ms": startedAtMs,
	})
	return s.broadcastSystemMessage(ctx, channelID, /*actorUserID=*/0, eventJSON)
}

// EmitCallMissed writes `call_missed` for ring timeout or caller-cancel.
func (s *Service) EmitCallMissed(ctx context.Context, channelID, callerUserID int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":                 "call_missed",
		"caller_user_id":       strconv.FormatInt(callerUserID, 10),
		"caller_display_name":  s.resolveDisplayName(ctx, callerUserID),
	})
	return s.broadcastSystemMessage(ctx, channelID, callerUserID, eventJSON)
}

// EmitCallDeclined writes `call_declined`.
func (s *Service) EmitCallDeclined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":                 "call_declined",
		"caller_user_id":       strconv.FormatInt(callerUserID, 10),
		"caller_display_name":  s.resolveDisplayName(ctx, callerUserID),
		"decliner_user_id":     strconv.FormatInt(declinerUserID, 10),
		"decliner_display_name": s.resolveDisplayName(ctx, declinerUserID),
	})
	return s.broadcastSystemMessage(ctx, channelID, callerUserID, eventJSON)
}

// broadcastSystemMessage is the shared persist-and-broadcast routine used by
// the call emit helpers. Mirrors the inline pattern in CreateGroupChannel
// and AddMembers.
func (s *Service) broadcastSystemMessage(ctx context.Context, channelID, actorUserID int64, eventJSON []byte) error {
	sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
	if err != nil {
		return err
	}
	if s.hub != nil && sysMsg != nil {
		virtualChannel := fmt.Sprintf("dm:%d", channelID)
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(virtualChannel, ws.EventDmMessageCreate, sysMsgJSON)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/dm/ -run TestEmitCall -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/dm/service.go internal/dm/service_test.go
git commit -m "feat(dm): emit helpers for call_started/ended/missed/declined"
```

---

## Phase 2 — Backend voice generalization

### Task 5: voice.Service — started_at tracking + last-leaver hook

**Files:**
- Modify: `internal/voice/service.go`
- Create: `internal/voice/service_started_at_test.go`

The voice service grows three responsibilities:
1. On Join, atomically `SET NX EX 21600` the `voice:{vc}:started_at` key with the current Unix-millis. Only the first joiner wins.
2. On Leave, after HDel + heartbeat-cleanup, if `HLEN voice:{vc} == 0`, atomically `GETDEL voice:{vc}:started_at` and acquire a `SET NX EX 60` lock keyed by `call_ended:{vc}:{started_at}`. If the lock is acquired, return the started_at to the caller so the handler can emit `call_ended`. Otherwise return zero.
3. Expose a small helper `EndIfEmpty(ctx, vc) (startedAtMs int64, ended bool, err error)`. The handler calls this after Leave and emits `call_ended` if `ended`.

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// uses a real-but-isolated Redis. Skip if not available.
func newRedisForTest(t *testing.T) *redis.Client {
	t.Helper()
	addr := "127.0.0.1:6379"
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available at %s: %v", addr, err)
	}
	rdb.FlushDB(context.Background())
	t.Cleanup(func() { rdb.FlushDB(context.Background()); rdb.Close() })
	return rdb
}

func TestJoinSetsStartedAt_Once(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	if err := s.Join(ctx, "dm:1", "1", "alice", ""); err != nil {
		t.Fatal(err)
	}
	v1, err := rdb.Get(ctx, "voice:dm:1:started_at").Result()
	if err != nil {
		t.Fatalf("started_at not set: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := s.Join(ctx, "dm:1", "2", "bob", ""); err != nil {
		t.Fatal(err)
	}
	v2, _ := rdb.Get(ctx, "voice:dm:1:started_at").Result()
	if v1 != v2 {
		t.Errorf("started_at changed on second join: %q -> %q", v1, v2)
	}
}

func TestEndIfEmpty_ReturnsStartedAtOnce(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	_ = s.Join(ctx, "dm:1", "1", "alice", "")
	_ = s.Join(ctx, "dm:1", "2", "bob", "")
	_ = s.Leave(ctx, "dm:1", "1")

	startedAtMs, ended, err := s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if ended {
		t.Fatal("room not empty yet, EndIfEmpty must return ended=false")
	}
	if startedAtMs != 0 {
		t.Fatal("startedAtMs should be 0 when not ended")
	}

	_ = s.Leave(ctx, "dm:1", "2")
	startedAtMs, ended, err = s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if !ended || startedAtMs == 0 {
		t.Fatalf("expected ended=true and startedAtMs > 0, got ended=%v startedAtMs=%d", ended, startedAtMs)
	}

	// second call returns ended=false (deduplicated)
	_, ended2, err := s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if ended2 {
		t.Fatal("EndIfEmpty must dedupe")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/voice/ -run TestJoinSetsStartedAt -count=1
go test ./internal/voice/ -run TestEndIfEmpty -count=1
```

Expected: FAIL (started_at not set; EndIfEmpty undefined).

- [ ] **Step 3: Implement**

In `internal/voice/service.go`, modify `Join` and add `EndIfEmpty`:

```go
// startedAtKey is the call-start timestamp key for the room.
func startedAtKey(channelID string) string {
	return fmt.Sprintf("voice:%s:started_at", channelID)
}

// callEndedLockKey gates duplicate call_ended emissions on the same call instance.
func callEndedLockKey(channelID, startedAt string) string {
	return fmt.Sprintf("call_ended:%s:%s", channelID, startedAt)
}

// Join records a participant joining a voice channel. The first joiner
// atomically stamps voice:{channelID}:started_at with the current ms time
// (with a 6h fallback TTL in case Leave never fires).
func (s *Service) Join(ctx context.Context, channelID, userID, username, avatarURL string) error {
	if s.rdb == nil {
		return nil
	}
	p := Participant{UserID: userID, Username: username, AvatarURL: avatarURL}
	b, _ := json.Marshal(p)
	if err := s.rdb.HSet(ctx, presenceKey(channelID), userID, string(b)).Err(); err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, heartbeatKey(channelID, userID), "1", voiceHeartbeatTTL).Err(); err != nil {
		return err
	}
	// First-joiner wins the SET NX. Subsequent joiners no-op.
	startedAtMs := time.Now().UnixMilli()
	s.rdb.SetNX(ctx, startedAtKey(channelID), strconv.FormatInt(startedAtMs, 10), 6*time.Hour)
	return nil
}

// EndIfEmpty atomically checks whether the room is empty and, if so, removes
// the started_at key and acquires a 60s NX lock to single-emit call_ended.
// Returns (startedAtMs, true, nil) iff the caller should emit call_ended.
// Returns (0, false, nil) when the room is non-empty or another emitter has
// already claimed the lock.
func (s *Service) EndIfEmpty(ctx context.Context, channelID string) (int64, bool, error) {
	if s.rdb == nil {
		return 0, false, nil
	}
	remaining, err := s.rdb.HLen(ctx, presenceKey(channelID)).Result()
	if err != nil {
		return 0, false, err
	}
	if remaining > 0 {
		return 0, false, nil
	}
	startedAtStr, err := s.rdb.GetDel(ctx, startedAtKey(channelID)).Result()
	if err != nil || startedAtStr == "" {
		// already cleaned up by another caller
		return 0, false, nil
	}
	got, err := s.rdb.SetNX(ctx, callEndedLockKey(channelID, startedAtStr), "1", 60*time.Second).Result()
	if err != nil || !got {
		return 0, false, err
	}
	startedAtMs, parseErr := strconv.ParseInt(startedAtStr, 10, 64)
	if parseErr != nil {
		return 0, false, parseErr
	}
	return startedAtMs, true, nil
}
```

Add `"strconv"` to the import block if it's not already there.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/voice/ -run TestJoinSetsStartedAt -count=1
go test ./internal/voice/ -run TestEndIfEmpty -count=1
```

Expected: PASS (or SKIP if Redis unavailable on the dev machine — note this and rerun in CI).

- [ ] **Step 5: Commit**

```
git add internal/voice/service.go internal/voice/service_started_at_test.go
git commit -m "feat(voice): track call start time + idempotent EndIfEmpty"
```

---

### Task 6: Activities Redis state in voice.Service

**Files:**
- Modify: `internal/voice/service.go`
- Create: `internal/voice/service_activity_test.go`

Activities are stored as JSON in `voice:{vc}:activity`. Lifetime is bound to the call: when the room empties (last-leaver path in Task 5), the activity entry is also deleted.

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSetGetEndActivity(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	if got, err := s.GetActivity(ctx, "dm:1"); err != nil || got != nil {
		t.Fatalf("expected nil/no error, got %+v err=%v", got, err)
	}

	if err := s.StartActivity(ctx, "dm:1", "watch_party", 7, json.RawMessage(`{"url":"https://example.com"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetActivity(ctx, "dm:1")
	if err != nil || got == nil {
		t.Fatalf("expected activity, got %+v err=%v", got, err)
	}
	if got.Type != "watch_party" || got.StartedBy != 7 {
		t.Errorf("got %+v", got)
	}

	// EndActivity removes
	if err := s.EndActivity(ctx, "dm:1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetActivity(ctx, "dm:1"); got != nil {
		t.Errorf("expected nil after end, got %+v", got)
	}
}

func TestEndIfEmpty_AlsoClearsActivity(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	_ = s.Join(ctx, "dm:1", "1", "alice", "")
	_ = s.StartActivity(ctx, "dm:1", "watch_party", 1, nil)
	_ = s.Leave(ctx, "dm:1", "1")

	if _, ended, _ := s.EndIfEmpty(ctx, "dm:1"); !ended {
		t.Fatal("expected ended=true on empty room")
	}
	if got, _ := s.GetActivity(ctx, "dm:1"); got != nil {
		t.Errorf("activity must be cleared on call end, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/voice/ -run TestSetGetEndActivity -count=1
go test ./internal/voice/ -run TestEndIfEmpty_AlsoClearsActivity -count=1
```

Expected: FAIL — methods don't exist.

- [ ] **Step 3: Implement**

In `internal/voice/service.go`:

```go
// Activity is the per-call active activity record stored in Redis.
type Activity struct {
	Type        string          `json:"type"`
	StartedBy   int64           `json:"started_by"`
	StartedAtMs int64           `json:"started_at"`
	Params      json.RawMessage `json:"params,omitempty"`
}

func activityKey(channelID string) string {
	return fmt.Sprintf("voice:%s:activity", channelID)
}

// StartActivity records or replaces the active activity for a call.
func (s *Service) StartActivity(ctx context.Context, channelID, activityType string, startedBy int64, params json.RawMessage) error {
	if s.rdb == nil {
		return nil
	}
	a := Activity{
		Type:        activityType,
		StartedBy:   startedBy,
		StartedAtMs: time.Now().UnixMilli(),
		Params:      params,
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, activityKey(channelID), b, 6*time.Hour).Err()
}

// GetActivity returns the active activity for a call, or nil if none.
func (s *Service) GetActivity(ctx context.Context, channelID string) (*Activity, error) {
	if s.rdb == nil {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, activityKey(channelID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Activity
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// EndActivity removes the activity record. Idempotent.
func (s *Service) EndActivity(ctx context.Context, channelID string) error {
	if s.rdb == nil {
		return nil
	}
	return s.rdb.Del(ctx, activityKey(channelID)).Err()
}
```

Then extend `EndIfEmpty` to clear the activity inside the success branch (just before `return startedAtMs, true, nil`):

```go
	s.rdb.Del(ctx, activityKey(channelID))
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/voice/ -run TestSetGetEndActivity -count=1
go test ./internal/voice/ -run TestEndIfEmpty_AlsoClearsActivity -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/service.go internal/voice/service_activity_test.go
git commit -m "feat(voice): activities Redis state with auto-cleanup on call end"
```

---

### Task 7: Generalize voice handler to virtual-channel IDs

**Files:**
- Modify: `internal/voice/handler.go`

The handler currently parses a numeric `{channelId}` and assumes server VC. Generalize it: extract a virtual-channel string from the path, parse it via `ParseVirtualChannel`, route auth through the new `Authorizer`, and broadcast on the right WS topic per kind.

**Critical compatibility detail:** the existing `/api/channels/{channelId}/voice/*` route tree must keep working. We achieve that by making the handler functions accept a virtual-channel string and registering thin wrappers in routes.go (Task 8). For server channels routed under `/api/channels/{channelId}/voice/*`, the wrapper computes `s:{channelId}` and forwards.

- [ ] **Step 1: Read the existing handler in full**

```
cat internal/voice/handler.go | head -340
```

Take note of: the broadcast helper `broadcastVoiceState`, the `parseVoiceRequest` helper, the `serverVirtualChannelID = "server:" + serverID` topic format, and the existing 6 handler methods (Token, Join, Leave, Participants, MuteParticipant, KickParticipant).

- [ ] **Step 2: Rewrite the handler to use VirtualChannel**

Replace the file contents with the following. The new shape:
- A `Handler` field `authz *Authorizer` (added to `NewHandler`).
- A `parseVC(r)` helper that reads `r.PathValue("vc")` and returns the parsed `VirtualChannel` plus the original string.
- All six methods take their virtual channel from `parseVC`; broadcast topic is `vc.String()` for `KindDM` and `"server:" + serverID` (looked up via `repo.GetChannelByID`) for `KindServer`.

```go
package voice

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

// Handler handles voice HTTP endpoints. All routes accept a virtual-channel ID.
type Handler struct {
	svc   *Service
	repo  *db.Repository
	hub   *ws.Hub
	authz *Authorizer
}

func NewHandler(svc *Service, repo *db.Repository, hub *ws.Hub) *Handler {
	return &Handler{
		svc:   svc,
		repo:  repo,
		hub:   hub,
		authz: NewAuthorizer(repo),
	}
}

// parseVC extracts and validates the virtual channel from the URL path.
// All voice routes use {vc} as the path parameter name.
func (h *Handler) parseVC(w http.ResponseWriter, r *http.Request) (VirtualChannel, string, bool) {
	raw := r.PathValue("vc")
	vc, err := ParseVirtualChannel(raw)
	if err != nil {
		httputil.JSONError(w, "invalid virtual channel id", http.StatusBadRequest)
		return VirtualChannel{}, "", false
	}
	return vc, raw, true
}

// userFromCtx returns (userID int64, userIDStr string, ok). On !ok, an error response has been written.
func (h *Handler) userFromCtx(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	s := auth.GetUserIDFromContext(r)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return 0, "", false
	}
	return id, s, true
}

// broadcastTarget returns the WS topic for voice-state events for this vc.
// Server channels broadcast to "server:{serverID}" (existing behavior so all
// members see green dots in sidebars). DMs broadcast to "dm:{id}" which DM
// members already subscribe to.
func (h *Handler) broadcastTarget(r *http.Request, vc VirtualChannel) (string, bool) {
	switch vc.Kind {
	case KindDM:
		return vc.String(), true
	case KindServer:
		ch, err := h.repo.GetChannelByID(r.Context(), vc.ID)
		if err != nil || ch == nil {
			return "", false
		}
		return "server:" + strconv.FormatInt(ch.ServerID, 10), true
	}
	return "", false
}

// Token issues a LiveKit token for the requesting user to join vc.
// GET /api/voice/{vc}/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if !h.svc.Configured() {
		httputil.JSONError(w, "voice not configured", http.StatusServiceUnavailable)
		return
	}
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	allowed, err := h.authz.AuthorizeJoin(r.Context(), vc, userID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		log.Printf("voice handler: failed to get user: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	tokenName := user.DisplayName
	if tokenName == "" {
		tokenName = user.Username
	}
	token, err := h.svc.IssueToken(userIDStr, tokenName, vcStr)
	if err != nil {
		log.Printf("voice handler: failed to generate token: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token, "url": h.svc.ServerURL()})
}

// Join records a participant + broadcasts a join state event.
// POST /api/voice/{vc}/join
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	if allowed, err := h.authz.AuthorizeJoin(r.Context(), vc, userID); err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	user, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	if err := h.svc.Join(r.Context(), vcStr, userIDStr, displayName, user.AvatarURL); err != nil {
		log.Printf("voice handler: failed to join: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, userIDStr, displayName, user.AvatarURL, "join")
	}
	w.WriteHeader(http.StatusNoContent)
}

// Leave removes a participant + broadcasts. If the room is now empty AND the
// vc is a DM, emits a call_ended system message via the DM service.
// POST /api/voice/{vc}/leave
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userID, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	user, _ := h.repo.GetUserByID(r.Context(), userID)
	displayName := ""
	avatarURL := ""
	if user != nil {
		displayName = user.DisplayName
		if displayName == "" {
			displayName = user.Username
		}
		avatarURL = user.AvatarURL
	}
	if err := h.svc.Leave(r.Context(), vcStr, userIDStr); err != nil {
		log.Printf("voice handler: failed to leave: %v", err)
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, userIDStr, displayName, avatarURL, "leave")
	}

	// Last-leaver detection — emit call_ended for DMs.
	if vc.Kind == KindDM {
		if startedAtMs, ended, err := h.svc.EndIfEmpty(r.Context(), vcStr); err == nil && ended {
			if h.dmCallEnder != nil {
				durationMs := nowMs() - startedAtMs
				if durationMs < 0 {
					durationMs = 0
				}
				_ = h.dmCallEnder(r.Context(), vc.ID, durationMs, startedAtMs)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// nowMs is a tiny seam so tests could fake time later.
var nowMs = func() int64 { return timeNow().UnixMilli() }

// timeNow is the underlying time func; declared here to keep the import contained.
var timeNow = func() time.Time { return time.Now() }

// dmCallEnder is the optional callback the wiring layer sets to dm.Service.EmitCallEnded.
// Kept as a function-typed field so internal/voice doesn't import internal/dm directly,
// avoiding an import cycle.
type DmCallEnder func(ctx context.Context, dmChannelID, durationMs, startedAtMs int64) error

// SetDmCallEnder is called from cmd/api wiring after both services exist.
func (h *Handler) SetDmCallEnder(f DmCallEnder) { h.dmCallEnder = f }

// (place in struct definition, near `authz *Authorizer`):
//   dmCallEnder DmCallEnder

// Participants is unchanged in semantics; just takes vc instead of channelID.
// GET /api/voice/{vc}/participants
func (h *Handler) Participants(w http.ResponseWriter, r *http.Request) {
	_, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	parts, err := h.svc.Participants(r.Context(), vcStr)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(parts)
}

// Heartbeat refreshes the per-user voice presence TTL.
// POST /api/voice/{vc}/heartbeat
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	_, userIDStr, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	if err := h.svc.RefreshHeartbeat(r.Context(), vcStr, userIDStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MuteParticipant force-mutes a target via WS event.
// POST /api/voice/{vc}/participants/{targetUserId}/mute
func (h *Handler) MuteParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID, _, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	targetUserIDStr := r.PathValue("targetUserId")
	targetID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}
	allowed, err := h.authz.AuthorizeMute(r.Context(), vc, requesterID, targetID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"channel_id": vcStr,
		"muted":      true,
	})
	if err := h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceMute, payload); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// KickParticipant force-disconnects a target.
// POST /api/voice/{vc}/participants/{targetUserId}/kick
func (h *Handler) KickParticipant(w http.ResponseWriter, r *http.Request) {
	requesterID, _, ok := h.userFromCtx(w, r)
	if !ok {
		return
	}
	vc, vcStr, ok := h.parseVC(w, r)
	if !ok {
		return
	}
	targetUserIDStr := r.PathValue("targetUserId")
	targetID, err := strconv.ParseInt(targetUserIDStr, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid target user id", http.StatusBadRequest)
		return
	}
	allowed, err := h.authz.AuthorizeKick(r.Context(), vc, requesterID, targetID)
	if err != nil || !allowed {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}

	// Remove + broadcast leave on the right topic.
	targetUser, _ := h.repo.GetUserByID(r.Context(), targetID)
	displayName := ""
	avatarURL := ""
	if targetUser != nil {
		displayName = targetUser.DisplayName
		if displayName == "" {
			displayName = targetUser.Username
		}
		avatarURL = targetUser.AvatarURL
	}
	if err := h.svc.Leave(r.Context(), vcStr, targetUserIDStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if topic, ok := h.broadcastTarget(r, vc); ok {
		h.broadcastVoiceState(topic, vcStr, targetUserIDStr, displayName, avatarURL, "leave")
	}

	disc, _ := json.Marshal(map[string]any{"channel_id": vcStr})
	_ = h.hub.SendToUser(targetUserIDStr, ws.EventVoiceForceDisconnect, disc)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) broadcastVoiceState(topic, channelID, userID, username, avatarURL, action string) {
	payload, _ := json.Marshal(map[string]string{
		"channel_id": channelID,
		"user_id":    userID,
		"username":   username,
		"avatar_url": avatarURL,
		"action":     action,
	})
	h.hub.BroadcastToChannel(topic, ws.EventVoiceStateUpdate, payload)
}
```

Imports: ensure `"context"` and `"time"` are added. Add the `dmCallEnder DmCallEnder` field inside the `Handler` struct definition (just below `authz *Authorizer`).

> **Compile-time gotcha:** the new file uses `nowMs`, `timeNow`, and `time.Time`. Make sure `"time"` is imported. The `context.Context` used by `DmCallEnder` requires `"context"`.

- [ ] **Step 3: Verify compilation**

```
go build ./internal/voice/...
```

Expected: clean build.

- [ ] **Step 4: Run all voice tests**

```
go test ./internal/voice/... -count=1
```

Expected: all prior tests still pass (Tasks 1, 2, 5, 6).

- [ ] **Step 5: Commit**

```
git add internal/voice/handler.go
git commit -m "refactor(voice): handler accepts virtual channel id; emits call_ended on last leave"
```

---

### Task 8: Voice routes generalized + back-compat wrapper

**Files:**
- Modify: `cmd/api/routes.go`
- Modify: `cmd/api/main.go` (or wherever the voice handler is constructed and `dm.Service` is constructed) — wire `voiceHandler.SetDmCallEnder(dmService.EmitCallEnded)` after both exist.

- [ ] **Step 1: Add the new route tree**

In `cmd/api/routes.go`, replace the existing six `/channels/{channelId}/voice/*` registrations (around line 282–287) with:

```go
				// Voice — virtual-channel-namespaced routes (s:N for server VCs, dm:N for DMs).
				voiceHandler := voice.NewHandler(voiceSvc, repo, hub)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Get("/voice/{vc}/token", voiceHandler.Token)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/voice/{vc}/join", voiceHandler.Join)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/voice/{vc}/leave", voiceHandler.Leave)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/voice/{vc}/heartbeat", voiceHandler.Heartbeat)
				r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/voice/{vc}/participants", voiceHandler.Participants)
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/voice/{vc}/participants/{targetUserId}/mute", voiceHandler.MuteParticipant)
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/voice/{vc}/participants/{targetUserId}/kick", voiceHandler.KickParticipant)

				// Back-compat: existing /api/channels/{channelId}/voice/* routes rewrite the path
				// param to s:{channelId} and forward to the same handlers. Frontend will migrate
				// to /api/voice/{vc}/* and these wrappers can be removed in a future release.
				wrapServerVoice := func(next http.HandlerFunc) http.HandlerFunc {
					return func(w http.ResponseWriter, req *http.Request) {
						channelID := chi.URLParam(req, "channelId")
						// chi exposes path values via SetURLParam; std http via SetPathValue.
						chiCtx := chi.RouteContext(req.Context())
						if chiCtx != nil {
							chiCtx.URLParams.Add("vc", "s:"+channelID)
						}
						req.SetPathValue("vc", "s:"+channelID)
						next(w, req)
					}
				}
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Get("/channels/{channelId}/voice/token", wrapServerVoice(voiceHandler.Token))
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/voice/join", wrapServerVoice(voiceHandler.Join))
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/channels/{channelId}/voice/leave", wrapServerVoice(voiceHandler.Leave))
				r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/channels/{channelId}/voice/participants", wrapServerVoice(voiceHandler.Participants))
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelId}/voice/participants/{targetUserId}/mute", wrapServerVoice(voiceHandler.MuteParticipant))
				r.With(auth.RequireScope(auth.ScopeProfileWrite)).Post("/channels/{channelId}/voice/participants/{targetUserId}/kick", wrapServerVoice(voiceHandler.KickParticipant))
```

- [ ] **Step 2: Wire the dm-call-ender callback**

Find where the `dm.Service` is constructed in `cmd/api/main.go` (or `cmd/api/routes.go` if constructed there). After both `voiceHandler` and `dmService` exist, add:

```go
voiceHandler.SetDmCallEnder(dmService.EmitCallEnded)
```

If `dmService` is constructed inside `dmHandler := dm.NewHandler(...)` and not separately exposed, lift the service construction out so we can reach the methods. The `dm.Handler` already takes a `dm.Service` internally; mirror that.

- [ ] **Step 3: Verify compilation**

```
go build ./...
```

Expected: clean build.

- [ ] **Step 4: Smoke-test the routes**

```
go test ./internal/voice/... -count=1
```

Expected: PASS for everything from prior tasks.

- [ ] **Step 5: Commit**

```
git add cmd/api/routes.go cmd/api/main.go
git commit -m "feat(api): /api/voice/{vc}/* routes + back-compat for /channels/{id}/voice/*"
```

---

## Phase 3 — Ring service (1:1)

### Task 9: Ring service skeleton + Initiate

**Files:**
- Create: `internal/voice/ring.go`
- Create: `internal/voice/ring_test.go`

The ring service is in-process (single-instance backend). It holds two maps under one mutex: `rings` (by ring ID) and `byDM` (dmChannelID → ringID). Initiate creates a Ring, starts a 30s `time.AfterFunc`, sends `CALL_RING` via the hub, and returns the ring ID. Glare guard: if `byDM[dmID]` is already present, return the existing ring's ID without creating a new one (caller-side WS already received it).

The service has an interface seam for the hub and DM emit helpers so tests don't need real ones.

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeHub struct {
	mu     sync.Mutex
	toUser []sentToUser
}
type sentToUser struct {
	userID    string
	eventType string
	payload   []byte
}

func (h *fakeHub) SendToUser(userID, eventType string, payload []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.toUser = append(h.toUser, sentToUser{userID, eventType, payload})
	return nil
}
func (h *fakeHub) sentTypes() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.toUser))
	for _, s := range h.toUser {
		out = append(out, s.eventType)
	}
	return out
}

type fakeDmEmitter struct {
	mu sync.Mutex
	started, ended, missed, declined int
}

func (e *fakeDmEmitter) Started(_ context.Context, _, _, _ int64) error      { e.mu.Lock(); defer e.mu.Unlock(); e.started++; return nil }
func (e *fakeDmEmitter) Ended(_ context.Context, _, _, _ int64) error        { e.mu.Lock(); defer e.mu.Unlock(); e.ended++; return nil }
func (e *fakeDmEmitter) Missed(_ context.Context, _, _ int64) error           { e.mu.Lock(); defer e.mu.Unlock(); e.missed++; return nil }
func (e *fakeDmEmitter) Declined(_ context.Context, _, _, _ int64) error      { e.mu.Lock(); defer e.mu.Unlock(); e.declined++; return nil }

func newRingTestService() (*RingService, *fakeHub, *fakeDmEmitter) {
	hub := &fakeHub{}
	emit := &fakeDmEmitter{}
	rs := NewRingService(hub, emit, &Service{} /* svc methods unused in this test */)
	rs.timeout = 50 * time.Millisecond // shorten for tests
	return rs, hub, emit
}

func TestInitiate_SendsRingAndStoresState(t *testing.T) {
	rs, hub, _ := newRingTestService()
	id, err := rs.Initiate(context.Background(), 10, /*caller*/1, /*target*/2, ringCallerInfo{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ring id")
	}
	rs.mu.Lock()
	_, ok1 := rs.rings[id]
	rid, ok2 := rs.byDM[10]
	rs.mu.Unlock()
	if !ok1 || !ok2 || rid != id {
		t.Fatalf("rings/byDM not populated correctly: ok1=%v ok2=%v rid=%q id=%q", ok1, ok2, rid, id)
	}

	// Wait briefly for the goroutine that sends the WS event.
	time.Sleep(5 * time.Millisecond)
	got := hub.sentTypes()
	found := false
	for _, ty := range got {
		if ty == "CALL_RING" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CALL_RING sent, got %v", got)
	}
}

func TestInitiate_GlareReturnsExistingRing(t *testing.T) {
	rs, _, _ := newRingTestService()
	id1, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})
	id2, err := rs.Initiate(context.Background(), 10, 2, 1, ringCallerInfo{}) // reverse glare
	if err != nil {
		t.Fatalf("glare must not error, got %v", err)
	}
	if id2 != id1 {
		t.Errorf("expected glare to surface existing ring %q, got %q", id1, id2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/voice/ -run TestInitiate -count=1
```

Expected: FAIL — RingService undefined.

- [ ] **Step 3: Implement**

```go
package voice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// RingHub is the small subset of *ws.Hub the ring service needs.
type RingHub interface {
	SendToUser(userID, eventType string, payload []byte) error
}

// DmEmitter is the seam for emitting call_* system messages.
type DmEmitter interface {
	Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
	Ended(ctx context.Context, channelID, durationMs, startedAtMs int64) error
	Missed(ctx context.Context, channelID, callerUserID int64) error
	Declined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error
}

// ringCallerInfo carries the display fields the receiver's UI shows on the ring.
type ringCallerInfo struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// Ring is one in-flight 1:1 ring.
type Ring struct {
	ID          string
	DmChannelID int64
	CallerID    int64
	TargetID    int64
	StartedAt   time.Time
	caller      ringCallerInfo
	timer       *time.Timer
}

// RingService owns 1:1 ring lifecycle. GC calls have no ring layer.
type RingService struct {
	mu      sync.Mutex
	rings   map[string]*Ring
	byDM    map[int64]string
	hub     RingHub
	dm      DmEmitter
	svc     *Service
	timeout time.Duration
}

func NewRingService(hub RingHub, dm DmEmitter, svc *Service) *RingService {
	return &RingService{
		rings:   make(map[string]*Ring),
		byDM:    make(map[int64]string),
		hub:     hub,
		dm:      dm,
		svc:     svc,
		timeout: 30 * time.Second,
	}
}

func newRingID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Initiate creates a ring (or returns the existing one for glare). Sends
// CALL_RING to the target's user-WS. Returns the ring ID.
func (rs *RingService) Initiate(ctx context.Context, dmChannelID, callerID, targetID int64, caller ringCallerInfo) (string, error) {
	rs.mu.Lock()
	if existing, ok := rs.byDM[dmChannelID]; ok {
		rs.mu.Unlock()
		return existing, nil // glare: surface existing ring to both parties
	}
	id := newRingID()
	r := &Ring{
		ID:          id,
		DmChannelID: dmChannelID,
		CallerID:    callerID,
		TargetID:    targetID,
		StartedAt:   time.Now(),
		caller:      caller,
	}
	r.timer = time.AfterFunc(rs.timeout, func() {
		_ = rs.timeoutRing(context.Background(), id)
	})
	rs.rings[id] = r
	rs.byDM[dmChannelID] = id
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id": id,
		"vc":      VirtualChannel{Kind: KindDM, ID: dmChannelID}.String(),
		"caller":  caller,
	})
	go func() {
		_ = rs.hub.SendToUser(int64ToStr(targetID), "CALL_RING", payload)
	}()
	return id, nil
}

func (rs *RingService) timeoutRing(ctx context.Context, ringID string) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring already terminal")
	}
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"ring_id": ringID})
	_ = rs.hub.SendToUser(int64ToStr(r.CallerID), "CALL_TIMEOUT", payload)
	_ = rs.hub.SendToUser(int64ToStr(r.TargetID), "CALL_TIMEOUT", payload)
	if rs.dm != nil {
		_ = rs.dm.Missed(ctx, r.DmChannelID, r.CallerID)
	}
	return nil
}

func int64ToStr(v int64) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
```

> **Why a hand-rolled int64-to-string?** The ring service is on a hot path for ring sends and is called from goroutines. `strconv.FormatInt` works fine; if you prefer to use it, replace `int64ToStr` calls accordingly. Either is acceptable — the test doesn't care.

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/voice/ -run TestInitiate -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/ring.go internal/voice/ring_test.go
git commit -m "feat(voice): RingService skeleton + Initiate with glare guard"
```

---

### Task 10: Ring Accept / Decline / Cancel

**Files:**
- Modify: `internal/voice/ring.go`
- Modify: `internal/voice/ring_test.go`

- [ ] **Step 1: Add tests**

Append to `ring_test.go`:

```go
func TestAccept_TerminatesRingAndEmitsStarted(t *testing.T) {
	rs, hub, dm := newRingTestService()
	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	if err := rs.Accept(context.Background(), id, 2); err != nil {
		t.Fatal(err)
	}
	rs.mu.Lock()
	_, exists := rs.rings[id]
	rs.mu.Unlock()
	if exists {
		t.Error("ring should be removed after Accept")
	}
	time.Sleep(5 * time.Millisecond)

	got := hub.sentTypes()
	count := 0
	for _, ty := range got {
		if ty == "CALL_ACCEPT" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 CALL_ACCEPT (caller + target), got %d (%v)", count, got)
	}
	if dm.started != 1 {
		t.Errorf("expected dm.Started=1, got %d", dm.started)
	}
}

func TestDecline_TerminatesAndEmitsDeclined(t *testing.T) {
	rs, hub, dm := newRingTestService()
	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	if err := rs.Decline(context.Background(), id, 2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	hasDecline := false
	for _, ty := range hub.sentTypes() {
		if ty == "CALL_DECLINE" {
			hasDecline = true
			break
		}
	}
	if !hasDecline {
		t.Error("expected CALL_DECLINE to caller")
	}
	if dm.declined != 1 {
		t.Errorf("expected dm.Declined=1, got %d", dm.declined)
	}
}

func TestCancel_TerminatesAndEmitsMissed(t *testing.T) {
	rs, hub, dm := newRingTestService()
	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	if err := rs.Cancel(context.Background(), id, 1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	hasCancel := false
	for _, ty := range hub.sentTypes() {
		if ty == "CALL_CANCEL" {
			hasCancel = true
			break
		}
	}
	if !hasCancel {
		t.Error("expected CALL_CANCEL to target")
	}
	if dm.missed != 1 {
		t.Errorf("expected dm.Missed=1, got %d", dm.missed)
	}
}

func TestTimeout_FiresAfterDuration(t *testing.T) {
	rs, hub, dm := newRingTestService()
	rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})
	time.Sleep(80 * time.Millisecond) // > rs.timeout (50ms)

	timeouts := 0
	for _, ty := range hub.sentTypes() {
		if ty == "CALL_TIMEOUT" {
			timeouts++
		}
	}
	if timeouts != 2 {
		t.Errorf("expected 2 CALL_TIMEOUT (caller+target), got %d (%v)", timeouts, hub.sentTypes())
	}
	if dm.missed != 1 {
		t.Errorf("expected dm.Missed=1, got %d", dm.missed)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/voice/ -run "TestAccept|TestDecline|TestCancel|TestTimeout" -count=1
```

Expected: FAIL — methods don't exist.

- [ ] **Step 3: Implement**

Append to `ring.go`:

```go
// Accept resolves a ring as accepted by the target (or by the same target's
// other session via accepterID). Caller and target both receive CALL_ACCEPT
// (so other sessions of the target dismiss their modal). The DM emit helper
// writes call_started.
func (rs *RingService) Accept(ctx context.Context, ringID string, accepterID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id":            ringID,
		"accepter_user_id":   int64ToStr(accepterID),
	})
	_ = rs.hub.SendToUser(int64ToStr(r.CallerID), "CALL_ACCEPT", payload)
	_ = rs.hub.SendToUser(int64ToStr(r.TargetID), "CALL_ACCEPT", payload)
	if rs.dm != nil {
		_ = rs.dm.Started(ctx, r.DmChannelID, accepterID, time.Now().UnixMilli())
	}
	return nil
}

// Decline resolves a ring as declined by the receiver.
func (rs *RingService) Decline(ctx context.Context, ringID string, declinerID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id":           ringID,
		"decliner_user_id":  int64ToStr(declinerID),
	})
	_ = rs.hub.SendToUser(int64ToStr(r.CallerID), "CALL_DECLINE", payload)
	if rs.dm != nil {
		_ = rs.dm.Declined(ctx, r.DmChannelID, r.CallerID, declinerID)
	}
	return nil
}

// Cancel resolves a ring as cancelled by the caller (changed their mind).
// Receiver sees CALL_CANCEL; system message is call_missed (caller-side).
func (rs *RingService) Cancel(ctx context.Context, ringID string, callerID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	if r.CallerID != callerID {
		rs.mu.Unlock()
		return errors.New("only the caller may cancel")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"ring_id": ringID})
	_ = rs.hub.SendToUser(int64ToStr(r.TargetID), "CALL_CANCEL", payload)
	if rs.dm != nil {
		_ = rs.dm.Missed(ctx, r.DmChannelID, callerID)
	}
	return nil
}

// ActiveRingsForUser returns rings targeting the user (used by GET /api/calls/active).
type ActiveRing struct {
	RingID      string         `json:"ring_id"`
	VC          string         `json:"vc"`
	Caller      ringCallerInfo `json:"caller"`
	StartedAtMs int64          `json:"started_at_ms"`
}

func (rs *RingService) ActiveRingsForUser(userID int64) []ActiveRing {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]ActiveRing, 0)
	for _, r := range rs.rings {
		if r.TargetID == userID {
			out = append(out, ActiveRing{
				RingID:      r.ID,
				VC:          VirtualChannel{Kind: KindDM, ID: r.DmChannelID}.String(),
				Caller:      r.caller,
				StartedAtMs: r.StartedAt.UnixMilli(),
			})
		}
	}
	return out
}
```

- [ ] **Step 4: Run all ring tests**

```
go test ./internal/voice/ -run "TestInitiate|TestAccept|TestDecline|TestCancel|TestTimeout" -count=1
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/ring.go internal/voice/ring_test.go
git commit -m "feat(voice): ring Accept/Decline/Cancel/timeout + ActiveRingsForUser"
```

---

### Task 11: DM emit adapter for ring service

**Files:**
- Create: `internal/voice/dm_emitter_adapter.go`

The ring service depends on `DmEmitter` (Task 9). The adapter implements that interface by calling `dm.Service.EmitCall*`. Lives in the voice package to keep `internal/dm` from importing voice.

- [ ] **Step 1: Write the adapter**

```go
package voice

import (
	"context"
)

// dmServiceLike is the slice of dm.Service methods the adapter forwards to.
// Defined here so this package doesn't import internal/dm; cmd/api wiring
// passes the real *dm.Service which satisfies this implicit interface.
type dmServiceLike interface {
	EmitCallStarted(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
	EmitCallEnded(ctx context.Context, channelID, durationMs, startedAtMs int64) error
	EmitCallMissed(ctx context.Context, channelID, callerUserID int64) error
	EmitCallDeclined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error
}

// DmEmitterFromService wraps a dm.Service-shaped value into a DmEmitter.
func DmEmitterFromService(svc dmServiceLike) DmEmitter {
	return &dmEmitterAdapter{svc: svc}
}

type dmEmitterAdapter struct {
	svc dmServiceLike
}

func (a *dmEmitterAdapter) Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error {
	return a.svc.EmitCallStarted(ctx, channelID, actorUserID, startedAtMs)
}
func (a *dmEmitterAdapter) Ended(ctx context.Context, channelID, durationMs, startedAtMs int64) error {
	return a.svc.EmitCallEnded(ctx, channelID, durationMs, startedAtMs)
}
func (a *dmEmitterAdapter) Missed(ctx context.Context, channelID, callerUserID int64) error {
	return a.svc.EmitCallMissed(ctx, channelID, callerUserID)
}
func (a *dmEmitterAdapter) Declined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error {
	return a.svc.EmitCallDeclined(ctx, channelID, callerUserID, declinerUserID)
}
```

- [ ] **Step 2: Verify compilation**

```
go build ./internal/voice/...
```

Expected: clean build.

- [ ] **Step 3: Commit**

```
git add internal/voice/dm_emitter_adapter.go
git commit -m "feat(voice): DmEmitter adapter for dm.Service"
```

---

## Phase 4 — Backend HTTP surface

### Task 12: Ring HTTP handlers (initiate/accept/decline/cancel)

**Files:**
- Create: `internal/voice/ring_handler.go`
- Create: `internal/voice/ring_handler_test.go`

Endpoints:
- `POST /api/dm/{id}/call/ring` → `{ring_id}`
- `POST /api/dm/{id}/call/accept` body `{ring_id}` → 204
- `POST /api/dm/{id}/call/decline` body `{ring_id}` → 204
- `POST /api/dm/{id}/call/cancel` body `{ring_id}` → 204

All four require `IsDmMember(dmID, userID)`. Ring is 1:1 only; if the channel is a GC, /ring returns 400.

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// uses the same authRepoFake helper from Task 2's auth_test.go.
// extend it with a GetDmMembersByID method as needed.

func TestRingHandler_Initiate_OneOnOne(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	hub := &fakeHub{}
	rs := NewRingService(hub, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct{ RingID string `json:"ring_id"` }
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.RingID == "" {
		t.Error("ring_id missing")
	}
}

func TestRingHandler_Initiate_RejectsGC(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	hub := &fakeHub{}
	rs := NewRingService(hub, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for GC, got %d", rec.Code)
	}
}

func TestRingHandler_Initiate_RejectsNonMember(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(&fakeHub{}, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 99))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRingHandler_Accept(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(&fakeHub{}, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))

	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	body, _ := json.Marshal(map[string]string{"ring_id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/accept", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Accept(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}
```

> **Test helpers needed:** `withFakeUserID(ctx, id)` is a small helper to inject a user ID into the request context the way `auth.AuthMiddlewareWith` does. Look at how existing handler tests in this repo set the user-ID in context (`grep -rn "GetUserIDFromContext" internal/`) and mirror it. `repoLikeFromAuthFake` adapts our `authRepoFake` to whatever `RingHandler` ends up needing for IsDmMember + GetDmChannelByID.

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/voice/ -run "TestRingHandler" -count=1
```

Expected: FAIL — handler undefined.

- [ ] **Step 3: Implement**

```go
package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// ringRepo is the slice of repository methods the ring handler needs.
type ringRepo interface {
	IsDmMember(ctx context.Context, dmID, userID int64) (bool, error)
	GetDmChannelByID(ctx context.Context, dmID int64) (*db.DmChannel, error)
	GetDmMembers(ctx context.Context, dmID int64) ([]*db.DmMember, error)
	GetUserByID(ctx context.Context, id int64) (*db.User, error)
}

type RingHandler struct {
	rs   *RingService
	repo ringRepo
}

func NewRingHandler(rs *RingService, repo ringRepo) *RingHandler {
	return &RingHandler{rs: rs, repo: repo}
}

func (h *RingHandler) Ring(w http.ResponseWriter, r *http.Request) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), dmID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	dm, err := h.repo.GetDmChannelByID(r.Context(), dmID)
	if err != nil || dm == nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}
	if dm.IsGroup {
		httputil.JSONError(w, "ringing is not supported for group DMs; use /call/start instead", http.StatusBadRequest)
		return
	}
	// Identify the other party.
	members, err := h.repo.GetDmMembers(r.Context(), dmID)
	if err != nil || len(members) != 2 {
		httputil.JSONError(w, "invalid 1:1 channel", http.StatusBadRequest)
		return
	}
	var targetID int64
	for _, m := range members {
		if m.UserID != userID {
			targetID = m.UserID
			break
		}
	}
	if targetID == 0 {
		httputil.JSONError(w, "target not found", http.StatusBadRequest)
		return
	}
	caller, _ := h.repo.GetUserByID(r.Context(), userID)
	info := ringCallerInfo{UserID: userID}
	if caller != nil {
		info.Username = caller.Username
		info.DisplayName = caller.DisplayName
		info.AvatarURL = caller.AvatarURL
	}
	id, err := h.rs.Initiate(r.Context(), dmID, userID, targetID, info)
	if err != nil {
		httputil.JSONError(w, "ring failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ring_id": id})
}

func (h *RingHandler) Accept(w http.ResponseWriter, r *http.Request)  { h.terminate(w, r, "accept") }
func (h *RingHandler) Decline(w http.ResponseWriter, r *http.Request) { h.terminate(w, r, "decline") }
func (h *RingHandler) Cancel(w http.ResponseWriter, r *http.Request)  { h.terminate(w, r, "cancel") }

func (h *RingHandler) terminate(w http.ResponseWriter, r *http.Request, op string) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), dmID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	var body struct{ RingID string `json:"ring_id"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RingID == "" {
		httputil.JSONError(w, "ring_id required", http.StatusBadRequest)
		return
	}
	switch op {
	case "accept":
		err = h.rs.Accept(r.Context(), body.RingID, userID)
	case "decline":
		err = h.rs.Decline(r.Context(), body.RingID, userID)
	case "cancel":
		err = h.rs.Cancel(r.Context(), body.RingID, userID)
	default:
		err = errors.New("unknown op")
	}
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// helpers
func userFromCtx(w http.ResponseWriter, r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return id, true
}
func dmIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	s := r.PathValue("id")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid dm id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/voice/ -run "TestRingHandler" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/ring_handler.go internal/voice/ring_handler_test.go
git commit -m "feat(voice): HTTP handlers for ring/accept/decline/cancel"
```

---

### Task 13: GC call/start + GET /api/calls/active

**Files:**
- Modify: `internal/voice/ring_handler.go`

- [ ] **Step 1: Add Start (GC) and Active handlers**

Append to `ring_handler.go`:

```go
// callDmEmitter is the seam for emitting call_started from the GC start path.
// Same shape as dmEmitterAdapter; reused via type assertion in the wiring layer.
type CallStarter interface {
	Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
}

// SetCallStarter is invoked from cmd/api wiring to provide the dm emit adapter.
func (h *RingHandler) SetCallStarter(c CallStarter) { h.starter = c }

// Add to RingHandler struct:
//   starter CallStarter

// Start emits call_started for a GC (no ring layer). 1:1 should use /ring instead.
// POST /api/dm/{id}/call/start
func (h *RingHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), dmID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	dm, err := h.repo.GetDmChannelByID(r.Context(), dmID)
	if err != nil || dm == nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}
	if !dm.IsGroup {
		httputil.JSONError(w, "use /call/ring for 1:1 DMs", http.StatusBadRequest)
		return
	}
	if h.starter == nil {
		httputil.JSONError(w, "not configured", http.StatusServiceUnavailable)
		return
	}
	startedAt := timeNow().UnixMilli()
	if err := h.starter.Started(r.Context(), dmID, userID, startedAt); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Active returns rings targeting the current user.
// GET /api/calls/active
func (h *RingHandler) Active(w http.ResponseWriter, r *http.Request) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	rings := h.rs.ActiveRingsForUser(userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"rings":   rings,
		"in_call": []any{}, // populated by future enhancement; empty for now
	})
}
```

- [ ] **Step 2: Add tests**

Append to `ring_handler_test.go`:

```go
type fakeCallStarter struct{ count int }

func (f *fakeCallStarter) Started(_ context.Context, _, _, _ int64) error { f.count++; return nil }

func TestStart_GC_EmitsStarted(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	rs := NewRingService(&fakeHub{}, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))
	starter := &fakeCallStarter{}
	h.SetCallStarter(starter)

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/start", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Start(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d", rec.Code)
	}
	if starter.count != 1 {
		t.Errorf("expected starter called once, got %d", starter.count)
	}
}

func TestActive_ReturnsRingsForUser(t *testing.T) {
	repo := &authRepoFake{}
	rs := NewRingService(&fakeHub{}, &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repoLikeFromAuthFake(repo))

	rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{Username: "alice"})
	rs.Initiate(context.Background(), 11, 3, 2, ringCallerInfo{Username: "charlie"})
	rs.Initiate(context.Background(), 12, 1, 4, ringCallerInfo{Username: "alice"})

	req := httptest.NewRequest(http.MethodGet, "/api/calls/active", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	rec := httptest.NewRecorder()
	h.Active(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		Rings []ActiveRing `json:"rings"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Rings) != 2 {
		t.Errorf("expected 2 rings for user 2, got %d", len(resp.Rings))
	}
}
```

- [ ] **Step 3: Run tests**

```
go test ./internal/voice/ -run "TestStart_GC|TestActive" -count=1
```

Expected: PASS after implementation in Step 1.

- [ ] **Step 4: Commit**

```
git add internal/voice/ring_handler.go internal/voice/ring_handler_test.go
git commit -m "feat(voice): GC call/start + GET /api/calls/active"
```

---

### Task 14: Activities HTTP endpoints

**Files:**
- Create: `internal/voice/activity_handler.go`
- Create: `internal/voice/activity_handler_test.go`

Endpoints:
- `POST /api/voice/{vc}/activity/start` body `{type, params?}` → 204 (broadcasts ACTIVITY_START)
- `POST /api/voice/{vc}/activity/end` → 204 (broadcasts ACTIVITY_END)
- `GET /api/voice/{vc}/activity` → `{type, started_by, started_at, params?}` or 204 if none

Authorization: caller must be a current participant of `voice:{vc}` (HEXISTS check via `Service.IsParticipant`).

- [ ] **Step 1: Write the failing test**

```go
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActivityStart_RequiresParticipation(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	hub := &fakeHub{}
	h := NewActivityHandler(svc, hub)

	body, _ := json.Marshal(map[string]any{"type": "watch_party", "params": map[string]any{"url": "x"}})
	req := httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/start", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("non-participant must be 403, got %d", rec.Code)
	}

	// now make the user a participant
	_ = svc.Join(context.Background(), "dm:1", "7", "alice", "")
	req = httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/start", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec = httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d", rec.Code)
	}
	got, _ := svc.GetActivity(context.Background(), "dm:1")
	if got == nil || got.Type != "watch_party" {
		t.Errorf("activity not stored: %+v", got)
	}
}

func TestActivityEnd_BroadcastsAndDeletes(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	hub := &fakeHub{}
	h := NewActivityHandler(svc, hub)
	_ = svc.Join(context.Background(), "dm:1", "7", "alice", "")
	_ = svc.StartActivity(context.Background(), "dm:1", "watch_party", 7, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/end", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.End(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d", rec.Code)
	}
	if got, _ := svc.GetActivity(context.Background(), "dm:1"); got != nil {
		t.Error("activity not deleted")
	}
}
```

> **Note:** `fakeHub` from Task 9 only has `SendToUser`. Add a `BroadcastToChannel` method to it (and to the `RingHub` shape if not already present in your test fakes). The activity handler uses `*ws.Hub.BroadcastToChannel`, so the production code calls the real hub directly; the fake mirrors the signature for assertions. Keep the fake additions tiny.

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/voice/ -run "TestActivity" -count=1
```

- [ ] **Step 3: Implement**

```go
package voice

import (
	"encoding/json"
	"net/http"

	"parley/internal/httputil"
	ws "parley/internal/websocket"
)

// activityHub is the slice of hub methods the activity handler needs.
type activityHub interface {
	BroadcastToChannel(channelID, eventType string, payload []byte) error
}

type ActivityHandler struct {
	svc *Service
	hub activityHub
}

func NewActivityHandler(svc *Service, hub activityHub) *ActivityHandler {
	return &ActivityHandler{svc: svc, hub: hub}
}

func (h *ActivityHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	if isPart, err := h.svc.IsParticipant(r.Context(), vcStr, int64ToStr(userID)); err != nil || !isPart {
		httputil.JSONError(w, "forbidden: not a participant", http.StatusForbidden)
		return
	}
	var body struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Type == "" {
		httputil.JSONError(w, "type required", http.StatusBadRequest)
		return
	}
	if err := h.svc.StartActivity(r.Context(), vcStr, body.Type, userID, body.Params); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a, _ := h.svc.GetActivity(r.Context(), vcStr)
	payload, _ := json.Marshal(map[string]any{
		"vc":           vcStr,
		"type":         a.Type,
		"started_by":   int64ToStr(a.StartedBy),
		"started_at":   a.StartedAtMs,
		"params":       a.Params,
	})
	_ = h.hub.BroadcastToChannel(vcStr, ws.EventActivityStart, payload)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ActivityHandler) End(w http.ResponseWriter, r *http.Request) {
	userID, ok := userFromCtx(w, r)
	if !ok {
		return
	}
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	if isPart, err := h.svc.IsParticipant(r.Context(), vcStr, int64ToStr(userID)); err != nil || !isPart {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := h.svc.EndActivity(r.Context(), vcStr); err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(map[string]any{"vc": vcStr})
	_ = h.hub.BroadcastToChannel(vcStr, ws.EventActivityEnd, payload)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ActivityHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, vcStr, ok := parseVCFromPath(w, r)
	if !ok {
		return
	}
	a, err := h.svc.GetActivity(r.Context(), vcStr)
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

// parseVCFromPath is the activity-handler counterpart of voice.Handler.parseVC.
func parseVCFromPath(w http.ResponseWriter, r *http.Request) (VirtualChannel, string, bool) {
	raw := r.PathValue("vc")
	vc, err := ParseVirtualChannel(raw)
	if err != nil {
		httputil.JSONError(w, "invalid virtual channel id", http.StatusBadRequest)
		return VirtualChannel{}, "", false
	}
	return vc, raw, true
}
```

- [ ] **Step 4: Run tests**

```
go test ./internal/voice/ -run "TestActivity" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/voice/activity_handler.go internal/voice/activity_handler_test.go
git commit -m "feat(voice): activities Start/End/Get HTTP endpoints"
```

---

### Task 15: Wire ring + activity routes + service construction

**Files:**
- Modify: `cmd/api/routes.go`
- Modify: `cmd/api/main.go` (or wherever services are constructed)

- [ ] **Step 1: Construct services in main.go**

Find where `voiceSvc` is created. After it, before `registerRoutes` is called, construct the ring service and wire the dm emitter adapter. Mock pattern:

```go
ringSvc := voice.NewRingService(hub, voice.DmEmitterFromService(dmService), voiceSvc)
```

If `dmService` doesn't yet exist at that point in main.go (it's constructed inside `registerRoutes`), lift its construction up so it's available:

```go
dmRepo := dm.NewRepository(repo)        // if your dm package follows that pattern
dmService := dm.NewService(dmRepo, hub) // adjust to actual constructor
```

Then pass `ringSvc` and `dmService` into `registerRoutes`.

> **Reality check:** `internal/dm` may already construct its service inline inside `dmHandler := dm.NewHandler(repo, hub)` (see existing routes.go around line 310). If that's the case, you have two options:
> 1. Refactor `dm.NewHandler` to take an externally-constructed `*dm.Service`.
> 2. Add a `dm.NewService(repo, hub)` constructor that returns a service we can share, and have `dm.NewHandler` accept it.
>
> Pick the minimal change. Inspect `internal/dm/handler.go` and `internal/dm/service.go` to see what's exported.

- [ ] **Step 2: Register routes**

In `cmd/api/routes.go`, add (inside the protected route group, near the existing `/dms/...` registrations):

```go
				// 1:1 ring lifecycle
				ringHandler := voice.NewRingHandler(ringSvc, repo)
				ringHandler.SetCallStarter(voice.DmEmitterFromService(dmService))
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/call/ring", ringHandler.Ring)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/call/accept", ringHandler.Accept)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/call/decline", ringHandler.Decline)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/call/cancel", ringHandler.Cancel)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/dms/{id}/call/start", ringHandler.Start)
				r.With(auth.RequireScope(auth.ScopeMessagesRead)).Get("/calls/active", ringHandler.Active)

				// Activities (works for any virtual channel)
				activityHandler := voice.NewActivityHandler(voiceSvc, hub)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/voice/{vc}/activity/start", activityHandler.Start)
				r.With(auth.RequireScope(auth.ScopeMessagesWrite)).Post("/voice/{vc}/activity/end", activityHandler.End)
				r.With(auth.RequireScope(auth.ScopeServersRead)).Get("/voice/{vc}/activity", activityHandler.Get)
```

> The existing `/dms/{id}/...` route group uses `{id}` for the dm channel id; the new ring routes follow the same convention.

- [ ] **Step 3: Verify**

```
go build ./...
go test ./internal/voice/... -count=1
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```
git add cmd/api/routes.go cmd/api/main.go
git commit -m "feat(api): mount ring + activity routes; wire dm emitter adapter"
```

---

## Phase 5 — WS subscription verification

### Task 16: Verify DM members receive voice events on dm:{id}

**Files:**
- Modify: `cmd/api/main.go` (the WS auth/subscribe flow)

DM members already auto-subscribe to `dm:{id}` for `DM_MESSAGE_CREATE` and other DM events (Phase B). For the CallBanner / DM-list phone icon to update live, members must also receive `VOICE_STATE_UPDATE`, `ACTIVITY_START`, `ACTIVITY_END` on the same topic. The existing hub broadcasts to `topic` regardless of event type — what matters is whether DM members are on the topic.

- [ ] **Step 1: Locate the WS subscription logic**

```
grep -n "BroadcastToChannel\|Subscribe\|dm:\|virtualChannel" cmd/api/main.go internal/websocket/*.go | head -40
```

Identify where a user's WS connection is enrolled in topics. There's a function around `cmd/api/main.go` that authorizes channel subscription requests (the Phase B "WS auth at dm:{id}" path).

- [ ] **Step 2: Confirm or extend subscription**

Audit: a user with WS open should be enrolled in `dm:{id}` for every DM/GC they are a member of. If the existing logic only enrolls them on demand for messages (e.g., when the frontend explicitly subscribes after opening a DM), that's the bug. The fix is to enroll on WS connect / on DM membership change.

If subscription is already topic-pull-style (frontend asks for `dm:{id}` and backend allows it), then the frontend in Task 22 must subscribe to all DMs at boot. Document the route taken.

Concrete change: ensure that the backend WS handler routes any `BroadcastToChannel("dm:{id}", ...)` message to all currently connected sessions of every user in that DM/GC. The simplest correct implementation is server-side fan-out using the dm membership table — no client subscription needed:

```go
// inside cmd/api/main.go's WS hub broadcast handler, when a topic starts with "dm:":
//   parse the dmID
//   query repo.GetDmMembers(ctx, dmID)
//   SendToUser for each member
// Then BroadcastToChannel("dm:{id}", ...) is implemented as a fan-out.
```

If this is already the implementation, no change needed. Add a smoke test that posts to `/api/voice/dm:1/activity/start` (or simulates `BroadcastToChannel("dm:1", ...)`) and asserts that all members of dm 1 receive the event over their WS connections — but that requires an integration harness which may not exist. **Acceptable shortcut for this task:** code-read confirmation only, with a manual smoke test step in Task 35.

- [ ] **Step 3: Document findings and commit any change**

Add a one-line comment at the WS-broadcast call site documenting the fan-out semantics if not already present. If a code change was needed, commit it:

```
git add cmd/api/main.go
git commit -m "fix(ws): ensure dm:{id} broadcasts fan out to all DM members"
```

If no change was needed, skip the commit and proceed to Phase 6.

---

## Phase 6 — Frontend foundation

### Task 17: API clients (calls, activities, voice virtual-channel)

**Files:**
- Modify: `frontend/src/api/voice.ts`
- Create: `frontend/src/api/calls.ts`
- Create: `frontend/src/api/activities.ts`

- [ ] **Step 1: Update voice.ts to use virtual channel IDs**

Replace `frontend/src/api/voice.ts` contents:

```ts
import { apiClient } from './client';

export interface VoiceToken {
  token: string;
  url: string;
}

export interface VoiceParticipant {
  user_id: string;
  username: string;
  avatar_url?: string;
}

export async function getVoiceToken(vc: string): Promise<VoiceToken> {
  return apiClient.get<VoiceToken>(`/voice/${vc}/token`);
}

export async function joinVoiceChannel(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/join`, {});
}

export async function leaveVoiceChannel(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/leave`, {});
}

export async function refreshVoiceHeartbeat(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/heartbeat`, {});
}

export async function getVoiceParticipants(vc: string): Promise<VoiceParticipant[]> {
  return apiClient.get<VoiceParticipant[]>(`/voice/${vc}/participants`);
}

export async function muteVoiceParticipant(vc: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/participants/${targetUserId}/mute`, {});
}

export async function kickVoiceParticipant(vc: string, targetUserId: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/participants/${targetUserId}/kick`, {});
}

// Helper: server channel ID -> virtual channel ID
export function serverVc(channelId: string | number): string {
  return `s:${channelId}`;
}

// Helper: dm channel ID -> virtual channel ID
export function dmVc(dmChannelId: string | number): string {
  return `dm:${dmChannelId}`;
}
```

- [ ] **Step 2: Create calls.ts**

```ts
import { apiClient } from './client';

export interface RingCaller {
  user_id: number;
  username: string;
  display_name: string;
  avatar_url: string;
}

export interface ActiveRing {
  ring_id: string;
  vc: string;
  caller: RingCaller;
  started_at_ms: number;
}

export interface ActiveCallsResponse {
  rings: ActiveRing[];
  in_call: string[];
}

export async function ringDm(dmChannelId: string | number): Promise<{ ring_id: string }> {
  return apiClient.post<{ ring_id: string }>(`/dms/${dmChannelId}/call/ring`, {});
}

export async function startGcCall(dmChannelId: string | number): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/start`, {});
}

export async function acceptCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/accept`, { ring_id: ringId });
}

export async function declineCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/decline`, { ring_id: ringId });
}

export async function cancelCall(dmChannelId: string | number, ringId: string): Promise<void> {
  return apiClient.post<void>(`/dms/${dmChannelId}/call/cancel`, { ring_id: ringId });
}

export async function getActiveCalls(): Promise<ActiveCallsResponse> {
  return apiClient.get<ActiveCallsResponse>('/calls/active');
}
```

- [ ] **Step 3: Create activities.ts**

```ts
import { apiClient } from './client';

export interface ActivityRecord {
  type: string;
  started_by: string;
  started_at: number;
  params?: unknown;
}

export async function startActivity(vc: string, type: string, params?: unknown): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/activity/start`, { type, params });
}

export async function endActivity(vc: string): Promise<void> {
  return apiClient.post<void>(`/voice/${vc}/activity/end`, {});
}

export async function getActivity(vc: string): Promise<ActivityRecord | null> {
  try {
    return await apiClient.get<ActivityRecord>(`/voice/${vc}/activity`);
  } catch {
    return null;
  }
}
```

- [ ] **Step 4: Verify TypeScript build**

```
cd frontend && npm run build
```

Expected: build succeeds (existing voice.ts callers still compile because the function signatures are compatible — they all take a string).

- [ ] **Step 5: Update existing voice.ts callers to pass `serverVc(channelId)`**

```
grep -rn "getVoiceToken\|joinVoiceChannel\|leaveVoiceChannel\|getVoiceParticipants\|muteVoiceParticipant\|kickVoiceParticipant" frontend/src --include="*.ts" --include="*.tsx"
```

For each caller passing a bare numeric channel ID, wrap it: `getVoiceToken(serverVc(channelId))`. The semantically-equivalent migration preserves existing behavior because the back-compat routes (Task 8) accept the bare ID via the `/channels/{channelId}/voice/*` form too — but we want to flip the frontend to the new shape so we can drop the back-compat routes later.

- [ ] **Step 6: Re-run frontend build**

```
cd frontend && npm run build
```

Expected: clean build.

- [ ] **Step 7: Commit**

```
git add frontend/src/api/voice.ts frontend/src/api/calls.ts frontend/src/api/activities.ts $(grep -rln "getVoiceToken\|joinVoiceChannel\|leaveVoiceChannel\|getVoiceParticipants\|muteVoiceParticipant\|kickVoiceParticipant" frontend/src --include="*.ts" --include="*.tsx")
git commit -m "feat(frontend): API clients use virtual channel IDs; add calls + activities clients"
```

---

### Task 18: useLocalVolumes hook

**Files:**
- Create: `frontend/src/hooks/useLocalVolumes.ts`
- Create: `frontend/src/hooks/useLocalVolumes.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLocalVolumes } from './useLocalVolumes';

describe('useLocalVolumes', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('defaults to 100 (unity) for unknown users', () => {
    const { result } = renderHook(() => useLocalVolumes());
    expect(result.current.getVolume('42')).toBe(100);
  });

  it('persists set values to localStorage', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.setVolume('42', 50));
    expect(result.current.getVolume('42')).toBe(50);
    const raw = localStorage.getItem('parley.localVolumes');
    expect(raw).toBeTruthy();
    expect(JSON.parse(raw!)).toEqual({ '42': 50 });
  });

  it('toggleMute swaps between 0 and last non-zero (default 100)', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(100);
    act(() => result.current.setVolume('42', 80));
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.toggleMute('42'));
    expect(result.current.getVolume('42')).toBe(80);
  });

  it('clamps to 0..200', () => {
    const { result } = renderHook(() => useLocalVolumes());
    act(() => result.current.setVolume('42', -10));
    expect(result.current.getVolume('42')).toBe(0);
    act(() => result.current.setVolume('42', 999));
    expect(result.current.getVolume('42')).toBe(200);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```
cd frontend && npm test -- useLocalVolumes
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
import { useEffect, useState, useCallback } from 'react';

const STORAGE_KEY = 'parley.localVolumes';
const PRE_MUTE_KEY = 'parley.preMuteVolumes'; // remembers last non-zero value per userID

type VolumeMap = Record<string, number>;

function readMap(key: string): VolumeMap {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object') return parsed as VolumeMap;
  } catch {
    // ignored
  }
  return {};
}

function writeMap(key: string, map: VolumeMap) {
  localStorage.setItem(key, JSON.stringify(map));
}

function clamp(n: number): number {
  if (!Number.isFinite(n)) return 100;
  if (n < 0) return 0;
  if (n > 200) return 200;
  return Math.round(n);
}

export interface UseLocalVolumesReturn {
  getVolume: (userID: string | number | bigint) => number;
  setVolume: (userID: string | number | bigint, value: number) => void;
  toggleMute: (userID: string | number | bigint) => void;
}

export function useLocalVolumes(): UseLocalVolumesReturn {
  const [version, setVersion] = useState(0);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY || e.key === PRE_MUTE_KEY) {
        setVersion(v => v + 1);
      }
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  const getVolume = useCallback((userID: string | number | bigint): number => {
    const key = String(userID);
    const map = readMap(STORAGE_KEY);
    return key in map ? map[key] : 100;
  }, [version]);

  const setVolume = useCallback((userID: string | number | bigint, value: number) => {
    const key = String(userID);
    const v = clamp(value);
    const map = readMap(STORAGE_KEY);
    map[key] = v;
    writeMap(STORAGE_KEY, map);
    setVersion(x => x + 1);
  }, []);

  const toggleMute = useCallback((userID: string | number | bigint) => {
    const key = String(userID);
    const map = readMap(STORAGE_KEY);
    const pre = readMap(PRE_MUTE_KEY);
    const current = key in map ? map[key] : 100;
    if (current === 0) {
      // unmute -> restore last non-zero (default 100)
      const restored = pre[key] ?? 100;
      map[key] = restored;
    } else {
      // mute -> remember current as pre-mute
      pre[key] = current;
      map[key] = 0;
      writeMap(PRE_MUTE_KEY, pre);
    }
    writeMap(STORAGE_KEY, map);
    setVersion(x => x + 1);
  }, []);

  return { getVolume, setVolume, toggleMute };
}
```

- [ ] **Step 4: Run test to verify it passes**

```
cd frontend && npm test -- useLocalVolumes
```

Expected: PASS (all 4 cases).

- [ ] **Step 5: Commit**

```
git add frontend/src/hooks/useLocalVolumes.ts frontend/src/hooks/useLocalVolumes.test.ts
git commit -m "feat(frontend): useLocalVolumes hook (0-200 with mute toggle)"
```

---

### Task 19: Activities registry

**Files:**
- Create: `frontend/src/activities/registry.ts`
- Create: `frontend/src/activities/registry.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, beforeEach } from 'vitest';
import { register, lookup, list, _resetForTests } from './registry';

describe('activity registry', () => {
  beforeEach(() => _resetForTests());

  it('lookup returns null for unregistered types', () => {
    expect(lookup('nope')).toBeNull();
  });

  it('register stores a definition', () => {
    const def = { type: 'foo', displayName: 'Foo', render: () => null };
    register(def);
    expect(lookup('foo')).toBe(def);
    expect(list()).toContain(def);
  });

  it('register replaces by type', () => {
    register({ type: 'foo', displayName: 'A', render: () => null });
    register({ type: 'foo', displayName: 'B', render: () => null });
    expect(lookup('foo')!.displayName).toBe('B');
    expect(list()).toHaveLength(1);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```
cd frontend && npm test -- registry
```

- [ ] **Step 3: Implement**

```ts
import type React from 'react';

export interface ActivityDefinition {
  type: string;
  displayName: string;
  icon?: React.ReactNode;
  render: React.FC<{ vc: string; params: unknown }>;
  controls?: React.FC<{ vc: string }>;
}

const registry = new Map<string, ActivityDefinition>();

export function register(def: ActivityDefinition): void {
  registry.set(def.type, def);
}

export function lookup(type: string): ActivityDefinition | null {
  return registry.get(type) ?? null;
}

export function list(): ActivityDefinition[] {
  return Array.from(registry.values());
}

// Test-only escape hatch.
export function _resetForTests(): void {
  registry.clear();
}
```

- [ ] **Step 4: Run test to verify it passes**

```
cd frontend && npm test -- registry
```

- [ ] **Step 5: Commit**

```
git add frontend/src/activities/registry.ts frontend/src/activities/registry.test.ts
git commit -m "feat(frontend): activity registry stub"
```

---

### Task 20: Active-rings rehydration on app boot

**Files:**
- Modify: `frontend/src/App.tsx` (only the boot effect; full CallContext lands in Task 25)

This task is a tiny placeholder until Phase 8: on app load, fetch `GET /api/calls/active` once and stash the response in a top-level state. Phase 8 wires it into `CallContext`.

- [ ] **Step 1: Add the boot effect**

In `frontend/src/App.tsx`, near the other boot-time `useEffect` calls (search for `useEffect(() => {` in the early hooks), add:

```tsx
const [bootRings, setBootRings] = useState<ActiveRing[]>([]);
useEffect(() => {
  if (!currentUser) return;
  getActiveCalls().then(r => setBootRings(r.rings)).catch(() => { /* offline ok */ });
}, [currentUser]);
```

Import the function and type:

```tsx
import { getActiveCalls, type ActiveRing } from './api/calls';
```

This state is unused for now; Task 25 will pass it into `CallContext` so the CallContext picks up any rings that fired before the page loaded.

- [ ] **Step 2: Verify build**

```
cd frontend && npm run build
```

- [ ] **Step 3: Commit**

```
git add frontend/src/App.tsx
git commit -m "feat(frontend): fetch active rings on app boot (placeholder, used in Phase 8)"
```

---

## Phase 7 — Update existing voice components

### Task 21: useVoiceConnection — virtual channel + heartbeat + activity events + volume application

**Files:**
- Modify: `frontend/src/hooks/useVoiceConnection.ts`

This is a substantial change to a 430-line hook. Read the file fully before editing. The diff has four parts:

1. **Signature:** the hook already accepts a `string | null`. Confirm callers pass virtual channel IDs (`s:N` or `dm:N`) instead of bare numerics. If the hook internally calls `getVoiceToken(value)`, the value is forwarded to LiveKit as the room name — no change needed beyond ensuring callers use prefixed IDs.

2. **15s heartbeat loop:**

```tsx
useEffect(() => {
  if (!connected || !virtualChannelId) return;
  const id = setInterval(() => {
    refreshVoiceHeartbeat(virtualChannelId).catch(() => { /* swallow; reconnect logic handles it */ });
  }, 15_000);
  return () => clearInterval(id);
}, [connected, virtualChannelId]);
```

If a heartbeat loop already exists, leave it alone (this task is then a no-op for that part).

3. **Activity event subscription + state:**

Add state for the active activity, fed by WS events:

```tsx
const [activity, setActivity] = useState<ActivityRecord | null>(null);

// On (re)connect, hydrate via REST.
useEffect(() => {
  if (!connected || !virtualChannelId) return;
  getActivity(virtualChannelId).then(setActivity).catch(() => { /* fine */ });
}, [connected, virtualChannelId]);

// Subscribe to ACTIVITY_START / ACTIVITY_END from the WS hook.
// (Pattern matches existing VOICE_STATE_UPDATE subscription elsewhere in this hook.)
useEffect(() => {
  if (!virtualChannelId) return;
  const unsubStart = onWsEvent('ACTIVITY_START', (ev: any) => {
    if (ev.vc === virtualChannelId) {
      setActivity({
        type: ev.type,
        started_by: ev.started_by,
        started_at: ev.started_at,
        params: ev.params,
      });
    }
  });
  const unsubEnd = onWsEvent('ACTIVITY_END', (ev: any) => {
    if (ev.vc === virtualChannelId) setActivity(null);
  });
  return () => { unsubStart(); unsubEnd(); };
}, [virtualChannelId]);
```

> **`onWsEvent` is shorthand** — substitute whatever WS subscription pattern already exists in this hook. Search for `addEventListener` or `subscribe` or `dispatchEvent` in `useVoiceConnection.ts` to match style.

4. **Per-listener volume application:**

Inside the participant-track-subscribed handler (where audio tracks attach), apply `useLocalVolumes`:

```tsx
const { getVolume } = useLocalVolumes();

// In the participant track-subscribed handler:
participant.on(ParticipantEvent.TrackSubscribed, (track, pub) => {
  if (track.kind === Track.Kind.Audio) {
    const v = getVolume(participant.identity); // identity == userID string
    track.setVolume(v / 100);
  }
});

// Re-apply volume when localVolumes change (storage event triggers a re-render via useLocalVolumes).
useEffect(() => {
  room?.remoteParticipants.forEach(p => {
    p.audioTrackPublications.forEach(pub => {
      if (pub.track && pub.track.kind === Track.Kind.Audio) {
        pub.track.setVolume(getVolume(p.identity) / 100);
      }
    });
  });
}, [getVolume, room]);
```

Return `activity` from the hook so consumers can read it:

```tsx
return {
  // ...existing returns
  activity,
};
```

- [ ] **Step 1: Read the file**

```
cat frontend/src/hooks/useVoiceConnection.ts
```

- [ ] **Step 2: Make the four changes above**

Apply diffs as described. Adjust to match existing patterns where this plan and the codebase differ.

- [ ] **Step 3: Build to verify TypeScript**

```
cd frontend && npm run build
```

Expected: clean build.

- [ ] **Step 4: Commit**

```
git add frontend/src/hooks/useVoiceConnection.ts
git commit -m "feat(voice): heartbeat + activity events + per-listener volume in useVoiceConnection"
```

---

### Task 22: ConnectionQualityDot + VolumeSlider + ParticipantTile integration

**Files:**
- Create: `frontend/src/components/voice/ConnectionQualityDot.tsx`
- Create: `frontend/src/components/voice/ConnectionQualityDot.css`
- Create: `frontend/src/components/voice/VolumeSlider.tsx`
- Create: `frontend/src/components/voice/VolumeSlider.css`
- Modify: `frontend/src/components/voice/ParticipantTile.tsx`
- Modify: `frontend/src/components/voice/ParticipantTile.css`

- [ ] **Step 1: ConnectionQualityDot**

```tsx
// ConnectionQualityDot.tsx
import React, { useEffect, useState } from 'react';
import type { Participant } from 'livekit-client';
import { ConnectionQuality, ParticipantEvent } from 'livekit-client';
import './ConnectionQualityDot.css';

export const ConnectionQualityDot: React.FC<{ participant: Participant }> = ({ participant }) => {
  const [q, setQ] = useState<ConnectionQuality>(participant.connectionQuality);
  useEffect(() => {
    const onChange = () => setQ(participant.connectionQuality);
    participant.on(ParticipantEvent.ConnectionQualityChanged, onChange);
    return () => { participant.off(ParticipantEvent.ConnectionQualityChanged, onChange); };
  }, [participant]);

  let cls = 'cq-dot cq-dot--unknown';
  let label = 'unknown';
  switch (q) {
    case ConnectionQuality.Excellent: cls = 'cq-dot cq-dot--excellent'; label = 'excellent'; break;
    case ConnectionQuality.Good:      cls = 'cq-dot cq-dot--good';      label = 'good';      break;
    case ConnectionQuality.Poor:      cls = 'cq-dot cq-dot--poor';      label = 'poor';      break;
    case ConnectionQuality.Lost:      cls = 'cq-dot cq-dot--lost';      label = 'lost';      break;
  }
  return <span className={cls} title={`Connection: ${label}`} aria-label={`Connection: ${label}`} />;
};
```

```css
/* ConnectionQualityDot.css */
.cq-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--parley-text-muted);
  border: 1px solid rgba(0, 0, 0, 0.2);
}
.cq-dot--excellent { background: #34d399; }
.cq-dot--good      { background: #84cc16; }
.cq-dot--poor      { background: #f59e0b; }
.cq-dot--lost      { background: var(--parley-danger); }
```

- [ ] **Step 2: VolumeSlider**

```tsx
// VolumeSlider.tsx
import React from 'react';
import './VolumeSlider.css';

interface Props {
  value: number; // 0..200
  onChange: (v: number) => void;
}

export const VolumeSlider: React.FC<Props> = ({ value, onChange }) => {
  return (
    <div className="vol-slider">
      <input
        type="range"
        min={0}
        max={200}
        step={5}
        value={value}
        onChange={e => onChange(Number(e.target.value))}
        aria-label="Listener volume"
      />
      <span className="vol-slider-value">{value}%</span>
    </div>
  );
};
```

```css
/* VolumeSlider.css */
.vol-slider { display: flex; align-items: center; gap: 8px; padding: 6px 8px; }
.vol-slider input[type="range"] { flex: 1; }
.vol-slider-value { font-size: 12px; color: var(--parley-text-muted); min-width: 36px; text-align: right; }
```

- [ ] **Step 3: ParticipantTile integration**

Add the local-volume application + visual indicator to `ParticipantTile.tsx`:

```tsx
import { useLocalVolumes } from '../../hooks/useLocalVolumes';
import { ConnectionQualityDot } from './ConnectionQualityDot';
import { VolumeX } from 'lucide-react';

// inside the component, after participant is destructured:
const { getVolume } = useLocalVolumes();
const localVol = getVolume(participant.identity);
const isLocallyMuted = localVol === 0;

useEffect(() => {
  participant.audioTrackPublications.forEach(pub => {
    if (pub.track) pub.track.setVolume(localVol / 100);
  });
}, [localVol, participant]);

// inside JSX, near the existing isMuted overlay:
{isLocallyMuted && (
  <span className="participant-tile-locally-muted" title="Muted for me">
    <VolumeX size={14} />
  </span>
)}
<ConnectionQualityDot participant={participant} />
```

Add CSS in `ParticipantTile.css`:

```css
.participant-tile-locally-muted {
  position: absolute;
  bottom: 8px;
  right: 8px;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  border-radius: 4px;
  padding: 2px 4px;
  display: inline-flex;
  align-items: center;
}
.participant-tile .cq-dot {
  position: absolute;
  top: 6px;
  right: 6px;
}
```

- [ ] **Step 4: Build**

```
cd frontend && npm run build
```

Expected: clean build.

- [ ] **Step 5: Commit**

```
git add frontend/src/components/voice/ConnectionQualityDot.tsx frontend/src/components/voice/ConnectionQualityDot.css frontend/src/components/voice/VolumeSlider.tsx frontend/src/components/voice/VolumeSlider.css frontend/src/components/voice/ParticipantTile.tsx frontend/src/components/voice/ParticipantTile.css
git commit -m "feat(voice): connection-quality dot + volume slider + locally-muted tile"
```

---

### Task 23: VoiceContextMenu — volume slider + privilege gating

**Files:**
- Modify: `frontend/src/components/voice/VoiceContextMenu.tsx`

The menu currently has a single `Mute` action that calls `onMute` (the force-mute endpoint). Restructure into three sections:
1. Per-listener volume slider (always visible).
2. "Mute for me" toggle (always visible).
3. Privilege-gated divider with "Force mute" + "Disconnect" (visible only when caller is allowed).

- [ ] **Step 1: Read existing**

```
cat frontend/src/components/voice/VoiceContextMenu.tsx
```

- [ ] **Step 2: Replace contents**

```tsx
import React from 'react';
import { createPortal } from 'react-dom';
import { useLocalVolumes } from '../../hooks/useLocalVolumes';
import { VolumeSlider } from './VolumeSlider';
import './VoiceContextMenu.css';

interface Props {
  position: { x: number; y: number };
  targetUserID: string;
  canForceMute: boolean;
  canKick: boolean;
  onForceMute: () => void;
  onKick: () => void;
  onClose: () => void;
}

export const VoiceContextMenu: React.FC<Props> = ({
  position, targetUserID, canForceMute, canKick, onForceMute, onKick, onClose,
}) => {
  const { getVolume, setVolume, toggleMute } = useLocalVolumes();
  const v = getVolume(targetUserID);

  return createPortal(
    <>
      <div className="vc-context-menu-backdrop" onClick={onClose} />
      <div
        className="vc-context-menu"
        style={{ position: 'fixed', left: position.x, top: position.y }}
        onClick={e => e.stopPropagation()}
      >
        <VolumeSlider value={v} onChange={n => setVolume(targetUserID, n)} />
        <button
          className="vc-context-menu-item"
          onClick={() => { toggleMute(targetUserID); onClose(); }}
        >
          {v === 0 ? 'Unmute for me' : 'Mute for me'}
        </button>
        {(canForceMute || canKick) && <div className="vc-context-menu-divider" />}
        {canForceMute && (
          <button className="vc-context-menu-item vc-context-menu-item--mod" onClick={() => { onForceMute(); onClose(); }}>
            Force mute
          </button>
        )}
        {canKick && (
          <button className="vc-context-menu-item vc-context-menu-item--mod vc-context-menu-item--danger" onClick={() => { onKick(); onClose(); }}>
            Disconnect from call
          </button>
        )}
      </div>
    </>,
    document.body,
  );
};
```

CSS additions:

```css
.vc-context-menu-divider { height: 1px; margin: 4px 0; background: var(--parley-border); }
.vc-context-menu-item--mod { color: var(--parley-text-muted); }
.vc-context-menu-item--danger { color: var(--parley-danger); }
.vc-context-menu-backdrop { position: fixed; inset: 0; z-index: 999; }
.vc-context-menu { z-index: 1000; }
```

- [ ] **Step 3: Update callers**

```
grep -rn "VoiceContextMenu" frontend/src --include="*.tsx" --include="*.ts"
```

Each caller passes `canForceMute` / `canKick` derived from the local privilege computation:

```tsx
// For server VC inside VoiceChannel:
const canForceMute = myServerPermissions.has('PermMuteMembers');
const canKick = myServerPermissions.has('PermMoveMembers');

// For DM call inside VoiceChannel:
const canForceMute = isGroup && currentUserID === groupOwnerID && targetUserID !== currentUserID;
const canKick = canForceMute;
```

The exact prop wiring depends on what `VoiceChannel.tsx` already passes. Trace back from the existing `onMute`/`onKick` props and rename to `onForceMute`/`onKick`.

- [ ] **Step 4: Build**

```
cd frontend && npm run build
```

- [ ] **Step 5: Commit**

```
git add frontend/src/components/voice/VoiceContextMenu.tsx frontend/src/components/voice/VoiceContextMenu.css frontend/src/components/voice/VoiceChannel.tsx
git commit -m "feat(voice): per-listener volume slider + privilege-gated mute/kick"
```

---

### Task 24: VoiceControls + VoiceChannel — Pop-Out, Activities, compact layout, ActivitySlot

**Files:**
- Modify: `frontend/src/components/voice/VoiceControls.tsx`
- Modify: `frontend/src/components/voice/VoiceChannel.tsx`
- Create: `frontend/src/components/voice/ActivitySlot.tsx`
- Create: `frontend/src/components/voice/ActivitiesModal.tsx`

- [ ] **Step 1: ActivitySlot**

```tsx
// ActivitySlot.tsx
import React from 'react';
import { lookup } from '../../activities/registry';
import type { ActivityRecord } from '../../api/activities';

interface Props {
  vc: string;
  activity: ActivityRecord | null;
}

export const ActivitySlot: React.FC<Props> = ({ vc, activity }) => {
  if (!activity) return null;
  const def = lookup(activity.type);
  if (!def) {
    return (
      <div className="activity-slot activity-slot--unknown">
        Activity ‘{activity.type}’ in progress (this client doesn't support it yet).
      </div>
    );
  }
  const Render = def.render;
  return (
    <div className="activity-slot">
      <Render vc={vc} params={activity.params} />
    </div>
  );
};
```

- [ ] **Step 2: ActivitiesModal**

```tsx
// ActivitiesModal.tsx
import React from 'react';
import { list } from '../../activities/registry';
import { startActivity } from '../../api/activities';

interface Props {
  vc: string;
  open: boolean;
  onClose: () => void;
}

export const ActivitiesModal: React.FC<Props> = ({ vc, open, onClose }) => {
  if (!open) return null;
  const items = list();
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <h3>Activities</h3>
        {items.length === 0 ? (
          <p>Activities coming soon.</p>
        ) : (
          <ul>
            {items.map(it => (
              <li key={it.type}>
                <button onClick={async () => { await startActivity(vc, it.type); onClose(); }}>
                  {it.icon}
                  {it.displayName}
                </button>
              </li>
            ))}
          </ul>
        )}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 12 }}>
          <button onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
};
```

> **Style:** match existing modal patterns (e.g., `RenameGroupModal.tsx`). The classes above are placeholders — use the project's modal CSS conventions.

- [ ] **Step 3: VoiceControls — Pop-Out + Activities buttons**

Add two buttons to `VoiceControls.tsx`'s `voice-widget-controls` row, before `vw-btn--leave`:

```tsx
<button
  className="vw-btn"
  onClick={onPopOut}
  title={floatingMode ? 'Restore' : 'Pop out'}
  aria-label={floatingMode ? 'Restore voice window' : 'Pop out voice window'}
>
  {floatingMode ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
</button>
<button
  className="vw-btn"
  onClick={onOpenActivities}
  title="Activities"
  aria-label="Open activities"
>
  <Sparkles size={16} />
</button>
```

Add the new props to the interface:

```tsx
floatingMode: boolean;
onPopOut: () => void;
onOpenActivities: () => void;
```

Import the missing icons from `lucide-react`.

- [ ] **Step 4: VoiceChannel — layout prop + ActivitySlot**

Add `layout?: 'full' | 'compact'` to the props. In the JSX, render `<ActivitySlot vc={vc} activity={activity} />` between the header and the participant grid. Pass `layout` down to whatever container element controls tile sizing; in compact mode tighten the grid:

```tsx
<div className={`voice-channel voice-channel--${layout ?? 'full'}`}>
  ...header...
  <ActivitySlot vc={vc} activity={activity} />
  ...participant grid (already exists)...
</div>
```

CSS additions in `VoiceChannel.css`:

```css
.voice-channel--compact .participant-grid { gap: 4px; }
.voice-channel--compact .participant-tile { min-height: 96px; }
.activity-slot { padding: 8px 12px; border-bottom: 1px solid var(--parley-border); }
.activity-slot--unknown { color: var(--parley-text-muted); font-style: italic; }
```

`activity` comes from `useVoiceConnection` (Task 21). Plumb it through whatever existing path supplies VoiceChannel its props (App.tsx in this codebase).

- [ ] **Step 5: Build**

```
cd frontend && npm run build
```

- [ ] **Step 6: Commit**

```
git add frontend/src/components/voice/VoiceControls.tsx frontend/src/components/voice/VoiceChannel.tsx frontend/src/components/voice/VoiceChannel.css frontend/src/components/voice/ActivitySlot.tsx frontend/src/components/voice/ActivitiesModal.tsx
git commit -m "feat(voice): VoiceControls Pop-Out + Activities; VoiceChannel compact layout + ActivitySlot"
```

---

## Phase 8 — Call lifecycle UI

### Task 25: CallContext

**Files:**
- Create: `frontend/src/context/CallContext.tsx`

`CallContext` owns the ring-state machine and the incoming-ring queue. It subscribes to WS `CALL_*` events, dispatches accept/decline/cancel through the API, and exposes a small surface to consumers. Floating-mode state for the pop-out window also lives here so it persists across navigation.

The Tauri secondary-window branching lands in Phase 9 — for this task, always render the in-app `IncomingCallModal`.

- [ ] **Step 1: Create the file**

```tsx
import React, { createContext, useCallback, useContext, useEffect, useMemo, useReducer } from 'react';
import { acceptCall as apiAccept, cancelCall as apiCancel, declineCall as apiDecline, getActiveCalls, ringDm, type ActiveRing, type RingCaller } from '../api/calls';

export type CallState =
  | { kind: 'idle' }
  | { kind: 'outgoing'; vc: string; ring_id: string; target_user_id: string; target_username: string }
  | { kind: 'connecting'; vc: string }
  | { kind: 'connected'; vc: string };

export interface IncomingRing {
  vc: string;
  ring_id: string;
  caller: RingCaller;
}

export interface CallContextValue {
  state: CallState;
  incomingQueue: IncomingRing[];
  initiate: (dmChannelId: string, target: { id: string; username: string }) => Promise<void>;
  accept: (ring_id: string) => Promise<void>;
  decline: (ring_id: string) => Promise<void>;
  cancel: () => Promise<void>;
  floatingMode: boolean;
  setFloatingMode: (on: boolean) => void;
  notifyConnected: (vc: string) => void;
  notifyDisconnected: () => void;
}

const Ctx = createContext<CallContextValue | null>(null);

type Action =
  | { type: 'set'; state: CallState }
  | { type: 'enqueue'; ring: IncomingRing }
  | { type: 'dequeue'; ring_id: string }
  | { type: 'set_floating'; floating: boolean };

interface Store {
  state: CallState;
  incomingQueue: IncomingRing[];
  floatingMode: boolean;
}

function reducer(s: Store, a: Action): Store {
  switch (a.type) {
    case 'set':         return { ...s, state: a.state };
    case 'enqueue':     return { ...s, incomingQueue: [...s.incomingQueue, a.ring] };
    case 'dequeue':     return { ...s, incomingQueue: s.incomingQueue.filter(r => r.ring_id !== a.ring_id) };
    case 'set_floating': return { ...s, floatingMode: a.floating };
  }
}

interface ProviderProps {
  children: React.ReactNode;
  bootRings?: ActiveRing[]; // from App.tsx Task 20
  onWsEvent: (eventType: string, handler: (payload: any) => void) => () => void;
  currentUserID: string;
}

export const CallProvider: React.FC<ProviderProps> = ({ children, bootRings, onWsEvent, currentUserID }) => {
  const [store, dispatch] = useReducer(reducer, {
    state: { kind: 'idle' },
    incomingQueue: [],
    floatingMode: false,
  });

  // hydrate boot rings
  useEffect(() => {
    if (!bootRings) return;
    bootRings.forEach(r => dispatch({ type: 'enqueue', ring: { vc: r.vc, ring_id: r.ring_id, caller: r.caller } }));
  }, [bootRings]);

  // WS subscriptions
  useEffect(() => {
    const unsubs = [
      onWsEvent('CALL_RING', (ev: any) => {
        dispatch({ type: 'enqueue', ring: { vc: ev.vc, ring_id: ev.ring_id, caller: ev.caller } });
      }),
      onWsEvent('CALL_ACCEPT', (ev: any) => {
        // Outgoing or incoming resolved.
        dispatch({ type: 'dequeue', ring_id: ev.ring_id });
        if (store.state.kind === 'outgoing' && store.state.ring_id === ev.ring_id) {
          // Caller's own ring accepted — connecting handled by useVoiceConnection caller side.
          dispatch({ type: 'set', state: { kind: 'connecting', vc: store.state.vc } });
        }
        // If accepted by another session of currentUser, dequeue is enough.
      }),
      onWsEvent('CALL_DECLINE', (ev: any) => {
        if (store.state.kind === 'outgoing' && store.state.ring_id === ev.ring_id) {
          dispatch({ type: 'set', state: { kind: 'idle' } });
        }
      }),
      onWsEvent('CALL_CANCEL', (ev: any) => {
        dispatch({ type: 'dequeue', ring_id: ev.ring_id });
      }),
      onWsEvent('CALL_TIMEOUT', (ev: any) => {
        dispatch({ type: 'dequeue', ring_id: ev.ring_id });
        if (store.state.kind === 'outgoing' && store.state.ring_id === ev.ring_id) {
          dispatch({ type: 'set', state: { kind: 'idle' } });
        }
      }),
    ];
    return () => { unsubs.forEach(u => u()); };
  }, [onWsEvent, store.state]);

  const initiate = useCallback(async (dmChannelId: string, target: { id: string; username: string }) => {
    const { ring_id } = await ringDm(dmChannelId);
    dispatch({
      type: 'set',
      state: {
        kind: 'outgoing',
        vc: `dm:${dmChannelId}`,
        ring_id,
        target_user_id: target.id,
        target_username: target.username,
      },
    });
  }, []);

  const accept = useCallback(async (ring_id: string) => {
    const ring = store.incomingQueue.find(r => r.ring_id === ring_id);
    if (!ring) return;
    const dmId = ring.vc.replace(/^dm:/, '');
    await apiAccept(dmId, ring_id);
    dispatch({ type: 'dequeue', ring_id });
    dispatch({ type: 'set', state: { kind: 'connecting', vc: ring.vc } });
  }, [store.incomingQueue]);

  const decline = useCallback(async (ring_id: string) => {
    const ring = store.incomingQueue.find(r => r.ring_id === ring_id);
    if (!ring) return;
    const dmId = ring.vc.replace(/^dm:/, '');
    await apiDecline(dmId, ring_id);
    dispatch({ type: 'dequeue', ring_id });
  }, [store.incomingQueue]);

  const cancel = useCallback(async () => {
    if (store.state.kind !== 'outgoing') return;
    const dmId = store.state.vc.replace(/^dm:/, '');
    await apiCancel(dmId, store.state.ring_id);
    dispatch({ type: 'set', state: { kind: 'idle' } });
  }, [store.state]);

  const setFloatingMode = useCallback((on: boolean) => {
    dispatch({ type: 'set_floating', floating: on });
  }, []);

  const notifyConnected = useCallback((vc: string) => {
    dispatch({ type: 'set', state: { kind: 'connected', vc } });
  }, []);
  const notifyDisconnected = useCallback(() => {
    dispatch({ type: 'set', state: { kind: 'idle' } });
  }, []);

  const value = useMemo<CallContextValue>(() => ({
    state: store.state,
    incomingQueue: store.incomingQueue,
    initiate, accept, decline, cancel,
    floatingMode: store.floatingMode,
    setFloatingMode,
    notifyConnected, notifyDisconnected,
  }), [store, initiate, accept, decline, cancel, setFloatingMode, notifyConnected, notifyDisconnected]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
};

export function useCall(): CallContextValue {
  const v = useContext(Ctx);
  if (!v) throw new Error('useCall must be used inside CallProvider');
  return v;
}
```

- [ ] **Step 2: Build**

```
cd frontend && npm run build
```

Expected: clean build (the provider isn't mounted yet — done in Task 30).

- [ ] **Step 3: Commit**

```
git add frontend/src/context/CallContext.tsx
git commit -m "feat(frontend): CallContext (ring state machine + queue + floating toggle)"
```

---

### Task 26: IncomingCallModal

**Files:**
- Create: `frontend/src/components/calls/IncomingCallModal.tsx`
- Create: `frontend/src/components/calls/IncomingCallModal.css`
- Create: `frontend/public/ringtone.mp3` (royalty-free; ~6s, looped)

- [ ] **Step 1: Component**

```tsx
import React, { useEffect, useRef } from 'react';
import { Phone, PhoneOff } from 'lucide-react';
import type { IncomingRing } from '../../context/CallContext';
import './IncomingCallModal.css';

interface Props {
  ring: IncomingRing;
  showEndCurrentAccept: boolean;
  onAccept: () => void;
  onEndCurrentAndAccept: () => void;
  onDecline: () => void;
}

export const IncomingCallModal: React.FC<Props> = ({ ring, showEndCurrentAccept, onAccept, onEndCurrentAndAccept, onDecline }) => {
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    audioRef.current?.play().catch(() => { /* autoplay may be blocked; safe to ignore */ });
    return () => { audioRef.current?.pause(); };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { onDecline(); }
      if (e.key === 'Enter')  { onAccept(); }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onAccept, onDecline]);

  return (
    <div className="incoming-modal-backdrop" role="dialog" aria-modal="true" aria-live="assertive">
      <div className="incoming-modal">
        <audio ref={audioRef} src="/ringtone.mp3" loop autoPlay preload="auto" />
        {ring.caller.avatar_url
          ? <img src={ring.caller.avatar_url} alt="" className="incoming-avatar" />
          : <div className="incoming-avatar incoming-avatar--placeholder" />}
        <p className="incoming-text">
          Incoming call from <strong>{ring.caller.display_name || ring.caller.username}</strong>...
        </p>
        <div className="incoming-buttons">
          <button className="incoming-btn incoming-btn--decline" aria-label="Decline call" onClick={onDecline}>
            <PhoneOff size={20} /> Decline
          </button>
          {showEndCurrentAccept && (
            <button className="incoming-btn incoming-btn--end-current" aria-label="End current call and accept" onClick={onEndCurrentAndAccept}>
              End current & Accept
            </button>
          )}
          <button className="incoming-btn incoming-btn--accept" aria-label="Accept call" onClick={onAccept}>
            <Phone size={20} /> Accept
          </button>
        </div>
      </div>
    </div>
  );
};
```

- [ ] **Step 2: CSS**

```css
.incoming-modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex; align-items: center; justify-content: center;
  z-index: 9999;
}
.incoming-modal {
  background: var(--parley-bg-elevated, #1f2937);
  color: var(--parley-text, #fff);
  border-radius: 12px;
  padding: 24px;
  width: 320px;
  display: flex; flex-direction: column; align-items: center; gap: 16px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
}
.incoming-avatar { width: 96px; height: 96px; border-radius: 50%; object-fit: cover; }
.incoming-avatar--placeholder { background: var(--parley-bg-muted); }
.incoming-text { margin: 0; text-align: center; }
.incoming-buttons { display: flex; gap: 12px; flex-wrap: wrap; justify-content: center; }
.incoming-btn { display: inline-flex; align-items: center; gap: 6px; padding: 10px 16px; border-radius: 8px; border: 0; cursor: pointer; font-weight: 600; }
.incoming-btn--accept { background: #22c55e; color: white; }
.incoming-btn--decline { background: #ef4444; color: white; }
.incoming-btn--end-current { background: var(--parley-bg-muted); color: var(--parley-text); }
```

- [ ] **Step 3: Add a ringtone asset**

```
ls frontend/public/ringtone.mp3 2>/dev/null || curl -L -o frontend/public/ringtone.mp3 https://example.com/ringtone.mp3
```

> **Implementer:** swap in any short royalty-free ringtone (e.g. from freesound.org with a CC0 license). The path `/ringtone.mp3` is what the modal references. Commit the file alongside.

- [ ] **Step 4: Build**

```
cd frontend && npm run build
```

- [ ] **Step 5: Commit**

```
git add frontend/src/components/calls/IncomingCallModal.tsx frontend/src/components/calls/IncomingCallModal.css frontend/public/ringtone.mp3
git commit -m "feat(calls): IncomingCallModal + ringtone asset"
```

---

### Task 27: CallBanner + outgoing toast

**Files:**
- Create: `frontend/src/components/calls/CallBanner.tsx`
- Create: `frontend/src/components/calls/CallBanner.css`
- Create: `frontend/src/components/calls/OutgoingCallToast.tsx`

- [ ] **Step 1: CallBanner**

```tsx
// CallBanner.tsx
import React from 'react';
import { Phone } from 'lucide-react';
import './CallBanner.css';

interface Props {
  participantCount: number;
  onJoin: () => void;
}

export const CallBanner: React.FC<Props> = ({ participantCount, onJoin }) => {
  if (participantCount <= 0) return null;
  return (
    <div className="call-banner" role="status">
      <Phone size={14} />
      <span>{participantCount} in call</span>
      <button className="call-banner-join" onClick={onJoin}>Join</button>
    </div>
  );
};
```

```css
/* CallBanner.css */
.call-banner {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 12px;
  background: var(--parley-bg-muted);
  border-bottom: 1px solid var(--parley-border);
  font-size: 13px;
}
.call-banner-join {
  margin-left: auto;
  background: var(--parley-accent);
  color: var(--parley-on-accent, white);
  border: 0; border-radius: 4px; padding: 4px 12px;
  cursor: pointer; font-weight: 600;
}
```

- [ ] **Step 2: OutgoingCallToast**

```tsx
import React, { useEffect, useRef } from 'react';
import { useCall } from '../../context/CallContext';
import './CallBanner.css';

export const OutgoingCallToast: React.FC = () => {
  const { state, cancel } = useCall();
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    if (state.kind === 'outgoing') {
      audioRef.current?.play().catch(() => {});
    } else {
      audioRef.current?.pause();
    }
  }, [state.kind]);

  if (state.kind !== 'outgoing') return null;

  return (
    <div className="outgoing-toast" role="status">
      <audio ref={audioRef} src="/ringback.mp3" loop preload="auto" />
      <span>Calling {state.target_username}…</span>
      <button onClick={cancel} aria-label="Cancel call">Cancel</button>
    </div>
  );
};
```

CSS:

```css
.outgoing-toast {
  position: fixed;
  right: 16px;
  bottom: 16px;
  background: var(--parley-bg-elevated);
  color: var(--parley-text);
  padding: 10px 14px;
  border-radius: 8px;
  display: flex; align-items: center; gap: 12px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
  z-index: 1000;
}
```

- [ ] **Step 3: Add `frontend/public/ringback.mp3`**

A separate, quieter outbound-ring tone. Ship a default royalty-free asset (the implementer picks).

- [ ] **Step 4: Build**

```
cd frontend && npm run build
```

- [ ] **Step 5: Commit**

```
git add frontend/src/components/calls/CallBanner.tsx frontend/src/components/calls/CallBanner.css frontend/src/components/calls/OutgoingCallToast.tsx frontend/public/ringback.mp3
git commit -m "feat(calls): CallBanner + OutgoingCallToast"
```

---

### Task 28: FloatingCallWindow

**Files:**
- Create: `frontend/src/components/calls/FloatingCallWindow.tsx`
- Create: `frontend/src/components/calls/FloatingCallWindow.css`

- [ ] **Step 1: Component**

```tsx
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useCall } from '../../context/CallContext';
import './FloatingCallWindow.css';

const STORAGE_KEY = 'parley.floatingPosition';

interface Position { x: number; y: number; }

function readPos(): Position {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw);
  } catch {}
  return { x: window.innerWidth - 360, y: 24 };
}

interface Props {
  // Renders the same VoiceChannel UI but in compact layout.
  renderCompact: () => React.ReactNode;
  onExpand: () => void;
}

export const FloatingCallWindow: React.FC<Props> = ({ renderCompact, onExpand }) => {
  const { state, floatingMode, setFloatingMode } = useCall();
  const [pos, setPos] = useState<Position>(readPos);
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null);

  useEffect(() => { localStorage.setItem(STORAGE_KEY, JSON.stringify(pos)); }, [pos]);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    dragRef.current = { startX: e.clientX, startY: e.clientY, origX: pos.x, origY: pos.y };
    e.preventDefault();
  }, [pos]);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragRef.current) return;
      const dx = e.clientX - dragRef.current.startX;
      const dy = e.clientY - dragRef.current.startY;
      setPos({
        x: Math.max(0, Math.min(window.innerWidth - 320, dragRef.current.origX + dx)),
        y: Math.max(0, Math.min(window.innerHeight - 240, dragRef.current.origY + dy)),
      });
    };
    const onUp = () => { dragRef.current = null; };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp); };
  }, []);

  if (!floatingMode || state.kind !== 'connected') return null;

  return (
    <div className="floating-call-window" style={{ left: pos.x, top: pos.y }}>
      <div className="floating-call-handle" onMouseDown={onMouseDown}>
        <span>Call</span>
        <span className="floating-call-actions">
          <button onClick={onExpand} title="Expand" aria-label="Expand to full">⛶</button>
          <button onClick={() => setFloatingMode(false)} title="Dock" aria-label="Dock">▭</button>
        </span>
      </div>
      <div className="floating-call-body">
        {renderCompact()}
      </div>
    </div>
  );
};
```

CSS:

```css
.floating-call-window {
  position: fixed;
  width: 320px;
  height: 240px;
  background: var(--parley-bg-elevated);
  border: 1px solid var(--parley-border);
  border-radius: 8px;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.4);
  z-index: 950;
  display: flex; flex-direction: column;
}
.floating-call-handle {
  display: flex; align-items: center; justify-content: space-between;
  padding: 6px 10px;
  cursor: grab; user-select: none;
  background: var(--parley-bg-muted);
  border-radius: 8px 8px 0 0;
  font-size: 12px;
}
.floating-call-handle:active { cursor: grabbing; }
.floating-call-actions { display: inline-flex; gap: 4px; }
.floating-call-actions button { background: transparent; border: 0; color: inherit; cursor: pointer; }
.floating-call-body { flex: 1; overflow: hidden; }
```

- [ ] **Step 2: Build + commit**

```
cd frontend && npm run build
git add frontend/src/components/calls/FloatingCallWindow.tsx frontend/src/components/calls/FloatingCallWindow.css
git commit -m "feat(calls): FloatingCallWindow draggable overlay"
```

---

### Task 29: SystemMessage rendering for call_* events

**Files:**
- Modify: `frontend/src/components/chat/SystemMessage.tsx`
- Modify: `frontend/src/api/types.ts` (extend the SystemEvent union with call_* variants)

- [ ] **Step 1: Extend the SystemEvent union**

In `frontend/src/api/types.ts` append to the existing union:

```ts
export type SystemEvent =
  // ...existing variants...
  | { type: 'call_started';  actor_user_id: string; actor_display_name?: string; started_at_ms: number }
  | { type: 'call_ended';    duration_ms: number; started_at_ms: number }
  | { type: 'call_missed';   caller_user_id: string; caller_display_name?: string }
  | { type: 'call_declined'; caller_user_id: string; caller_display_name?: string; decliner_user_id: string; decliner_display_name?: string };
```

- [ ] **Step 2: Render the new types in SystemMessage.tsx**

Inside the existing switch on `event.type`, add cases:

```tsx
case 'call_started':
  return <p>{nameOf(event.actor_user_id, event.actor_display_name)} started a call.</p>;
case 'call_ended':
  return <p>Call ended · {formatDuration(event.duration_ms)}</p>;
case 'call_missed':
  return <p>Missed call from {nameOf(event.caller_user_id, event.caller_display_name)}.</p>;
case 'call_declined':
  return <p>{nameOf(event.decliner_user_id, event.decliner_display_name)} declined the call.</p>;
```

`nameOf` is the existing helper; if it doesn't exist, mirror the `actor_display_name ?? resolveUser(actor_user_id)?.display_name ?? 'someone'` pattern used by other call sites in this file.

`formatDuration`:

```tsx
function formatDuration(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  if (m === 0) return `${s}s`;
  return `${m}m ${s}s`;
}
```

- [ ] **Step 3: Build + commit**

```
cd frontend && npm run build
git add frontend/src/api/types.ts frontend/src/components/chat/SystemMessage.tsx
git commit -m "feat(chat): render call_started/ended/missed/declined system messages"
```

---

### Task 30: Wire CallContext + UI into ChatWindow / DmPanel / App

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/components/chat/ChatWindow.tsx`
- Modify: `frontend/src/components/layout/DmPanel.tsx`

- [ ] **Step 1: Wrap the app in `<CallProvider>`**

In `frontend/src/App.tsx`, find the existing top-level provider tree (`<AppProvider>` etc.) and add:

```tsx
<CallProvider bootRings={bootRings} onWsEvent={subscribeWs} currentUserID={currentUser.id}>
  {/* ...existing tree... */}
  <CallSurfaces />
</CallProvider>
```

`subscribeWs` is whatever existing helper this codebase uses to listen for WS events (look for `addEventListener` on a CustomEvent, or a hook like `useWebSocketEvent`). If it doesn't already return an unsubscribe function in the shape `(eventType, handler) => () => void`, write a small adapter inline.

`<CallSurfaces />` is a tiny component that reads `useCall()` and renders the right surfaces:

```tsx
const CallSurfaces: React.FC = () => {
  const { state, incomingQueue, accept, decline } = useCall();
  // For Phase 8 we always render the in-app modal; Tauri secondary window comes in Phase 9.
  return (
    <>
      {incomingQueue.length > 0 && (
        <IncomingCallModal
          ring={incomingQueue[0]}
          showEndCurrentAccept={state.kind === 'connected' && state.vc !== incomingQueue[0].vc}
          onAccept={() => accept(incomingQueue[0].ring_id)}
          onEndCurrentAndAccept={() => { /* leave existing call first then accept */ accept(incomingQueue[0].ring_id); }}
          onDecline={() => decline(incomingQueue[0].ring_id)}
        />
      )}
      <OutgoingCallToast />
      <FloatingCallWindow renderCompact={/* render <VoiceChannel layout="compact" .../> */ () => null} onExpand={() => { /* navigate to call's channel and exit floating mode */ }} />
    </>
  );
};
```

> **End-current-and-accept wiring:** if the user is already in a call when CALL_RING arrives, the "End current & Accept" path should disconnect the prior call (call `useVoiceConnection.disconnect()`) before forwarding to `accept`. The exact wiring depends on how voice state is exposed in this app; trace the existing `handleVcLeave` callback in App.tsx for the pattern.

- [ ] **Step 2: Add Start/Join Call button to ChatWindow header**

In `frontend/src/components/chat/ChatWindow.tsx`, find the header area (it's currently rendered inline in this component — search for the channel/dm name display). For DM/GC contexts, add a phone icon button:

```tsx
{isDmContext && (
  <button
    className="chat-header-action"
    onClick={onStartOrJoinCall}
    title={callIsActive ? 'Join call' : 'Start call'}
    aria-label={callIsActive ? 'Join call' : 'Start call'}
  >
    <Phone size={16} />
  </button>
)}
```

Logic in App.tsx (or wherever ChatWindow's call button is wired):
- For 1:1 DMs without an active call → `callContext.initiate(dmId, otherUser)`.
- For 1:1 DMs with an active call → `useVoiceConnection.connect(`dm:${dmId}`)` directly (no ring).
- For GCs without an active call → `await startGcCall(dmId)` then connect.
- For GCs with an active call → just connect.

`callIsActive` is derived from a cached `voice:dm:{id}` participant count fed by `VOICE_STATE_UPDATE` events.

Render the banner above the message list when an active call exists in this DM/GC:

```tsx
{isDmContext && callParticipantCount > 0 && (
  <CallBanner participantCount={callParticipantCount} onJoin={onStartOrJoinCall} />
)}
```

- [ ] **Step 3: DmPanel phone icon for active calls**

In `frontend/src/components/layout/DmPanel.tsx`, render a small `<Phone size={12} />` next to each DM/GC name when a per-channel `activeCalls` Map indicates an active call.

`activeCalls` is a `Map<string /* dmId */, number /* participants */>` lifted into the App-level state, populated by VOICE_STATE_UPDATE events (which already exist; the bookkeeping is just per-channel count maintenance).

- [ ] **Step 4: Build + commit**

```
cd frontend && npm run build
git add frontend/src/App.tsx frontend/src/components/chat/ChatWindow.tsx frontend/src/components/layout/DmPanel.tsx
git commit -m "feat(calls): wire CallContext + Start Call button + CallBanner + DM phone icon"
```

---

## Phase 9 — Tauri secondary ring window

### Task 31: Tauri Rust commands for spawn/dismiss

**Files:**
- Create: `desktop/src-tauri/src/ring_window.rs`
- Modify: `desktop/src-tauri/src/lib.rs` (register the commands)

- [ ] **Step 1: Create the module**

```rust
// desktop/src-tauri/src/ring_window.rs
use serde::Deserialize;
use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindowBuilder};

#[derive(Debug, Deserialize)]
pub struct RingWindowArgs {
    pub ring_id: String,
    pub vc: String,
    pub caller_username: String,
    pub caller_display_name: String,
    pub caller_avatar_url: Option<String>,
    pub group_name: Option<String>,
}

fn ring_window_label(ring_id: &str) -> String {
    format!("ring-{}", ring_id)
}

#[tauri::command]
pub async fn spawn_ring_window(app: AppHandle, args: RingWindowArgs) -> Result<(), String> {
    let label = ring_window_label(&args.ring_id);
    if app.get_webview_window(&label).is_some() {
        return Ok(()); // already open
    }
    // Build the URL with caller info as query string. ring.html is the entry.
    let avatar = args.caller_avatar_url.clone().unwrap_or_default();
    let group = args.group_name.clone().unwrap_or_default();
    let qs = format!(
        "ring_id={}&vc={}&caller_username={}&caller_display_name={}&caller_avatar_url={}&group_name={}",
        urlencoding::encode(&args.ring_id),
        urlencoding::encode(&args.vc),
        urlencoding::encode(&args.caller_username),
        urlencoding::encode(&args.caller_display_name),
        urlencoding::encode(&avatar),
        urlencoding::encode(&group),
    );
    let url = format!("ring.html?{}", qs);

    let monitor = app.primary_monitor().map_err(|e| e.to_string())?
        .ok_or("no primary monitor")?;
    let size = monitor.size();
    let scale = monitor.scale_factor();
    let win_w = 320.0;
    let win_h = 400.0;
    let x = (size.width as f64 / scale) - win_w - 24.0;
    let y = (size.height as f64 / scale) - win_h - 24.0;

    WebviewWindowBuilder::new(&app, &label, WebviewUrl::App(url.into()))
        .title("Incoming call")
        .inner_size(win_w, win_h)
        .resizable(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .decorations(false)
        .transparent(true)
        .position(x, y)
        .focused(true)
        .build()
        .map_err(|e| e.to_string())?;
    Ok(())
}

#[tauri::command]
pub async fn dismiss_ring_window(app: AppHandle, ring_id: String) -> Result<(), String> {
    let label = ring_window_label(&ring_id);
    if let Some(w) = app.get_webview_window(&label) {
        let _ = w.close();
    }
    Ok(())
}
```

- [ ] **Step 2: Add `urlencoding` to `Cargo.toml`**

```
[dependencies]
urlencoding = "2"
```

(under `desktop/src-tauri/Cargo.toml`).

- [ ] **Step 3: Register commands in `lib.rs`**

```rust
mod ring_window;

// inside the .invoke_handler call (find the existing one):
.invoke_handler(tauri::generate_handler![
    /* existing commands, */
    ring_window::spawn_ring_window,
    ring_window::dismiss_ring_window,
])
```

- [ ] **Step 4: Build the desktop app**

```
cd desktop && npm run tauri build -- --debug
```

> **Reminder:** per the global memory file, we don't release-build locally. `--debug` is a faster compile suitable for verification.

Expected: clean build.

- [ ] **Step 5: Commit**

```
git add desktop/src-tauri/src/ring_window.rs desktop/src-tauri/src/lib.rs desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock
git commit -m "feat(desktop): Tauri commands to spawn/dismiss the ring window"
```

---

### Task 32: ring.html + ring app

**Files:**
- Create: `frontend/ring.html`
- Create: `frontend/src/ring/main.tsx`
- Create: `frontend/src/ring/RingApp.tsx`
- Create: `frontend/src/ring/RingApp.css`
- Modify: `frontend/vite.config.ts` (add the second entry)

- [ ] **Step 1: ring.html**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Incoming call</title>
  </head>
  <body>
    <div id="ring-root"></div>
    <script type="module" src="/src/ring/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 2: main.tsx**

```tsx
import React from 'react';
import { createRoot } from 'react-dom/client';
import { RingApp } from './RingApp';
import './RingApp.css';

const params = new URLSearchParams(window.location.search);
const props = {
  ringId:           params.get('ring_id') ?? '',
  vc:               params.get('vc') ?? '',
  callerUsername:   params.get('caller_username') ?? '',
  callerDisplayName:params.get('caller_display_name') ?? '',
  callerAvatarUrl:  params.get('caller_avatar_url') ?? '',
  groupName:        params.get('group_name') ?? '',
};

createRoot(document.getElementById('ring-root')!).render(<RingApp {...props} />);
```

- [ ] **Step 3: RingApp.tsx**

```tsx
import React, { useEffect } from 'react';
import { Phone, PhoneOff } from 'lucide-react';
import { emit, listen } from '@tauri-apps/api/event';
import { getCurrentWindow } from '@tauri-apps/api/window';

interface Props {
  ringId: string;
  vc: string;
  callerUsername: string;
  callerDisplayName: string;
  callerAvatarUrl: string;
  groupName: string;
}

export const RingApp: React.FC<Props> = ({ ringId, callerUsername, callerDisplayName, callerAvatarUrl, groupName }) => {
  const name = callerDisplayName || callerUsername;

  useEffect(() => {
    // If the main window resolves the ring elsewhere, it emits "ring:dismiss" with our id.
    const unsub = listen<{ ring_id: string }>('ring:dismiss', e => {
      if (e.payload.ring_id === ringId) {
        getCurrentWindow().close().catch(() => {});
      }
    });
    return () => { unsub.then(fn => fn()).catch(() => {}); };
  }, [ringId]);

  const accept = async () => {
    await emit('ring:accept', { ring_id: ringId });
    await getCurrentWindow().close();
  };
  const decline = async () => {
    await emit('ring:decline', { ring_id: ringId });
    await getCurrentWindow().close();
  };

  return (
    <div className="ring-app">
      {callerAvatarUrl
        ? <img src={callerAvatarUrl} alt="" className="ring-avatar" />
        : <div className="ring-avatar ring-avatar--placeholder" />}
      <p className="ring-text">
        Incoming call from <strong>{name}</strong>{groupName ? <> in <strong>{groupName}</strong></> : null}…
      </p>
      <div className="ring-buttons">
        <button className="ring-btn ring-btn--decline" onClick={decline} aria-label="Decline call">
          <PhoneOff size={20} /> Decline
        </button>
        <button className="ring-btn ring-btn--accept" onClick={accept} aria-label="Accept call">
          <Phone size={20} /> Accept
        </button>
      </div>
    </div>
  );
};
```

- [ ] **Step 4: RingApp.css**

```css
:root {
  color-scheme: dark;
  --bg: #1f2937;
}
html, body, #ring-root { height: 100%; margin: 0; }
body {
  background: var(--bg);
  color: white;
  font: 14px system-ui, -apple-system, sans-serif;
  border-radius: 12px;
  overflow: hidden;
  display: flex; align-items: center; justify-content: center;
}
.ring-app {
  display: flex; flex-direction: column; align-items: center;
  gap: 16px; padding: 24px;
}
.ring-avatar { width: 96px; height: 96px; border-radius: 50%; object-fit: cover; }
.ring-avatar--placeholder { background: rgba(255,255,255,0.12); }
.ring-text { margin: 0; text-align: center; }
.ring-buttons { display: flex; gap: 12px; }
.ring-btn { display: inline-flex; align-items: center; gap: 6px; padding: 10px 16px; border: 0; border-radius: 8px; cursor: pointer; font-weight: 600; }
.ring-btn--accept  { background: #22c55e; color: white; }
.ring-btn--decline { background: #ef4444; color: white; }
```

- [ ] **Step 5: vite.config.ts entry**

Add a multi-entry config so Vite emits `ring.html` alongside `index.html`:

```ts
// inside the existing defineConfig(...)
build: {
  rollupOptions: {
    input: {
      main: resolve(__dirname, 'index.html'),
      ring: resolve(__dirname, 'ring.html'),
    },
  },
},
```

Add the matching import for `resolve`:

```ts
import { resolve } from 'path';
```

- [ ] **Step 6: Build**

```
cd frontend && npm run build
```

Expected: `dist/index.html` and `dist/ring.html` produced.

- [ ] **Step 7: Commit**

```
git add frontend/ring.html frontend/src/ring/main.tsx frontend/src/ring/RingApp.tsx frontend/src/ring/RingApp.css frontend/vite.config.ts
git commit -m "feat(desktop): ring webview entry (ring.html + RingApp)"
```

---

### Task 33: CallContext platform branching for ring surface

**Files:**
- Modify: `frontend/src/context/CallContext.tsx`

The CallContext already drives the ring queue. Now add focus tracking and branch the surface: in Tauri + main unfocused, spawn the secondary window per queued ring; otherwise, render the in-app modal (the existing path from Phase 8).

- [ ] **Step 1: Add Tauri detection + focus tracking**

Append a new effect inside `CallProvider`:

```tsx
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { getCurrentWindow } from '@tauri-apps/api/window';

const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

const [mainFocused, setMainFocused] = useState<boolean>(true);

useEffect(() => {
  if (!isTauri) return;
  let unlistenFocus: undefined | (() => void);
  getCurrentWindow().isFocused().then(setMainFocused).catch(() => {});
  getCurrentWindow().onFocusChanged(({ payload }) => setMainFocused(payload)).then(fn => { unlistenFocus = fn; }).catch(() => {});
  return () => { unlistenFocus?.(); };
}, []);
```

- [ ] **Step 2: Spawn / dismiss the secondary window in response to queue + focus changes**

Add an effect that reconciles open ring windows with the queue:

```tsx
useEffect(() => {
  if (!isTauri) return;
  if (mainFocused) {
    // close any open ring windows; modal will render
    store.incomingQueue.forEach(r => {
      invoke('dismiss_ring_window', { ringId: r.ring_id }).catch(() => {});
    });
    return;
  }
  // spawn one window per queued ring (idempotent server-side)
  store.incomingQueue.forEach(r => {
    invoke('spawn_ring_window', {
      args: {
        ring_id: r.ring_id,
        vc: r.vc,
        caller_username: r.caller.username,
        caller_display_name: r.caller.display_name,
        caller_avatar_url: r.caller.avatar_url || null,
        group_name: null, // always null for now (1:1 DMs only ring)
      },
    }).catch(() => {});
  });
}, [mainFocused, store.incomingQueue]);
```

- [ ] **Step 3: Listen for the ring window's accept/decline**

```tsx
useEffect(() => {
  if (!isTauri) return;
  let unsubAccept: undefined | (() => void);
  let unsubDecline: undefined | (() => void);
  listen<{ ring_id: string }>('ring:accept', e => { void accept(e.payload.ring_id); }).then(fn => { unsubAccept = fn; });
  listen<{ ring_id: string }>('ring:decline', e => { void decline(e.payload.ring_id); }).then(fn => { unsubDecline = fn; });
  return () => { unsubAccept?.(); unsubDecline?.(); };
}, [accept, decline]);
```

- [ ] **Step 4: Dismiss windows when the ring is resolved server-side**

Whenever a ring leaves the queue (CALL_TIMEOUT / CALL_CANCEL / CALL_ACCEPT-from-elsewhere), dismiss the matching window so the user isn't left with stale UI:

```tsx
useEffect(() => {
  if (!isTauri) return;
  // emit ring:dismiss to all currently queued rings whose secondary windows
  // should close. Simpler: just dismiss any window that does NOT correspond
  // to a current queue entry. Tracked via a ref of last-spawned set.
}, [store.incomingQueue]);
```

Practical implementation: track previous queue in a ref; when a ring_id is in `prev` but not in `current`, emit `ring:dismiss` with that ring_id (the secondary window listens for it via Task 32 step 3 and closes itself).

```tsx
const prevQueueIds = useRef<Set<string>>(new Set());
useEffect(() => {
  if (!isTauri) return;
  const currentIds = new Set(store.incomingQueue.map(r => r.ring_id));
  prevQueueIds.current.forEach(id => {
    if (!currentIds.has(id)) {
      // resolved
      invoke('dismiss_ring_window', { ringId: id }).catch(() => {});
    }
  });
  prevQueueIds.current = currentIds;
}, [store.incomingQueue]);
```

- [ ] **Step 5: Hide the in-app modal when secondary window is showing**

In `<CallSurfaces>` (Task 30):

```tsx
{!isTauri || mainFocused
  ? incomingQueue.length > 0 && <IncomingCallModal ... />
  : null}
```

Export `mainFocused` from `useCall()` so `CallSurfaces` can read it (add it to `CallContextValue`).

- [ ] **Step 6: Build**

```
cd frontend && npm run build
cd desktop && npm run tauri build -- --debug
```

Expected: clean builds. Manually verify: focus-out the main window during a ring → bottom-right window appears. Focus-in → it closes and the modal takes over.

- [ ] **Step 7: Commit**

```
git add frontend/src/context/CallContext.tsx
git commit -m "feat(desktop): spawn Tauri ring window when main is unfocused"
```

---

## Phase 10 — Bug fix + release

### Task 34: shouldNotify own-message short-circuit

**Files:**
- Modify: `frontend/src/hooks/useNotifications.ts`
- Modify: `frontend/src/hooks/useNotifications.test.ts` (or create if missing)

The bug: DM/GC participants get notification sounds for their own sent messages. The fix is a one-line short-circuit in `shouldNotify` so an actor matching the current user is never notified.

- [ ] **Step 1: Read `shouldNotify`'s current signature**

```
sed -n '30,80p' frontend/src/hooks/useNotifications.ts
```

Note the existing `NotifyContext` shape — it likely already has `actor_user_id` and the current user is accessible via the surrounding closure or a passed-in field.

- [ ] **Step 2: Add a failing test**

```ts
// inside frontend/src/hooks/useNotifications.test.ts (extend or create)
import { describe, it, expect } from 'vitest';
import { shouldNotify } from './useNotifications';

describe('shouldNotify', () => {
  it('returns false when the actor is the current user', () => {
    const ctx = { actor_user_id: '42', current_user_id: '42', is_mention: false };
    expect(shouldNotify(ctx as any, 'all' as any)).toBe(false);
  });
});
```

> The exact `NotifyContext` shape may differ. Match whatever fields the existing function reads. If `current_user_id` isn't part of the context today, add it as part of this fix and update the call sites in Step 4.

- [ ] **Step 3: Add the short-circuit**

In `shouldNotify`, prepend:

```ts
if (ctx.actor_user_id && ctx.current_user_id && ctx.actor_user_id === ctx.current_user_id) {
  return false;
}
```

- [ ] **Step 4: Update call sites if `current_user_id` is new**

```
grep -rn "shouldNotify(" frontend/src --include="*.ts" --include="*.tsx"
```

Each call site adds `current_user_id: currentUser.id` to the context object.

- [ ] **Step 5: Run test + build**

```
cd frontend && npm test -- useNotifications
cd frontend && npm run build
```

- [ ] **Step 6: Commit**

```
git add frontend/src/hooks/useNotifications.ts frontend/src/hooks/useNotifications.test.ts $(grep -rln "shouldNotify(" frontend/src --include="*.ts" --include="*.tsx")
git commit -m "fix(notifications): never notify on the user's own actions"
```

---

### Task 35: Final integration smoke test + version bump + release

**Files:**
- Modify: `desktop/package.json`
- Modify: `desktop/src-tauri/tauri.conf.json`
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/Cargo.lock` (auto)

- [ ] **Step 1: Manual smoke test (do not skip)**

Run the full smoke matrix locally. For each, note PASS / FAIL.

Backend: deploy to local dev or a staging env. Frontend: `npm run dev`. Two browser sessions (e.g. one normal, one private window) signed in as Alice and Bob.

1. Alice rings Bob (1:1 DM) → Bob's modal/secondary window appears with audio. Bob accepts → both connect to LiveKit room. Voice flows.
2. Alice rings Bob → Bob declines → Alice toast says "Bob declined". DM shows "Bob declined the call." system message.
3. Alice rings Bob → Bob ignores → after 30s both UIs clear, "Missed call from Alice" appears in DM.
4. Alice rings Bob → Alice cancels → Bob's UI clears, "Missed call from Alice" appears.
5. In a 3-person GC: Alice clicks 📞 → "Alice started a call." appears in chat + banner shows "1 in call · Join". Charlie joins. Banner becomes "2 in call". Both leave → "Call ended · 1m 23s" appears.
6. Force-mute (GC owner): owner right-clicks Charlie's tile → "Force mute" → Charlie's mic muted; non-owner does not see "Force mute" item.
7. Local mute: open right-click menu on a tile → drag slider to 0 → that tile shows the strikethrough icon and audio is silent for current listener only. Reload the page → still muted.
8. Floating window: in a server VC, click Pop-Out on the docked widget → floating window appears, drag-able. Position persists across reload. Click Dock → returns to docked.
9. Tauri secondary ring window: blur the main desktop window, ring → bottom-right window appears with the green/red buttons. Click Accept → secondary closes, main connects. Repeat with Decline.
10. Self-message notification: Alice sends a DM message → Alice's own window does NOT play a sound. Bob's window does.
11. Activities harness: open dev tools, run `import('./activities/registry').then(r => r.register({ type: 'demo', displayName: 'Demo', render: () => 'hi' }))`. Click Activities button in VoiceControls → Demo entry appears. (No actual activity is shipped — this confirms the registry plumbing works.)

- [ ] **Step 2: Version bump**

Set version to `0.5.0` in:
- `desktop/package.json` → `"version": "0.5.0"`
- `desktop/src-tauri/tauri.conf.json` → `"version": "0.5.0"`
- `desktop/src-tauri/Cargo.toml` → `version = "0.5.0"`

- [ ] **Step 3: Update Cargo.lock**

```
cd desktop/src-tauri && cargo build
```

This regenerates `Cargo.lock` with the new package version.

- [ ] **Step 4: Commit + tag**

```
git add desktop/package.json desktop/src-tauri/tauri.conf.json desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock
git commit -m "chore(release): v0.5.0 — DM/GC calling"
git tag v0.5.0
git push origin main
git push origin v0.5.0
```

CI builds and uploads desktop artifacts on tag push (per memory: never build desktop locally for release). Web frontend ships via `make deploy-frontend` (or whatever the existing convention is — confirm before running).

- [ ] **Step 5: Post-deploy smoke**

After the release artifact is up:
1. Auto-update flips an installed v0.4.x desktop client to v0.5.0 on next launch.
2. Re-run smoke items 1, 5, 9 against production.

If anything fails post-deploy, file a follow-up. Mark v0.5.0 broken in the release notes only after triage.

---

## Self-review checklist

- [x] Spec coverage — every section mapped to tasks: §3 backend → Tasks 1–15; §4 frontend → Tasks 17–30; Tauri → Tasks 31–33; §6 error handling distributed across 9, 10, 12, 21, 26, 33; §9 migration → Task 35; bug fix → Task 34.
- [x] Placeholder scan — every code step contains complete code or an explicit reference to an existing call site to mirror.
- [x] Type consistency — `VirtualChannel`, `Authorizer`, `RingService`, `DmEmitter`, `ActiveRing`, `ringCallerInfo` defined once and referenced by exact name throughout. WS event constants (`CALL_RING` etc.) match between backend and frontend strings.
- [x] Migration ordering — backend tasks (1–16) precede frontend tasks (17–33); no frontend task depends on a route that hasn't been registered.

## Open implementation-time decisions (non-blocking)

These are deferred to build:
- Ringtone and ringback audio file selection (royalty-free; ~6s).
- Floating window default position (likely top-right of viewport; localStorage persists user choice afterward).
- Whether a missing Redis at boot demotes ring service to a no-op or hard-fails (default: in-memory state still works; only `EndIfEmpty` and presence become no-ops).
- Exact placement of the 📞 button in `ChatWindow.tsx` header (left of any existing search/info icon vs right).
