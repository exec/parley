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
