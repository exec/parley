# Bin Channels Design Spec

## Overview

Bin channels (`ChannelType = 2`) are forum-style channels with first-class code support. They combine Discord's forum functionality with pastebin capabilities — posts can be pure discussion (CTF writeups, questions), pure code, or a mix.

This feature also introduces three platform-wide capabilities:
- **Facebook-style nested replies** on messages (one level deep)
- **Edit history** for messages (90-day retention)
- **Syntax-highlighted source code attachments** everywhere via Shiki

## Data Model

### New Tables

#### `bin_posts`

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | Uses `gen_bin_post_id()` (9-digit, created in migration following the `gen_server_id()` pattern) |
| channel_id | BIGINT FK → channels | The bin container channel |
| thread_channel_id | BIGINT FK → channels | Dedicated thread channel for general comments |
| author_id | BIGINT FK → users | |
| title | VARCHAR(200) | Required |
| description | TEXT | Optional markdown body. Supports triple-backtick code blocks with language tags (e.g. \`\`\`python) rendered with Shiki, same as messages. |
| tags | TEXT[] | Postgres array, mix of admin-defined + freeform |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

#### `bin_post_files`

Current version of files attached to a post.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| post_id | BIGINT FK → bin_posts | |
| filename | VARCHAR(255) | e.g. `exploit.py`, `config.yaml` |
| language | VARCHAR(50) | Shiki language ID, nullable for auto-detect |
| content | TEXT | The actual code |
| position | INT | Ordering within the post, UNIQUE(post_id, position) |

#### `bin_post_versions`

A version is created on post creation (version 1) and on each subsequent edit (version 2, 3, ...). The initial version snapshot is needed so that line comments always have a version to anchor to.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| post_id | BIGINT FK → bin_posts | |
| version | INT | Sequential: 1 (initial), 2, 3... |
| description | TEXT | Description at time of snapshot |
| created_at | TIMESTAMP | When this version was saved |

#### `bin_post_version_files`

File snapshots per version.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| version_id | BIGINT FK → bin_post_versions | |
| filename | VARCHAR(255) | |
| language | VARCHAR(50) | |
| content | TEXT | |
| position | INT | |

#### `bin_line_comments`

Comments anchored to a specific line in a specific version/file.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| post_id | BIGINT FK → bin_posts | |
| version_id | BIGINT FK → bin_post_versions | |
| file_id | BIGINT FK → bin_post_version_files | Which file |
| line_number | INT | |
| author_id | BIGINT FK → users | |
| content | TEXT | |
| parent_id | BIGINT FK → bin_line_comments (nullable) | Nested replies (one level) |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

#### `bin_channel_tags`

Admin-defined tags per bin channel.

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| channel_id | BIGINT FK → channels | |
| name | VARCHAR(50) | UNIQUE(channel_id, name) |
| color | VARCHAR(7) | Hex color |

### Modifications to Existing Tables

#### `messages` — add `parent_id`

| Column | Type | Notes |
|--------|------|-------|
| parent_id | BIGINT FK → messages (nullable) | NULL = top-level, set = reply |

Enables Facebook-style nested replies platform-wide. One level of nesting only — replies to replies are treated as replies to the parent.

#### `message_versions` — new table

| Column | Type | Notes |
|--------|------|-------|
| id | BIGSERIAL PK | |
| message_id | BIGINT FK → messages | |
| content | TEXT | Previous content |
| edited_at | TIMESTAMP | When this version was superseded |

Previous content is saved here before each edit. Versions older than 90 days are periodically purged.

#### `channels` — no schema change needed

The `channel_type` column already supports arbitrary integer values. Add `ChannelTypeBin = 2` constant to `internal/db/models.go` alongside the existing `ChannelTypeText` and `ChannelTypeVoice` constants.

### Indexes

Key indexes beyond PKs and FKs:

- `bin_posts`: `(channel_id, created_at DESC)` — post listing
- `bin_post_files`: `(post_id, position)` — file ordering
- `bin_line_comments`: `(version_id, file_id, line_number)` — line comment queries
- `bin_channel_tags`: unique on `(channel_id, name)`
- `message_versions`: `(message_id, edited_at)` — version history lookup
- `messages`: index on `parent_id WHERE parent_id IS NOT NULL` — reply tree queries

## API Endpoints

### Bin Post CRUD

```
POST   /api/channels/{channelID}/posts          — Create a bin post
GET    /api/channels/{channelID}/posts           — List posts (paginated, filterable)
GET    /api/posts/{postID}                       — Get a single post with current files
PUT    /api/posts/{postID}                       — Edit post (creates new version)
DELETE /api/posts/{postID}                       — Delete post
```

### Post Versions

```
GET    /api/posts/{postID}/versions              — List version history
GET    /api/posts/{postID}/versions/{versionID}  — Get a specific version with its files
```

### Line Comments

```
GET    /api/posts/{postID}/line-comments?version_id=X&file_id=Y  — Get line comments for a file version
POST   /api/posts/{postID}/line-comments         — Create line comment
PUT    /api/line-comments/{id}                   — Edit line comment
DELETE /api/line-comments/{id}                   — Delete line comment
```

Line comment creation body: `{ version_id, file_id, line_number, content, parent_id? }`

### General Comments

Each bin post gets a dedicated thread channel. General comments use the existing message endpoints unchanged:

```
GET    /api/channels/{threadChannelID}/messages  — existing
POST   /api/channels/{threadChannelID}/messages  — existing
```

The `parent_id` field on messages enables nested replies within these threads.

### Bin Channel Tags

```
GET    /api/channels/{channelID}/tags            — List available tags
POST   /api/channels/{channelID}/tags            — Create tag (manage_channels perm)
DELETE /api/channels/{channelID}/tags/{tagID}    — Delete tag
```

### Platform-wide Addition

```
GET    /api/messages/{id}/versions               — Get edit history for any message
```

### Error Handling

All bin post endpoints validate that the target channel is `ChannelType = 2`. Requests against non-bin channels return `400 Bad Request` with `"channel is not a bin channel"`. Post mutations (edit, delete) check that the requesting user is the post author or has `manage_messages` permission. Tag CRUD requires `manage_channels` permission. All new tables use `ON DELETE CASCADE` on their foreign keys — deleting a post cascades to its files, versions, version files, line comments, and thread channel.

### Post Listing Query Parameters

`GET /api/channels/{channelID}/posts` supports:
- `?tag=malware` — filter by tag
- `?language=python` — filter by language (any file in the post)
- `?author_id=123` — filter by author
- `?sort=newest|oldest|recently_active` — sort order
- `?limit=25&offset=0` — pagination (max 50)

## WebSocket Events

### New Events

| Event | Payload | When |
|-------|---------|------|
| `BIN_POST_CREATE` | Full post object with files | New post created |
| `BIN_POST_UPDATE` | Updated post + new version number | Post edited |
| `BIN_POST_DELETE` | `{ post_id, channel_id }` | Post deleted |
| `BIN_LINE_COMMENT_CREATE` | Line comment object | New line comment |
| `BIN_LINE_COMMENT_UPDATE` | Updated line comment | Line comment edited |
| `BIN_LINE_COMMENT_DELETE` | `{ id, post_id }` | Line comment deleted |

### Events That Work for Free

- `MESSAGE_CREATE` / `MESSAGE_UPDATE` / `MESSAGE_DELETE` — general comments (they're messages in the thread channel)
- `REACTION_UPDATE` — reactions on comments
- `CHANNEL_CREATE` — when the thread channel is created for a new post

### Subscription Model

Users subscribe to the bin channel ID to receive post-level events (`BIN_POST_CREATE/UPDATE/DELETE`). When viewing a specific post, they additionally subscribe to the post's thread channel for general comment events (`MESSAGE_CREATE` etc.). Line comment events are broadcast to all subscribers of the post's thread channel — since only users viewing that post are subscribed, this avoids noise. The existing `subscribe`/`unsubscribe` WebSocket messages are sufficient; no new subscription types needed.

## Frontend Architecture

### Shared Components

#### `CodeBlock`

Shiki-powered syntax-highlighted code display. Used in bin post view, message attachments with code file extensions, and inline markdown code blocks.

Props: `content`, `language`, `filename`, optional `onLineClick` for comment gutter.

Lazy-loads Shiki core (~15KB gzipped) and language grammars on demand.

#### `NestedReplies`

Facebook-style reply rendering. Takes a flat list of messages, builds a tree from `parent_id`. One level of nesting only — replies to replies show as replies to the parent. Includes inline reply input.

Used in bin post general comments and available for future use in regular channels.

### Bin-Specific Components

#### `BinChannel`

Replaces `ChatWindow` when `channel_type === 2`. Shows post list with title, author, tags, file count, comment count, timestamp. Includes filter bar (tag pills, language dropdown, sort selector) and "New Post" button.

#### `CreatePostModal`

Post creation form: title, description (markdown with Shiki-highlighted code blocks, optional), file editor (add multiple files with filename, language selector, code textarea), tag selector (admin tags as pills + freeform input), preview toggle.

#### `PostView`

Single post display with header (title, author, tags, version dropdown) and tabbed content:
- **Files tab** — each file rendered with `CodeBlock`, tab bar for multiple files, line number gutter for clicking to add line comments
- **Comments tab** — general discussion using `NestedReplies` over the thread channel's messages
- **Line Notes tab** — grouped by file, line-anchored comments with "Line 42" badges, clicking jumps to the file and highlights the line

#### `PostListItem`

Card in the bin channel list: title, tag pills, author avatar + name, file count, comment count, time ago, language indicators.

### Changes to Existing Components

#### `ChannelList.tsx`

Add bin channel section with `</>` icon. Filter: `channels.filter(ch => ch.type === 2)`.

#### `CreateChannelModal.tsx`

Add third radio option: "Bin" with description "Code sharing & discussion".

#### `Message.tsx`

- Render `parent_id` replies inline ("replying to @user" link above message)
- "(edited)" click handler opens version history popover

#### `MessageInput.tsx`

Add `parent_id` support. When replying, show reply bar above input (partial infrastructure already exists via `replyTo` prop).

#### Attachment Rendering (platform-wide)

When a message attachment has a known code extension (`.py`, `.go`, `.js`, `.sh`, `.yaml`, `.rs`, `.c`, `.toml`, `.ps1`, `.lua`, `.rb`, `.java`, `.asm`, etc.), render with `CodeBlock` instead of a generic download link. Fetch file content and display with syntax highlighting. Files over 100KB fall back to download with a "Preview" button.

## Syntax Highlighting

### Shiki Configuration

Singleton highlighter instance at `frontend/src/lib/shiki.ts`. Lazy-loads language grammars on demand — only languages visible on screen are fetched.

### Custom Theme

Matches the Parley green terminal aesthetic:

| Token | Color |
|-------|-------|
| Background | `#0a0a0a` |
| Default text | `#e0e0e0` |
| Keywords | `#32CD32` (lime green) |
| Strings | `#228B22` (dark green) |
| Comments | `#555555` |
| Numbers/constants | `#66ff66` |
| Types | `#44aa44` |
| Functions | `#ffffff` |

### Extension → Language Mapping

```
.py → python       .go → go            .rs → rust
.js → javascript   .ts → typescript    .sh/.bash → bash
.ps1 → powershell  .lua → lua          .c/.h → c
.yaml/.yml → yaml  .json → json        .toml → toml
.rb → ruby         .java → java        .asm/.s → asm
```

### Render Locations

1. **Bin post files** — full `CodeBlock` with line numbers and comment gutter
2. **Message attachments** — `CodeBlock` in collapsible container (filename header, click to expand/collapse)
3. **Inline markdown code blocks** — triple-backtick blocks get Shiki highlighting

## Edit History

### Message Edit History (Platform-wide)

When a message is edited, the previous content is saved to `message_versions` before the update. The `messages` table always holds the latest version.

Frontend: "(edited)" indicator (already displayed) gets a click handler opening a popover with version list and timestamps. Clicking a version shows the previous content.

### Bin Post Edit History

When a post is edited:
1. Current files are snapshot into `bin_post_version_files` linked to a new `bin_post_versions` row
2. Current description is saved on the version row
3. `bin_post_files` rows are updated with the new content
4. `bin_posts.updated_at` is set

Frontend: Version dropdown in post header ("v3 · edited 2h ago"). Selecting a previous version shows that snapshot read-only with a banner. Line comments are filtered to the selected version.

### Retention

Versions older than 90 days are purged. Applies to both `message_versions` and `bin_post_versions` / `bin_post_version_files`. Line comments anchored to purged versions are purged along with them (cascading delete on `version_id` FK). Cleanup runs as a goroutine on app startup that executes the purge query once, then on a 24-hour ticker thereafter.

## Implementation Scope Note

While this spec covers three capabilities (bin channels, nested replies, edit history), they are delivered together because bin channels depend on both nested replies and edit history. The implementation plan should order them so that platform-wide features (nested replies, edit history, syntax highlighting) land first, then bin-specific work builds on top.

## Out of Scope

- DM edit history (same pattern, separate effort)
- Custom Shiki grammars for YARA/Sigma rules (future)
- Voting/scoring on posts
- Post access control beyond channel permissions
