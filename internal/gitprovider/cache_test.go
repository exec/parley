package gitprovider

import (
	"context"
	"testing"
	"time"
)

// TestCacheNilNoOp verifies a nil-redis Cache silently no-ops.
func TestCacheNilNoOp(t *testing.T) {
	c := NewCache(nil)
	ctx := context.Background()
	key := RepoKey("github", "foo", "bar")

	if r, ok, err := c.GetRepo(ctx, key); err != nil || ok || r != nil {
		t.Errorf("GetRepo on nil cache: got (%v, %v, %v), want (nil, false, nil)", r, ok, err)
	}
	if err := c.SetRepo(ctx, key, &Repo{Owner: "foo"}); err != nil {
		t.Errorf("SetRepo on nil cache: err=%v", err)
	}
	if missing, err := c.CheckNotFound(ctx, key); err != nil || missing {
		t.Errorf("CheckNotFound on nil cache: missing=%v err=%v", missing, err)
	}
}

func TestKeyShape(t *testing.T) {
	cases := []struct{ got, want string }{
		{RepoKey("github", "anthropics", "claude-code"), "git:github:repo:anthropics:claude-code"},
		{TreeKey("github", "a", "b", "main", "src/api"), "git:github:tree:a:b:main:src/api"},
		{BlobKey("github", "a", "b", "deadbeef"), "git:github:blob:a:b:deadbeef"},
		{BlobResolveKey("github", "a", "b", "main", "x.go"), "git:github:blob-resolve:a:b:main:x.go"},
		{ReleasesKey("gitwise", "a", "b"), "git:gitwise:releases:a:b"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("key mismatch: got %q, want %q", c.got, c.want)
		}
	}
}

// TestTTLConsistency guards against accidental TTL changes that would silently
// alter cache freshness windows.
func TestTTLConsistency(t *testing.T) {
	if TTLRepo != 5*time.Minute {
		t.Errorf("TTLRepo = %v, want 5m", TTLRepo)
	}
	if TTLBlob != 24*time.Hour {
		t.Errorf("TTLBlob = %v, want 24h", TTLBlob)
	}
	if TTLNotFound != 5*time.Minute {
		t.Errorf("TTLNotFound = %v, want 5m", TTLNotFound)
	}
}
