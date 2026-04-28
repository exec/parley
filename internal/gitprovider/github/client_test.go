package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"parley/internal/gitprovider"
)

// newTestClient builds a Client that points at the given test server (no
// auth — Config is zero-valued, so installationToken returns "" and requests
// go out unauthenticated).
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(Config{})
	// Swap the API base by overriding via a transport that rewrites Host.
	// Simpler: monkey-patch the package-level apiBaseURL via a thin wrapper.
	// Since apiBaseURL is a const, we instead use an httptest.Server and
	// configure the test client's http.Client to redirect via a custom
	// RoundTripper that rewrites the URL.
	c.http.Transport = &rewriteTransport{base: srv.URL}
	return c
}

type rewriteTransport struct {
	base string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite https://api.github.com to the test server's URL.
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(r.base, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

func TestGetRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/anthropics/claude-code" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing Accept header: %v", r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":             "claude-code",
			"description":      "the cli",
			"private":          false,
			"html_url":         "https://github.com/anthropics/claude-code",
			"default_branch":   "main",
			"language":         "TypeScript",
			"stargazers_count": 12345,
			"forks_count":      67,
			"pushed_at":        "2026-04-27T18:00:00Z",
			"updated_at":       "2026-04-27T18:00:00Z",
			"owner": map[string]any{
				"login":      "anthropics",
				"avatar_url": "https://avatars.githubusercontent.com/u/12345",
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.GetRepo(context.Background(), "anthropics", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "anthropics" || got.Name != "claude-code" || got.Stars != 12345 {
		t.Errorf("got %+v", got)
	}
	if got.Language != "TypeScript" || got.DefaultBranch != "main" {
		t.Errorf("metadata mismatch: %+v", got)
	}
}

func TestGetRepoNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.GetRepo(context.Background(), "ghost", "missing")
	if !errors.Is(err, gitprovider.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetRepoRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, `{"message":"rate limited"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.GetRepo(context.Background(), "anthropics", "claude-code")
	if !errors.Is(err, gitprovider.ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestGetRepoInvalidArg(t *testing.T) {
	c := NewClient(Config{})
	_, err := c.GetRepo(context.Background(), "bad/owner", "repo")
	if !errors.Is(err, gitprovider.ErrInvalidArg) {
		t.Errorf("expected ErrInvalidArg, got %v", err)
	}
}

func TestGetTreeDirectory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Directory contents = JSON array
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"type": "file", "name": "README.md", "path": "README.md", "sha": "abc1", "size": 1234, "html_url": "h1"},
			{"type": "dir", "name": "src", "path": "src", "sha": "abc2", "size": 0, "html_url": "h2"},
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.GetTree(context.Background(), "owner", "repo", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "README.md" || got[1].Type != "dir" {
		t.Errorf("got %+v", got)
	}
}

func TestGetTreeFilePathErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Single file response = JSON object, not array — caller asked for a
		// file path on the tree endpoint by mistake.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "file", "name": "x.go", "path": "x.go",
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.GetTree(context.Background(), "owner", "repo", "main", "x.go")
	if !errors.Is(err, gitprovider.ErrInvalidArg) {
		t.Errorf("expected ErrInvalidArg, got %v", err)
	}
}

func TestGetBlobText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A small text file: base64("hello world\n")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":     "file",
			"name":     "x.txt",
			"path":     "x.txt",
			"sha":      "deadbeef",
			"size":     12,
			"encoding": "base64",
			"content":  "aGVsbG8gd29ybGQK",
			"html_url": "h",
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	b, err := c.GetBlob(context.Background(), "owner", "repo", "main", "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if b.ContentType != "text" || string(b.Content) != "hello world\n" || b.SHA != "deadbeef" {
		t.Errorf("got %+v content=%q", b, string(b.Content))
	}
}

func TestGetBlobBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// base64("\x00\x01\x02\x03") = "AAECAw=="
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":     "file",
			"name":     "x.bin",
			"path":     "x.bin",
			"sha":      "binsha",
			"size":     4,
			"encoding": "base64",
			"content":  "AAECAw==",
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	b, err := c.GetBlob(context.Background(), "owner", "repo", "main", "x.bin")
	if err != nil {
		t.Fatal(err)
	}
	if b.ContentType != "binary" || b.Content != nil {
		t.Errorf("expected binary content nil, got %+v", b)
	}
}

func TestGetBlobTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "file",
			"name": "huge.bin",
			"path": "huge.bin",
			"sha":  "huge",
			"size": gitprovider.MaxBlobBytes + 1,
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	b, err := c.GetBlob(context.Background(), "owner", "repo", "main", "huge.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Truncated || b.Content != nil {
		t.Errorf("expected truncated, got %+v", b)
	}
}

func TestListReleasesSkipsDrafts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v2", "name": "v2", "draft": false, "published_at": "2026-04-01T00:00:00Z", "html_url": "u2"},
			{"tag_name": "v3-draft", "draft": true},
			{"tag_name": "v1", "name": "v1", "draft": false, "published_at": "2026-03-01T00:00:00Z", "html_url": "u1"},
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.ListReleases(context.Background(), "owner", "repo", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TagName != "v2" || got[1].TagName != "v1" {
		t.Errorf("got %+v", got)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// All unset → zero config, no error.
	cfg, err := LoadConfigFromEnv(func(string) string { return "" }, func(string) ([]byte, error) { return nil, nil })
	if err != nil || cfg.AppID != 0 {
		t.Errorf("expected zero config, got %+v err=%v", cfg, err)
	}

	// Partial config → error.
	_, err = LoadConfigFromEnv(func(k string) string {
		if k == "GITHUB_APP_ID" {
			return "123"
		}
		return ""
	}, func(string) ([]byte, error) { return nil, nil })
	if err == nil {
		t.Errorf("expected partial-config error")
	}

	// Bad ID → error.
	_, err = LoadConfigFromEnv(func(k string) string {
		switch k {
		case "GITHUB_APP_ID":
			return "abc"
		case "GITHUB_APP_INSTALLATION_ID":
			return "456"
		case "GITHUB_APP_PRIVATE_KEY":
			return "xx"
		}
		return ""
	}, func(string) ([]byte, error) { return nil, nil })
	if err == nil {
		t.Errorf("expected bad-id error")
	}
}
