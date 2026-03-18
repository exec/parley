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
	members map[string]memberEntry // key: "m:serverID:userID"
	chSrv   map[string]chSrvEntry  // key: "c:channelID"
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
