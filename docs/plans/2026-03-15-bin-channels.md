# Bin Channels Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add forum-style "Bin" channels with code sharing, syntax highlighting, line-anchored comments, and platform-wide nested replies + edit history.

**Architecture:** Bin channels are a new `ChannelType = 2`. Posts are stored in dedicated tables (`bin_posts`, `bin_post_files`, etc.) while general comments reuse the existing message system via per-post thread channels. Platform-wide features (nested replies via `parent_id` on messages, edit history via `message_versions`, Shiki syntax highlighting) are built first as shared infrastructure, then bin-specific features build on top.

**Tech Stack:** Go 1.25, PostgreSQL, chi router, React 18, TypeScript, Shiki (syntax highlighting), existing WebSocket infrastructure.

**Spec:** `docs/specs/2026-03-15-bin-channels-design.md`

---

## Chunk 1: Platform-Wide Backend — Nested Replies, Edit History, and ChannelTypeBin

These are foundational changes that bin channels depend on, but are also useful across the entire platform.

### Task 1: Database Migration — `parent_id`, `message_versions`, `ChannelTypeBin`

**Files:**
- Modify: `internal/db/migrations.go` — append new migration
- Modify: `internal/db/models.go` — add `ChannelTypeBin`, update `Message` struct

- [ ] **Step 1: Add migration to `internal/db/migrations.go`**

Append this migration to the `Migrations` slice:

```go
`-- Add parent_id to messages for nested replies
ALTER TABLE messages ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES messages(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_messages_parent_id ON messages(parent_id) WHERE parent_id IS NOT NULL;

-- Create message_versions table for edit history
CREATE TABLE IF NOT EXISTS message_versions (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    edited_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_message_versions_message_id ON message_versions(message_id, edited_at);
`,
```

- [ ] **Step 2: Update `internal/db/models.go`**

Add `ChannelTypeBin` constant:

```go
const (
    ChannelTypeText  ChannelType = 0
    ChannelTypeVoice ChannelType = 1
    ChannelTypeBin   ChannelType = 2
)
```

Add `ParentID` field to the `Message` struct (after `Nonce`):

```go
ParentID        *int64    `json:"parent_id,omitempty" db:"parent_id"`
```

- [ ] **Step 3: Verify migration runs**

Run: `go build ./cmd/api && echo "Build OK"`

Expected: Build succeeds. Migration will auto-apply on next server start via the existing migration runner.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations.go internal/db/models.go
git commit -m "feat: add parent_id for nested replies, message_versions table, ChannelTypeBin constant"
```

---

### Task 2: Message Edit History — Repository Layer

**Files:**
- Create: `internal/db/message_version_repository.go`
- Modify: `internal/db/message_repository.go` — save version before edit

- [ ] **Step 1: Create `internal/db/message_version_repository.go`**

```go
package db

import (
    "context"
    "time"
)

// MessageVersion represents a previous version of a message.
type MessageVersion struct {
    ID        int64     `json:"id" db:"id"`
    MessageID int64     `json:"message_id" db:"message_id"`
    Content   string    `json:"content" db:"content"`
    EditedAt  time.Time `json:"edited_at" db:"edited_at"`
}

// SaveMessageVersion saves the current content of a message before it is edited.
func (r *Repository) SaveMessageVersion(ctx context.Context, messageID int64, content string) error {
    _, err := r.db.ExecContext(ctx,
        `INSERT INTO message_versions (message_id, content, edited_at) VALUES ($1, $2, NOW())`,
        messageID, content)
    return err
}

// GetMessageVersions returns all saved versions for a message, newest first.
func (r *Repository) GetMessageVersions(ctx context.Context, messageID int64) ([]MessageVersion, error) {
    rows, err := r.db.QueryContext(ctx,
        `SELECT id, message_id, content, edited_at FROM message_versions
         WHERE message_id = $1 ORDER BY edited_at DESC`, messageID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var versions []MessageVersion
    for rows.Next() {
        var v MessageVersion
        if err := rows.Scan(&v.ID, &v.MessageID, &v.Content, &v.EditedAt); err != nil {
            return nil, err
        }
        versions = append(versions, v)
    }
    return versions, rows.Err()
}

// PurgeOldMessageVersions deletes message versions older than 90 days.
func (r *Repository) PurgeOldMessageVersions(ctx context.Context) (int64, error) {
    result, err := r.db.ExecContext(ctx,
        `DELETE FROM message_versions WHERE edited_at < NOW() - INTERVAL '90 days'`)
    if err != nil {
        return 0, err
    }
    return result.RowsAffected()
}
```

- [ ] **Step 2: Modify `internal/db/message_repository.go` — save version before edit**

Find the `UpdateMessage` (or `EditMessage`) method. Before the UPDATE query, add a call to save the current content. The pattern is:

1. Fetch the current message content
2. Save it to `message_versions`
3. Perform the update

Add this before the UPDATE statement:

```go
// Save current content as a version before editing
var oldContent string
err := r.db.QueryRowContext(ctx,
    `SELECT content FROM messages WHERE id = $1`, messageID).Scan(&oldContent)
if err != nil {
    return nil, err
}
if err := r.SaveMessageVersion(ctx, messageID, oldContent); err != nil {
    return nil, err
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/db/message_version_repository.go internal/db/message_repository.go
git commit -m "feat: save message versions on edit, add version query and purge"
```

---

### Task 3: Message Edit History — Service and Handler

**Files:**
- Modify: `internal/message/service.go` — expose version retrieval
- Modify: `internal/message/handler.go` — add GET versions endpoint
- Modify: `cmd/api/routes.go` — register new route

- [ ] **Step 1: Add `GetMessageVersions` to `internal/message/service.go`**

```go
// GetMessageVersions returns the edit history for a message.
func (s *MessageService) GetMessageVersions(ctx context.Context, messageID string) ([]db.MessageVersion, error) {
    id, err := strconv.ParseInt(messageID, 10, 64)
    if err != nil {
        return nil, fmt.Errorf("invalid message ID")
    }
    return s.repo.GetMessageVersions(ctx, id)
}
```

- [ ] **Step 2: Add handler in `internal/message/handler.go`**

```go
// GetMessageVersions returns the edit history for a message.
func (h *Handler) GetMessageVersions(w http.ResponseWriter, r *http.Request) {
    messageID := chi.URLParam(r, "id")
    versions, err := h.service.GetMessageVersions(r.Context(), messageID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    if versions == nil {
        versions = []db.MessageVersion{}
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(versions)
}
```

- [ ] **Step 3: Register route in `cmd/api/routes.go`**

Add after the existing message routes (around line 107):

```go
r.Get("/messages/{id}/versions", messageHandler.GetMessageVersions)
```

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add internal/message/service.go internal/message/handler.go cmd/api/routes.go
git commit -m "feat: add GET /messages/{id}/versions endpoint for edit history"
```

---

### Task 4: Nested Replies — Repository, Service, Handler

**Files:**
- Modify: `internal/db/message_repository.go` — include `parent_id` in create and query
- Modify: `internal/message/service.go` — accept `parent_id` in SendMessage
- Modify: `internal/message/handler.go` — parse `parent_id` from request

- [ ] **Step 1: Modify `internal/db/message_repository.go`**

Update the `CreateMessage` method to accept and insert `parent_id`. Add `parent_id` as a parameter (type `*int64`) and include it in the INSERT column list and VALUES.

Update the `GetChannelMessages` query to include `m.parent_id` in the SELECT and scan it into the `Message.ParentID` field.

- [ ] **Step 2: Modify `internal/message/service.go`**

Update `SendMessage` to accept an optional `parentID string` parameter. If non-empty, parse it to `*int64` and pass to the repository. If the parent message exists but is in a different channel, return an error.

Include `ParentID` in the broadcast payload by setting it on the returned message.

- [ ] **Step 3: Modify `internal/message/handler.go`**

Update the `SendMessage` handler to parse `parent_id` from the JSON request body and pass it to the service.

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add internal/db/message_repository.go internal/message/service.go internal/message/handler.go
git commit -m "feat: support parent_id on messages for nested replies"
```

---

### Task 5: Version Purge Goroutine

**Files:**
- Modify: `cmd/api/main.go` — add startup purge + 24h ticker

- [ ] **Step 1: Add purge goroutine to `cmd/api/main.go`**

After migrations run and the repository is initialized, add:

```go
// Start version purge goroutine
go func() {
    // Purge on startup
    if n, err := repo.PurgeOldMessageVersions(context.Background()); err != nil {
        log.Printf("version purge error: %v", err)
    } else if n > 0 {
        log.Printf("purged %d old message versions", n)
    }

    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    for range ticker.C {
        if n, err := repo.PurgeOldMessageVersions(context.Background()); err != nil {
            log.Printf("version purge error: %v", err)
        } else if n > 0 {
            log.Printf("purged %d old message versions", n)
        }
    }
}()
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add cmd/api/main.go
git commit -m "feat: add 24h goroutine to purge message versions older than 90 days"
```

---

## Chunk 2: Bin Channels Backend — Posts, Files, Versions, Line Comments

### Task 6: Database Migration — Bin Tables

**Files:**
- Modify: `internal/db/migrations.go` — append bin tables migration

- [ ] **Step 1: Add migration for bin tables**

Append to the `Migrations` slice:

```go
`-- Random 9-digit ID generator for bin posts
CREATE OR REPLACE FUNCTION gen_bin_post_id() RETURNS BIGINT LANGUAGE plpgsql VOLATILE AS $$
DECLARE new_id BIGINT;
BEGIN
  LOOP
    new_id := floor(random() * 900000000 + 100000000)::BIGINT;
    EXIT WHEN NOT EXISTS (SELECT 1 FROM bin_posts WHERE id = new_id);
  END LOOP;
  RETURN new_id;
END; $$;

-- Bin posts
CREATE TABLE IF NOT EXISTS bin_posts (
    id BIGSERIAL PRIMARY KEY DEFAULT gen_bin_post_id(),
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    thread_channel_id BIGINT REFERENCES channels(id) ON DELETE SET NULL,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_posts_channel_id ON bin_posts(channel_id, created_at DESC);

-- Bin post files (current version)
CREATE TABLE IF NOT EXISTS bin_post_files (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    language VARCHAR(50) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0,
    UNIQUE(post_id, position)
);
CREATE INDEX IF NOT EXISTS idx_bin_post_files_post_id ON bin_post_files(post_id, position);

-- Bin post versions (snapshots on edit)
CREATE TABLE IF NOT EXISTS bin_post_versions (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    version INT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_post_versions_post_id ON bin_post_versions(post_id);

-- Bin post version files (file snapshots per version)
CREATE TABLE IF NOT EXISTS bin_post_version_files (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES bin_post_versions(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    language VARCHAR(50) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bin_post_version_files_version_id ON bin_post_version_files(version_id);

-- Line comments anchored to specific version/file/line
CREATE TABLE IF NOT EXISTS bin_line_comments (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES bin_posts(id) ON DELETE CASCADE,
    version_id BIGINT NOT NULL REFERENCES bin_post_versions(id) ON DELETE CASCADE,
    file_id BIGINT NOT NULL REFERENCES bin_post_version_files(id) ON DELETE CASCADE,
    line_number INT NOT NULL,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    parent_id BIGINT REFERENCES bin_line_comments(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bin_line_comments_lookup ON bin_line_comments(version_id, file_id, line_number);

-- Admin-defined tags per bin channel
CREATE TABLE IF NOT EXISTS bin_channel_tags (
    id BIGSERIAL PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '#99aab5',
    UNIQUE(channel_id, name)
);
CREATE INDEX IF NOT EXISTS idx_bin_channel_tags_channel_id ON bin_channel_tags(channel_id);
`,
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat: add database migration for bin channels tables"
```

---

### Task 7: Bin Models

**Files:**
- Modify: `internal/db/models.go` — add bin-related structs

- [ ] **Step 1: Add bin structs to `internal/db/models.go`**

Add `"github.com/lib/pq"` to the imports if not already present.

```go
// BinPost represents a post in a bin channel.
type BinPost struct {
    ID              int64     `json:"id" db:"id"`
    ChannelID       int64     `json:"channel_id" db:"channel_id"`
    ThreadChannelID int64     `json:"thread_channel_id" db:"thread_channel_id"`
    AuthorID        int64     `json:"author_id" db:"author_id"`
    Title           string    `json:"title" db:"title"`
    Description     string    `json:"description" db:"description"`
    Tags            pq.StringArray `json:"tags" db:"tags"`
    CreatedAt       time.Time `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
    // Computed fields
    AuthorUsername    string        `json:"author_username" db:"-"`
    AuthorAvatarURL  string        `json:"author_avatar_url,omitempty" db:"-"`
    Files            []BinPostFile `json:"files,omitempty" db:"-"`
    CommentCount     int           `json:"comment_count" db:"-"`
    LineCommentCount int           `json:"line_comment_count" db:"-"`
    VersionCount     int           `json:"version_count" db:"-"`
}

// BinPostFile represents a code file attached to a bin post.
type BinPostFile struct {
    ID       int64  `json:"id" db:"id"`
    PostID   int64  `json:"post_id" db:"post_id"`
    Filename string `json:"filename" db:"filename"`
    Language string `json:"language" db:"language"`
    Content  string `json:"content" db:"content"`
    Position int    `json:"position" db:"position"`
}

// BinPostVersion represents a version snapshot of a bin post.
type BinPostVersion struct {
    ID          int64     `json:"id" db:"id"`
    PostID      int64     `json:"post_id" db:"post_id"`
    Version     int       `json:"version" db:"version"`
    Description string    `json:"description" db:"description"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    // Computed
    Files []BinPostVersionFile `json:"files,omitempty" db:"-"`
}

// BinPostVersionFile represents a file snapshot within a version.
type BinPostVersionFile struct {
    ID        int64  `json:"id" db:"id"`
    VersionID int64  `json:"version_id" db:"version_id"`
    Filename  string `json:"filename" db:"filename"`
    Language  string `json:"language" db:"language"`
    Content   string `json:"content" db:"content"`
    Position  int    `json:"position" db:"position"`
}

// BinLineComment represents a comment anchored to a specific line in a file version.
type BinLineComment struct {
    ID         int64     `json:"id" db:"id"`
    PostID     int64     `json:"post_id" db:"post_id"`
    VersionID  int64     `json:"version_id" db:"version_id"`
    FileID     int64     `json:"file_id" db:"file_id"`
    LineNumber int       `json:"line_number" db:"line_number"`
    AuthorID   int64     `json:"author_id" db:"author_id"`
    Content    string    `json:"content" db:"content"`
    ParentID   *int64    `json:"parent_id,omitempty" db:"parent_id"`
    CreatedAt  time.Time `json:"created_at" db:"created_at"`
    UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
    // Computed
    AuthorUsername  string `json:"author_username" db:"-"`
    AuthorAvatarURL string `json:"author_avatar_url,omitempty" db:"-"`
}

// BinChannelTag represents an admin-defined tag for a bin channel.
type BinChannelTag struct {
    ID        int64  `json:"id" db:"id"`
    ChannelID int64  `json:"channel_id" db:"channel_id"`
    Name      string `json:"name" db:"name"`
    Color     string `json:"color" db:"color"`
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/db/models.go
git commit -m "feat: add bin post, file, version, line comment, and tag model structs"
```

---

### Task 8: Bin Repository — Post CRUD

**Files:**
- Create: `internal/db/bin_post_repository.go`

- [ ] **Step 1: Create `internal/db/bin_post_repository.go`** with post CRUD operations:

Implement these methods on `*Repository`:

- `CreateBinPost(ctx, channelID, authorID int64, title, description string, tags []string) (*BinPost, error)` — INSERT into `bin_posts`, return the new post. Also creates the thread channel (INSERT into `channels` with `channel_type=0`, `parent_id=channelID`, name derived from post title). Also creates the initial version (version 1) in `bin_post_versions`.
- `GetBinPost(ctx, postID int64) (*BinPost, error)` — SELECT post with JOIN on users for author info. Include `version_count` via subquery on `bin_post_versions`, `comment_count` via subquery on thread channel messages, `line_comment_count` via subquery on `bin_line_comments`.
- `GetBinPostsByChannel(ctx, channelID int64, tag, language, authorID string, sort string, limit, offset int) ([]BinPost, error)` — SELECT with optional filters. `tag`: `WHERE $1 = ANY(tags)`. `language`: `WHERE EXISTS (SELECT 1 FROM bin_post_files WHERE post_id = bin_posts.id AND language = $2)`. Sort: `newest` = `ORDER BY created_at DESC`, `oldest` = `ASC`, `recently_active` = `ORDER BY updated_at DESC`.
- `UpdateBinPost(ctx, postID int64, title, description string, tags []string) (*BinPost, error)` — UPDATE title, description, tags, updated_at.
- `DeleteBinPost(ctx, postID int64) error` — DELETE (cascades handle cleanup).
- `GetBinPostAuthorID(ctx, postID int64) (int64, error)` — quick author lookup for permission checks.

Use the `lib/pq` `pq.Array()` function for reading/writing the `TEXT[]` tags column.

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/db/bin_post_repository.go
git commit -m "feat: add bin post CRUD repository methods"
```

---

### Task 9: Bin Repository — Files and Versions

**Files:**
- Create: `internal/db/bin_version_repository.go`

- [ ] **Step 1: Create `internal/db/bin_version_repository.go`** with file and version operations:

File operations on `*Repository`:
- `CreateBinPostFiles(ctx, postID int64, files []BinPostFile) ([]BinPostFile, error)` — batch INSERT, return with IDs.
- `GetBinPostFiles(ctx, postID int64) ([]BinPostFile, error)` — SELECT ordered by position.
- `ReplaceBinPostFiles(ctx, postID int64, files []BinPostFile) ([]BinPostFile, error)` — DELETE existing + INSERT new (used on edit).

Version operations:
- `CreateBinPostVersion(ctx, postID int64, version int, description string) (*BinPostVersion, error)` — INSERT into `bin_post_versions`.
- `CreateBinPostVersionFiles(ctx, versionID int64, files []BinPostFile) error` — INSERT current files as version snapshot into `bin_post_version_files`.
- `GetBinPostVersions(ctx, postID int64) ([]BinPostVersion, error)` — list versions (without files).
- `GetBinPostVersion(ctx, versionID int64) (*BinPostVersion, error)` — single version with files.
- `GetLatestVersionNumber(ctx, postID int64) (int, error)` — `SELECT MAX(version)`.
- `PurgeOldBinPostVersions(ctx) (int64, error)` — DELETE versions older than 90 days (cascades to version files and line comments).

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/db/bin_version_repository.go
git commit -m "feat: add bin file and version repository methods"
```

---

### Task 9b: Bin Repository — Line Comments and Tags

**Files:**
- Create: `internal/db/bin_line_comment_repository.go`
- Create: `internal/db/bin_tag_repository.go`

- [ ] **Step 1: Create `internal/db/bin_line_comment_repository.go`**

- `CreateBinLineComment(ctx, postID, versionID, fileID int64, lineNumber int, authorID int64, content string, parentID *int64) (*BinLineComment, error)` — INSERT with author JOIN for username/avatar.
- `GetBinLineComments(ctx, postID int64, versionID, fileID *int64) ([]BinLineComment, error)` — SELECT with optional version/file filters, JOIN users for author info.
- `UpdateBinLineComment(ctx, commentID int64, content string) (*BinLineComment, error)` — UPDATE content and updated_at.
- `DeleteBinLineComment(ctx, commentID int64) error`
- `GetBinLineCommentAuthorID(ctx, commentID int64) (int64, error)`

- [ ] **Step 2: Create `internal/db/bin_tag_repository.go`**

- `CreateBinChannelTag(ctx, channelID int64, name, color string) (*BinChannelTag, error)` — INSERT.
- `GetBinChannelTags(ctx, channelID int64) ([]BinChannelTag, error)` — SELECT.
- `DeleteBinChannelTag(ctx, tagID int64) error`

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/db/bin_line_comment_repository.go internal/db/bin_tag_repository.go
git commit -m "feat: add bin line comment and tag repository methods"
```

---

### Task 10: Bin WebSocket Events and Service

**Files:**
- Modify: `internal/websocket/events.go` — add bin event constants
- Create: `internal/bin/service.go`

- [ ] **Step 1: Add event constants to `internal/websocket/events.go`**

```go
const (
    // ... existing events
    EventBinPostCreate        = "BIN_POST_CREATE"
    EventBinPostUpdate        = "BIN_POST_UPDATE"
    EventBinPostDelete        = "BIN_POST_DELETE"
    EventBinLineCommentCreate = "BIN_LINE_COMMENT_CREATE"
    EventBinLineCommentUpdate = "BIN_LINE_COMMENT_UPDATE"
    EventBinLineCommentDelete = "BIN_LINE_COMMENT_DELETE"
)
```

- [ ] **Step 2: Create `internal/bin/service.go`**

```go
package bin

import (
    "context"
    "fmt"
    "strconv"
    "sync"

    "parley/internal/db"
    "parley/internal/permissions"
    ws "parley/internal/websocket"
)

type Service struct {
    mu   sync.RWMutex
    repo *db.Repository
    hub  *ws.Hub
}

func NewService(repo *db.Repository) *Service {
    return &Service{repo: repo}
}

func (s *Service) SetHub(hub *ws.Hub) {
    s.mu.Lock()
    s.hub = hub
    s.mu.Unlock()
}
```

Then implement these methods:

- `CreatePost(ctx, channelID, userID string, title, description string, tags []string, files []db.BinPostFile) (*db.BinPost, error)` — validate channel is ChannelTypeBin, parse IDs, call repo.CreateBinPost, create files, create initial version + version files, broadcast `BIN_POST_CREATE`.
- `GetPost(ctx, postID string) (*db.BinPost, error)` — get post with files.
- `ListPosts(ctx, channelID string, tag, language, authorID, sort string, limit, offset int) ([]db.BinPost, error)` — validate channel type, call repo.
- `EditPost(ctx, postID, userID string, title, description string, tags []string, files []db.BinPostFile) (*db.BinPost, error)` — check author/perms, snapshot current files as new version, replace files, update post, broadcast `BIN_POST_UPDATE`.
- `DeletePost(ctx, postID, userID string) error` — check author/perms, delete, broadcast `BIN_POST_DELETE`.
- `GetVersions(ctx, postID string) ([]db.BinPostVersion, error)`
- `GetVersion(ctx, versionID string) (*db.BinPostVersion, error)`
- `CreateLineComment(ctx, postID, userID string, req CreateLineCommentRequest) (*db.BinLineComment, error)` — validate version/file exist, create comment, broadcast `BIN_LINE_COMMENT_CREATE`.
- `GetLineComments(ctx, postID string, versionID, fileID *string) ([]db.BinLineComment, error)`
- `UpdateLineComment(ctx, commentID, userID, content string) (*db.BinLineComment, error)` — check author, update, broadcast.
- `DeleteLineComment(ctx, commentID, userID string) error` — check author/perms, delete, broadcast.
- `CreateTag(ctx, channelID, userID, name, color string) (*db.BinChannelTag, error)` — check manage_channels perm.
- `GetTags(ctx, channelID string) ([]db.BinChannelTag, error)`
- `DeleteTag(ctx, tagID, userID string) error` — check perm.

Helper struct for line comment creation:

```go
type CreateLineCommentRequest struct {
    VersionID  string `json:"version_id"`
    FileID     string `json:"file_id"`
    LineNumber int    `json:"line_number"`
    Content    string `json:"content"`
    ParentID   string `json:"parent_id,omitempty"`
}
```

For broadcasting, use the constants from `internal/websocket/events.go`:

```go
s.mu.RLock()
if s.hub != nil {
    s.hub.BroadcastToChannel(channelID, ws.EventBinPostCreate, post)
}
s.mu.RUnlock()
```

Split service methods across files within the `bin` package:
- `service.go` — struct, constructor, SetHub, post CRUD methods (CreatePost, GetPost, ListPosts, EditPost, DeletePost, GetVersions, GetVersion)
- `line_comments.go` — CreateLineComment, GetLineComments, UpdateLineComment, DeleteLineComment
- `tags.go` — CreateTag, GetTags, DeleteTag

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add internal/websocket/events.go internal/bin/
git commit -m "feat: add bin service with post, version, line comment, and tag operations"
```

---

### Task 11: Bin Handler

**Files:**
- Create: `internal/bin/handler.go`

- [ ] **Step 1: Create `internal/bin/handler.go`**

Implement HTTP handlers following the pattern in `internal/message/handler.go`:

```go
package bin

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"

    "parley/internal/auth"
    "parley/internal/db"
)

type Handler struct {
    service *Service
}

func NewHandler(service *Service) *Handler {
    return &Handler{service: service}
}
```

Then implement these handler methods:

- `CreatePost(w, r)` — parse JSON body `{title, description, tags, files}`, extract userID from context, call service.CreatePost.
- `ListPosts(w, r)` — parse query params (tag, language, author_id, sort, limit, offset), call service.ListPosts.
- `GetPost(w, r)` — parse postID from URL, call service.GetPost.
- `EditPost(w, r)` — parse JSON body, call service.EditPost.
- `DeletePost(w, r)` — call service.DeletePost, return 204.
- `GetVersions(w, r)` — call service.GetVersions.
- `GetVersion(w, r)` — call service.GetVersion.
- `CreateLineComment(w, r)` — parse JSON body, call service.CreateLineComment.
- `GetLineComments(w, r)` — parse query params (version_id, file_id), call service.GetLineComments.
- `UpdateLineComment(w, r)` — parse JSON body `{content}`, call service.UpdateLineComment.
- `DeleteLineComment(w, r)` — call service.DeleteLineComment, return 204.
- `CreateTag(w, r)` — parse JSON body `{name, color}`, call service.CreateTag.
- `GetTags(w, r)` — call service.GetTags.
- `DeleteTag(w, r)` — call service.DeleteTag, return 204.

All handlers extract `userID` via `auth.GetUserIDFromContext(r)` and respond with JSON using `json.NewEncoder(w).Encode()`. Return `400` for validation errors, `403` for permission errors, `404` for not found.

- [ ] **Step 2: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add internal/bin/handler.go
git commit -m "feat: add bin HTTP handlers for posts, versions, line comments, and tags"
```

---

### Task 12: Register Bin Routes and Initialize Service

**Files:**
- Modify: `cmd/api/routes.go` — add bin routes
- Modify: `cmd/api/main.go` — initialize bin service

- [ ] **Step 1: Add bin routes to `cmd/api/routes.go`**

Update the function signature to accept `binService *bin.Service`, then add inside the protected route group:

```go
// Bin channel routes
binHandler := bin.NewHandler(binService)
r.Post("/channels/{channelID}/posts", binHandler.CreatePost)
r.Get("/channels/{channelID}/posts", binHandler.ListPosts)
r.Get("/posts/{postID}", binHandler.GetPost)
r.Put("/posts/{postID}", binHandler.EditPost)
r.Delete("/posts/{postID}", binHandler.DeletePost)
r.Get("/posts/{postID}/versions", binHandler.GetVersions)
r.Get("/posts/{postID}/versions/{versionID}", binHandler.GetVersion)
r.Post("/posts/{postID}/line-comments", binHandler.CreateLineComment)
r.Get("/posts/{postID}/line-comments", binHandler.GetLineComments)
r.Put("/line-comments/{id}", binHandler.UpdateLineComment)
r.Delete("/line-comments/{id}", binHandler.DeleteLineComment)
r.Get("/channels/{channelID}/tags", binHandler.GetTags)
r.Post("/channels/{channelID}/tags", binHandler.CreateTag)
r.Delete("/channels/{channelID}/tags/{tagID}", binHandler.DeleteTag)
```

- [ ] **Step 2: Initialize bin service in `cmd/api/main.go`**

After the existing service initializations, add:

```go
binService := bin.NewService(repo)
binService.SetHub(hub)
```

Update the `registerRoutes` call to pass `binService`.

Add the bin version purge to the existing purge goroutine:

```go
if n, err := repo.PurgeOldBinPostVersions(context.Background()); err != nil {
    log.Printf("bin version purge error: %v", err)
} else if n > 0 {
    log.Printf("purged %d old bin post versions", n)
}
```

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/api && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add cmd/api/routes.go cmd/api/main.go
git commit -m "feat: register bin routes and initialize bin service"
```

---

## Chunk 3: Frontend — Shiki, CodeBlock, Nested Replies, Edit History

### Task 13: Install Shiki and Create CodeBlock Component

**Files:**
- Create: `frontend/src/lib/shiki.ts` — Shiki singleton with lazy loading
- Create: `frontend/src/components/ui/CodeBlock.tsx` — shared code display component
- Create: `frontend/src/components/ui/CodeBlock.css`

- [ ] **Step 1: Install Shiki**

Run: `cd frontend && npm install shiki`

- [ ] **Step 2: Create `frontend/src/lib/shiki.ts`**

```typescript
import { createHighlighter, type Highlighter } from 'shiki';

let highlighterPromise: Promise<Highlighter> | null = null;
const loadedLanguages = new Set<string>();

// Custom Parley terminal theme
const parleyTheme = {
  name: 'parley',
  type: 'dark' as const,
  colors: {
    'editor.background': '#0a0a0a',
    'editor.foreground': '#e0e0e0',
  },
  tokenColors: [
    { scope: ['keyword', 'storage.type', 'storage.modifier'], settings: { foreground: '#32CD32' } },
    { scope: ['string', 'string.quoted'], settings: { foreground: '#228B22' } },
    { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: '#555555' } },
    { scope: ['constant.numeric', 'constant.language'], settings: { foreground: '#66ff66' } },
    { scope: ['entity.name.type', 'support.type'], settings: { foreground: '#44aa44' } },
    { scope: ['entity.name.function', 'support.function'], settings: { foreground: '#ffffff' } },
    { scope: ['variable', 'variable.other'], settings: { foreground: '#e0e0e0' } },
    { scope: ['punctuation'], settings: { foreground: '#888888' } },
    { scope: ['entity.name.tag'], settings: { foreground: '#32CD32' } },
    { scope: ['entity.other.attribute-name'], settings: { foreground: '#44aa44' } },
  ],
};

const DEFAULT_LANG = 'plaintext';

export async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: [parleyTheme],
      langs: [DEFAULT_LANG],
    });
  }
  return highlighterPromise;
}

export async function highlight(code: string, lang: string): Promise<string> {
  const hl = await getHighlighter();
  const language = lang || DEFAULT_LANG;

  if (!loadedLanguages.has(language) && language !== DEFAULT_LANG) {
    try {
      await hl.loadLanguage(language as any);
      loadedLanguages.add(language);
    } catch {
      // Fall back to plaintext if language not supported
      return hl.codeToHtml(code, { lang: DEFAULT_LANG, theme: 'parley' });
    }
  }

  return hl.codeToHtml(code, { lang: language, theme: 'parley' });
}

// Map file extensions to Shiki language IDs
const EXT_MAP: Record<string, string> = {
  '.py': 'python', '.go': 'go', '.rs': 'rust',
  '.js': 'javascript', '.jsx': 'jsx', '.ts': 'typescript', '.tsx': 'tsx',
  '.sh': 'bash', '.bash': 'bash', '.zsh': 'bash',
  '.ps1': 'powershell', '.lua': 'lua',
  '.c': 'c', '.h': 'c', '.cpp': 'cpp', '.hpp': 'cpp',
  '.yaml': 'yaml', '.yml': 'yaml', '.json': 'json', '.toml': 'toml',
  '.rb': 'ruby', '.java': 'java',
  '.asm': 'asm', '.s': 'asm',
  '.html': 'html', '.css': 'css', '.scss': 'scss',
  '.sql': 'sql', '.md': 'markdown',
  '.dockerfile': 'dockerfile', '.tf': 'hcl',
  '.xml': 'xml', '.csv': 'csv',
};

export function languageFromFilename(filename: string): string {
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  return EXT_MAP[ext] || '';
}

export function isCodeFile(filename: string): boolean {
  const ext = '.' + filename.split('.').pop()?.toLowerCase();
  return ext in EXT_MAP;
}
```

- [ ] **Step 3: Create `frontend/src/components/ui/CodeBlock.css`**

```css
.code-block {
  position: relative;
  background: #0a0a0a;
  border: 1px solid rgba(50, 205, 50, 0.15);
  border-radius: 6px;
  overflow: hidden;
  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 13px;
  line-height: 1.5;
}

.code-block-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  background: #111111;
  border-bottom: 1px solid rgba(50, 205, 50, 0.1);
  font-size: 12px;
  color: #888;
}

.code-block-filename {
  color: #32CD32;
  font-weight: 500;
}

.code-block-language {
  color: #555;
  text-transform: uppercase;
  font-size: 10px;
  letter-spacing: 0.5px;
}

.code-block-body {
  overflow-x: auto;
  padding: 12px 0;
}

.code-block-body pre {
  margin: 0;
  padding: 0 16px;
}

.code-block-body code {
  font-family: inherit;
}

/* Line numbers */
.code-block-body .line {
  display: inline;
}

.code-block-line {
  display: flex;
  padding: 0 16px 0 0;
}

.code-block-line:hover {
  background: rgba(50, 205, 50, 0.05);
}

.code-block-line-number {
  flex-shrink: 0;
  width: 50px;
  padding-right: 16px;
  text-align: right;
  color: #444;
  user-select: none;
  cursor: pointer;
}

.code-block-line-number:hover {
  color: #32CD32;
}

.code-block-line-content {
  flex: 1;
  white-space: pre;
}

.code-block-line.highlighted {
  background: rgba(50, 205, 50, 0.1);
}

/* Collapsible variant for message attachments */
.code-block-collapsible .code-block-header {
  cursor: pointer;
}

.code-block-collapsible .code-block-header:hover {
  background: #1a1a1a;
}

.code-block-collapsed .code-block-body {
  display: none;
}

.code-block-toggle {
  color: #555;
  margin-right: 8px;
  transition: transform 0.15s;
}

.code-block-collapsed .code-block-toggle {
  transform: rotate(-90deg);
}
```

- [ ] **Step 4: Create `frontend/src/components/ui/CodeBlock.tsx`**

Build a React component that:
- Takes props: `content`, `language`, `filename`, `showLineNumbers`, `collapsible`, `defaultCollapsed`, `highlightedLines`, `onLineClick`
- Uses the `highlight()` function from `lib/shiki.ts` to get syntax-highlighted HTML
- Renders with line numbers when `showLineNumbers` is true — splits content by newlines, renders each line with a clickable line number gutter
- For the highlighted code rendering, use Shiki's output which is already sanitized (Shiki generates its own HTML from parsed tokens, it does not pass through user HTML). Render via React's `dangerouslySetInnerHTML` only for Shiki's sanitized output
- Supports collapsible mode for message attachments
- Shows a loading state (plain text) while Shiki loads

- [ ] **Step 5: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 6: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/src/lib/shiki.ts frontend/src/components/ui/CodeBlock.tsx frontend/src/components/ui/CodeBlock.css
git commit -m "feat: add Shiki syntax highlighting with CodeBlock component and custom Parley theme"
```

---

### Task 14: Syntax-Highlighted Message Attachments

**Files:**
- Modify: `frontend/src/components/chat/Message.tsx` — render code attachments with CodeBlock

- [ ] **Step 1: Modify `frontend/src/components/chat/Message.tsx`**

Import `CodeBlock` and `isCodeFile` at the top:

```tsx
import CodeBlock from '../ui/CodeBlock';
import { isCodeFile, languageFromFilename } from '../../lib/shiki';
```

Find the attachment rendering section. When `attachment_name` is present and `isCodeFile(attachment_name)` is true, render a `CodeAttachment` component instead of the default file download link.

Create a `CodeAttachment` component within the same file that:
- Fetches the file content from `attachment_url` via `fetch().then(res => res.text())`
- Checks content-length header — if over 100KB, falls back to download link with a "Preview" button
- Renders the fetched content with `CodeBlock` in collapsible mode (collapsed by default if over 50 lines)
- Shows a download link fallback on fetch error

- [ ] **Step 2: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/chat/Message.tsx
git commit -m "feat: render code file attachments with syntax highlighting"
```

---

### Task 15: Shiki for Inline Markdown Code Blocks

**Files:**
- Create: `frontend/src/components/ui/ShikiCodeBlock.tsx` — custom renderer for react-markdown

- [ ] **Step 1: Create `frontend/src/components/ui/ShikiCodeBlock.tsx`**

A custom `code` component for `react-markdown` that:
- Extracts language from `className` (e.g., `"language-python"`)
- For fenced code blocks (not inline): calls `highlight()` from `lib/shiki.ts` and renders the result. Shiki produces sanitized HTML from parsed tokens (not user HTML), so rendering its output is safe
- For inline code or blocks without a language: renders a plain `<code>` element

- [ ] **Step 2: Integrate with react-markdown in Message.tsx**

Find where `react-markdown` is used to render message content. Add the custom `code` component:

```tsx
import ShikiCodeBlock from '../ui/ShikiCodeBlock';

// In the ReactMarkdown usage:
<ReactMarkdown
  remarkPlugins={[remarkGfm]}
  components={{
    code: ShikiCodeBlock,
    // ...existing components
  }}
>
  {message.content}
</ReactMarkdown>
```

- [ ] **Step 3: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/ui/ShikiCodeBlock.tsx frontend/src/components/chat/Message.tsx
git commit -m "feat: use Shiki for syntax highlighting in markdown code blocks"
```

---

### Task 16: Nested Replies — Frontend

**Files:**
- Modify: `frontend/src/api/types.ts` — add `parent_id` to Message
- Create: `frontend/src/components/chat/NestedReplies.tsx`
- Create: `frontend/src/components/chat/NestedReplies.css`
- Modify: `frontend/src/components/chat/Message.tsx` — show "replying to @user" indicator
- Modify: `frontend/src/components/chat/MessageInput.tsx` — support parent_id when replying

- [ ] **Step 1: Update `frontend/src/api/types.ts`**

Add to the `Message` interface:

```typescript
parent_id?: string;
```

- [ ] **Step 2: Create `frontend/src/components/chat/NestedReplies.css`**

```css
.nested-replies {
  display: flex;
  flex-direction: column;
}

.reply-thread {
  margin-left: 48px;
  border-left: 2px solid rgba(50, 205, 50, 0.15);
  padding-left: 12px;
}

.reply-indicator {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #555;
  margin-bottom: 2px;
  padding-left: 56px;
}

.reply-indicator-name {
  color: #32CD32;
  cursor: pointer;
}

.reply-indicator-name:hover {
  text-decoration: underline;
}
```

- [ ] **Step 3: Create `frontend/src/components/chat/NestedReplies.tsx`**

Utility functions for building reply trees:

- `buildReplyTree(messages)` — takes flat message array, returns tree of `{ message, replies[] }`. One level of nesting only — replies to replies attach to the grandparent.
- `getParentAuthor(parentId, messages)` — finds the display name of the parent message author.

- [ ] **Step 4: Modify `Message.tsx` — add reply indicator**

In the message header area, before the author name, add a reply indicator when `parent_id` is set:

```tsx
{message.parent_id && parentAuthorName && (
  <div className="reply-indicator">
    <span>↳ replying to </span>
    <span className="reply-indicator-name">{parentAuthorName}</span>
  </div>
)}
```

- [ ] **Step 5: Modify `MessageInput.tsx` — send parent_id**

When the `replyTo` state is set, include `parent_id` in the message POST body. The existing `replyTo` / `onClearReply` infrastructure should be wired up to set this value.

- [ ] **Step 6: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 7: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/components/chat/NestedReplies.tsx frontend/src/components/chat/NestedReplies.css frontend/src/components/chat/Message.tsx frontend/src/components/chat/MessageInput.tsx
git commit -m "feat: add nested replies UI with reply indicator and tree building"
```

---

### Task 17: Edit History — Frontend

**Files:**
- Modify: `frontend/src/api/messages.ts` (or create if needed) — add `getMessageVersions` API call
- Create: `frontend/src/components/chat/EditHistoryPopover.tsx`
- Create: `frontend/src/components/chat/EditHistoryPopover.css`
- Modify: `frontend/src/components/chat/Message.tsx` — make "(edited)" clickable

- [ ] **Step 1: Add API call for message versions**

Find or create the messages API module. Add:

```typescript
export interface MessageVersion {
  id: string;
  message_id: string;
  content: string;
  edited_at: string;
}

export async function getMessageVersions(messageId: string): Promise<MessageVersion[]> {
  const res = await fetch(`/api/messages/${messageId}/versions`, {
    headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
  });
  if (!res.ok) throw new Error('Failed to fetch versions');
  return res.json();
}
```

- [ ] **Step 2: Create `frontend/src/components/chat/EditHistoryPopover.css`**

```css
.edit-history-popover {
  position: absolute;
  bottom: 100%;
  left: 0;
  background: #111;
  border: 1px solid rgba(50, 205, 50, 0.2);
  border-radius: 6px;
  padding: 8px;
  min-width: 300px;
  max-width: 500px;
  max-height: 400px;
  overflow-y: auto;
  z-index: 100;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
}

.edit-history-title {
  font-size: 12px;
  font-weight: 600;
  color: #32CD32;
  margin-bottom: 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid rgba(50, 205, 50, 0.1);
}

.edit-history-version {
  padding: 6px 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.edit-history-version:last-child {
  border-bottom: none;
}

.edit-history-timestamp {
  font-size: 11px;
  color: #555;
  margin-bottom: 4px;
}

.edit-history-content {
  font-size: 13px;
  color: #ccc;
  white-space: pre-wrap;
  word-break: break-word;
}
```

- [ ] **Step 3: Create `frontend/src/components/chat/EditHistoryPopover.tsx`**

A popover component that:
- Fetches versions via `getMessageVersions(messageId)` on mount
- Renders a list of previous versions with timestamps
- Closes when clicking outside (via `mousedown` event listener on document)
- Shows nothing if no versions exist

- [ ] **Step 4: Modify `Message.tsx` — make "(edited)" clickable**

Add state `showEditHistory` and toggle it on click of the "(edited)" span. Render `EditHistoryPopover` when active, positioned above the edited indicator.

- [ ] **Step 5: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/messages.ts frontend/src/components/chat/EditHistoryPopover.tsx frontend/src/components/chat/EditHistoryPopover.css frontend/src/components/chat/Message.tsx
git commit -m "feat: add clickable (edited) indicator with version history popover"
```

---

## Chunk 4: Frontend — Bin Channel Components

### Task 18: Bin API Client and Types

**Files:**
- Modify: `frontend/src/api/types.ts` — add bin types
- Create: `frontend/src/api/bin.ts` — API client for bin endpoints

- [ ] **Step 1: Add bin types to `frontend/src/api/types.ts`**

```typescript
export interface BinPost {
  id: string;
  channel_id: string;
  thread_channel_id: string;
  author_id: string;
  title: string;
  description: string;
  tags: string[];
  created_at: string;
  updated_at: string;
  author_username: string;
  author_avatar_url?: string;
  files: BinPostFile[];
  comment_count: number;
  line_comment_count: number;
  version_count: number;
}

export interface BinPostFile {
  id: string;
  post_id: string;
  filename: string;
  language: string;
  content: string;
  position: number;
}

export interface BinPostVersion {
  id: string;
  post_id: string;
  version: number;
  description: string;
  created_at: string;
  files?: BinPostVersionFile[];
}

export interface BinPostVersionFile {
  id: string;
  version_id: string;
  filename: string;
  language: string;
  content: string;
  position: number;
}

export interface BinLineComment {
  id: string;
  post_id: string;
  version_id: string;
  file_id: string;
  line_number: number;
  author_id: string;
  content: string;
  parent_id?: string;
  created_at: string;
  updated_at: string;
  author_username: string;
  author_avatar_url?: string;
}

export interface BinChannelTag {
  id: string;
  channel_id: string;
  name: string;
  color: string;
}
```

- [ ] **Step 2: Create `frontend/src/api/bin.ts`**

API client module with functions for all bin endpoints:
- `createPost(channelId, data)` — POST `/api/channels/{channelId}/posts`
- `listPosts(channelId, params?)` — GET with query params (tag, language, author_id, sort, limit, offset)
- `getPost(postId)` — GET `/api/posts/{postId}`
- `editPost(postId, data)` — PUT `/api/posts/{postId}`
- `deletePost(postId)` — DELETE `/api/posts/{postId}`
- `getVersions(postId)` — GET `/api/posts/{postId}/versions`
- `getVersion(postId, versionId)` — GET `/api/posts/{postId}/versions/{versionId}`
- `createLineComment(postId, data)` — POST `/api/posts/{postId}/line-comments`
- `getLineComments(postId, params?)` — GET with query params (version_id, file_id)
- `updateLineComment(id, content)` — PUT `/api/line-comments/{id}`
- `deleteLineComment(id)` — DELETE `/api/line-comments/{id}`
- `getTags(channelId)` — GET `/api/channels/{channelId}/tags`
- `createTag(channelId, name, color)` — POST `/api/channels/{channelId}/tags`
- `deleteTag(channelId, tagId)` — DELETE `/api/channels/{channelId}/tags/{tagId}`

All functions use `Authorization: Bearer` header from localStorage and return typed responses.

- [ ] **Step 3: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/bin.ts
git commit -m "feat: add bin channel API client and TypeScript types"
```

---

### Task 19: Channel List — Bin Channel Support

**Files:**
- Modify: `frontend/src/components/layout/ChannelList.tsx` — add bin channel section
- Modify: `frontend/src/components/layout/ChannelList.css` — style bin icon
- Modify: `frontend/src/components/modals/CreateChannelModal.tsx` — add bin type option

- [ ] **Step 1: Modify `ChannelList.tsx`**

Add a filtered list for bin channels:

```tsx
const binChannels = channels.filter(ch => ch.type === 2);
```

Render a "BIN CHANNELS" section (following the pattern of text/voice sections) with a `</>` icon prefix for each channel.

- [ ] **Step 2: Modify `CreateChannelModal.tsx`**

Add a third type option in the type selector:

```tsx
<div
  className={`channel-type-option ${type === 2 ? 'selected' : ''}`}
  onClick={() => setType(2)}
>
  <span className="channel-type-icon">&lt;/&gt;</span>
  <div>
    <div className="channel-type-name">Bin</div>
    <div className="channel-type-desc">Code sharing & discussion</div>
  </div>
</div>
```

When type is 2, hide the topic field (bins don't use it).

- [ ] **Step 3: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/ChannelList.tsx frontend/src/components/layout/ChannelList.css frontend/src/components/modals/CreateChannelModal.tsx
git commit -m "feat: add bin channel type to channel list and creation modal"
```

---

### Task 20: BinChannel — Post List View

**Files:**
- Create: `frontend/src/components/bin/BinChannel.tsx`
- Create: `frontend/src/components/bin/BinChannel.css`
- Create: `frontend/src/components/bin/PostListItem.tsx`

- [ ] **Step 1: Create `frontend/src/components/bin/BinChannel.css`**

Styles for the post list view: filter bar with tag pills and sort dropdown, post list as cards, "New Post" button. Follow the existing green terminal theme — dark backgrounds, lime green accents, same font choices.

Key classes: `.bin-channel`, `.bin-filter-bar`, `.bin-tag-pill`, `.bin-sort-select`, `.bin-new-post-btn`, `.bin-post-list`.

- [ ] **Step 2: Create `frontend/src/components/bin/PostListItem.tsx`**

A card component for each post in the list showing: title, timestamp, author avatar + name, tag pills, file count, comment count, line note count.

- [ ] **Step 3: Create `frontend/src/components/bin/BinChannel.tsx`**

The main bin channel component that:
- Fetches posts via `listPosts()` and tags via `getTags()` on mount and when filters change
- Renders a filter bar with tag pills (clickable to toggle), sort dropdown, and "New Post" button
- Renders the post list using `PostListItem` components
- Handles empty state and loading state
- Calls `onOpenPost(postId)` when a post is clicked
- Calls `onNewPost()` when the new post button is clicked

- [ ] **Step 4: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/bin/
git commit -m "feat: add BinChannel post list view with filtering and sorting"
```

---

### Task 21: CreatePostModal

**Files:**
- Create: `frontend/src/components/bin/CreatePostModal.tsx`
- Create: `frontend/src/components/bin/CreatePostModal.css`

- [ ] **Step 1: Create `frontend/src/components/bin/CreatePostModal.css`**

Styles for the post creation modal: title input, description textarea, file editor section (add/remove files, filename + language inputs, code textarea per file), tag selector, preview area. Match existing modal styles from `frontend/src/components/ui/styles.css`.

- [ ] **Step 2: Create `frontend/src/components/bin/CreatePostModal.tsx`**

A modal component with:
- `title` text input (required)
- `description` textarea (optional markdown, placeholder mentions ``` support)
- `files` array — each entry has filename input, language input (auto-detects from filename via `languageFromFilename()`), code textarea. "Add File" button, remove button per file (if >1)
- `tags` — shows admin-defined tags as clickable pills, freeform input + Enter to add custom tags, selected tags shown as removable pills
- Submit calls `createPost()` API, validates title is non-empty
- Error and loading states

- [ ] **Step 3: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/bin/CreatePostModal.tsx frontend/src/components/bin/CreatePostModal.css
git commit -m "feat: add CreatePostModal with multi-file support and tag selection"
```

---

### Task 22: PostView — Main Post Display

**Files:**
- Create: `frontend/src/components/bin/PostView.tsx`
- Create: `frontend/src/components/bin/PostView.css`

- [ ] **Step 1: Create `frontend/src/components/bin/PostView.css`**

Styles for: post header (title, author, tags, version dropdown), tab bar (Files / Comments / Line Notes), file display with CodeBlock, comment section, line comment section. Dark theme, green accents, consistent with rest of app.

- [ ] **Step 2: Create `frontend/src/components/bin/PostView.tsx`**

The full post view with:
- **Header**: back button, title, author, tags, version dropdown (if >1 version), version banner when viewing old version
- **Description**: rendered with ReactMarkdown + ShikiCodeBlock for inline code blocks
- **Three tabs**:
  - **Files**: file tab bar (if multiple files), active file rendered with `CodeBlock` (line numbers, clickable gutter for line comments)
  - **Comments**: placeholder for thread channel messages (wired in Task 23)
  - **Line Notes**: list of line comments grouped by file, with "Line N" badges
- Fetches post, versions, and line comments on mount
- Version switching: loads version files from API, shows read-only banner

- [ ] **Step 3: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/bin/PostView.tsx frontend/src/components/bin/PostView.css
git commit -m "feat: add PostView with files, comments, line notes tabs and version switching"
```

---

### Task 23: Wire Up Line Comments and Thread Comments

**Files:**
- Create: `frontend/src/components/bin/LineCommentForm.tsx`
- Modify: `frontend/src/components/bin/PostView.tsx` — wire line comment form and thread channel messages

- [ ] **Step 1: Create `frontend/src/components/bin/LineCommentForm.tsx`**

Inline form that appears when clicking a line number:
- Shows "Comment on line N" header
- Textarea for content
- Cancel and Submit buttons
- Calls `createLineComment()` API on submit

- [ ] **Step 2: Wire line comment form into PostView**

Add state for `commentingOnLine` (number | null) and the current version/file IDs. When `onLineClick` fires on the CodeBlock, set the line state to show `LineCommentForm` below the clicked line. On `onCreated`, refresh line comments and clear the form.

- [ ] **Step 3: Wire thread channel comments into PostView**

In the Comments tab, render the existing `MessageList` and `MessageInput` components, passing `post.thread_channel_id` as the channel ID. Subscribe to the thread channel via WebSocket when the tab is active.

This reuses all existing message infrastructure — reactions, attachments, nested replies all work for free.

- [ ] **Step 4: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/bin/LineCommentForm.tsx frontend/src/components/bin/PostView.tsx
git commit -m "feat: wire line comment form and thread channel comments in PostView"
```

---

### Task 24: Route Bin Channels in App

**Files:**
- Modify: `frontend/src/App.tsx` or the main channel rendering component — render BinChannel for type 2

- [ ] **Step 1: Find where `ChatWindow` is rendered based on channel type**

In the component that renders the main content area (likely where `channelId` is used to decide what to show), add a condition:

```tsx
if (activeChannel?.type === 2) {
  if (activePostId) {
    return <PostView postId={activePostId} onBack={() => setActivePostId(null)} />;
  }
  return (
    <BinChannel
      channelId={channelId}
      onOpenPost={setActivePostId}
      onNewPost={() => setShowCreatePost(true)}
    />
  );
}
```

Add state for `activePostId` to toggle between post list and single post view.

- [ ] **Step 2: Add CreatePostModal trigger**

Add state `showCreatePost` and render `CreatePostModal` when true. On `onCreated`, navigate to the new post.

- [ ] **Step 3: Handle WebSocket events**

Listen for `BIN_POST_CREATE`, `BIN_POST_UPDATE`, `BIN_POST_DELETE` events to update the post list in real time. Add these event types to the WebSocket event handler.

- [ ] **Step 4: Build and verify**

Run: `cd frontend && npx tsc --noEmit && echo "Build OK"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/components/bin/
git commit -m "feat: route bin channels to BinChannel/PostView and handle WebSocket events"
```

---

### Task 25: Final Integration and Verification

- [ ] **Step 1: Full backend build**

Run: `go build ./cmd/api && echo "Backend build OK"`

- [ ] **Step 2: Full frontend build**

Run: `cd frontend && npx tsc --noEmit && npm run build && echo "Frontend build OK"`

- [ ] **Step 5: Manual smoke test**

1. Start the server
2. Create a bin channel in a server
3. Create a post with title, description with ```python code block, and an attached .py file
4. Verify syntax highlighting renders correctly
5. Add a comment on the post
6. Add a line comment on the code
7. Edit the post, verify version history
8. Edit a regular message in a text channel, click "(edited)", verify version popover
9. Reply to a message, verify nested reply indicator shows
