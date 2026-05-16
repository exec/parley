package cache

import (
	"fmt"
	"sync"
	"time"
)

// MembershipCache is a short-lived in-memory cache for four common lookup types:
//   - Server membership: (serverID, userID) → bool
//   - Channel-to-server mapping: channelID → serverID
//   - Permission check results: (serverID, userID, channelID, perm) → bool
//   - Effective channel permission mask: (serverID, userID, channelID) → int64
//
// All are stable within a TTL window (memberships/roles change rarely; channel
// server assignments never change). This cache eliminates repeated DB round-
// trips from the WebSocket channelAccessChecker during mass-subscribe events,
// and from permission checks on every message operation.
type MembershipCache struct {
	mu        sync.RWMutex
	members   map[string]memberEntry   // key: "m:serverID:userID"
	chSrv     map[string]chSrvEntry    // key: "c:channelID"
	perms     map[string]permEntry     // key: "p:serverID:userID:channelID:perm"
	permMasks map[string]permMaskEntry // key: "pm:serverID:userID:channelID"
	ttl       time.Duration
}

type permEntry struct {
	allowed   bool
	expiresAt time.Time
}

type permMaskEntry struct {
	mask      int64
	expiresAt time.Time
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
		members:   make(map[string]memberEntry),
		chSrv:     make(map[string]chSrvEntry),
		perms:     make(map[string]permEntry),
		permMasks: make(map[string]permMaskEntry),
		ttl:       ttl,
	}
	go c.cleanup()
	return c
}

// GetMember returns (isMember, true) on cache hit, (false, false) on miss or expiry.
func (c *MembershipCache) GetMember(serverID, userID int64) (isMember bool, ok bool) {
	key := fmt.Sprintf("m:%d:%d", serverID, userID)
	c.mu.RLock()
	e, exists := c.members[key] // copied by value; expiry check is safe outside the lock
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
	e, exists := c.chSrv[key] // copied by value; expiry check is safe outside the lock
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

// GetPerm returns (allowed, true) on cache hit.
func (c *MembershipCache) GetPerm(serverID, userID, channelID, perm int64) (allowed bool, ok bool) {
	key := fmt.Sprintf("p:%d:%d:%d:%d", serverID, userID, channelID, perm)
	c.mu.RLock()
	e, exists := c.perms[key]
	c.mu.RUnlock()
	if !exists || time.Now().After(e.expiresAt) {
		return false, false
	}
	return e.allowed, true
}

// SetPerm caches a permission check result.
func (c *MembershipCache) SetPerm(serverID, userID, channelID, perm int64, allowed bool) {
	key := fmt.Sprintf("p:%d:%d:%d:%d", serverID, userID, channelID, perm)
	c.mu.Lock()
	c.perms[key] = permEntry{allowed: allowed, expiresAt: time.Now().Add(45 * time.Second)}
	c.mu.Unlock()
}

// InvalidatePermsForUser removes all cached permission entries (both the per-bit
// perms map and the channel mask map) for a user in a server. Call this when
// roles are reassigned or a member is kicked.
func (c *MembershipCache) InvalidatePermsForUser(serverID, userID int64) {
	permPrefix := fmt.Sprintf("p:%d:%d:", serverID, userID)
	maskPrefix := fmt.Sprintf("pm:%d:%d:", serverID, userID)
	c.mu.Lock()
	for k := range c.perms {
		if len(k) >= len(permPrefix) && k[:len(permPrefix)] == permPrefix {
			delete(c.perms, k)
		}
	}
	for k := range c.permMasks {
		if len(k) >= len(maskPrefix) && k[:len(maskPrefix)] == maskPrefix {
			delete(c.permMasks, k)
		}
	}
	c.mu.Unlock()
}

// GetChannelPermMask returns (mask, true) on cache hit. The mask is the full
// computed channel permission bitset for (serverID, userID, channelID) — the
// output of ComputeChannelPermissions, already accounting for base perms,
// overwrites, and admin/owner shortcuts.
func (c *MembershipCache) GetChannelPermMask(serverID, userID, channelID int64) (mask int64, ok bool) {
	key := fmt.Sprintf("pm:%d:%d:%d", serverID, userID, channelID)
	c.mu.RLock()
	e, exists := c.permMasks[key]
	c.mu.RUnlock()
	if !exists || time.Now().After(e.expiresAt) {
		return 0, false
	}
	return e.mask, true
}

// SetChannelPermMask caches a computed channel permission mask. TTL matches
// the per-bit perm cache (45 s).
func (c *MembershipCache) SetChannelPermMask(serverID, userID, channelID, mask int64) {
	key := fmt.Sprintf("pm:%d:%d:%d", serverID, userID, channelID)
	c.mu.Lock()
	c.permMasks[key] = permMaskEntry{mask: mask, expiresAt: time.Now().Add(45 * time.Second)}
	c.mu.Unlock()
}

// InvalidateChannelMasks drops every cached mask for a channel across all
// users. Call this when channel overwrites change (a single overwrite change
// affects every member's computed mask in that channel).
func (c *MembershipCache) InvalidateChannelMasks(channelID int64) {
	suffix := fmt.Sprintf(":%d", channelID)
	prefix := "pm:"
	permSuffix := suffix // also clear per-bit perm cache entries for this channel
	c.mu.Lock()
	for k := range c.permMasks {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix && hasSuffix(k, suffix) {
			delete(c.permMasks, k)
		}
	}
	// Per-bit perm keys are "p:srv:user:channel:perm" — channelID is not the
	// suffix. Walk and parse by splitting on ':'.
	for k := range c.perms {
		if perChannelKeyMatches(k, channelID) {
			delete(c.perms, k)
		}
	}
	_ = permSuffix
	c.mu.Unlock()
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// perChannelKeyMatches returns true if k is a "p:serverID:userID:channelID:perm"
// key whose channelID matches the given id.
func perChannelKeyMatches(k string, channelID int64) bool {
	if len(k) < 2 || k[:2] != "p:" {
		return false
	}
	want := fmt.Sprintf(":%d:", channelID)
	// Find the third ':' position: skip "p:", find next two ':' separators.
	colons := 0
	start := 2
	for i := 2; i < len(k); i++ {
		if k[i] == ':' {
			colons++
			if colons == 2 {
				start = i // position of the colon BEFORE channelID
				break
			}
		}
	}
	if colons < 2 {
		return false
	}
	// Match ":channelID:" starting at `start`.
	if start+len(want) > len(k) {
		return false
	}
	return k[start:start+len(want)] == want
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
		for k, e := range c.perms {
			if now.After(e.expiresAt) {
				delete(c.perms, k)
			}
		}
		for k, e := range c.permMasks {
			if now.After(e.expiresAt) {
				delete(c.permMasks, k)
			}
		}
		c.mu.Unlock()
	}
}
