// Package gitprovider exposes a uniform interface for reading repositories
// from external git hosting providers (GitHub, Gitwise) so the parley frontend
// can render link unfurls and an in-app code Explorer without coupling to any
// single provider's API surface.
//
// V1 ships only a GitHub backend; Gitwise plugs in by implementing the same
// interface (see docs/specs/2026-04-28-gitwise-devbox-vision.md).
package gitprovider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Errors callers should distinguish.
var (
	ErrNotFound      = errors.New("gitprovider: not found")
	ErrRateLimited   = errors.New("gitprovider: upstream rate-limited")
	ErrTooLarge      = errors.New("gitprovider: resource exceeds size cap")
	ErrInvalidArg    = errors.New("gitprovider: invalid argument")
	ErrUnknownProv   = errors.New("gitprovider: unknown provider")
)

// Provider is implemented by each git host (GitHub, Gitwise).
//
// Implementations MUST be safe for concurrent use, MUST validate owner/repo
// against the package-level regex before issuing upstream calls, and MUST
// honor ctx for cancellation/timeout.
type Provider interface {
	// Name returns the stable provider identifier used in URLs and cache keys
	// (e.g. "github", "gitwise"). Lowercase, no underscores.
	Name() string

	// GetRepo returns repository metadata for an unfurl embed.
	GetRepo(ctx context.Context, owner, repo string) (*Repo, error)

	// GetTree lists entries at path on ref. ref="" means default branch.
	// path="" means repo root. Implementations SHOULD pin ref to a commit SHA
	// when serving cached results so tree views stay self-consistent.
	GetTree(ctx context.Context, owner, repo, ref, path string) ([]TreeEntry, error)

	// GetBlob returns the file at path on ref. Files larger than the
	// implementation's MaxBlobBytes return a Blob with ContentType="text" and
	// Content=nil so the caller can render a "file too large" placeholder.
	// Binary files return ContentType="binary" with Content=nil.
	GetBlob(ctx context.Context, owner, repo, ref, path string) (*Blob, error)

	// ListReleases returns up to limit recent releases, newest first. limit<=0
	// uses an implementation default (currently 5).
	ListReleases(ctx context.Context, owner, repo string, limit int) ([]Release, error)
}

// Repo is the metadata payload powering a repo unfurl embed.
type Repo struct {
	Owner          string    `json:"owner"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	DefaultBranch  string    `json:"default_branch"`
	Language       string    `json:"language,omitempty"`
	OwnerAvatarURL string    `json:"owner_avatar_url,omitempty"`
	HTMLURL        string    `json:"html_url"`
	Stars          int       `json:"stars"`
	Forks          int       `json:"forks"`
	Private        bool      `json:"private"`
	PushedAt       time.Time `json:"pushed_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LatestRelease  *Release  `json:"latest_release,omitempty"`
}

// TreeEntry is one row in a directory listing.
type TreeEntry struct {
	Path string `json:"path"` // full path from repo root
	Name string `json:"name"` // basename
	Type string `json:"type"` // "file" | "dir" | "symlink" | "submodule"
	Size int64  `json:"size"` // 0 for non-files
	SHA  string `json:"sha"`
}

// Blob is the contents of one file. Content is base64-decoded text bytes
// when ContentType=="text" and the file is within MaxBlobBytes; nil otherwise.
type Blob struct {
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"` // "text" | "binary"
	Content     []byte `json:"content,omitempty"`
	HTMLURL     string `json:"html_url"`
	Truncated   bool   `json:"truncated,omitempty"` // size exceeds MaxBlobBytes
}

// Release is one published release/tag.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body,omitempty"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

// MaxBlobBytes is the size cap for inline blob content. Files beyond this are
// returned with Content=nil and Truncated=true; the frontend renders a
// "file too large — view raw" link.
const MaxBlobBytes int64 = 1 << 20 // 1 MiB

// ownerRepoRe enforces the GitHub-compatible identifier rules used across
// all provider implementations. Matches the most permissive subset to
// stay portable across providers (Gitwise allows the same charset).
var ownerRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// ValidateOwnerRepo returns ErrInvalidArg if either argument fails the
// owner/repo regex. Implementations should call this before any upstream
// HTTP request to keep path-segment hygiene off the network.
func ValidateOwnerRepo(owner, repo string) error {
	if !ownerRepoRe.MatchString(owner) {
		return fmt.Errorf("%w: owner %q", ErrInvalidArg, owner)
	}
	if !ownerRepoRe.MatchString(repo) {
		return fmt.Errorf("%w: repo %q", ErrInvalidArg, repo)
	}
	return nil
}

// Registry holds the concrete Provider instances configured at startup.
// A nil entry means the provider is not configured and requests targeted
// at it should return ErrUnknownProv.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider. Calling twice with the same Name() replaces the
// earlier registration.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns the provider by name, or ErrUnknownProv if unregistered.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownProv, name)
	}
	return p, nil
}

// Names returns the registered provider names in undefined order.
// Useful for the frontend's allow-list rendering.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for n := range r.providers {
		out = append(out, n)
	}
	return out
}
