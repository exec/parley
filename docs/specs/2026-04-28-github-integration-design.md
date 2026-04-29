# GitHub Integration — Design Spec

**Date:** 2026-04-28
**Status:** Draft (pre-implementation)
**Scope:** Public-repo link embeds + in-app code Explorer
**Out of scope (this spec):** Gitwise integration, Dev Box, private repos, OAuth flow, write operations

This spec defines the first slice of parley's broader source-code integration story. A separate spec — `2026-04-28-gitwise-devbox-vision.md` — covers the longer arc (Gitwise unfurl, Dev Box voice activity, Claude Code as VC activity). The architecture here is deliberately built so that spec slots in without rework.

---

## 1. Goals

When a user posts a `https://github.com/{owner}/{repo}` link in chat:

1. Render an embed (the existing `EmbedCard` shape) with:
   - Owner avatar, owner login, repo name (`owner/repo`)
   - Description, primary language (color dot), star count, fork count, default branch, last push timestamp, latest release tag (if any)
   - "View on GitHub" link
   - **"Explore" button**
2. Clicking **Explore** takes over the chat + member-list area (everything right of the channel sidebar) and replaces it with a read-only repo browser:
   - Left pane: file tree for the default branch
   - Center pane: file content with Shiki syntax highlighting (reusing `frontend/src/lib/shiki.ts` and the same styling as bin posts)
   - A breadcrumb back to chat dismisses the takeover
3. The user can browse files and read code without leaving parley. No editing, no comments, no PRs in V1.

## 2. Non-Goals (V1)

- Private repos. (Requires GitHub OAuth + per-user encrypted token storage. Punted to V2.)
- Editing, commit/PR creation, file write, branch switching beyond default branch (V1.1 can add ref selector).
- Issue/PR/commit/release-notes embeds. Repo embeds only — issue/PR embeds are a follow-up.
- Markdown rendering of READMEs (V1 shows the description from repo metadata only; rendering README.md inside the explorer is V1.1).
- Search inside the explorer.
- Gitwise. Covered in companion spec.

## 3. Architecture: `GitProvider` interface, backend-proxied

### 3.1 Why backend-proxied (not frontend-direct)

| Concern | Frontend-direct | Backend-proxy (chosen) |
|---|---|---|
| Rate limits | 60/hr per **client IP** (devastating on shared NAT) | 60/hr (or 5k with `GITHUB_APP_TOKEN`) on **backend IP**, amortized via Redis cache |
| Cache reuse across users | None | First user warms cache; everyone in the server reads from Redis |
| User IP exposure to GitHub | Yes, per render | No |
| Server-side moderation/blocklist | Impossible | Easy |
| Path to Gitwise / private repos | Requires rewrite | Same code path |
| Server bandwidth cost | None | Real but bounded by size caps |

The bandwidth cost is the only real downside, and it's bounded by the size caps in §6.

### 3.2 Interface (Go)

```go
// internal/gitprovider/provider.go
package gitprovider

import "context"

type Provider interface {
    Name() string                                                                  // "github" | "gitwise"
    GetRepo(ctx context.Context, owner, repo string) (*Repo, error)
    GetTree(ctx context.Context, owner, repo, ref, path string) ([]TreeEntry, error)
    GetBlob(ctx context.Context, owner, repo, ref, path string) (*Blob, error)
    ListReleases(ctx context.Context, owner, repo string, limit int) ([]Release, error)
}

type Repo struct {
    Owner, Name, Description, DefaultBranch, Language string
    OwnerAvatarURL, HTMLURL                            string
    Stars, Forks                                       int
    PushedAt, UpdatedAt                                time.Time
    Private                                            bool
    LatestRelease                                      *Release // nil if none
}

type TreeEntry struct {
    Path string // full path from repo root
    Name string // basename
    Type string // "file" | "dir" | "symlink" | "submodule"
    Size int64  // 0 for non-files
    SHA  string
}

type Blob struct {
    Path        string
    SHA         string
    Size        int64
    ContentType string // "text" | "binary" | "image"
    Content     []byte // present only when ContentType=="text" and Size <= MaxBlobBytes
    HTMLURL     string
}

type Release struct {
    TagName, Name, Body, HTMLURL string
    PublishedAt                   time.Time
}
```

Two implementations: `GitHubProvider` (this spec) and `GitwiseProvider` (companion spec). Provider is selected per-request via the URL.

### 3.3 Backend endpoints

All under `/api/git/{provider}/...`. Authenticated (regular session), rate-limited per-user (200/hr/user, separate bucket from chat).

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/git/{provider}/repo?owner=X&repo=Y` | Repo metadata for embed |
| GET | `/api/git/{provider}/tree?owner=X&repo=Y&ref=Z&path=P` | Directory listing (default ref=default branch, default path=root) |
| GET | `/api/git/{provider}/blob?owner=X&repo=Y&ref=Z&path=P` | File content (text only, capped) |
| GET | `/api/git/{provider}/releases?owner=X&repo=Y&limit=N` | Recent releases (V1.1 — embed shows latest from `GetRepo` for now) |

`provider` is `github` for V1. Validating it against a known set keeps the path closed.

### 3.4 Caching

Redis-backed, keyed by canonical request:

| Key pattern | TTL | Notes |
|---|---|---|
| `git:gh:repo:{owner}:{repo}` | 5 min | Cheapest call, refreshed on miss |
| `git:gh:tree:{owner}:{repo}:{ref}:{path}` | 5 min | Ref pinned to commit SHA after first resolve |
| `git:gh:blob:{owner}:{repo}:{sha}` | 24 hr | Keyed by **SHA**, not path — immutable |
| `git:gh:releases:{owner}:{repo}` | 15 min | |

Negative caching: 404 cached for 5 min to stop link-spam from abusing GitHub. ETags from GitHub are used to do conditional refresh on miss (avoids burning rate-limit budget on unchanged repos).

### 3.5 GitHub client

`internal/gitprovider/github/` — thin wrapper over GitHub REST v3 (sufficient for V1; GraphQL only earns us batching, which we don't need yet).

#### Auth strategy: parley GitHub App, no user auth required

V1 ships with a parley-owned GitHub App. The backend authenticates as the App itself (JWT signed with the App private key → exchanged for a short-lived installation/app token) on every cache miss. This gives us **5000/hr shared rate limit** across all of parley, plus a clean upgrade path to per-user OAuth (R3 in the vision spec) and write operations (R3+).

We deliberately do **not** require users to authenticate to see public-repo embeds. The unfurl pattern only works because it's frictionless. User auth is a future opt-in for private repos + per-user budgets, never a gate on basic embeds.

Config:
- `GITHUB_APP_ID` — env, the numeric App ID
- `GITHUB_APP_PRIVATE_KEY` — env, the PEM contents (not a path; supports Doppler injection)
- Both optional in dev: when missing, fall back to unauthenticated `api.github.com` at 60/hr. Local development continues to work; CI sets the App secrets.

App permissions requested: **Repository contents: Read**, **Metadata: Read**. Nothing else for V1. No webhook.

#### Capacity math

With cache TTLs in §3.4, steady-state usage is well below 5000/hr until parley has thousands of active users posting unique repos. Per-SHA blob caching at 24h is the big saver — once a tree is browsed, file reads are essentially free.

#### Degradation on rate limit

When GitHub returns 403/429:
1. Serve stale cache if present, with a subtle "cached" indicator on the embed subtitle.
2. If no cache, return a minimal repo descriptor `{owner, name, html_url}` only. The frontend renders a degraded embed: owner+name+"View on GitHub" button, no Explore, no description, no metadata.
3. Emit a metric `gh_rate_limit_remaining` (gauge) and `gh_429_total` (counter) for Grafana alerting.

#### Other client details

- HTTP client: `http.Client` with 10s timeout, `User-Agent: parley/1.x (+https://parley.app)`.
- SSRF: hardcoded base URL `https://api.github.com`. No user-controlled URL fragments other than path-segment owner/repo, both validated against `^[A-Za-z0-9._-]{1,100}$` server-side.
- Errors: distinguish 404 (cache + return 404), 403/429 (degradation path above), 5xx (transient; one retry then 503).
- Token caching: installation tokens are valid for 1 hour; cache them in-memory and refresh at 50 minutes.

## 4. Frontend

### 4.1 Link detection

In the message renderer (`frontend/src/components/chat/Message.tsx` or wherever URL extraction lives now), match `^https?://github\.com/([^/]+)/([^/]+)/?$` plus `tree/{ref}` and `blob/{ref}/{path}` variants. For V1, only the bare repo URL produces an embed; the ref/path variants pre-fill the explorer when **Explore** is clicked from a future variant of the embed.

Detection runs on every rendered message; embed fetch runs once per `(owner,repo)` per session via TanStack-Query-style cache (or whatever's already in use — match it).

### 4.2 `GitHubRepoEmbed` component

`frontend/src/components/embeds/GitHubRepoEmbed.tsx`. Wraps `EmbedCard`:

- `icon`: owner avatar (`<img>` + circular crop)
- `title`: `owner/repo`, with badge if `Private` (V2)
- `subtitle`: description, language dot + name, star count, last push relative time
- `children`: small grid — primary language, default branch, latest release tag
- `actions`: `<a target="_blank">View on GitHub</a>` + `<button>Explore</button>`

CSS additions in `EmbedCard.css` or a new `GitHubRepoEmbed.css` — match the existing card aesthetic.

### 4.3 Explorer takeover

`frontend/src/components/explorer/RepoExplorer.tsx`. Mounted into the same slot the chat occupies (i.e., the area between the channel sidebar and the rightmost panel). Implementation:

- New top-level UI state: `activeExplorer: { provider, owner, repo, ref, path } | null` (Zustand or whatever's already global).
- When non-null, the route renders `RepoExplorer` instead of the channel content. The channel sidebar stays. The member list is replaced by the file tree (or a "Tree" toggle on mobile).
- Esc + back-button + breadcrumb-click set `activeExplorer = null`.
- WS messages still arrive; the channel underneath remains "joined."

Layout:

```
┌──────────────┬──────────────────────────────┬──────────────┐
│ channel side │ file viewer (Shiki)          │ tree         │
│ (unchanged)  │ ────────────                  │ src/         │
│              │ path/to/file.ts               │  ▾ api/      │
│              │  1  import ...                │    voice.ts  │
│              │  2  ...                       │    ...       │
└──────────────┴──────────────────────────────┴──────────────┘
```

Tree on the right (where members were) is intentional — keeps the chat-like spatial mapping.

### 4.4 File viewer

Reuse `frontend/src/lib/shiki.ts`:

- `languageFromFilename(blob.path)` picks a language; falls back to `plaintext`.
- For binaries (image / unknown content type), show `[Binary file: 12 KB]` with a "View raw on GitHub" link instead of decoding.
- Size cap (see §6) — over the cap, show a banner + raw link.
- Line numbers + click-to-anchor (URL fragment `#L42`) — same UX as bin's `PostView`.

Reuse the bin `PostView.css` rules where possible. If splitting CSS feels right, factor a shared `code-viewer.css` partial both bin and explorer pull from.

### 4.5 State + routing

URL representation: `/explore/github/{owner}/{repo}?ref=main&path=src/api/voice.ts`. Deep-linkable, so a user can paste `parley.app/explore/github/anthropics/claude-code` and land in the explorer.

If the URL points at a repo the user wasn't viewing in chat, parley fetches metadata fresh — no chat context required.

## 5. Data flow (P0 — repo unfurl)

1. User posts `https://github.com/foo/bar` in `#general`.
2. Message renders. Frontend extracts `(github, foo, bar)` and calls `/api/git/github/repo?owner=foo&repo=bar`.
3. Backend cache hit → return cached. Miss → call GitHub `/repos/foo/bar`, store, return.
4. Frontend renders `GitHubRepoEmbed` inline.
5. User clicks **Explore**. Frontend sets `activeExplorer` and navigates to `/explore/github/foo/bar`.
6. `RepoExplorer` calls `/api/git/github/tree?owner=foo&repo=bar&ref=main&path=` → renders tree.
7. User clicks `src/api/voice.ts`. Frontend calls `/api/git/github/blob?...` → renders highlighted source.

## 6. Limits / safety

| Limit | Value | Why |
|---|---|---|
| Max blob bytes (returned) | 1 MiB | UI is unusable beyond this; show banner + raw link |
| Max tree entries (per dir) | 1000 | GitHub's own per-page max; paginate beyond this in V1.1 |
| Max repo size (refuse Explore) | 500 MiB total (from `Repo.size`) | Avoids accidental 10 GB monorepo cache spam |
| Per-user request budget | 200 calls/hr | Shared across endpoints; embed renders count |
| Per-server request budget | 1000 calls/hr | Limits link-spam abuse |
| Owner/repo regex | `^[A-Za-z0-9._-]{1,100}$` | Path segment hygiene |
| URL host whitelist | `github.com` and `api.github.com` only | SSRF |

Binary detection: `git_provider/github` decodes blob `content` (base64 from GitHub) and runs a UTF-8 validity check + null-byte check. Failing either → `ContentType: "binary"`, `Content: nil`.

## 7. Phasing

| Phase | Scope | Status |
|---|---|---|
| **P0** | Backend `Provider` interface + GitHub client + cache; frontend `GitHubRepoEmbed` | ✅ Complete (`659b89f`) |
| **P1** | `RepoExplorer` (tree + blob viewer + Shiki) **+ bonus branch switcher** | ✅ Complete (`0df790f`, `ade7a76`, `6954bf2`) |
| **P2** | Deep-link `/explore/...` URLs + tree/blob URL parsing in chat | ✅ Complete (`7f0a6a9`) |
| **P3** | Issue/PR/commit unfurl variants | Separate spec — deferred |
| **P4** | OAuth-linked GitHub for private repo support | Separate spec — deferred (revisit after Phase A of `2026-04-29-phase-a-ai-dev-platform.md`) |

The releases-in-embed sub-piece of the original P2 was dropped: GitHub's
`/repos/:o/:r` endpoint doesn't include the latest release inline and a
second API call per embed is hard to justify until someone asks for it.
The branch switcher (out of scope in the original spec) was added during
P1 because the explorer felt incomplete without it.

## 8. Open questions

1. **Where does the explorer live in the URL space?** `/explore/...` (top-level) vs. `/server/{id}/explore/...` (server-scoped). Top-level is simpler and matches the takeover model where you're not really "in" a channel anymore. Recommend top-level.
2. **Mobile layout.** Member-list-replacement strategy doesn't survive narrow viewports. Tabbed (Tree / Code) is the obvious fallback. Decide during P1 when the layout gets built.
3. **Embed cache invalidation.** 5 min is fine for V1. If users complain about stale star counts, we add a force-refresh button gated by a per-user 1/min limit.
4. **Should bin posts be able to embed a GitHub file the same way?** Probably yes — bin already shows code; an "Import from GitHub" button on `CreatePostModal` would reuse `Provider.GetBlob`. Not in this spec; tracked as a follow-up.
5. ~~Auth token strategy~~ — decided: parley GitHub App from day one (see §3.5). Per-user OAuth deferred to R3 in the vision spec.

## 9. Files this spec touches

New:
- `internal/gitprovider/provider.go` — interface + types
- `internal/gitprovider/github/client.go` — GitHub REST wrapper
- `internal/gitprovider/cache.go` — Redis cache
- `internal/gitprovider/handler.go` — HTTP handlers
- `frontend/src/components/embeds/GitHubRepoEmbed.tsx`
- `frontend/src/components/embeds/GitHubRepoEmbed.css`
- `frontend/src/components/explorer/RepoExplorer.tsx`
- `frontend/src/components/explorer/RepoExplorer.css`
- `frontend/src/api/git.ts`

Modified:
- `cmd/api/main.go` — register handlers + provider construction
- `frontend/src/components/chat/Message.tsx` (or wherever embed dispatch lives) — add GitHub URL match
- `frontend/src/App.tsx` — explorer route + state
