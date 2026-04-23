# D3 — Bot API Key Scopes

**Audit finding.** `docs/security/2026-04-23-adversarial-audit.md` — D3 (HIGH).
Before this change, any leaked `plk_…` token granted full user-session
authority for the bot user across every server it was in. There was no
scope model; `IsAPIKeyAuth` was consulted in exactly one place
(`internal/message/service.go:187`) only to annotate messages with a
`via_api` flag.

**What shipped.** A seven-scope model plus a `full` meta-scope:

| Scope                    | Gates                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------ |
| `messages:read`          | Read channel/DM message history, versions, reactions, pins, bin posts, bin line comments               |
| `messages:write`         | Post/edit/delete messages + reactions + pins, forward, typing indicators, voice join/leave, giphy, upload |
| `commands:write`         | Register/update/delete the bot's slash commands (`PUT/POST/DELETE /api/bots/@me/servers/{id}/commands`) |
| `interactions:respond`   | **Reserved.** Interaction responses authenticate with a per-interaction URL token, not an API key — this scope is defined for forward-compatibility and not enforced on any current route. |
| `profile:write`          | Mutate bot's own account (profile, email, phone, passkeys, themes, preferences, friends, server admin actions, soundboard mutations) |
| `servers:read`           | List/read servers the bot is in, channels, roles, members, invites (preview), notifications inbox, audit log |
| `developer:manage`       | Manage the bot's own API keys and bot-user profile (`/api/developer/*`). A narrower-scope leaked token cannot rotate the bot's credentials. |
| `full`                   | Meta-scope granting every scope above. Existing pre-D3 keys are backfilled to this during the migration. |

## DB migration

The migration runs automatically at service start as part of the existing
`RunMigrations` path (`internal/db/repository.go`). The appended migration
is a single SQL block:

```sql
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS scopes TEXT[] NOT NULL DEFAULT '{}';
UPDATE api_keys SET scopes = ARRAY['full']::TEXT[] WHERE scopes = '{}'::TEXT[];
```

**Ordering.** The migration is tracked in `schema_migrations` by index, so
it runs exactly once on each DB regardless of how many API nodes come up.
No operator action required for a deploy.

**Backfill strategy.** Every pre-D3 row becomes `{'full'}` so deployed bots
keep working across the cutover. New rows inserted via `CreateAPIKey` or
`CreateBotWithKey` have `DEFAULT '{}'` but the code path always passes an
explicit non-empty `scopes` array (the handler validates before reaching
the repo).

**Rolling deploy.** During the window where migrated DB + old API nodes
coexist, new keys minted by old nodes will land with `scopes = '{}'`. The
middleware treats empty scopes on an API-key request as "no permissions"
(`HasScope` returns false) — those keys are inert until the caller
re-issues. This is the safe failure mode.

## Post-deploy audit checklist (operators and key owners)

1. `SELECT id, user_id, owner_id, name, scopes, created_at FROM api_keys WHERE scopes @> ARRAY['full']::TEXT[];`
2. For each row, contact the owner and ask them to re-issue with the
   narrowest scope set that covers their bot's actual workload. Typical
   minimal profiles:
   - Read-only monitoring bot → `['messages:read', 'servers:read']`
   - Posting-only announcer bot → `['messages:write']`
   - Slash-command bot that only responds to users → `['commands:write', 'servers:read']` (responses use the per-interaction URL token, not the API key)
   - Full-surface bot (match old behavior) → `['full']` — strongly
     discourage; keep only for bots that genuinely need it.
3. Revoke the old `full` key once the owner confirms the new one works.

## Telemetry

`RequireScope` returns a structured 403 body naming the missing scope:

```json
{"error":"bot token missing scope: messages:write"}
```

Recommended observability additions (not shipped in this change):

- Log `audit: scope_rejection key_id=<id> user_id=<id> path=<path> missing=<scope>`
  on every 403 so ops can track keys that are hitting scope walls. This
  helps owners narrow their issued scopes iteratively without guesswork.
- A Grafana panel tracking `count by (missing_scope)` of scope rejections
  per hour surfaces keys that were under-scoped at creation time.

## Per-endpoint scope map

The authoritative wiring is in `cmd/api/routes.go`. Summary of the
non-obvious decisions:

- **Bot writes to channel metadata** (create/delete channel, roles,
  overwrites, server settings) → `profile:write`, not a bespoke
  `servers:write`. A narrow messages bot should not be able to reshape
  the server; a full-surface admin bot holds `profile:write` anyway.
- **Interaction response** (`POST /api/interactions/{token}/respond`) is
  authenticated by URL token, not API key. No scope gate is applied; the
  `interactions:respond` scope is defined but not consulted on any route.
- **Notifications inbox** splits: list → `servers:read`; mark-read →
  `profile:write` (mutates account state).
- **Soundboard play** → `messages:write` (fan-out audio is a broadcast
  event analogous to sending a message).
- **Slash-command invocation** (`POST /api/channels/{id}/interactions`)
  → `messages:write`, because invoking a command triggers a message
  fan-out from the bot; a read-only token should not be able to
  indirectly cause writes.

## Backward compatibility

Existing bot tokens grandfather to `full`. This is a temporary
compatibility window: the intent is to require narrow scopes at creation
time in the next major release. Reflected in the README / developer docs:

> API keys created before scope support landed grandfather to `full`
> access. We strongly recommend rotating to narrow scopes — once v1.0
> ships, `full` will remain valid only for keys explicitly flagged at
> creation time.

## Test evidence

- `internal/auth/scopes_test.go`: unit tests for `ValidateScopes`,
  `HasScope` (user JWT pass-through, exact-scope match, full-scope
  grant-all, empty-scopes fail-closed), `RequireScope` (403 body shape,
  pass cases, user JWT pass-through), and a chained middleware test
  simulating the production wiring.
- `internal/auth/middleware_test.go`: existing impersonation + dedup
  tests continue to pass; the API-key path's scope stash is covered by
  the chained test above.
- Full suite: `JWT_SECRET=test-user IMPERSONATION_JWT_SECRET=test-imp go test ./...` green.
