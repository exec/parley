package voice

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestService spins up a miniredis instance and returns a Service backed by it.
func newTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := &Service{rdb: rdb}
	return svc, mr
}

// TestJoinSetsPresenceAndHeartbeat verifies that Join writes both the presence
// hash entry and the heartbeat key with the correct TTL.
func TestJoinSetsPresenceAndHeartbeat(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if _, err := svc.Join(ctx, "ch1", "u1", "Alice", "https://img/a.png"); err != nil {
		t.Fatalf("Join: %v", err)
	}

	// Presence hash should contain user
	val := mr.HGet("voice:ch1", "u1")
	if val == "" {
		t.Fatal("presence hash entry is empty")
	}

	// Heartbeat key should exist with TTL
	if !mr.Exists("voice:heartbeat:ch1:u1") {
		t.Fatal("heartbeat key should exist after Join")
	}
	ttl := mr.TTL("voice:heartbeat:ch1:u1")
	if ttl <= 0 {
		t.Errorf("heartbeat TTL should be positive, got %v", ttl)
	}
	if ttl > voiceHeartbeatTTL {
		t.Errorf("heartbeat TTL %v exceeds configured %v", ttl, voiceHeartbeatTTL)
	}
}

// TestLeaveRemovesPresenceAndHeartbeat verifies that Leave removes both the
// presence hash entry and the heartbeat key.
func TestLeaveRemovesPresenceAndHeartbeat(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	if _, err := svc.Join(ctx, "ch1", "u1", "Alice", ""); err != nil {
		t.Fatalf("Join: %v", err)
	}

	if err := svc.Leave(ctx, "ch1", "u1"); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	// Presence hash entry should be gone
	if mr.HGet("voice:ch1", "u1") != "" {
		t.Error("presence hash entry should be removed after Leave")
	}

	// Heartbeat key should be gone
	if mr.Exists("voice:heartbeat:ch1:u1") {
		t.Error("heartbeat key should be removed after Leave")
	}
}

// TestParticipantsReturnsJoinedUsers verifies that Participants returns all
// users who have joined a channel.
func TestParticipantsReturnsJoinedUsers(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.Join(ctx, "ch1", "u1", "Alice", "")
	svc.Join(ctx, "ch1", "u2", "Bob", "https://img/b.png")

	participants, err := svc.Participants(ctx, "ch1")
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}

	if len(participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(participants))
	}

	byID := map[string]Participant{}
	for _, p := range participants {
		byID[p.UserID] = p
	}

	if p, ok := byID["u1"]; !ok {
		t.Error("u1 not found in participants")
	} else if p.Username != "Alice" {
		t.Errorf("u1 username = %q, want Alice", p.Username)
	}

	if p, ok := byID["u2"]; !ok {
		t.Error("u2 not found in participants")
	} else if p.AvatarURL != "https://img/b.png" {
		t.Errorf("u2 avatar = %q, want https://img/b.png", p.AvatarURL)
	}
}

// TestParticipantsEmptyChannel returns an empty slice for a channel with no
// participants.
func TestParticipantsEmptyChannel(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	participants, err := svc.Participants(ctx, "empty-ch")
	if err != nil {
		t.Fatalf("Participants: %v", err)
	}
	if len(participants) != 0 {
		t.Errorf("expected 0 participants, got %d", len(participants))
	}
}

// TestIsParticipant verifies that IsParticipant correctly reports membership.
func TestIsParticipant(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Not joined yet
	ok, err := svc.IsParticipant(ctx, "ch1", "u1")
	if err != nil {
		t.Fatalf("IsParticipant: %v", err)
	}
	if ok {
		t.Error("should be false before joining")
	}

	svc.Join(ctx, "ch1", "u1", "Alice", "")

	ok, err = svc.IsParticipant(ctx, "ch1", "u1")
	if err != nil {
		t.Fatalf("IsParticipant: %v", err)
	}
	if !ok {
		t.Error("should be true after joining")
	}

	svc.Leave(ctx, "ch1", "u1")

	ok, err = svc.IsParticipant(ctx, "ch1", "u1")
	if err != nil {
		t.Fatalf("IsParticipant: %v", err)
	}
	if ok {
		t.Error("should be false after leaving")
	}
}

// TestRefreshHeartbeatResetsExpiry verifies that RefreshHeartbeat resets the
// TTL on the heartbeat key.
func TestRefreshHeartbeatResetsExpiry(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	svc.Join(ctx, "ch1", "u1", "Alice", "")

	// Simulate time passing by fast-forwarding miniredis
	mr.FastForward(voiceHeartbeatTTL / 2)

	// Refresh should reset TTL
	if err := svc.RefreshHeartbeat(ctx, "ch1", "u1"); err != nil {
		t.Fatalf("RefreshHeartbeat: %v", err)
	}

	ttl := mr.TTL("voice:heartbeat:ch1:u1")
	// After refresh, TTL should be close to voiceHeartbeatTTL, not voiceHeartbeatTTL/2
	if ttl <= voiceHeartbeatTTL/2 {
		t.Errorf("TTL after refresh = %v, expected > %v", ttl, voiceHeartbeatTTL/2)
	}
}

// TestHeartbeatExpiresNaturally verifies that after voiceHeartbeatTTL elapses,
// the heartbeat key is gone (simulated with miniredis FastForward).
func TestHeartbeatExpiresNaturally(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	svc.Join(ctx, "ch1", "u1", "Alice", "")

	mr.FastForward(voiceHeartbeatTTL + 1)

	if mr.Exists("voice:heartbeat:ch1:u1") {
		t.Error("heartbeat key should have expired")
	}
}

// TestNilRedisNoOps verifies that all methods are safe to call when rdb is nil.
func TestNilRedisNoOps(t *testing.T) {
	svc := &Service{rdb: nil}
	ctx := context.Background()

	if _, err := svc.Join(ctx, "ch", "u", "n", ""); err != nil {
		t.Errorf("Join with nil redis: %v", err)
	}
	if err := svc.Leave(ctx, "ch", "u"); err != nil {
		t.Errorf("Leave with nil redis: %v", err)
	}
	if err := svc.RefreshHeartbeat(ctx, "ch", "u"); err != nil {
		t.Errorf("RefreshHeartbeat with nil redis: %v", err)
	}
	participants, err := svc.Participants(ctx, "ch")
	if err != nil {
		t.Errorf("Participants with nil redis: %v", err)
	}
	if len(participants) != 0 {
		t.Errorf("expected empty participants with nil redis, got %d", len(participants))
	}
	ok, err := svc.IsParticipant(ctx, "ch", "u")
	if err != nil {
		t.Errorf("IsParticipant with nil redis: %v", err)
	}
	if ok {
		t.Error("IsParticipant should return false with nil redis")
	}
}

// TestConfiguredReturnsFalseWhenEmpty verifies that Configured() is false
// when no env vars are set.
func TestConfiguredReturnsFalseWhenEmpty(t *testing.T) {
	svc := &Service{}
	if svc.Configured() {
		t.Error("Configured() should return false when keys are empty")
	}
}

// TestConfiguredReturnsTrueWhenSet verifies that Configured() is true when
// all required fields are populated.
func TestConfiguredReturnsTrueWhenSet(t *testing.T) {
	svc := &Service{
		apiKey:    "key",
		apiSecret: "secret",
		serverURL: "wss://lk.example.com",
	}
	if !svc.Configured() {
		t.Error("Configured() should return true when all fields are set")
	}
}

// TestServerURLReturnsConfiguredValue verifies the ServerURL accessor.
func TestServerURLReturnsConfiguredValue(t *testing.T) {
	svc := &Service{serverURL: "wss://lk.example.com"}
	if svc.ServerURL() != "wss://lk.example.com" {
		t.Errorf("ServerURL() = %q, want wss://lk.example.com", svc.ServerURL())
	}
}

// TestMultipleChannelsIsolated verifies that joining channel A does not
// affect channel B.
func TestMultipleChannelsIsolated(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	svc.Join(ctx, "ch1", "u1", "Alice", "")
	svc.Join(ctx, "ch2", "u2", "Bob", "")

	p1, _ := svc.Participants(ctx, "ch1")
	p2, _ := svc.Participants(ctx, "ch2")

	if len(p1) != 1 || p1[0].UserID != "u1" {
		t.Errorf("ch1 should have only u1, got %v", p1)
	}
	if len(p2) != 1 || p2[0].UserID != "u2" {
		t.Errorf("ch2 should have only u2, got %v", p2)
	}

	// Leaving ch1 should not affect ch2
	svc.Leave(ctx, "ch1", "u1")

	p2After, _ := svc.Participants(ctx, "ch2")
	if len(p2After) != 1 {
		t.Errorf("ch2 should still have 1 participant after ch1 leave, got %d", len(p2After))
	}
}
