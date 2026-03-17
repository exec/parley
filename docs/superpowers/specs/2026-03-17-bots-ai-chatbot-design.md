# Bots & AI Chatbot Design

**Date:** 2026-03-17
**Status:** Approved

---

## Overview

Add a Bots system to Parley with a "Bots" tab in Server Settings. The initial bot is an **AI Chatbot** — a system-owned verified bot that responds to @mentions and replies in channels using a configurable LLM provider. Each server configures its own provider, model, system prompt, and (where required) API key. Parley hosts its own Ollama Cloud fleet for the default "Parley" provider with free monthly token allowances per server.

Also included: bot invite link infrastructure (shareable tokens that render as embeds) and universalization of the embed card component shared across theme previews and bot invites.

---

## Architecture

### Bot Infrastructure

A **bot** is a special user with `is_bot = true` and `is_verified = true` on the `users` table. The AI Chatbot is seeded via a database migration (same pattern as the existing `Parley` system user in `internal/db/migrations.go`) with a fixed username (`ai-chatbot`) and display name (`AI Chatbot`). It has no password and cannot log in.

`is_bot` already exists on the `users` table (migration index 18). The new migration entry must add only `is_verified` and the new tables/indexes — **do not** write a separate migration for `is_bot`.

The AI Chatbot's `id` is resolved at application startup (a single DB lookup by `username = 'ai-chatbot'`) and cached in memory for the lifetime of the process. This cached ID is what the message handler uses to detect bot mentions and attribute responses.

Bot presence in a server is tracked by `server_bots(server_id, bot_user_id, added_at)`. When a bot is added to a server, it is effectively a member of that server and can post messages through the system (not as a real user session).

### AI Chatbot — Message Flow

1. A message is posted in a channel where AI Chatbot is present.
2. The message creation handler checks: does this message @mention `<@{botUserId}>`, or is it a reply (`parent_id` non-null) to a message authored by `botUserId`?
3. If yes, fetch `server_ai_config` for this server. If not configured, use defaults (Parley, Ministral 3 14B).
4. For Parley provider: check `server_bot_usage` for current month. If `tokens_used >= allowance_for_configured_model` → silently skip.
5. Walk the `parent_id` reply chain (up to 50 hops or 8K estimated tokens) to build conversation context.
6. Dispatch to the configured provider in a goroutine. Post the bot's response as a normal message from the AI Chatbot user via the same message creation path as human messages (so it receives the same WebSocket broadcast to channel subscribers).
7. Upsert `server_bot_usage` incrementing `tokens_used` by the response token count.

### Providers

| Provider | ID | Notes |
|---|---|---|
| Parley (Ollama) | `parley` | Free; uses existing Ollama queue; monthly per-model allowance |
| Anthropic | `anthropic` | Requires server-supplied API key |
| OpenAI | `openai` | Requires server-supplied API key |
| xAI | `xai` | Requires server-supplied API key |
| Mistral | `mistral` | Requires server-supplied API key |
| Google | `google` | Requires server-supplied API key |

### Parley Models & Monthly Allowances

`server_ai_config.model` stores the **display-side model string** (no `:cloud` suffix). The backend appends `:cloud` silently at dispatch time before calling the Ollama queue. This means the default value in the schema is `'ministral-3:14b'` (no suffix), and the dispatch layer does `model + ":cloud"` unconditionally for Parley provider requests.

| Display Name | Stored Value | Backend Dispatch | Monthly Tokens/Server |
|---|---|---|---|
| Ministral 3 (14B) | `ministral-3:14b` | `ministral-3:14b-cloud` | 2,000,000 |
| GPT-OSS (20B) | `gpt-oss:20b` | `gpt-oss:20b-cloud` | 1,500,000 |
| Gemma 3 (27B) | `gemma3:27b` | `gemma3:27b-cloud` | 1,000,000 |
| GPT-OSS (120B) | `gpt-oss:120b` | `gpt-oss:120b-cloud` | 300,000 |
| Qwen3.5 (397B) | `qwen3:latest` | `qwen3:latest-cloud` | 100,000 |

Allowances reset on the 1st of each calendar month (UTC). The budget check compares `tokens_used` against the allowance for the **currently-configured model**. If a server switches to a lower-allowance model mid-month and already exceeds that model's allowance, responses are silently skipped until the month resets.

---

## Database Schema

Add a new entry to the `Migrations` slice in `internal/db/migrations.go`. **`is_bot` already exists** — include only `is_verified` and the new tables.

```sql
-- is_bot already exists (migration 18); add only is_verified
-- email is nullable (made so in migration 11), so omitting it below is safe
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Which bots are in which servers
CREATE TABLE IF NOT EXISTS server_bots (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  added_at    TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (server_id, bot_user_id)
);
CREATE INDEX IF NOT EXISTS idx_server_bots_bot ON server_bots(bot_user_id);

-- Per-server AI Chatbot configuration
CREATE TABLE IF NOT EXISTS server_ai_config (
  server_id        BIGINT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  provider         VARCHAR(32) NOT NULL DEFAULT 'parley',
  model            VARCHAR(128) NOT NULL DEFAULT 'ministral-3:14b',
  api_key_enc      TEXT,          -- AES-256-GCM encrypted, NULL for Parley provider
  system_prompt    TEXT NOT NULL DEFAULT '',
  updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Monthly token usage per server
CREATE TABLE IF NOT EXISTS server_bot_usage (
  server_id    BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  month        DATE NOT NULL,  -- always 1st of month (DATE_TRUNC('month', NOW()))
  tokens_used  BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id, month)
);

-- Bot invite tokens (must be created BEFORE the seed INSERT below)
CREATE TABLE IF NOT EXISTS bot_invite_tokens (
  id          BIGSERIAL PRIMARY KEY,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token       UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_invite_tokens_bot ON bot_invite_tokens(bot_user_id);

-- Seed AI Chatbot system user (idempotent). id is assigned by gen_user_id() default.
-- The bot's id is resolved at application startup by querying WHERE username = 'ai-chatbot'.
INSERT INTO users (username, display_name, password_hash, is_bot, is_verified)
VALUES ('ai-chatbot', 'AI Chatbot', '', TRUE, TRUE)
ON CONFLICT (username) DO NOTHING;

-- Seed permanent invite token for AI Chatbot (must come after both table and user exist)
INSERT INTO bot_invite_tokens (bot_user_id, token)
SELECT id, 'aaaaaaaa-0000-0000-0000-000000000001'::uuid
FROM users WHERE username = 'ai-chatbot'
ON CONFLICT DO NOTHING;
```

---

## API Endpoints

### Server Bots Tab

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/servers/{id}/bots` | Member | List bots in server |
| POST | `/api/servers/{id}/bots` | Admin | Add bot by invite token |
| DELETE | `/api/servers/{id}/bots/{botId}` | Admin | Remove bot from server |
| GET | `/api/servers/{id}/ai-config` | Admin | Get AI config for server |
| PUT | `/api/servers/{id}/ai-config` | Admin | Update AI config |
| GET | `/api/servers/{id}/ai-usage` | Admin | Get current month token usage |

**`GET /api/servers/{id}/bots` response** (array of `BotSummary`):
```json
[
  {
    "id": 42,
    "username": "ai-chatbot",
    "display_name": "AI Chatbot",
    "avatar_url": null,
    "is_verified": true,
    "added_at": "2026-03-17T00:00:00Z"
  }
]
```

**`POST /api/servers/{id}/bots` request body:**
```json
{ "invite_token": "aaaaaaaa-0000-0000-0000-000000000001" }
```
Errors: 404 (invalid token), 409 (bot already in server).

**`PUT /api/servers/{id}/ai-config` request body:**
```json
{
  "provider": "parley",
  "model": "ministral-3:14b",
  "api_key": "sk-...",      // optional; omit or empty string to keep existing
  "system_prompt": ""
}
```
`api_key` is write-only — the GET response never includes it (shows `"api_key_set": true/false` instead).

**`GET /api/servers/{id}/ai-config` response:**
```json
{
  "provider": "parley",
  "model": "ministral-3:14b",
  "api_key_set": true,
  "system_prompt": "",
  "updated_at": "2026-03-17T00:00:00Z"
}
```
`api_key_enc` is never included. `api_key_set` is `true` if a non-empty encrypted key is stored.

**`GET /api/servers/{id}/ai-usage` response:**
```json
{
  "tokens_used": 450000,
  "tokens_limit": 2000000,
  "model": "ministral-3:14b",
  "resets_at": "2026-04-01T00:00:00Z"
}
```

### Bot Invite Links

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/bots/invite/{token}` | Public | Resolve invite token → bot info for embed |
| POST | `/api/bots/invite/{token}/accept` | Auth | Add bot to a server |

**`GET /api/bots/invite/{token}` response:**
```json
{
  "bot_id": 42,
  "username": "ai-chatbot",
  "display_name": "AI Chatbot",
  "avatar_url": null,
  "is_verified": true
}
```
Errors: 404 (invalid token).

**`POST /api/bots/invite/{token}/accept` request body:**
```json
{ "server_id": 7 }
```
The handler must verify: (1) the invite token exists, (2) the caller is a member of the target server, (3) the caller has admin/owner role in that server. Return 404 for an invalid token or a server the caller is not a member of (do not distinguish — avoids server ID enumeration). Return 403 if the caller is a member but lacks admin/owner role. Return 409 if the bot is already in that server.

---

## Security & Privacy

- **`BOT_KEY_SECRET` env var**: 32-byte (256-bit) secret used for AES-256-GCM encryption of third-party API keys. Loaded alongside other env vars in `cmd/api/main.go`. If the env var is absent at startup, the process must **fatal-exit** — never store keys unencrypted silently. The key is never logged.
- **API key exposure**: `api_key_enc` is never returned to any client. The `GET /api/servers/{id}/ai-config` response includes `"api_key_set": true` (or `false`) only.
- **Server admin gate**: All bot config endpoints require the requesting user to have admin/owner role in the server (same permission check as other server settings endpoints).
- **Message privacy**: Bot context walks only messages in the channel where it's triggered via the `parent_id` chain; never cross-channel.
- **Rate limiting**: Bot responses are gated by the monthly token budget; no additional per-message rate limit needed for MVP.

---

## Frontend — Bots Tab

Location: Server Settings modal, new "Bots" tab alongside existing tabs.

### File Layout

```
frontend/src/components/settings/
  BotsTab.tsx              -- top-level tab component
  BotConfigPanel.tsx       -- right panel: provider/model/key/usage config
  BotsTab.css
frontend/src/components/
  EmbedCard.tsx            -- shared card shell
  EmbedCard.css
frontend/src/pages/
  BotInvitePage.tsx        -- route /bots/invite/:token → renders BotInviteEmbed
frontend/src/components/
  BotInviteEmbed.tsx       -- embed content using EmbedCard
```

Add route `/bots/invite/:token` in `frontend/src/App.tsx` (or wherever routes are defined).

### Layout

Two-panel layout:

**Left panel — Bot List**
- Lists bots currently in the server with avatar + name + verified badge (✓)
- "+ Add Bot" button opens a modal with an invite link input field
- Each bot row has a "Remove" button (admin only)
- Clicking a bot row selects it and shows its config in the right panel

**Right panel — Bot Config** (shown when a bot is selected)

For AI Chatbot:
- **Provider** dropdown: Parley, Anthropic, OpenAI, xAI, Mistral, Google
- **Model** dropdown: prefilled per provider (see below); selecting a provider resets the model to that provider's first option
- **System Prompt** textarea: optional override (placeholder: "You are a helpful assistant.")
- **API Key** field: shown only for non-Parley providers; write-only masked field with "Update Key" button
- **Usage bar** (Parley provider only): progress bar showing `X / Y tokens` + "Resets [date]" beneath
- **Save** button for provider/model/system-prompt changes

### Prefilled Models Per Provider

```typescript
const PROVIDER_MODELS: Record<string, { label: string; value: string }[]> = {
  parley: [
    { label: 'Ministral 3 (14B)', value: 'ministral-3:14b' },
    { label: 'GPT-OSS (20B)',      value: 'gpt-oss:20b' },
    { label: 'Gemma 3 (27B)',      value: 'gemma3:27b' },
    { label: 'GPT-OSS (120B)',     value: 'gpt-oss:120b' },
    { label: 'Qwen3.5 (397B)',     value: 'qwen3:latest' },
  ],
  anthropic: [
    { label: 'Claude Opus 4.5',    value: 'claude-opus-4-5' },
    { label: 'Claude Sonnet 4.5',  value: 'claude-sonnet-4-5' },
    { label: 'Claude Haiku 4.5',   value: 'claude-haiku-4-5-20251001' },
  ],
  openai: [
    { label: 'GPT-4.1',            value: 'gpt-4.1' },
    { label: 'GPT-4.1 Mini',       value: 'gpt-4.1-mini' },
    { label: 'GPT-4o',             value: 'gpt-4o' },
    { label: 'o3-mini',            value: 'o3-mini' },
  ],
  xai: [
    { label: 'Grok 3',             value: 'grok-3' },
    { label: 'Grok 3 Mini',        value: 'grok-3-mini' },
  ],
  mistral: [
    { label: 'Mistral Large',      value: 'mistral-large-latest' },
    { label: 'Mistral Small',      value: 'mistral-small-latest' },
    { label: 'Codestral',          value: 'codestral-latest' },
  ],
  google: [
    { label: 'Gemini 2.5 Pro',     value: 'gemini-2.5-pro' },
    { label: 'Gemini 2.0 Flash',   value: 'gemini-2.0-flash' },
    { label: 'Gemini 2.0 Flash Lite', value: 'gemini-2.0-flash-lite' },
  ],
};
```

---

## Embed Universalization

`ThemeLinkEmbed` wraps a theme preview iframe in a card. `BotInviteEmbed` wraps bot info + a server selector. Both share structural chrome: a card container, a header area, a content area, and action buttons.

Extract `EmbedCard` as a flexible shell. All props are optional except `title` and `actions` to accommodate the different shapes of each embed.

```tsx
// frontend/src/components/EmbedCard.tsx
interface EmbedCardProps {
  icon?: React.ReactNode;       // avatar/swatch — absent in ThemeLinkEmbed
  title: string;
  subtitle?: string;
  badge?: React.ReactNode;      // e.g. verified ✓ chip
  preview?: React.ReactNode;    // theme iframe or null
  children?: React.ReactNode;   // extra body content (server selector, etc.)
  actions: React.ReactNode;
}
```

**`ThemeLinkEmbed` mapping:** `icon` = undefined, `title` = theme name, `subtitle` = "by {author}", `preview` = `<iframe>`, `actions` = Install button.

**`BotInviteEmbed` mapping:** `icon` = bot avatar/initials, `title` = bot display name, `subtitle` = "AI Chatbot · Verified", `badge` = ✓ chip, `children` = server selector dropdown, `actions` = "Add to Server" button.

---

## Bot Invite Link Flow

1. AI Chatbot's permanent invite token (`aaaaaaaa-0000-0000-0000-000000000001`) is seeded in the migration.
2. The link `https://parley.app/bots/invite/{token}` renders `BotInvitePage` → `BotInviteEmbed`.
3. `BotInviteEmbed` fetches `GET /api/bots/invite/{token}` for bot info, and `GET /api/servers` for the user's servers where they have Manage Server permission.
4. User selects a server and clicks "Add to Server" → `POST /api/bots/invite/{token}/accept` with `{ "server_id": N }`.
5. Bot appears in server member list with `is_bot` + `is_verified` set.

---

## AI Chatbot — Trigger Logic

Triggers (checked inside the message creation handler, after a message is persisted):

1. Message content contains `<@{botUserId}>` (mention)
2. Message has `parent_id` non-null pointing to a message where `author_id = botUserId` (reply)

**DM channels are excluded**: if the channel has no associated `server_id` (i.e., it is a DM), skip silently.

If triggered, check that AI Chatbot is in `server_bots` for this server. If not → skip.

Fetch `server_ai_config`. Missing → use defaults (Parley, `ministral-3:14b`).

For Parley provider: check `server_bot_usage` for current month (`DATE_TRUNC('month', NOW())`). Allowances by model:

| Model value | Allowance |
|---|---|
| `ministral-3:14b` | 2,000,000 |
| `gpt-oss:20b` | 1,500,000 |
| `gemma3:27b` | 1,000,000 |
| `gpt-oss:120b` | 300,000 |
| `qwen3:latest` | 100,000 |

If `tokens_used >= allowance` → silently skip. No error posted to channel.

Build context: walk `parent_id` chain collecting messages. Stop at 50 hops or when cumulative estimated token count (total chars / 4) exceeds 8,000. Collect messages oldest-first. Assign roles: `author_id == botUserId` → `assistant`, all others → `user`. If the resulting array's first element has `role: assistant` (can happen when the trigger is a reply to the bot), prepend a synthetic `{role: "user", content: "..."}` placeholder so the array always starts with a user turn — required by Anthropic, OpenAI, and most providers.

Dispatch goroutine with a **30-second context timeout**. On provider error or timeout: log the error, silently skip (do not post an error message to the channel). On success: post the response via the normal message creation path (same WebSocket broadcast as human messages). Then upsert token usage:

```sql
INSERT INTO server_bot_usage (server_id, month, tokens_used)
VALUES ($1, DATE_TRUNC('month', NOW()), $2)
ON CONFLICT (server_id, month)
DO UPDATE SET tokens_used = server_bot_usage.tokens_used + EXCLUDED.tokens_used
```

where `$2` is the token delta for this response.

---

## Token Estimation

- **Parley (Ollama):** No token counts in response. Estimate: input chars / 4 + output chars / 4.
- **Anthropic, OpenAI, Mistral, Google, xAI:** Use `usage.input_tokens` + `usage.output_tokens` (or equivalent field name) from API response.
- Budget pre-check uses 4-char estimate on context size before dispatch.

---

## TODOs (Out of Scope for This Spec)

- **User-built API bots**: `server_bots` + `bot_invite_tokens` scaffold this; actual bot API token issuance and webhook/WS delivery are future work.
- **Bot permissions**: Future — restrict which channels a bot can read/post in.
- **Multi-bot support**: Only AI Chatbot is implemented. The infrastructure supports multiple bots.
- **`BOT_KEY_SECRET` rotation**: If the secret changes, all stored `api_key_enc` values become unreadable. There is no migration path in this spec. Key rotation requires re-encryption of all affected rows or a server wipe of stored keys. This must be addressed before the feature reaches a state where many servers have API keys stored.

---

## Success Criteria

- Server admins can add AI Chatbot to their server via Bots tab
- Mentioning or replying to the bot triggers an AI response in the channel
- Parley provider token usage is tracked and shown with a reset date
- Non-Parley providers work with a server-supplied API key (encrypted at rest)
- Startup fatal-exits if `BOT_KEY_SECRET` is not set
- Bot invite link resolves to an embed with server-add flow
- Theme embed and bot invite embed share the same `EmbedCard` shell
- No API keys are exposed in any API response
