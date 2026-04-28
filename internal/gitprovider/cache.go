package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client with typed helpers for the four payload kinds
// the package serves (Repo, []TreeEntry, Blob, []Release). Keys are namespaced
// per-provider so different backends never collide.
//
// A nil *Cache is a no-op: every Get returns (zero, false, nil) and every Set
// returns nil. This lets callers wire optional Redis without sprinkling nil
// checks at every call site.
type Cache struct {
	rdb *redis.Client
}

// NewCache returns a Cache backed by rdb. rdb may be nil for a no-op cache
// (useful in tests and dev mode without Redis).
func NewCache(rdb *redis.Client) *Cache {
	return &Cache{rdb: rdb}
}

// TTL constants per spec §3.4. Exported so handlers/tests can reason about
// expected freshness windows.
const (
	TTLRepo      = 5 * time.Minute
	TTLTree      = 5 * time.Minute
	TTLBlob      = 24 * time.Hour
	TTLReleases  = 15 * time.Minute
	TTLNotFound  = 5 * time.Minute // negative cache: stops link-spam
)

// notFoundSentinel is the cached payload for known-404 entries.
const notFoundSentinel = `{"_404":true}`

func (c *Cache) get(ctx context.Context, key string) (string, bool, error) {
	if c == nil || c.rdb == nil {
		return "", false, nil
	}
	v, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (c *Cache) set(ctx context.Context, key, val string, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

// MarkNotFound caches a 404 sentinel for the given key so repeated requests
// for missing repos don't burn upstream budget.
func (c *Cache) MarkNotFound(ctx context.Context, key string) error {
	return c.set(ctx, key, notFoundSentinel, TTLNotFound)
}

// CheckNotFound returns true if the key has a cached 404 sentinel.
func (c *Cache) CheckNotFound(ctx context.Context, key string) (bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok {
		return false, err
	}
	return v == notFoundSentinel, nil
}

// Repo / Tree / Blob / Releases keyspace builders.
//
// Layout:
//   git:{prov}:repo:{owner}:{repo}
//   git:{prov}:tree:{owner}:{repo}:{ref}:{path}
//   git:{prov}:blob:{owner}:{repo}:{sha}            -- keyed by SHA, immutable
//   git:{prov}:blob-resolve:{owner}:{repo}:{ref}:{path} -- path→SHA mapping
//   git:{prov}:releases:{owner}:{repo}

func RepoKey(prov, owner, repo string) string {
	return fmt.Sprintf("git:%s:repo:%s:%s", prov, owner, repo)
}

func TreeKey(prov, owner, repo, ref, path string) string {
	return fmt.Sprintf("git:%s:tree:%s:%s:%s:%s", prov, owner, repo, ref, path)
}

func BlobKey(prov, owner, repo, sha string) string {
	return fmt.Sprintf("git:%s:blob:%s:%s:%s", prov, owner, repo, sha)
}

func BlobResolveKey(prov, owner, repo, ref, path string) string {
	return fmt.Sprintf("git:%s:blob-resolve:%s:%s:%s:%s", prov, owner, repo, ref, path)
}

func ReleasesKey(prov, owner, repo string) string {
	return fmt.Sprintf("git:%s:releases:%s:%s", prov, owner, repo)
}

func BranchesKey(prov, owner, repo string) string {
	return fmt.Sprintf("git:%s:branches:%s:%s", prov, owner, repo)
}

// GetRepo / SetRepo

func (c *Cache) GetRepo(ctx context.Context, key string) (*Repo, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return nil, false, err
	}
	var r Repo
	if err := json.Unmarshal([]byte(v), &r); err != nil {
		return nil, false, err
	}
	return &r, true, nil
}

func (c *Cache) SetRepo(ctx context.Context, key string, r *Repo) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return c.set(ctx, key, string(b), TTLRepo)
}

// GetTree / SetTree

func (c *Cache) GetTree(ctx context.Context, key string) ([]TreeEntry, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return nil, false, err
	}
	var t []TreeEntry
	if err := json.Unmarshal([]byte(v), &t); err != nil {
		return nil, false, err
	}
	return t, true, nil
}

func (c *Cache) SetTree(ctx context.Context, key string, entries []TreeEntry) error {
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return c.set(ctx, key, string(b), TTLTree)
}

// GetBlob / SetBlob — keyed by SHA so the 24h TTL is safe (content is immutable).

func (c *Cache) GetBlob(ctx context.Context, key string) (*Blob, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return nil, false, err
	}
	var b Blob
	if err := json.Unmarshal([]byte(v), &b); err != nil {
		return nil, false, err
	}
	return &b, true, nil
}

func (c *Cache) SetBlob(ctx context.Context, key string, b *Blob) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return c.set(ctx, key, string(raw), TTLBlob)
}

// GetBlobSHA / SetBlobSHA — path→SHA pointer, used to translate a
// (ref, path) request into the SHA-keyed content cache.

func (c *Cache) GetBlobSHA(ctx context.Context, key string) (string, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return "", false, err
	}
	return v, true, nil
}

func (c *Cache) SetBlobSHA(ctx context.Context, key, sha string) error {
	return c.set(ctx, key, sha, TTLTree) // same window as tree — refs move together
}

// GetReleases / SetReleases

func (c *Cache) GetReleases(ctx context.Context, key string) ([]Release, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return nil, false, err
	}
	var rs []Release
	if err := json.Unmarshal([]byte(v), &rs); err != nil {
		return nil, false, err
	}
	return rs, true, nil
}

func (c *Cache) SetReleases(ctx context.Context, key string, rs []Release) error {
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return c.set(ctx, key, string(b), TTLReleases)
}

// GetBranches / SetBranches — same TTL as tree, since branch lists move
// roughly as often as a tree at HEAD.

func (c *Cache) GetBranches(ctx context.Context, key string) ([]Branch, bool, error) {
	v, ok, err := c.get(ctx, key)
	if err != nil || !ok || v == notFoundSentinel {
		return nil, false, err
	}
	var bs []Branch
	if err := json.Unmarshal([]byte(v), &bs); err != nil {
		return nil, false, err
	}
	return bs, true, nil
}

func (c *Cache) SetBranches(ctx context.Context, key string, bs []Branch) error {
	raw, err := json.Marshal(bs)
	if err != nil {
		return err
	}
	return c.set(ctx, key, string(raw), TTLTree)
}
