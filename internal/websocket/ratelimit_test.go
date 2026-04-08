package websocket

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// allowWSMessage token-bucket rate-limiter tests
// ---------------------------------------------------------------------------

// TestAllowWSMessageBurstAllowed verifies that a fresh client can send up to
// the burst limit (60) without being throttled.
func TestAllowWSMessageBurstAllowed(t *testing.T) {
	c := newTestClient(NewHub(), "user1")
	for i := 0; i < 60; i++ {
		if !c.allowWSMessage() {
			t.Fatalf("message %d should be allowed within burst", i+1)
		}
	}
}

// TestAllowWSMessageBurstExceeded verifies that the 61st message in a burst
// (with no time passing) is rejected.
func TestAllowWSMessageBurstExceeded(t *testing.T) {
	c := newTestClient(NewHub(), "user1")
	for i := 0; i < 60; i++ {
		c.allowWSMessage()
	}
	if c.allowWSMessage() {
		t.Error("61st message should be rejected after burst exhausted")
	}
}

// TestAllowWSMessageTokenRefill verifies that tokens replenish over time at
// the configured rate (30/s). We manually advance lastSeen to simulate elapsed
// time without sleeping.
func TestAllowWSMessageTokenRefill(t *testing.T) {
	c := newTestClient(NewHub(), "user1")

	// Exhaust all burst tokens
	for i := 0; i < 60; i++ {
		c.allowWSMessage()
	}
	if c.allowWSMessage() {
		t.Fatal("should be rate-limited after exhausting burst")
	}

	// Simulate 1 second passing (should refill 30 tokens)
	c.wsMsgBucket.mu.Lock()
	c.wsMsgBucket.lastSeen = c.wsMsgBucket.lastSeen.Add(-1 * time.Second)
	c.wsMsgBucket.mu.Unlock()

	// Should be able to send 30 more messages
	allowed := 0
	for i := 0; i < 35; i++ {
		if c.allowWSMessage() {
			allowed++
		}
	}
	if allowed != 30 {
		t.Errorf("expected 30 messages allowed after 1s refill, got %d", allowed)
	}
}

// TestAllowWSMessageTokensCappedAtBurst verifies that tokens never exceed the
// burst cap, even if a long time passes between messages.
func TestAllowWSMessageTokensCappedAtBurst(t *testing.T) {
	c := newTestClient(NewHub(), "user1")

	// Send one message to initialise the bucket
	c.allowWSMessage()

	// Simulate 10 seconds passing (would be 300 tokens uncapped, but burst is 60)
	c.wsMsgBucket.mu.Lock()
	c.wsMsgBucket.lastSeen = c.wsMsgBucket.lastSeen.Add(-10 * time.Second)
	c.wsMsgBucket.mu.Unlock()

	allowed := 0
	for i := 0; i < 100; i++ {
		if c.allowWSMessage() {
			allowed++
		}
	}
	if allowed != 60 {
		t.Errorf("tokens should be capped at burst (60), got %d allowed", allowed)
	}
}

// TestAllowWSMessageConcurrentSafety verifies that concurrent calls to
// allowWSMessage do not race. Run with -race to confirm.
func TestAllowWSMessageConcurrentSafety(t *testing.T) {
	c := newTestClient(NewHub(), "user1")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.allowWSMessage()
		}()
	}
	wg.Wait()
	// Success: no race detected (run with -race)
}
