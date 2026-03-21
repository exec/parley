# Server Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public server directory where owners opt in via an `is_public` toggle (requires vanity URL), pick up to 3 admin-curated categories, and write a description — browsable from a DiscoveryPage accessed via a globe icon in the sidebar.

**Architecture:** `is_public` and vanity URL are decoupled on the `servers` table. A `server_categories` table (admin-managed, mirrors `report_categories`) and `server_category_assignments` junction table (max 3 per server) power filtering. A public `GET /api/discover` endpoint (registered outside auth middleware) returns paginated results. Frontend adds a DiscoveryPage component, a globe button in the Sidebar, and extends the ServerSettings Overview tab.

**Tech Stack:** Go 1.22, chi router, PostgreSQL (lib/pq), React 18, TypeScript

**Spec:** `docs/superpowers/specs/2026-03-21-server-discovery-design.md`

---

## File Map

**New files:**
- `internal/db/discovery_repository.go` — server category + discovery repo methods
- `frontend/src/api/discovery.ts` — API client for categories, discover, category assignments
- `frontend/src/components/discovery/DiscoveryPage.tsx` — discovery page component
- `frontend/src/components/discovery/DiscoveryPage.css` — discovery page styles

**Modified files:**
- `internal/db/migrations.go` — migration #51: servers columns + server_categories + server_category_assignments
- `internal/db/models.go` — add Description/IsPublic to Server; add ServerCategory, PublicServerRow structs
- `internal/server/server_crud.go` — extend UpdateServer signature + validation
- `internal/server/service.go` — add service.Server fields Description/IsPublic; add PublicServer type; update dbServerToService
- `internal/server/handler.go` — add 4 new handlers; extend UpdateServerRequest
- `cmd/api/routes.go` — register 2 public + 2 protected routes
- `cmd/admin/handlers.go` — add 3 server category admin handlers
- `cmd/admin/server.go` — register server category admin routes
- `frontend/src/api/types.ts` — add ServerCategory/PublicServer; extend Server interface
- `frontend/src/api/servers.ts` — extend updateServer function
- `frontend/src/components/settings/ServerSettings.tsx` — description + is_public + categories in Overview tab
- `frontend/src/components/layout/Sidebar.tsx` — add globe/discovery button
- `frontend/src/components/layout/MainLayout.tsx` — pass-through discovery props
- `frontend/src/App.tsx` — wire DiscoveryPage + showDiscovery state

---

## Task 1: DB Migration + Model Updates

**Files:**
- Modify: `internal/db/migrations.go` (append to Migrations slice)
- Modify: `internal/db/models.go` (Server struct + new structs)

### Background
The `Migrations` slice in `internal/db/migrations.go` is the authoritative migration source — each entry is a raw SQL string. The last migration is #50 (notifications table). Add migration #51 as a new string appended to the slice (before the closing `}`). The `db.Server` struct in `models.go` currently has 7 fields (ID, Name, IconURL, OwnerID, VanityURL, CreatedAt, UpdatedAt) and must gain `Description` and `IsPublic`.

- [ ] **Step 1: Add migration #51 to `internal/db/migrations.go`**

Find the closing `}` of the `Migrations` slice (it follows the last migration string ending with `idx_notifications_user_id` index). Append before the closing `}`:

```go
,

`-- Server discovery: description + is_public on servers; server_categories; server_category_assignments
ALTER TABLE servers ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_servers_is_public ON servers(is_public) WHERE is_public = TRUE;

CREATE TABLE IF NOT EXISTS server_categories (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS server_category_assignments (
    server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES server_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_sca_category_id ON server_category_assignments(category_id);`
```

- [ ] **Step 2: Update `db.Server` struct in `internal/db/models.go`**

The struct currently ends with `UpdatedAt time.Time`. Add two fields after `VanityURL`:

```go
type Server struct {
	ID          int64          `json:"id"          db:"id"`
	Name        string         `json:"name"        db:"name"`
	IconURL     sql.NullString `json:"icon_url"    db:"icon_url"`
	OwnerID     int64          `json:"owner_id"    db:"owner_id"`
	VanityURL   sql.NullString `json:"vanity_url"  db:"vanity_url"`
	Description sql.NullString `json:"description" db:"description"`
	IsPublic    bool           `json:"is_public"   db:"is_public"`
	CreatedAt   time.Time      `json:"created_at"  db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"  db:"updated_at"`
}
```

- [ ] **Step 3: Add new structs to `internal/db/models.go`**

After the `ReportCategory` struct (around line 79), add:

```go
type ServerCategory struct {
	ID        int64     `json:"id"         db:"id"`
	Name      string    `json:"name"       db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// PublicServerRow is the DB scan target for discovery queries.
// Categories are populated in a second pass after the main query.
type PublicServerRow struct {
	ID          int64          `db:"id"`
	Name        string         `db:"name"`
	IconURL     sql.NullString `db:"icon_url"`
	VanityURL   sql.NullString `db:"vanity_url"`
	Description sql.NullString `db:"description"`
	MemberCount int            `db:"member_count"`
}
```

- [ ] **Step 4: Verify the build compiles**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

Expected: no errors. If the Server struct change breaks any scan site, fix it by adding the new columns to the relevant SELECT queries.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations.go internal/db/models.go
git commit -m "feat: migration #51 — server discovery schema + model updates"
```

---

## Task 2: Discovery Repository

**Files:**
- Create: `internal/db/discovery_repository.go`

### Background
The existing repo pattern (`internal/db/invite_repository.go`, `internal/db/report_repository.go`) uses methods on `*Repository`. Follow that exact pattern. Each method takes a `context.Context` as the first arg. Queries use `r.db` (the `*sqlx.DB` or `*sql.DB` — check what `r.db` is typed as by looking at `internal/db/repository.go`).

The `UpdateServer` repo call is at `internal/server/server_crud.go:119` — it passes the full `*db.Server`. Find the `UpdateServer` repo method (look in `internal/db/server_repository.go` or wherever `func (r *Repository) UpdateServer` is defined) and add `description` and `is_public` to its SQL.

- [ ] **Step 1: Update the `UpdateServer` repo SQL in `internal/db/` (likely `server_repository.go`)**

Search for `func (r *Repository) UpdateServer`. The actual SQL in this codebase is:
```sql
UPDATE servers SET name = $1, icon_url = $2, owner_id = $3, vanity_url = $4, updated_at = $5 WHERE id = $6
```
Replace it with (add description and is_public, keep existing columns):
```sql
UPDATE servers
SET name = $1, icon_url = $2, owner_id = $3, vanity_url = $4,
    description = $5, is_public = $6, updated_at = NOW()
WHERE id = $7
```
And update the Go ExecContext args to match the new parameter positions:
```go
_, err := r.db.ExecContext(ctx, query,
    server.Name, server.IconURL, server.OwnerID, server.VanityURL,
    server.Description, server.IsPublic, server.ID)
```
Remove the explicit `updated_at` arg since the query now uses `NOW()`. Adjust if the actual SQL differs — the key is to add `description` and `is_public` to the SET clause and pass `server.Description` and `server.IsPublic` as args.

- [ ] **Step 2: Create `internal/db/discovery_repository.go`**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
)

// GetServerCategories returns all server categories ordered by name.
func (r *Repository) GetServerCategories(ctx context.Context) ([]ServerCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM server_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []ServerCategory
	for rows.Next() {
		var c ServerCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// CreateServerCategory inserts a new server category and returns it.
func (r *Repository) CreateServerCategory(ctx context.Context, name string) (*ServerCategory, error) {
	var c ServerCategory
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO server_categories (name) VALUES ($1) RETURNING id, name, created_at`,
		name,
	).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteServerCategory removes a server category by ID.
// CASCADE on server_category_assignments will remove all assignments automatically.
func (r *Repository) DeleteServerCategory(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM server_categories WHERE id = $1`, id)
	return err
}

// GetServerCategoryAssignments returns the categories assigned to a server.
func (r *Repository) GetServerCategoryAssignments(ctx context.Context, serverID int64) ([]ServerCategory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sc.id, sc.name, sc.created_at
		FROM server_category_assignments sca
		JOIN server_categories sc ON sc.id = sca.category_id
		WHERE sca.server_id = $1
		ORDER BY sc.name`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []ServerCategory
	for rows.Next() {
		var c ServerCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// SetServerCategories replaces all category assignments for a server in a single transaction.
// Returns ErrTooManyCategories if len(categoryIDs) > 3.
func (r *Repository) SetServerCategories(ctx context.Context, serverID int64, categoryIDs []int64) error {
	if len(categoryIDs) > 3 {
		return fmt.Errorf("maximum 3 categories allowed")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM server_category_assignments WHERE server_id = $1`, serverID,
	); err != nil {
		return err
	}

	for _, catID := range categoryIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO server_category_assignments (server_id, category_id) VALUES ($1, $2)`,
			serverID, catID,
		); err != nil {
			// FK violation = invalid category ID
			return fmt.Errorf("invalid category")
		}
	}
	return tx.Commit()
}

// GetPublicServers returns paginated public servers with optional name search and category filter.
// Returns the rows and total count (for pagination).
func (r *Repository) GetPublicServers(ctx context.Context, categoryID *int64, q string, limit, offset int) ([]PublicServerRow, int, error) {
	const baseWhere = `
		WHERE s.is_public = TRUE
		  AND ($1::BIGINT IS NULL OR EXISTS (
		      SELECT 1 FROM server_category_assignments sca
		      WHERE sca.server_id = s.id AND sca.category_id = $1
		  ))
		  AND ($2 = '' OR s.name ILIKE '%' || $2 || '%')`

	countQuery := `SELECT COUNT(*) FROM servers s` + baseWhere
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, categoryID, q).Scan(&total); err != nil {
		return nil, 0, err
	}

	rowQuery := `
		SELECT s.id, s.name, s.icon_url, s.vanity_url, s.description,
		       (SELECT COUNT(*) FROM server_members sm WHERE sm.server_id = s.id) AS member_count
		FROM servers s` + baseWhere + `
		ORDER BY s.name
		LIMIT $3 OFFSET $4`

	rows, err := r.db.QueryContext(ctx, rowQuery, categoryID, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []PublicServerRow
	for rows.Next() {
		var row PublicServerRow
		if err := rows.Scan(
			&row.ID, &row.Name, &row.IconURL, &row.VanityURL,
			&row.Description, &row.MemberCount,
		); err != nil {
			return nil, 0, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}
```

Note: if `r.db` is `*sqlx.DB`, replace `r.db.QueryContext` / `r.db.ExecContext` / `r.db.QueryRowContext` / `r.db.BeginTx` with the equivalent — they have the same signatures. Check `internal/db/repository.go` for the field type.

- [ ] **Step 3: Verify build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/db/discovery_repository.go
git commit -m "feat: discovery repository — server categories + public server query"
```

---

## Task 3: Service Layer

**Files:**
- Modify: `internal/server/service.go` (add service types + dbServerToService update)
- Modify: `internal/server/server_crud.go` (extend UpdateServer; add SetServerCategories, ListServerCategories, Discover)

### Background
The service package has a `Server` API type (in `service.go` around line 17) that mirrors the DB type. The `dbServerToService` function (also in `service.go`) maps `*db.Server → *Server`. The `ServerService` struct is in `service.go`. Business logic lives in the `server_crud.go` and `server_invites.go` files.

- [ ] **Step 1: Add `Description` and `IsPublic` to service `Server` struct in `internal/server/service.go`**

The current `Server` struct ends with `UpdatedAt time.Time`. Add after `VanityURL`:
```go
type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	IconURL     string    `json:"icon_url,omitempty"`
	OwnerID     string    `json:"owner_id"`
	VanityURL   string    `json:"vanity_url,omitempty"`
	Description string    `json:"description,omitempty"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Add `PublicServer` type to `internal/server/service.go`**

After the `Server` struct:
```go
// PublicServer is the API representation of a server in the public discovery directory.
type PublicServer struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	IconURL     string            `json:"icon_url,omitempty"`
	VanityURL   string            `json:"vanity_url"`
	Description string            `json:"description,omitempty"`
	MemberCount int               `json:"member_count"`
	Categories  []db.ServerCategory `json:"categories"`
}
```

- [ ] **Step 3: Update `dbServerToService` in `internal/server/service.go`**

Find `dbServerToService` and add the new fields:
```go
func dbServerToService(s *db.Server) *Server {
	return &Server{
		ID:          strconv.FormatInt(s.ID, 10),
		Name:        s.Name,
		IconURL:     s.IconURL.String,
		OwnerID:     strconv.FormatInt(s.OwnerID, 10),
		VanityURL:   s.VanityURL.String,
		Description: s.Description.String,   // NEW
		IsPublic:    s.IsPublic,              // NEW
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
```

- [ ] **Step 4: Extend `UpdateServer` in `internal/server/server_crud.go`**

The current signature is `func (s *ServerService) UpdateServer(ctx context.Context, id, name, iconURL string) (*Server, error)`. Change it to:

```go
func (s *ServerService) UpdateServer(ctx context.Context, id, name, iconURL, description string, isPublic bool) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if len(description) > 200 {
		return nil, errors.New("description must be 200 characters or fewer")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServer(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	// is_public requires a vanity URL
	if isPublic && !server.VanityURL.Valid {
		return nil, errors.New("a vanity URL is required to list your server publicly")
	}

	server.Name = name
	server.IconURL = nullString(iconURL)
	server.Description = sql.NullString{String: description, Valid: description != ""}
	server.IsPublic = isPublic

	if err = s.repo.UpdateServer(ctx, server); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	srv := dbServerToService(server)
	if s.hub != nil {
		if payload, err := json.Marshal(srv); err == nil {
			s.hub.BroadcastToChannel("server:"+id, ws.EventServerUpdate, payload)
		}
	}
	return srv, nil
}
```

Note: `nullString` helper is already defined in service.go/server_crud.go — use it. The `sql` import may need to be added; check existing imports.

- [ ] **Step 5: Add `SetServerCategories`, `ListServerCategories`, and `Discover` to `internal/server/server_crud.go`**

Append at the end of the file:

```go
// ListServerCategories returns all admin-managed server categories.
func (s *ServerService) ListServerCategories(ctx context.Context) ([]db.ServerCategory, error) {
	cats, err := s.repo.GetServerCategories(ctx)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	return cats, nil
}

// SetServerCategories replaces the category assignments for a server.
// The caller (handler) must verify the user is the server owner before calling.
func (s *ServerService) SetServerCategories(ctx context.Context, serverID string, categoryIDs []int64) ([]db.ServerCategory, error) {
	if len(categoryIDs) > 3 {
		return nil, errors.New("maximum 3 categories allowed")
	}
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	if err := s.repo.SetServerCategories(ctx, id, categoryIDs); err != nil {
		return nil, err
	}
	cats, err := s.repo.GetServerCategoryAssignments(ctx, id)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	return cats, nil
}

// GetServerCategoryAssignments returns the categories assigned to a specific server.
func (s *ServerService) GetServerCategoryAssignments(ctx context.Context, serverID string) ([]db.ServerCategory, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	cats, err := s.repo.GetServerCategoryAssignments(ctx, id)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	return cats, nil
}

const discoverPageSize = 24

// Discover returns paginated public servers.
func (s *ServerService) Discover(ctx context.Context, categoryID *int64, q string, page int) ([]PublicServer, int, error) {
	if page < 1 {
		page = 1
	}
	// Clamp search query to prevent abuse of ILIKE
	if len(q) > 100 {
		q = q[:100]
	}
	offset := (page - 1) * discoverPageSize

	rows, total, err := s.repo.GetPublicServers(ctx, categoryID, q, discoverPageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	servers := make([]PublicServer, 0, len(rows))
	for _, row := range rows {
		id := strconv.FormatInt(row.ID, 10)
		cats, _ := s.repo.GetServerCategoryAssignments(ctx, row.ID)
		if cats == nil {
			cats = []db.ServerCategory{}
		}
		servers = append(servers, PublicServer{
			ID:          id,
			Name:        row.Name,
			IconURL:     row.IconURL.String,
			VanityURL:   row.VanityURL.String,
			Description: row.Description.String,
			MemberCount: row.MemberCount,
			Categories:  cats,
		})
	}
	return servers, total, nil
}
```

- [ ] **Step 6: Verify build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

Fix any import errors (add `"database/sql"` to `server_crud.go` if nullString for Description needs it).

- [ ] **Step 7: Commit**

```bash
git add internal/server/service.go internal/server/server_crud.go
git commit -m "feat: server service — discovery, categories, extended UpdateServer"
```

---

## Task 4: HTTP Handlers + Route Registration

**Files:**
- Modify: `internal/server/handler.go`
- Modify: `cmd/api/routes.go`

### Background
The handler file currently has `UpdateServerRequest` with only `Name` and `IconURL`. The existing `UpdateServer` handler (find it in `handler.go` — searches for `func (h *Handler) UpdateServer`) reads those fields and calls the service. We extend the request struct and handler call.

New handlers follow the exact same pattern as existing ones: read from `chi.URLParam`, get `userID` from context via `auth.GetUserIDFromContext(r)` (confirmed pattern in this codebase), call service, render JSON.

In `cmd/api/routes.go`, all routes are inside `router.Route("/api", func(r chi.Router) { ... })`. Within that, there is an auth middleware sub-group starting around line 158 (`r.Group(func(r chi.Router) { r.Use(auth.AuthMiddlewareWith(...)) ... })`). Public routes go **outside** that sub-group, at the same level as the theme and bot-invite routes (around lines 344–349).

- [ ] **Step 1: Extend `UpdateServerRequest` and `UpdateServer` handler in `internal/server/handler.go`**

Find `UpdateServerRequest` and update:
```go
type UpdateServerRequest struct {
	Name        string `json:"name"`
	IconURL     string `json:"icon_url"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}
```

Find the `UpdateServer` handler method and change the service call from:
```go
server, err := h.service.UpdateServer(r.Context(), serverID, req.Name, req.IconURL)
```
to:
```go
server, err := h.service.UpdateServer(r.Context(), serverID, req.Name, req.IconURL, req.Description, req.IsPublic)
```

- [ ] **Step 2: Add 4 new handler methods to `internal/server/handler.go`**

Append at the end of the file:

```go
// ListServerCategories handles GET /api/server-categories (public, no auth)
func (h *Handler) ListServerCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.service.ListServerCategories(r.Context())
	if err != nil {
		httputil.JSONError(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, cats)
}

// Discover handles GET /api/discover (public, no auth)
func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	q := r.URL.Query().Get("q")

	var categoryID *int64
	if catStr := r.URL.Query().Get("category_id"); catStr != "" {
		if v, err := strconv.ParseInt(catStr, 10, 64); err == nil {
			categoryID = &v
		}
	}

	servers, total, err := h.service.Discover(r.Context(), categoryID, q, page)
	if err != nil {
		httputil.JSONError(w, "failed to load servers", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, map[string]interface{}{
		"servers": servers,
		"total":   total,
	})
}

// SetServerCategories handles PUT /api/servers/{id}/categories (owner only)
func (h *Handler) SetServerCategories(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	// Verify ownership using the same pattern as UpdateServer handler
	userID := auth.GetUserIDFromContext(r) // confirmed pattern in this codebase
	server, err := h.service.GetServer(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "server not found", http.StatusNotFound)
		return
	}
	if server.OwnerID != userID {
		httputil.JSONError(w, "only the server owner can update categories", http.StatusForbidden)
		return
	}

	var req struct {
		CategoryIDs []int64 `json:"category_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.CategoryIDs == nil {
		req.CategoryIDs = []int64{}
	}

	cats, err := h.service.SetServerCategories(r.Context(), serverID, req.CategoryIDs)
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	render.JSON(w, r, cats)
}

// GetServerCategoriesForServer handles GET /api/servers/{id}/categories (auth required)
func (h *Handler) GetServerCategoriesForServer(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	cats, err := h.service.GetServerCategoryAssignments(r.Context(), serverID)
	if err != nil {
		httputil.JSONError(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	render.JSON(w, r, cats)
}
```

Note: `auth.GetUserIDFromContext(r)` is the confirmed function for extracting user ID from the request in this codebase. Also check if `GetServerByID` is an existing service method or needs a helper — look at how other handlers validate server ownership (some handlers call `s.repo.GetServerByID` directly, some use a service wrapper).

Add `"strconv"` to the import block if not already present.

- [ ] **Step 3: Register routes in `cmd/api/routes.go`**

All routes in this file are inside `router.Route("/api", func(r chi.Router) { ... })` — the `/api` prefix is automatic. Within that there is an auth middleware sub-group. The public routes must go **outside the auth sub-group**, alongside the theme/bot-invite routes at approximately lines 344–349:

```go
// Public theme routes — no authentication required
r.Get("/themes/repo", themeHandler.GetThemeRepo)
r.Get("/themes/{token}", themeHandler.GetPublicTheme)

// Public bot invite route (no auth required)
r.Get("/bots/invite/{token}", botsHandler.ResolveInvite)

// Public discovery routes — no authentication required  ← ADD HERE
r.Get("/discover", serverHandler.Discover)
r.Get("/server-categories", serverHandler.ListServerCategories)
```

Then inside the auth middleware sub-group, find where other server routes are registered (look for `r.Put("/servers/{id}/vanity", ...)`) and add:
```go
r.Put("/servers/{id}/categories", serverHandler.SetServerCategories)
r.Get("/servers/{id}/categories", serverHandler.GetServerCategoriesForServer)
```

- [ ] **Step 4: Verify build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/handler.go cmd/api/routes.go
git commit -m "feat: discovery handlers + route registration"
```

---

## Task 5: Admin Panel — Server Category Management

**Files:**
- Modify: `cmd/admin/handlers.go`
- Modify: `cmd/admin/server.go`

### Background
The admin panel's category handlers (`handleListCategories`, `handleCreateCategory`, `handleDeleteCategory`) at lines 324–365 of `cmd/admin/handlers.go` use the package-level `repo` variable. Copy that exact pattern for server categories. The admin routes are registered in `cmd/admin/server.go` inside the `r.Group(func(r chi.Router) { r.Use(adminAuthMiddleware) ... })` block.

- [ ] **Step 1: Add server category handlers to `cmd/admin/handlers.go`**

Append after the existing `handleDeleteCategory` function:

```go
// Server category management

func handleListServerCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := repo.GetServerCategories(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cats == nil {
		cats = []db.ServerCategory{}
	}
	jsonOK(w, cats)
}

func handleCreateServerCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	cat, err := repo.CreateServerCategory(r.Context(), req.Name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, cat)
}

func handleDeleteServerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := repo.DeleteServerCategory(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}
```

- [ ] **Step 2: Register server category routes in `cmd/admin/server.go`**

In the admin auth middleware group, after the report categories block:
```go
// Report categories
r.Get("/categories", handleListCategories)
r.Post("/categories", handleCreateCategory)
r.Delete("/categories/{id}", handleDeleteCategory)
```

Add:
```go
// Server categories
r.Get("/server-categories", handleListServerCategories)
r.Post("/server-categories", handleCreateServerCategory)
r.Delete("/server-categories/{id}", handleDeleteServerCategory)
```

- [ ] **Step 3: Verify build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/admin/handlers.go cmd/admin/server.go
git commit -m "feat: admin panel — server category CRUD endpoints"
```

---

## Task 6: Frontend Types + API Client

**Files:**
- Modify: `frontend/src/api/types.ts`
- Modify: `frontend/src/api/servers.ts`
- Create: `frontend/src/api/discovery.ts`

### Background
`types.ts` exports interfaces consumed by all frontend components. The `Server` interface is imported by `ServerSettings.tsx` and dozens of other components. The `updateServer` function in `servers.ts` is at lines 40-49 and currently sends only `name` and `icon_url`.

- [ ] **Step 1: Add `ServerCategory` and `PublicServer` to `frontend/src/api/types.ts`**

Near the top of the file (or after existing interface definitions), add:

```ts
export interface ServerCategory {
  id: number;
  name: string;
  created_at?: string;
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

Also find the existing `Server` interface and add two optional fields:
```ts
  description?: string;
  is_public?: boolean;
```

- [ ] **Step 2: Extend `updateServer` in `frontend/src/api/servers.ts`**

Replace the current `updateServer` function (lines 40-49):
```ts
export async function updateServer(
  id: string,
  name: string,
  iconURL?: string,
  description?: string,
  isPublic?: boolean,
): Promise<Server> {
  return apiClient.put<Server>(`/servers/${id}`, {
    name,
    icon_url: iconURL,
    description: description ?? '',
    is_public: isPublic ?? false,
  });
}
```

- [ ] **Step 3: Create `frontend/src/api/discovery.ts`**

```ts
import apiClient from './client';
import { ServerCategory, PublicServer } from './types';

export async function getServerCategories(): Promise<ServerCategory[]> {
  return apiClient.get<ServerCategory[]>('/server-categories');
}

export async function discoverServers(params: {
  page?: number;
  categoryId?: number;  // camelCase in TS; converted to category_id query param below
  q?: string;
} = {}): Promise<{ servers: PublicServer[]; total: number }> {
  const search = new URLSearchParams();
  if (params.page) search.set('page', String(params.page));
  if (params.categoryId != null) search.set('category_id', String(params.categoryId)); // snake_case for backend
  if (params.q) search.set('q', params.q);
  const qs = search.toString();
  return apiClient.get<{ servers: PublicServer[]; total: number }>(
    `/discover${qs ? '?' + qs : ''}`,
  );
}

export async function getServerCategoryAssignments(serverId: string): Promise<ServerCategory[]> {
  return apiClient.get<ServerCategory[]>(`/servers/${serverId}/categories`);
}

export async function setServerCategories(serverId: string, categoryIds: number[]): Promise<ServerCategory[]> {
  return apiClient.put<ServerCategory[]>(`/servers/${serverId}/categories`, {
    category_ids: categoryIds,
  });
}
```

Note: check how the existing API client (`apiClient`) is structured in `frontend/src/api/client.ts`. If it uses `fetch` with a base URL and returns parsed JSON, the pattern above is correct. If it uses a different pattern, match it exactly.

- [ ] **Step 4: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -40
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/servers.ts frontend/src/api/discovery.ts
git commit -m "feat: frontend types + discovery API client"
```

---

## Task 7: DiscoveryPage Component

**Files:**
- Create: `frontend/src/components/discovery/DiscoveryPage.tsx`
- Create: `frontend/src/components/discovery/DiscoveryPage.css`

### Background
The DiscoveryPage replaces the main content area (same as the homepage view). Look at how `frontend/src/components/home/` or the homepage renders for layout reference. The `InviteModal` or join flow for joining via vanity URL already exists — find it (search for `joinServer` or `handleJoinInvite` in `App.tsx`) and understand how to trigger it. The DiscoveryPage will call `onJoin(vanityUrl)` which the parent (App.tsx) wires to the existing invite join flow.

- [ ] **Step 1: Create `frontend/src/components/discovery/DiscoveryPage.tsx`**

```tsx
import React, { useState, useEffect, useCallback, useRef } from 'react';
import { PublicServer, ServerCategory } from '../../api/types';
import { discoverServers, getServerCategories } from '../../api/discovery';
import './DiscoveryPage.css';

interface Props {
  currentUserId?: string;
  joinedServerIds: Set<string>;
  onJoin: (vanityUrl: string) => void;
}

const PAGE_SIZE = 24;

export const DiscoveryPage: React.FC<Props> = ({ currentUserId, joinedServerIds, onJoin }) => {
  const [categories, setCategories] = useState<ServerCategory[]>([]);
  const [servers, setServers] = useState<PublicServer[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [selectedCategory, setSelectedCategory] = useState<number | null>(null);
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Load categories once on mount
  useEffect(() => {
    getServerCategories().then(setCategories).catch(() => {});
  }, []);

  const loadServers = useCallback(async (q: string, catId: number | null, pg: number) => {
    setLoading(true);
    try {
      const result = await discoverServers({ page: pg, categoryId: catId ?? undefined, q: q || undefined });
      setServers(result.servers ?? []);
      setTotal(result.total ?? 0);
    } catch {
      setServers([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, []);

  // Fetch on page/category change (immediate)
  useEffect(() => {
    loadServers(query, selectedCategory, page);
  }, [page, selectedCategory]); // eslint-disable-line

  // Debounce search input
  const handleSearch = (val: string) => {
    setQuery(val);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setPage(1);
      loadServers(val, selectedCategory, 1);
    }, 300);
  };

  const handleCategorySelect = (id: number | null) => {
    setSelectedCategory(id);
    setPage(1);
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="discovery-page">
      <div className="discovery-header">
        <h1 className="discovery-title">Discover Servers</h1>
        <input
          className="discovery-search"
          type="text"
          placeholder="Search servers..."
          value={query}
          onChange={e => handleSearch(e.target.value)}
          maxLength={100}
        />
      </div>

      <div className="discovery-filters">
        <button
          className={`discovery-cat-pill${selectedCategory === null ? ' active' : ''}`}
          onClick={() => handleCategorySelect(null)}
        >
          All
        </button>
        {categories.map(cat => (
          <button
            key={cat.id}
            className={`discovery-cat-pill${selectedCategory === cat.id ? ' active' : ''}`}
            onClick={() => handleCategorySelect(cat.id)}
          >
            {cat.name}
          </button>
        ))}
      </div>

      {loading ? (
        <div className="discovery-loading">Loading...</div>
      ) : servers.length === 0 ? (
        <div className="discovery-empty">No servers found.</div>
      ) : (
        <div className="discovery-grid">
          {servers.map(server => {
            const isJoined = joinedServerIds.has(server.id);
            return (
              <div key={server.id} className="discovery-card">
                <div className="discovery-card-icon">
                  {server.icon_url ? (
                    <img src={server.icon_url} alt={server.name} />
                  ) : (
                    <span>{server.name.charAt(0).toUpperCase()}</span>
                  )}
                </div>
                <div className="discovery-card-body">
                  <div className="discovery-card-name">{server.name}</div>
                  {server.description && (
                    <div className="discovery-card-desc">{server.description}</div>
                  )}
                  <div className="discovery-card-meta">
                    <span>{server.member_count.toLocaleString()} members</span>
                    {server.categories.map(c => (
                      <span key={c.id} className="discovery-card-cat">{c.name}</span>
                    ))}
                  </div>
                </div>
                <button
                  className={`discovery-join-btn${isJoined ? ' joined' : ''}`}
                  onClick={() => !isJoined && onJoin(server.vanity_url)}
                  disabled={isJoined}
                >
                  {isJoined ? 'Joined' : 'Join'}
                </button>
              </div>
            );
          })}
        </div>
      )}

      {totalPages > 1 && (
        <div className="discovery-pagination">
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Prev</button>
          <span>Page {page} of {totalPages}</span>
          <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next</button>
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 2: Create `frontend/src/components/discovery/DiscoveryPage.css`**

```css
.discovery-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow-y: auto;
  padding: 32px;
  gap: 24px;
  background: var(--bg-primary, #1a1b1e);
  color: var(--text-primary, #dcddde);
}

.discovery-header {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.discovery-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0;
  flex: 1;
}

.discovery-search {
  background: var(--bg-secondary, #2f3136);
  border: 1px solid var(--border-color, #40444b);
  border-radius: 6px;
  color: var(--text-primary, #dcddde);
  padding: 8px 14px;
  font-size: 14px;
  width: 260px;
  outline: none;
}
.discovery-search:focus {
  border-color: var(--accent, #5865f2);
}

.discovery-filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.discovery-cat-pill {
  background: var(--bg-secondary, #2f3136);
  border: 1px solid var(--border-color, #40444b);
  border-radius: 20px;
  color: var(--text-secondary, #b9bbbe);
  padding: 6px 14px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s;
}
.discovery-cat-pill:hover {
  border-color: var(--accent, #5865f2);
  color: var(--text-primary, #dcddde);
}
.discovery-cat-pill.active {
  background: var(--accent, #5865f2);
  border-color: var(--accent, #5865f2);
  color: #fff;
}

.discovery-loading,
.discovery-empty {
  text-align: center;
  color: var(--text-muted, #72767d);
  padding: 48px 0;
  font-size: 15px;
}

.discovery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.discovery-card {
  background: var(--bg-secondary, #2f3136);
  border-radius: 8px;
  padding: 16px;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  border: 1px solid transparent;
  transition: border-color 0.15s;
}
.discovery-card:hover {
  border-color: var(--border-color, #40444b);
}

.discovery-card-icon {
  flex-shrink: 0;
  width: 48px;
  height: 48px;
  border-radius: 12px;
  overflow: hidden;
  background: var(--bg-tertiary, #202225);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary, #dcddde);
}
.discovery-card-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.discovery-card-body {
  flex: 1;
  min-width: 0;
}

.discovery-card-name {
  font-weight: 600;
  font-size: 15px;
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.discovery-card-desc {
  font-size: 13px;
  color: var(--text-secondary, #b9bbbe);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: 6px;
}

.discovery-card-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--text-muted, #72767d);
}

.discovery-card-cat {
  background: var(--bg-tertiary, #202225);
  border-radius: 4px;
  padding: 2px 6px;
  font-size: 11px;
}

.discovery-join-btn {
  flex-shrink: 0;
  background: var(--accent, #5865f2);
  border: none;
  border-radius: 6px;
  color: #fff;
  padding: 6px 16px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;
  align-self: center;
}
.discovery-join-btn:hover:not(:disabled) {
  opacity: 0.85;
}
.discovery-join-btn.joined,
.discovery-join-btn:disabled {
  background: var(--bg-tertiary, #202225);
  color: var(--text-muted, #72767d);
  cursor: default;
}

.discovery-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 8px 0 24px;
}
.discovery-pagination button {
  background: var(--bg-secondary, #2f3136);
  border: 1px solid var(--border-color, #40444b);
  border-radius: 6px;
  color: var(--text-primary, #dcddde);
  padding: 6px 16px;
  cursor: pointer;
  font-size: 13px;
}
.discovery-pagination button:disabled {
  opacity: 0.4;
  cursor: default;
}
```

- [ ] **Step 3: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -40
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/discovery/
git commit -m "feat: DiscoveryPage component + styles"
```

---

## Task 8: Sidebar Globe Button + App Wiring

**Files:**
- Modify: `frontend/src/components/layout/Sidebar.tsx`
- Modify: `frontend/src/components/layout/MainLayout.tsx`
- Modify: `frontend/src/App.tsx`

### Background
`Sidebar.tsx` has a home button at the top and props passed through `MainLayout.tsx` → `App.tsx`. Follow the exact same pattern as `onHomepage` / `activeServerId === null` for the active state detection. In `App.tsx`, the join flow for invite codes is already implemented — find `handleJoinInvite` or how `joinServer` is called and wire `onJoin` in DiscoveryPage to that same function using the vanity URL.

- [ ] **Step 1: Add discovery props + globe button to `frontend/src/components/layout/Sidebar.tsx`**

Add to `SidebarProps` interface:
```ts
onDiscovery?: () => void;
discoveryActive?: boolean;
```

Add to the destructured props in the component.

Add the globe button between the home button and the first `<div className="divider" />`:
```tsx
<div
  className={`home-button ${discoveryActive ? 'active' : ''}`}
  onClick={() => onDiscovery?.()}
>
  <svg viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
  </svg>
  <span className="tooltip">Discover</span>
</div>
```

- [ ] **Step 2: Pass-through props in `frontend/src/components/layout/MainLayout.tsx`**

Add to `MainLayoutProps` interface:
```ts
onDiscovery?: () => void;
discoveryActive?: boolean;
```

Add to the destructured props, and pass to `<Sidebar>`:
```tsx
<Sidebar
  ...existing props...
  onDiscovery={onDiscovery}
  discoveryActive={discoveryActive}
/>
```

- [ ] **Step 3: Wire DiscoveryPage in `frontend/src/App.tsx`**

Add import:
```tsx
import { DiscoveryPage } from './components/discovery/DiscoveryPage';
```

Add state near other panel state:
```tsx
const [showDiscovery, setShowDiscovery] = useState(false);
```

Find where `onHomepage` is handled — it likely sets `activeServerId` to null. Add:
```tsx
const handleDiscovery = () => {
  setShowDiscovery(true);
  setActiveServerId(null);  // deselect active server — adjust to match actual state setter name
};
```

When a server is selected (in `handleServerSelect` or similar), also `setShowDiscovery(false)`.

Pass to `<MainLayout>`:
```tsx
onDiscovery={handleDiscovery}
discoveryActive={showDiscovery}
```

Render DiscoveryPage as main content. Find the main content area render (the `children` of MainLayout) and add a conditional before the server/channel view:
```tsx
{showDiscovery ? (
  <DiscoveryPage
    currentUserId={currentUser?.id}
    joinedServerIds={new Set(servers.map(s => s.id))}
    onJoin={vanityUrl => {
      // Use existing join-by-invite-code flow with the vanity URL
      // Find the existing handleJoinInvite / handleAcceptInvite function and call it here
      handleJoinInvite(vanityUrl);  // adjust to match actual function name
    }}
  />
) : (
  /* existing server/channel view */
)}
```

Note: Look at how `joinServer` / `handleJoinInvite` is wired in `App.tsx` for the existing invite modal. The vanity URL is treated identically to an invite code by the backend (`POST /invites/:code` or `POST /api/join/:code` — check the exact endpoint). Call that same function.

- [ ] **Step 4: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -40
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/layout/Sidebar.tsx frontend/src/components/layout/MainLayout.tsx frontend/src/App.tsx
git commit -m "feat: sidebar discovery button + DiscoveryPage wiring in App"
```

---

## Task 9: ServerSettings Overview Tab Extension

**Files:**
- Modify: `frontend/src/components/settings/ServerSettings.tsx`

### Background
The Overview tab in `ServerSettings.tsx` currently has: Server Identity (name + icon), Vanity URL, Save/Reset buttons. We add Description (textarea) and Server Directory (is_public toggle + category picker) between Vanity URL and the save buttons.

The `hasOverviewChanges` function (search for it in the file) must include the new fields. The `handleSaveOverview` function must be wrapped in try/catch and call `onUpdate` only on full success.

State is initialized in a `useEffect` that runs when `server` changes (look for `useEffect` that calls `setName(server.name)` etc.).

- [ ] **Step 1: Add state variables for new fields**

Find the Overview fields state block (around line 48 where `name`, `vanityUrl`, `iconUrl` are declared). Add:
```tsx
const [description, setDescription] = useState('');
const [isPublic, setIsPublic] = useState(false);
const [selectedCategoryIds, setSelectedCategoryIds] = useState<number[]>([]);
const [initialCategoryIds, setInitialCategoryIds] = useState<number[]>([]);
const [allCategories, setAllCategories] = useState<ServerCategory[]>([]);
```

Add import at the top of the file:
```tsx
import { ServerCategory } from '../../api/types';
import { getServerCategories, getServerCategoryAssignments, setServerCategories } from '../../api/discovery';
```

- [ ] **Step 2: Initialize new state in the server `useEffect`**

Find the `useEffect` that runs when `server` changes and initializes `name`, `vanityUrl`, `iconUrl`. Add:
```tsx
setDescription(server.description ?? '');
setIsPublic(server.is_public ?? false);

// Fetch categories
Promise.all([
  getServerCategories(),
  getServerCategoryAssignments(server.id),
]).then(([cats, assigned]) => {
  setAllCategories(cats);
  const ids = assigned.map(c => c.id);
  setSelectedCategoryIds(ids);
  setInitialCategoryIds(ids);
}).catch(() => {});
```

- [ ] **Step 3: Add `arrayEquals` helper at module level**

Near the top of the file (before the component function), add:
```ts
function arrayEquals(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return false;
  const sa = [...a].sort();
  const sb = [...b].sort();
  return sa.every((v, i) => v === sb[i]);
}
```

- [ ] **Step 4: Update `hasOverviewChanges` to include new fields**

Find `hasOverviewChanges` and extend it:
```ts
const hasOverviewChanges = () =>
  name !== server.name ||
  iconUrl !== (server.icon_url ?? '') ||
  vanityUrl !== (server.vanity_url ?? '') ||
  description !== (server.description ?? '') ||
  isPublic !== (server.is_public ?? false) ||
  !arrayEquals(selectedCategoryIds, initialCategoryIds);
```

- [ ] **Step 5: Update `handleSaveOverview` with error handling and new calls**

Find `handleSaveOverview` and replace with:
```tsx
const handleSaveOverview = async () => {
  setOverviewLoading(true);
  setOverviewError('');
  try {
    const updated = await updateServer(
      server.id, name.trim(), iconUrl || undefined, description, isPublic,
    );
    if (vanityUrl.trim() !== (server.vanity_url ?? '')) {
      await setVanityURL(server.id, vanityUrl.trim());
    }
    await setServerCategories(server.id, selectedCategoryIds);
    setInitialCategoryIds(selectedCategoryIds);
    onUpdate(updated);
  } catch (err: any) {
    setOverviewError(err?.message ?? 'Failed to save changes');
  } finally {
    setOverviewLoading(false);
  }
};
```

Note: `setServerCategories` is imported from `../../api/discovery`. Make sure this import was added in Step 1. Also add `setServerCategories` to the import from `servers.ts` if needed — but since it's in `discovery.ts`, just confirm the discovery import includes it.

- [ ] **Step 6: Add Description and Server Directory sections to the Overview tab JSX**

Find the Vanity URL section (lines 358-372). After the closing `</div>` of the Vanity URL section and before the Save button block, insert:

```tsx
{/* Description */}
<div className="settings-section">
  <div className="settings-section-title">Description</div>
  <textarea
    className="settings-form-input settings-bio-input"
    value={description}
    onChange={e => setDescription(e.target.value.slice(0, 200))}
    placeholder="What's your server about?"
    rows={3}
    maxLength={200}
    disabled={overviewLoading}
  />
  <div className="settings-form-hint" style={{ textAlign: 'right', marginTop: 4 }}>
    {description.length} / 200
  </div>
</div>

{/* Server Directory */}
<div className="settings-section">
  <div className="settings-section-title">Server Directory</div>
  <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: vanityUrl.trim() ? 'pointer' : 'not-allowed' }}>
    <input
      type="checkbox"
      checked={isPublic}
      onChange={e => setIsPublic(e.target.checked)}
      disabled={!vanityUrl.trim() || overviewLoading}
    />
    <span>List this server in the public directory</span>
  </label>
  <div className="settings-form-hint">
    {vanityUrl.trim()
      ? 'Your server will appear in Discover when enabled.'
      : 'A vanity URL is required to list your server publicly.'}
  </div>

  {isPublic && allCategories.length > 0 && (
    <div style={{ marginTop: 12 }}>
      <div className="settings-section-title" style={{ marginBottom: 8 }}>
        Categories <span style={{ color: 'var(--text-muted, #72767d)', fontWeight: 400 }}>(up to 3)</span>
      </div>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        {allCategories.map(cat => {
          const selected = selectedCategoryIds.includes(cat.id);
          return (
            <button
              key={cat.id}
              className={`discovery-cat-pill${selected ? ' active' : ''}`}
              onClick={() => {
                if (selected) {
                  setSelectedCategoryIds(ids => ids.filter(id => id !== cat.id));
                } else if (selectedCategoryIds.length < 3) {
                  setSelectedCategoryIds(ids => [...ids, cat.id]);
                }
              }}
              disabled={overviewLoading}
              type="button"
            >
              {cat.name}
            </button>
          );
        })}
      </div>
    </div>
  )}
</div>
```

Note: The `.discovery-cat-pill` CSS class is defined in `DiscoveryPage.css`. For it to work here, either import that CSS file in `ServerSettings.tsx` or duplicate the relevant pill styles in `Settings.css`. Simplest: import `'../discovery/DiscoveryPage.css'` at the top of `ServerSettings.tsx`.

- [ ] **Step 7: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -40
```

Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/settings/ServerSettings.tsx
git commit -m "feat: server settings — description, is_public toggle, category picker"
```

---

## Task 10: End-to-End Build Verification

**Files:** none (verification only)

- [ ] **Step 1: Full Go build**

```bash
cd /home/dylan/Developer/parley && go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 2: Full TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit
```

Expected: exits 0, no output.

- [ ] **Step 3: Verify migration is wired correctly**

Check that the migration runner picks up the new migration. Look at `internal/db/migrations.go` — `MigrationSQL()` concatenates all migrations and the runner applies them. No additional wiring needed.

- [ ] **Step 4: Smoke test the public endpoints exist**

If a dev server is running, verify:
- `GET /api/discover` returns `{"servers":[],"total":0}` (no auth token needed)
- `GET /api/server-categories` returns `[]` (no auth token needed)

If not running, skip.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: server discovery — end-to-end build verified"
```
