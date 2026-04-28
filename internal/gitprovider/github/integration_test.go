//go:build integration
// +build integration

// Live integration tests that hit the real GitHub API. Gated behind the
// `integration` build tag so `go test ./...` stays offline.
//
// Run with:
//   GITHUB_APP_ID=... GITHUB_APP_INSTALLATION_ID=... \
//   GITHUB_APP_PRIVATE_KEY_PATH=... \
//   go test -tags integration -v -run TestLive ./internal/gitprovider/github/

package github

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveSmoke(t *testing.T) {
	cfg, err := LoadConfigFromEnv(os.Getenv, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	owner, repo := "anthropics", "claude-code"

	r, err := c.GetRepo(ctx, owner, repo)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if r.Owner != owner || r.Name != repo {
		t.Errorf("identity mismatch: %+v", r)
	}
	t.Logf("repo: %s/%s — %s lang=%s stars=%d default=%s", r.Owner, r.Name, r.Description, r.Language, r.Stars, r.DefaultBranch)

	tree, err := c.GetTree(ctx, owner, repo, r.DefaultBranch, "")
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	if len(tree) == 0 {
		t.Errorf("expected non-empty root tree")
	}
	t.Logf("root tree: %d entries", len(tree))

	rels, err := c.ListReleases(ctx, owner, repo, 3)
	if err != nil {
		t.Logf("ListReleases (non-fatal): %v", err)
	} else {
		t.Logf("releases: %d", len(rels))
	}

	b, err := c.GetBlob(ctx, owner, repo, r.DefaultBranch, "README.md")
	if err != nil {
		t.Fatalf("GetBlob README: %v", err)
	}
	if b.ContentType != "text" || len(b.Content) == 0 {
		t.Errorf("README expected text content, got %+v", b)
	}
	t.Logf("README: %d bytes, sha=%s", len(b.Content), b.SHA)
}
