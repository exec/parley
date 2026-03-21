# Server Discovery Implementation Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Public server directory where server owners opt in by enabling a toggle (requires vanity URL), pick up to 3 admin-curated categories, and write a description — users browse and join from a DiscoveryPage accessible from the sidebar.

**Architecture:** Vanity URL and `is_public` are decoupled — a server can have a vanity URL without being listed. Admin panel manages a `server_categories` table (same pattern as `report_categories`). A `server_category_assignments` junction table links servers to categories (max 3, enforced at service layer). A public `GET /api/discover` endpoint (no auth required) serves paginated results. Authorization for server updates stays in the handler layer, not the service layer — consistent with existing pattern.

**Tech Stack:** Go/chi backend, PostgreSQL, React/TypeScript frontend, existing admin panel pattern (cmd/admin)

---

## Database (internal/db/migrations.go)

Add a new migration block (next available number):

```sql
-- servers additions
ALTER TABLE servers ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_servers_is_public ON servers(is_public) WHERE is_public = TRUE;

-- server_categories (admin-managed, same pattern as report_categories)
CREATE TABLE IF NOT EXISTS server_categories (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- server_category_assignments junction (max 3 per server enforced at service layer)
CREATE TABLE IF NOT EXISTS server_category_assignments (
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES server_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_sca_category_id ON server_category_assignments(category_id);
```

---

## Models (internal/db/models.go)

### Update db.Server struct — add two fields
```go
type Server struct {
    ID          int64          `json:"id"          db:"id"`
    Name        string         `json:"name"        db:"name"`
    IconURL     sql.NullString `json:"icon_url"    db:"icon_url"`
    OwnerID     int64          `json:"owner_id"    db:"owner_id"`
    VanityURL   sql.NullString `json:"vanity_url"  db:"vanity_url"`
    Description sql.NullString `json:"description" db:"description"` // NEW
    IsPublic    bool           `json:"is_public"   db:"is_public"`   // NEW
    CreatedAt   time.Time      `json:"created_at"  db:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"  db:"updated_at"`
}
```

### New: ServerCategory
```go
type ServerCategory struct {
    ID        int64     `json:"id"         db:"id"`
    Name      string    `json:"name"       db:"name"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}
```

### New: PublicServerRow (repository scan target)
```go
// PublicServerRow is the DB scan target for discovery queries.
// Categories are populated in a second pass (not via JOIN to avoid row multiplication).
type PublicServerRow struct {
    ID          int64          `db:"id"`
    Name        string         `db:"name"`
    IconURL     sql.NullString `db:"icon_url"`
    VanityURL   sql.NullString `db:"vanity_url"`
    Description sql.NullString `db:"description"`
    MemberCount int            `db:"member_count"`
}
```

### New: PublicServer (service/API type, in service package)
```go
type PublicServer struct {
    ID          string           `json:"id"`
    Name        string           `json:"name"`
    IconURL     string           `json:"icon_url,omitempty"`
    VanityURL   string           `json:"vanity_url"`
    Description string           `json:"description,omitempty"`
    MemberCount int              `json:"member_count"`
    Categories  []db.ServerCategory `json:"categories"`
}
```

---

## Repository Layer (internal/db/)

New file `internal/db/discovery_repository.go`:

### Server categories (admin-managed)
```go
func (r *Repository) GetServerCategories(ctx context.Context) ([]ServerCategory, error)
// SELECT id, name, created_at FROM server_categories ORDER BY name

func (r *Repository) CreateServerCategory(ctx context.Context, name string) (*ServerCategory, error)
// INSERT INTO server_categories (name) VALUES ($1) RETURNING id, name, created_at

func (r *Repository) DeleteServerCategory(ctx context.Context, id int64) error
// DELETE FROM server_categories WHERE id = $1
```

### Category assignments (owner-managed)
```go
func (r *Repository) GetServerCategoryAssignments(ctx context.Context, serverID int64) ([]ServerCategory, error)
// SELECT sc.id, sc.name, sc.created_at
// FROM server_category_assignments sca
// JOIN server_categories sc ON sc.id = sca.category_id
// WHERE sca.server_id = $1
// ORDER BY sc.name

func (r *Repository) SetServerCategories(ctx context.Context, serverID int64, categoryIDs []int64) error
// In a transaction:
//   DELETE FROM server_category_assignments WHERE server_id = $1
//   INSERT INTO server_category_assignments (server_id, category_id) VALUES ($1, $2), ...
// Returns error if any category_id does not exist (FK violation caught and returned as friendly error)
```

### Discovery query
```go
func (r *Repository) GetPublicServers(ctx context.Context, categoryID *int64, q string, limit, offset int) ([]PublicServerRow, int, error)
```

Query shape:
```sql
SELECT s.id, s.name, s.icon_url, s.vanity_url, s.description,
       (SELECT COUNT(*) FROM server_members sm WHERE sm.server_id = s.id) AS member_count
FROM servers s
WHERE s.is_public = TRUE
  AND ($1::BIGINT IS NULL OR EXISTS (
      SELECT 1 FROM server_category_assignments sca WHERE sca.server_id = s.id AND sca.category_id = $1
  ))
  AND ($2 = '' OR s.name ILIKE '%' || $2 || '%')
ORDER BY s.name
LIMIT $3 OFFSET $4
```

Run a second `SELECT COUNT(*)` with the same WHERE (minus LIMIT/OFFSET) for the total count. Then for each returned row fetch its categories via `GetServerCategoryAssignments` and assemble into the service-layer `PublicServer` type. This N+1 is acceptable given page size of 24 and the small category join being indexed.

Also update the existing `UpdateServer` SQL in `internal/db/server_repository.go` (or wherever `UpdateServer` repo method lives) to include `description` and `is_public`:
```sql
UPDATE servers
SET name = $2, icon_url = $3, description = $4, is_public = $5, updated_at = NOW()
WHERE id = $1
```
And update the corresponding Go call to pass the new fields.

---

## Server Service (internal/server/)

### UpdateServer — keep authorization in the handler, extend service signature
The existing handler at `internal/server/handler.go` checks `server.OwnerID != userID` before calling the service. Keep that pattern — do not add `userID` to the service method. The service method signature becomes:

```go
// internal/server/server_crud.go
func (s *ServerService) UpdateServer(ctx context.Context, serverID, name, iconURL, description string, isPublic bool) (*Server, error)
```

Validation inside `UpdateServer`:
- If `isPublic == true`, verify the server has a non-empty `vanity_url`. If not, return `errors.New("a vanity URL is required to list your server publicly")`.
- If `len(description) > 200`, return `errors.New("description must be 200 characters or fewer")` (backend enforcement; frontend also enforces with maxLength).

The existing handler (`handleUpdateServer` or equivalent) already reads `userID` from context and checks ownership — extend it to decode `description` and `is_public` from the request body and pass them to the service.

Also update `dbServerToService` mapper to map the new fields.

### New: SetServerCategories
```go
// internal/server/server_crud.go (or server_categories.go)
func (s *ServerService) SetServerCategories(ctx context.Context, serverID string, categoryIDs []int64) ([]db.ServerCategory, error)
```
- Validates `len(categoryIDs) <= 3`; returns error otherwise
- Calls `repo.SetServerCategories`
- Returns updated list via `repo.GetServerCategoryAssignments`
- Authorization: checked at handler layer (owner-only, same as UpdateServer)

### New: ListServerCategories (public)
```go
func (s *ServerService) ListServerCategories(ctx context.Context) ([]db.ServerCategory, error)
```
Direct pass-through to `repo.GetServerCategories`. No cache needed — this is a tiny table (< 20 rows) and the query will complete in < 1ms.

### New: Discover
```go
func (s *ServerService) Discover(ctx context.Context, categoryID *int64, q string, page int) ([]PublicServer, int, error)
```
- Clamps `q` to 100 chars before passing to repo (prevent abuse of ILIKE)
- Page size = 24; offset = (page - 1) * 24
- Returns assembled `[]PublicServer` and total count

---

## HTTP Handlers (internal/server/handler.go)

### New handlers
```go
func (h *Handler) ListServerCategories(w http.ResponseWriter, r *http.Request)
// GET /api/server-categories — no auth, calls service.ListServerCategories

func (h *Handler) Discover(w http.ResponseWriter, r *http.Request)
// GET /api/discover — no auth
// Query params: page (int, default 1), category_id (int64, optional), q (string, optional)
// Returns: {"servers": [...], "total": N}

func (h *Handler) SetServerCategories(w http.ResponseWriter, r *http.Request)
// PUT /api/servers/{id}/categories — auth required, owner-only
// Body: {"category_ids": [1, 2, 3]}
// Returns: updated []ServerCategory
```

### Route registration (cmd/api/routes.go)

The two public routes must be registered **outside** all auth middleware groups, following the same pattern as the theme routes and bot invite route. In `cmd/api/routes.go`, look for the block that registers:
```go
r.Get("/themes/repo", ...)
r.Get("/bots/invite/{token}", ...)
```
These are registered directly on the top-level `r.Group`, outside any auth middleware wrapper. Add the discovery routes in the same block:

```go
// Public — no auth required
r.Get("/api/discover", serverHandler.Discover)
r.Get("/api/server-categories", serverHandler.ListServerCategories)
```

Do NOT place these inside the `r.Group(func(r chi.Router) { r.Use(auth.AuthMiddlewareWith(...)) ... })` block — the auth middleware in this project may not reject unauthenticated requests at the middleware level, but future tightening would break public discovery if routes are inside the auth group.

The protected category assignment route goes inside the auth group:
```go
r.Put("/api/servers/{id}/categories", serverHandler.SetServerCategories)
```

Also extend the existing `PUT /api/servers/{id}` handler to read `description` and `is_public` from the request body. The handler currently only reads `name` and `icon_url`.

---

## Admin Panel (cmd/admin/handlers.go + server.go)

Add three handlers following the identical pattern as `handleListCategories` / `handleCreateCategory` / `handleDeleteCategory`:

```go
func handleListServerCategories(w http.ResponseWriter, r *http.Request)
// GET /server-categories — SELECT * FROM server_categories ORDER BY name

func handleCreateServerCategory(w http.ResponseWriter, r *http.Request)
// POST /server-categories — body: {"name": "Gaming"}
// INSERT INTO server_categories (name) VALUES ($1) RETURNING *

func handleDeleteServerCategory(w http.ResponseWriter, r *http.Request)
// DELETE /server-categories/{id}
```

Register in `cmd/admin/server.go` inside the admin auth middleware group:
```go
r.Get("/server-categories", handleListServerCategories)
r.Post("/server-categories", handleCreateServerCategory)
r.Delete("/server-categories/{id}", handleDeleteServerCategory)
```

---

## Frontend — Types (frontend/src/api/types.ts)

Add to `types.ts`:
```ts
export interface ServerCategory {
  id: number;
  name: string;
}

export interface PublicServer {
  id: string;
  name: string;
  icon_url?: string;
  vanity_url: string;
  description?: string;
  member_count: number;
  categories: ServerCategory[];
}
```

Also update the existing `Server` interface to add:
```ts
description?: string;
is_public?: boolean;
```

---

## Frontend — API (frontend/src/api/)

Update `frontend/src/api/servers.ts` — extend `updateServer` to accept and send the new fields:
```ts
export async function updateServer(
  serverId: string,
  name: string,
  iconUrl?: string,
  description?: string,
  isPublic?: boolean,
): Promise<Server>
// PUT /servers/:id with { name, icon_url, description, is_public }
```

New file `frontend/src/api/discovery.ts`:
```ts
export async function getServerCategories(): Promise<ServerCategory[]>
// GET /api/server-categories

export async function discoverServers(params: {
  page?: number;
  categoryId?: number;
  q?: string;
}): Promise<{ servers: PublicServer[]; total: number }>
// GET /api/discover?page=&category_id=&q=

export async function setServerCategories(serverId: string, categoryIds: number[]): Promise<ServerCategory[]>
// PUT /api/servers/:id/categories with { category_ids }
```

---

## Frontend — DiscoveryPage

New component `frontend/src/components/discovery/DiscoveryPage.tsx`.

Layout (full-width, replaces main content area like homepage):
- Header row: title "Discover Servers" + search input (debounced 300ms)
- Category filter pills row (horizontal, fetched from `getServerCategories` on mount; "All" pill selected by default)
- 24-per-page card grid
- Pagination controls at bottom (Prev / Next + "Page N of M")

Server card shows: icon (or letter placeholder), server name, description (2-line clamp), member count, category pills, "Join" button.

**Join button behavior:**
- If user is not logged in: redirect to login (or show login modal)
- If already a member: button reads "Joined" and is disabled (check against the user's server list from AppContext)
- Otherwise: clicking Join calls `joinServer` using `vanity_url` (the existing invite join flow) — reuse or adapt `InviteModal`/`JoinInvite` flow

Companion CSS file: `frontend/src/components/discovery/DiscoveryPage.css`

---

## Frontend — Sidebar

Add a globe icon button to `Sidebar.tsx`. Place it between the home button and the first divider (top of sidebar, above server list).

New props on `SidebarProps`:
```ts
onDiscovery?: () => void;
discoveryActive?: boolean;
```

The button renders a compass/globe SVG with a "Discover" tooltip, styled the same as the home button (active state when `discoveryActive`).

Pass through `MainLayout.tsx` → `App.tsx` (same prop-threading pattern as `onHomepage`).

In `App.tsx`:
- Add `showDiscovery` state (boolean)
- When `showDiscovery` is true, render `<DiscoveryPage />` as the main content (replacing server/channel view, same as homepage)
- `onDiscovery` sets `showDiscovery(true)` and clears active server/channel

---

## Frontend — Server Settings Overview Tab (frontend/src/components/settings/ServerSettings.tsx)

Add below the Vanity URL section:

**Description section:**
```tsx
<div className="settings-section">
  <div className="settings-section-title">Description</div>
  <textarea
    className="settings-form-input"
    value={description}
    onChange={e => setDescription(e.target.value.slice(0, 200))}
    placeholder="What's your server about?"
    rows={3}
    maxLength={200}
  />
  <div className="settings-form-hint" style={{ textAlign: 'right' }}>
    {description.length} / 200
  </div>
</div>
```

**Server Directory section:**
```tsx
<div className="settings-section">
  <div className="settings-section-title">Server Directory</div>
  <label className="settings-toggle-row">
    <input
      type="checkbox"
      checked={isPublic}
      onChange={e => setIsPublic(e.target.checked)}
      disabled={!vanityUrl.trim()}
    />
    <span>List this server in the public directory</span>
  </label>
  <div className="settings-form-hint">
    {vanityUrl.trim()
      ? 'Your server will appear in Discover when enabled.'
      : 'A vanity URL is required to list your server publicly.'}
  </div>

  {isPublic && (
    <div style={{ marginTop: 12 }}>
      <div className="settings-section-title">
        Categories <span style={{ color: '#aaa', fontWeight: 400 }}>(up to 3)</span>
      </div>
      {/* Render allCategories as toggleable pills; selectedCategoryIds is int[] state */}
      <div className="settings-category-pills">
        {allCategories.map(cat => (
          <button
            key={cat.id}
            className={`settings-category-pill${selectedCategoryIds.includes(cat.id) ? ' active' : ''}`}
            onClick={() => toggleCategory(cat.id)}
            disabled={!selectedCategoryIds.includes(cat.id) && selectedCategoryIds.length >= 3}
          >
            {cat.name}
          </button>
        ))}
      </div>
    </div>
  )}
</div>
```

**State additions for Overview tab:**
- `description` / `setDescription` — initialized from `server.description ?? ''`
- `isPublic` / `setIsPublic` — initialized from `server.is_public ?? false`
- `selectedCategoryIds` / `setSelectedCategoryIds` — initialized from `serverCategories.map(c => c.id)`, fetched on settings open via `getServerCategoryAssignments` (add this endpoint or use the existing server data)
- `allCategories` — fetched once on settings open via `getServerCategories()`

**`hasOverviewChanges()` must include all four new fields:**
```ts
const hasOverviewChanges = () =>
  name !== server.name ||
  iconUrl !== (server.icon_url ?? '') ||
  vanityUrl !== (server.vanity_url ?? '') ||
  description !== (server.description ?? '') ||         // NEW
  isPublic !== (server.is_public ?? false) ||           // NEW
  !arrayEquals(selectedCategoryIds, initialCategoryIds); // NEW
```

**`handleSaveOverview` extension — wrap all calls in a single try/catch; only call `onUpdate` on full success:**
```ts
const handleSaveOverview = async () => {
  setOverviewLoading(true);
  setOverviewError('');
  try {
    const updated = await updateServer(server.id, name.trim(), iconUrl || undefined, description, isPublic);
    if (vanityUrl.trim() !== (server.vanity_url || '')) {
      await setVanityURL(server.id, vanityUrl.trim());
    }
    await setServerCategories(server.id, selectedCategoryIds);
    onUpdate(updated);
  } catch (err: any) {
    setOverviewError(err?.message ?? 'Failed to save changes');
  } finally {
    setOverviewLoading(false);
  }
};
```

**`arrayEquals` helper** — define at module level in ServerSettings.tsx:
```ts
function arrayEquals(a: number[], b: number[]): boolean {
  return a.length === b.length && [...a].sort().every((v, i) => [...b].sort()[i] === v);
}
```

**Fetching initial category assignments:** Add a dedicated `GET /api/servers/{id}/categories` endpoint:

- Route registration (inside the auth middleware group — reading is fine for any authenticated user): `r.Get("/api/servers/{id}/categories", serverHandler.GetServerCategoriesForServer)`
- Handler: read `serverID` from URL param, call `repo.GetServerCategoryAssignments(ctx, serverIDInt)`, return `[]ServerCategory` as JSON. No permission check beyond authentication needed (public-ish read).
- Frontend: add `getServerCategoryAssignments(serverId: string): Promise<ServerCategory[]>` to `frontend/src/api/discovery.ts` — `GET /api/servers/:id/categories`
- In ServerSettings.tsx: call `getServerCategoryAssignments(server.id)` on mount (alongside `getServerCategories()` for the full list) to initialize both `selectedCategoryIds` and `initialCategoryIds` state variables.

---

## TODO.md

Add to the Feature Backlog under "Message features":
```markdown
- [ ] **User and message reporting** — "Report" option in user right-click context menu and message context menu. Submits a report with a category (from existing `report_categories`) and optional description text. `POST /api/reports`. Admin panel already has report viewing; this adds the frontend submission flow and backend endpoint.
```

---

## Error Cases

| Condition | Response |
|-----------|----------|
| `is_public=true` without vanity URL | 400 "A vanity URL is required to list your server publicly" |
| `description` > 200 chars | 400 "Description must be 200 characters or fewer" |
| `category_ids` length > 3 | 400 "Maximum 3 categories allowed" |
| Invalid category ID in assignment | 400 "Invalid category" (catch FK violation) |
| Deleting a server category from admin | Cascade deletes assignments — acceptable |
| `GET /api/discover` with invalid `page` | Default to page 1 |
| `GET /api/discover` with `q` > 100 chars | Clamp silently at service layer |

---

## Non-Goals

- Server description shown in chat/invite preview (discovery-only)
- Server analytics (view count, join rate)
- Featured/promoted servers
- NSFW flag on discovery listings
- Rich server profiles (banner, social links)
