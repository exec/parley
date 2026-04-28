package gitprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"parley/internal/httputil"
)

// Handler exposes the registry over HTTP under /api/git/{provider}/...
//
// Caching: every endpoint reads from cache before falling through to the
// provider. On upstream rate-limiting (ErrRateLimited), handlers serve stale
// cache if any exists, otherwise return 503 with a "rate-limited" message so
// the frontend can show a degraded embed.
type Handler struct {
	registry *Registry
	cache    *Cache
}

// NewHandler builds the HTTP handler. cache may be nil for a no-op cache.
func NewHandler(registry *Registry, cache *Cache) *Handler {
	return &Handler{registry: registry, cache: cache}
}

// providerCtxKey is the context key the router uses to pass the {provider}
// path segment into handlers without coupling this package to chi.
type providerCtxKey struct{}

// WithProvider returns a request whose context carries the provider name.
// The route layer calls this from a tiny middleware that reads chi.URLParam.
func WithProvider(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, providerCtxKey{}, name)
}

// ProviderFromCtx is exported for the router's convenience.
func ProviderFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(providerCtxKey{}).(string)
	return v
}

// resolveProvider centralises the chi-free provider lookup. Errors are
// already written to w when this returns nil.
func (h *Handler) resolveProvider(w http.ResponseWriter, r *http.Request) Provider {
	name := ProviderFromCtx(r.Context())
	if name == "" {
		name = r.URL.Query().Get("provider")
	}
	if name == "" {
		httputil.JSONError(w, "missing provider", http.StatusBadRequest)
		return nil
	}
	p, err := h.registry.Get(name)
	if err != nil {
		httputil.JSONError(w, "unknown provider", http.StatusNotFound)
		return nil
	}
	return p
}

// validateOwnerRepo reads ?owner= and ?repo= from the request, validates
// them, and returns them or writes a 400.
func (h *Handler) validateOwnerRepo(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		httputil.JSONError(w, "owner and repo are required", http.StatusBadRequest)
		return "", "", false
	}
	if err := ValidateOwnerRepo(owner, repo); err != nil {
		httputil.JSONError(w, "invalid owner or repo", http.StatusBadRequest)
		return "", "", false
	}
	return owner, repo, true
}

// HandleRepo: GET /api/git/{provider}/repo?owner=X&repo=Y
func (h *Handler) HandleRepo(w http.ResponseWriter, r *http.Request) {
	p := h.resolveProvider(w, r)
	if p == nil {
		return
	}
	owner, repo, ok := h.validateOwnerRepo(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	key := RepoKey(p.Name(), owner, repo)

	if missing, _ := h.cache.CheckNotFound(ctx, key); missing {
		httputil.JSONError(w, "not found", http.StatusNotFound)
		return
	}
	if cached, ok, _ := h.cache.GetRepo(ctx, key); ok {
		writeJSON(w, cached)
		return
	}

	got, err := p.GetRepo(ctx, owner, repo)
	if err != nil {
		h.handleProviderErr(w, r, err, key, func() any { return nil })
		return
	}
	_ = h.cache.SetRepo(ctx, key, got)
	writeJSON(w, got)
}

// HandleTree: GET /api/git/{provider}/tree?owner=X&repo=Y&ref=Z&path=P
func (h *Handler) HandleTree(w http.ResponseWriter, r *http.Request) {
	p := h.resolveProvider(w, r)
	if p == nil {
		return
	}
	owner, repo, ok := h.validateOwnerRepo(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	path := r.URL.Query().Get("path")
	ctx := r.Context()
	key := TreeKey(p.Name(), owner, repo, ref, path)

	if missing, _ := h.cache.CheckNotFound(ctx, key); missing {
		httputil.JSONError(w, "not found", http.StatusNotFound)
		return
	}
	if cached, ok, _ := h.cache.GetTree(ctx, key); ok {
		writeJSON(w, cached)
		return
	}

	got, err := p.GetTree(ctx, owner, repo, ref, path)
	if err != nil {
		h.handleProviderErr(w, r, err, key, func() any {
			cached, ok, _ := h.cache.GetTree(ctx, key)
			if ok {
				return cached
			}
			return nil
		})
		return
	}
	_ = h.cache.SetTree(ctx, key, got)
	writeJSON(w, got)
}

// HandleBlob: GET /api/git/{provider}/blob?owner=X&repo=Y&ref=Z&path=P
//
// Resolves path→SHA via a tiny pointer cache (so multiple users browsing the
// same file share the SHA-keyed content cache).
func (h *Handler) HandleBlob(w http.ResponseWriter, r *http.Request) {
	p := h.resolveProvider(w, r)
	if p == nil {
		return
	}
	owner, repo, ok := h.validateOwnerRepo(w, r)
	if !ok {
		return
	}
	ref := r.URL.Query().Get("ref")
	path := r.URL.Query().Get("path")
	if path == "" {
		httputil.JSONError(w, "path is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	resolveKey := BlobResolveKey(p.Name(), owner, repo, ref, path)

	// Step 1: try the path→SHA pointer cache, then the SHA→content cache.
	if sha, ok, _ := h.cache.GetBlobSHA(ctx, resolveKey); ok {
		if cached, ok, _ := h.cache.GetBlob(ctx, BlobKey(p.Name(), owner, repo, sha)); ok {
			writeJSON(w, cached)
			return
		}
	}

	// Step 2: fetch from upstream.
	got, err := p.GetBlob(ctx, owner, repo, ref, path)
	if err != nil {
		h.handleProviderErr(w, r, err, resolveKey, func() any { return nil })
		return
	}
	if got.SHA != "" {
		_ = h.cache.SetBlob(ctx, BlobKey(p.Name(), owner, repo, got.SHA), got)
		_ = h.cache.SetBlobSHA(ctx, resolveKey, got.SHA)
	}
	writeJSON(w, got)
}

// HandleReleases: GET /api/git/{provider}/releases?owner=X&repo=Y&limit=N
func (h *Handler) HandleReleases(w http.ResponseWriter, r *http.Request) {
	p := h.resolveProvider(w, r)
	if p == nil {
		return
	}
	owner, repo, ok := h.validateOwnerRepo(w, r)
	if !ok {
		return
	}
	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		// Best-effort parse; spec caps at 30 inside the provider anyway.
		if _, err := fmt.Sscan(v, &limit); err != nil {
			limit = 5
		}
	}
	ctx := r.Context()
	key := ReleasesKey(p.Name(), owner, repo)

	if cached, ok, _ := h.cache.GetReleases(ctx, key); ok {
		writeJSON(w, cached)
		return
	}
	got, err := p.ListReleases(ctx, owner, repo, limit)
	if err != nil {
		h.handleProviderErr(w, r, err, key, func() any { return nil })
		return
	}
	_ = h.cache.SetReleases(ctx, key, got)
	writeJSON(w, got)
}

// handleProviderErr maps a provider error to an HTTP response. On rate-limit
// it tries the supplied stale-fetcher; on 404 it negative-caches and 404s.
func (h *Handler) handleProviderErr(w http.ResponseWriter, r *http.Request, err error, key string, stale func() any) {
	switch {
	case errors.Is(err, ErrNotFound):
		_ = h.cache.MarkNotFound(r.Context(), key)
		httputil.JSONError(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrRateLimited):
		if v := stale(); v != nil {
			w.Header().Set("X-Parley-Cached", "stale")
			writeJSON(w, v)
			return
		}
		httputil.JSONError(w, "upstream rate-limited; try again shortly", http.StatusServiceUnavailable)
	case errors.Is(err, ErrInvalidArg):
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
	default:
		httputil.JSONError(w, "upstream error", http.StatusBadGateway)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
