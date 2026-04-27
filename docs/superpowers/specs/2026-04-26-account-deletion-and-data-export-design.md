# Self-serve Account Deletion + Data Export Design

**Goal:** Ship the GDPR-mandated user-facing flows for account erasure and data portability so Parley is launch-ready under EU privacy law.

**Constraint:** Must preserve message history context for other users (Discord-style anonymization), not hard-delete every authored row.

---

## API contract

### `DELETE /api/me` — Account deletion (self-serve)

**Auth:** Bearer JWT (existing).

**Body:** `{"confirm_username": "<exact username>"}`. The server matches against the authenticated user's username; mismatch → `400 invalid_confirmation`. (Username confirmation is the friction; password re-prompt isn't needed since the bearer token already authenticates them and force-logout invalidates after delete.)

**Response codes:**
- `204 No Content` — deletion completed.
- `400 invalid_confirmation` — `confirm_username` didn't match.
- `409 has_blockers` — user owns servers with other members and/or group DMs with other members. Body:
  ```json
  {
    "error": "has_blockers",
    "blocking_servers": [{"id": "...", "name": "..."}],
    "blocking_group_dms": [{"id": "...", "name": "..."}]
  }
  ```
  Client must transfer ownership or disband first via existing UI.
- `500 internal_server_error` — unhandled.

**Side effects (in order, single Postgres transaction):**
1. Verify no blocking servers/group DMs (else 409).
2. Reassign authored content to the `[deleted]` sentinel user (see Schema below):
   - `messages.author_id`
   - `dm_messages.author_id`
   - `bin_posts.author_id`
   - `bin_line_comments.author_id`
3. Set `users.force_logout_at = NOW()` (kills any in-flight JWT).
4. Hard-delete bots owned by user: `DELETE FROM users WHERE bot_owner_id = <uid>`. Cascade scrubs their messages.
5. Hard-delete the user row. Existing `ON DELETE CASCADE` FKs scrub: `server_members`, `dm_channel_members`, `notifications`, `friend_requests`, `user_blocks`, `read_state`, `voice_presences`, `passkeys`, `sessions`, `audit_log` entries authored by the user, `themes` they published, etc.
6. Best-effort delete of avatar/banner objects in MinIO (separate bucket; outside the Postgres tx — log-and-continue on failure).

**Why username-confirmation only (no password):**
- Passkey-only users have no password.
- Bearer token already proves session ownership.
- Force-logout immediately after delete kills any stolen-token attack window.
- Username typing is sufficient friction against accidents.

### `GET /api/me/export` — Data export

**Auth:** Bearer JWT.

**Response:**
- `200 OK`, `Content-Type: application/json`, `Content-Disposition: attachment; filename="parley-export-<username>-<unix>.json"`.
- Body is one JSON object (single-shot, not streaming JSONL — keeps client-side parsing trivial; if it ever exceeds practical size, switch to async-job + email-link).

**Shape:**
```json
{
  "exported_at": "2026-04-26T...",
  "format_version": 1,
  "profile": { /* User row, sans password_hash, twofa secrets */ },
  "passkeys": [ { "credential_id": "...", "name": "...", "created_at": "..." } ],
  "friends": [ /* FriendUser[] */ ],
  "blocked_users": [ /* FriendUser[] */ ],
  "owned_servers": [ /* { id, name, created_at } */ ],
  "owned_bots":   [ /* { id, username, created_at } */ ],
  "themes_published": [ /* full theme rows */ ],
  "messages_authored": {
    "server_channels": [ /* { id, channel_id, content, created_at, ... } */ ],
    "dms":             [ /* { id, dm_channel_id, content, created_at, ... } */ ],
    "bin_posts":       [ /* { id, channel_id, title, content, created_at } */ ],
    "bin_line_comments": [ /* { id, post_id, line, content, created_at } */ ]
  },
  "dm_channels_member_of": [ /* DM channel summary { id, is_group, name, created_at } */ ],
  "notifications_received": [ /* AppNotification[] */ ],
  "audit_log_entries_about_me": [ /* audit_log rows where target_user_id == me */ ]
}
```

**No attachments inlined.** Attachment URLs live inside message rows; user can fetch them while authenticated. Bundling MinIO objects would 10× the payload and require a zip stream. Defer.

**Performance:** Build in memory and return as one JSON. Worst case ~50-100 MB for very heavy users; chunked transfer encoding handles it. Profile real usage post-launch and switch to async-job if needed.

---

## Schema changes

**Migration #69:** Insert the `[deleted]` sentinel user.

```sql
INSERT INTO users (username, email, password_hash, is_system, display_name, created_at, updated_at)
VALUES ('deleted', 'deleted@parley.invalid', '!', TRUE, 'Deleted User', NOW(), NOW())
ON CONFLICT (username) DO NOTHING;
```

The sentinel uses `is_system = TRUE` so existing system-user filters in queries naturally exclude it from member lists, search results, friend suggestions, etc. Username `deleted` is reserved (registration should already block any user-creatable account from claiming it; verify in `auth.RegisterUser`).

**No FK migration needed.** Existing `ON DELETE CASCADE` on author FKs is fine — the deletion path reassigns rows to the sentinel *before* deleting the original user, so the cascade has nothing to scrub for those tables.

**Reserve sentinel UID:** Look up the inserted user's id at startup and cache as `db.DeletedSentinelUserID()`. Or query by `username = 'deleted'` once on first delete.

---

## Frontend changes

`frontend/src/components/settings/UserSettings.tsx` — extend the existing `AccountTab` (line 417). Add a new section near the bottom titled "Privacy & data".

- **Download my data**: button that calls `GET /api/me/export` with `fetch`, reads as Blob, triggers browser download via a temporary anchor. Spinner while fetching. Toast on success/error.
- **Delete my account**: button (red, `var(--parley-danger)`) opens a `<Modal>` with:
  - Warning text listing what gets deleted vs anonymized.
  - Username-input field; the Delete button stays disabled until the input matches `currentUser.username` exactly.
  - On click → `DELETE /api/me` with `{confirm_username}`.
  - On `204`: clear local state, redirect to `/login` with toast "Account deleted".
  - On `409`: replace modal body with the blockers list and links into the relevant settings (Server Settings → Transfer Ownership / Group DM → Transfer Ownership). Keep cancel button.
  - On `400`: shake the input and show "Username didn't match".

`frontend/src/api/me.ts` (new file) for these two endpoints. Keep `users.ts` / `account.ts` style consistent with existing API layer (see `api/friends.ts` for pattern).

---

## Out of scope for v1

- Grace period / soft-delete with recovery window (Discord-style 14 days). Adds complexity; revisit if user feedback wants it.
- ZIP-with-attachments export.
- Async job / email-link delivery.
- Email confirmation step before deletion (extra friction; the username-typing confirm + force-logout window is sufficient).
- Audit-log emit on self-deletion (the user's audit rows are being deleted anyway; no one to read it).

---

## Coordination notes for the team

- **`backend-deletion`** owns: migration #69, `internal/account/` package (or extend an existing one — match the codebase's idioms), `DELETE /api/me` route in `cmd/api/routes.go`, blocker-detection logic, sentinel reassignment, transactional delete.
- **`backend-export`** owns: `GET /api/me/export` route, the data-collection layer (mostly `SELECT ... WHERE author_id/user_id = $1`), the JSON shape above. Live next to `backend-deletion`'s package.
- **`frontend-account`** owns: `frontend/src/api/me.ts`, the new "Privacy & data" section in `AccountTab`, the delete-confirmation modal, the export-trigger button.
- **API contract is locked above.** If a backend agent needs to deviate (e.g., a field name doesn't fit cleanly with existing models), DM `frontend-account` so they update the consumer.
- **Frontend** can start work immediately against the locked contract; it doesn't need to wait for backend.
