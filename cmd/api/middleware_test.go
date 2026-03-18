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

	// Wait 1.5 seconds — generous margin so at least 1 token refills
	// even on a loaded CI runner (1 token/sec rate, 500ms slack).
	time.Sleep(1500 * time.Millisecond)

	if !rl.Allow("192.0.2.1") {
		t.Error("should be allowed after 1 second refill")
	}
}
