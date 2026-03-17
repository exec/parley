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

A **bot** is a special user with `is_bot = true` and `is_verified = true` on the `users` table. The AI Chatbot is seeded at startup with a fixed username (`ai-chatbot`) and display name (`AI Chatbot`). It has no password and cannot log in.

Bot presence in a server is tracked by `server_bots(server_id, bot_user_id, added_at)`. When a bot is added to a server, it is effectively a member of that server and can post messages through the system (not as a real user session).

### AI Chatbot — Message Flow

1. A message is posted in a channel where AI Chatbot is present.
2. The WebSocket/message handler checks: does this message @mention `ai-chatbot`, or is it a reply to a message authored by `ai-chatbot`?
3. If yes, fetch `server_ai_config` for this server. If not configured or no budget, silently skip.
4. Walk the reply chain (up to 50 messages or 8K token budget) to build conversation context.
5. Dispatch async to the configured provider. Post the bot's response as a normal message from the AI Chatbot user.
6. Deduct tokens from `server_bot_usage` for the current calendar month.

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

Display labels strip the `:cloud` suffix shown to users. The backend appends `:cloud` silently when calling the Ollama queue.

| Display Name | Backend Model | Monthly Tokens/Server |
|---|---|---|
| Ministral 3 (14B) | `ministral-3:14b-cloud` | 2,000,000 |
| GPT-OSS (20B) | `gpt-oss:20b-cloud` | 1,500,000 |
| Gemma 3 (27B) | `gemma3:27b-cloud` | 1,000,000 |
| GPT-OSS (120B) | `gpt-oss:120b-cloud` | 300,000 |
| Qwen3.5 (397B) | `qwen3:latest-cloud` | 100,000 |

Allowances reset on the 1st of each calendar month (UTC). Token counts are estimated at ~4 chars/token for user messages and counted from API response for provider-reported token counts.

---

## Database Schema

```sql
-- Mark bot/verified users
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_bot BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN NOT NULL DEFAULT FALSE;

-- Which bots are in which servers
CREATE TABLE IF NOT EXISTS server_bots (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  added_at    TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (server_id, bot_user_id)
);

-- Per-server AI Chatbot configuration
CREATE TABLE IF NOT EXISTS server_ai_config (
  server_id        BIGINT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  provider         VARCHAR(32) NOT NULL DEFAULT 'parley',
  model            VARCHAR(128) NOT NULL DEFAULT 'ministral-3:14b-cloud',
  api_key_enc      TEXT,          -- encrypted with server-side secret, NULL for Parley provider
  system_prompt    TEXT NOT NULL DEFAULT '',
  updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Monthly token usage per server
CREATE TABLE IF NOT EXISTS server_bot_usage (
  server_id    BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  month        DATE NOT NULL,  -- always 1st of month
  tokens_used  BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id, month)
);

-- Bot invite tokens (for shareable invite links)
CREATE TABLE IF NOT EXISTS bot_invite_tokens (
  id          BIGSERIAL PRIMARY KEY,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token       UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_invite_tokens_bot ON bot_invite_tokens(bot_user_id);
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

### Bot Invite Links

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/bots/invite/{token}` | Public | Resolve invite token → bot info for embed |
| POST | `/api/bots/invite/{token}/accept` | Auth | Add bot to a server |

---

## Frontend — Bots Tab

Location: Server Settings modal, new "Bots" tab alongside existing tabs.

### Layout

Two-panel layout (matches existing server settings style):

**Left panel — Bot List**
- Lists bots currently in the server with avatar + name + verified badge
- "+ Add Bot" button opens an add-by-invite-link modal
- Each bot row has a "Remove" button (admin only)
- Clicking a bot row selects it and shows its config in the right panel

**Right panel — Bot Config** (shown when a bot is selected)

For AI Chatbot specifically:
- **Provider** dropdown: Parley, Anthropic, OpenAI, xAI, Mistral, Google
- **Model** dropdown/input: prefilled per provider (see model lists below)
- **System Prompt** textarea: optional override
- **API Key** field: shown only for non-Parley providers; masked, with "Save" button
- **Usage bar** (Parley provider only): shows `X / Y tokens used this month`, resets `[date]`
- Save button

### Prefilled Models Per Provider

**Parley:** Ministral 3 (14B), GPT-OSS (20B), Gemma 3 (27B), GPT-OSS (120B), Qwen3.5 (397B)
**Anthropic:** claude-opus-4-6, claude-sonnet-4-6, claude-haiku-4-5
**OpenAI:** gpt-4.1, gpt-4.1-mini, gpt-4o, o3-mini
**xAI:** grok-3, grok-3-mini
**Mistral:** mistral-large-latest, mistral-small-latest, codestral-latest
**Google:** gemini-2.5-pro, gemini-2.0-flash, gemini-2.0-flash-lite

---

## Embed Universalization

Currently `ThemeLinkEmbed` contains its own card shell markup. `BotInviteEmbed` will need similar structure.

Extract a shared `EmbedCard` component:

```tsx
// frontend/src/components/EmbedCard.tsx
interface EmbedCardProps {
  icon: React.ReactNode;       // left icon/avatar area
  title: string;
  subtitle?: string;
  badge?: React.ReactNode;     // e.g. verified checkmark
  preview?: React.ReactNode;   // optional preview area (theme iframe)
  actions: React.ReactNode;    // buttons
}
```

`ThemeLinkEmbed` and `BotInviteEmbed` both use `EmbedCard` for their shell.

**BotInviteEmbed** resolves `/api/bots/invite/{token}` and shows:
- Bot avatar + name + verified badge
- "This bot uses AI to answer questions in your server."
- Server selector dropdown (user's servers where they have Manage Server)
- "Add to Server" button

---

## Bot Invite Link Flow

1. AI Chatbot has a permanent invite token seeded at startup (stored in `bot_invite_tokens`).
2. Sharing the link (e.g. `https://parley.app/bots/invite/{token}`) renders `BotInviteEmbed`.
3. Accepting the invite POST to `/api/bots/invite/{token}/accept` with `{ server_id }` adds the bot to that server.
4. The bot appears in the server's member list with `is_bot` and `is_verified` flags.

---

## AI Chatbot — Trigger Logic

Triggers (checked in message creation handler):

1. Message @mentions `@ai-chatbot` (contains `<@{botUserId}>` in content)
2. Message is a reply to a message where `author_id = botUserId`

If triggered:
- Fetch `server_ai_config`. If missing → use defaults (Parley, Ministral 3 14B).
- Check budget: for Parley provider, fetch `server_bot_usage` for current month. If `tokens_used >= allowance_for_model` → skip (no response, no error shown).
- Build context: walk `reply_to_message_id` chain; stop at 50 hops or when estimated tokens > 8,000.
- Dispatch goroutine: call provider API, post response as AI Chatbot user in same channel.
- On success: upsert `server_bot_usage` incrementing `tokens_used` by response token count.

---

## Token Estimation

- For Parley (Ollama): response will not return token counts directly; estimate at **4 chars = 1 token** for both input and output.
- For providers that return token counts in their API response (Anthropic, OpenAI, etc.): use reported counts.
- Token budget check before dispatch uses the same 4-char estimate on the context being sent.

---

## Security & Privacy

- **API key encryption**: Server-supplied API keys are encrypted at rest using AES-256-GCM with a `BOT_KEY_SECRET` env var. Never returned to clients — the settings page shows only a masked placeholder and "Replace" button.
- **Server admin gate**: All bot config endpoints require the requesting user to have admin/owner role in the server.
- **Message privacy**: Bot context walks only messages in the channel where it's triggered; never cross-channel.
- **Rate limiting**: Bot responses are gated by the monthly token budget; no additional per-message rate limit needed for MVP.

---

## TODOs (Out of Scope for This Spec)

- **User-built API bots**: The `server_bots` + `bot_invite_tokens` tables scaffold this; actual bot API token issuance and webhook/WS delivery are future work.
- **Bot permissions**: Future — restrict which channels a bot can read/post in.
- **Multi-bot support**: For now, only AI Chatbot is implemented. The infrastructure supports multiple bots.

---

## Success Criteria

- Server admins can add AI Chatbot to their server via Bots tab
- Mentioning or replying to the bot triggers an AI response in the channel
- Parley provider token usage is tracked and shown with a reset date
- Non-Parley providers work with a server-supplied API key
- Bot invite link resolves to an embed with server-add flow
- Theme embed and bot invite embed share the same `EmbedCard` shell
- No user API keys are exposed in any API response
