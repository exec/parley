package gitprovider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateOwnerRepo(t *testing.T) {
	cases := []struct {
		owner, repo string
		ok          bool
	}{
		{"anthropics", "claude-code", true},
		{"foo_bar", "baz.qux", true},
		{"a", "b", true},
		{"", "repo", false},
		{"owner", "", false},
		{"owner/with/slash", "repo", false},
		{"owner", "repo with space", false},
		{strings.Repeat("a", 101), "repo", false}, // length cap
		{"owner", "repo;rm -rf /", false},
		{"owner", "..", true},  // technically valid charset; provider 404 handles
		{"owner", "../etc", false}, // slash forbidden
	}
	for _, c := range cases {
		err := ValidateOwnerRepo(c.owner, c.repo)
		got := err == nil
		if got != c.ok {
			t.Errorf("ValidateOwnerRepo(%q, %q) ok=%v, want %v (err=%v)", c.owner, c.repo, got, c.ok, err)
		}
	}
}

type fakeProvider struct{ name string }

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) GetRepo(ctx context.Context, o, r string) (*Repo, error) {
	return nil, nil
}
func (f *fakeProvider) GetTree(ctx context.Context, o, r, ref, p string) ([]TreeEntry, error) {
	return nil, nil
}
func (f *fakeProvider) GetBlob(ctx context.Context, o, r, ref, p string) (*Blob, error) {
	return nil, nil
}
func (f *fakeProvider) ListReleases(ctx context.Context, o, r string, n int) ([]Release, error) {
	return nil, nil
}
func (f *fakeProvider) ListBranches(ctx context.Context, o, r string) ([]Branch, error) {
	return nil, nil
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Get("github"); !errors.Is(err, ErrUnknownProv) {
		t.Errorf("empty registry should return ErrUnknownProv, got %v", err)
	}
	reg.Register(&fakeProvider{name: "github"})
	reg.Register(&fakeProvider{name: "gitwise"})
	if p, err := reg.Get("github"); err != nil || p.Name() != "github" {
		t.Errorf("Get(github) = (%v, %v)", p, err)
	}
	if got := reg.Names(); len(got) != 2 {
		t.Errorf("Names() = %v, want len 2", got)
	}
	// Re-register replaces
	reg.Register(&fakeProvider{name: "github"})
	if got := reg.Names(); len(got) != 2 {
		t.Errorf("after re-register, len(Names) = %d, want 2", len(got))
	}
}

func TestProviderCtx(t *testing.T) {
	ctx := WithProvider(context.Background(), "github")
	if got := ProviderFromCtx(ctx); got != "github" {
		t.Errorf("ProviderFromCtx = %q, want github", got)
	}
	if got := ProviderFromCtx(context.Background()); got != "" {
		t.Errorf("ProviderFromCtx (empty) = %q, want empty", got)
	}
}
