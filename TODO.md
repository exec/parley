# TODO

## Slash commands — v1 scope (locked)

Phases to ship first; detailed design in conversation history.

- **Interaction delivery**: WebSocket push to the bot's live connection (via the existing gateway). Interactions persist server-side with a 15min token so a brief WS blip doesn't lose the ACK — the bot can reconnect and respond against the token from a new session.
- **Offline / degraded bot UX**: in the `/` autocomplete dropdown, an offline bot's commands render disabled with "Bot is offline"; a degraded bot's commands render with an amber dot + tooltip but stay selectable. Uses the existing bot status + `set_degraded(True)` SDK hook.
- **Command cap**: 100 commands per bot per server (matches Discord). Phase 1 must include a simple in-process per-server command cache with invalidation on register/update/delete — the `/` autocomplete hits "list commands for this channel" on every keystroke.
- **Registration rate limit**: 50 upserts/hour per bot.
- **Deviations from Discord** locked: single permission layer, one-level subcommands only, per-server registration only (no global scope), tighter embed limits (2000 desc / 10 fields / 4 embeds / 3000 char sum), no `ATTACHMENT` option type, no autocomplete on options, no modals, no context-menu commands, **native user-lock primitive** (backend-enforced `locked_to_user_id` on components — the main developer-ergonomics win over Discord).
- **Phases**: (1) DB + registration API + plain-text invocation; (2) embeds; (3) components + native user-lock + ephemeral; (4) Python SDK decorators + `docs/bots.md` rewrite.

## Slash commands — v2

Deferred from the v1 design in favor of shipping a small, focused surface first. Add these once v1 lands and real bots are exercising the system.

- **Autocomplete on options** — runtime `{name, value}[]` suggestions as the user types an option. Requires a synchronous request/response path with a hard 3s deadline (cannot defer). Add a new `INTERACTION_AUTOCOMPLETE` WS event + response endpoint. Static `choices[]` stays as the default; autocomplete is opt-in per option via `autocomplete: true`.
- **Modals** — bot responds to an initial interaction or component click with a modal form (text inputs). Adds a `MODAL_SUBMIT` interaction type, `text_input` component kind (short / paragraph), modal open/submit round-trip, and modal field validation (min/max length, placeholder).
- **`ATTACHMENT` option type** — lets users attach a file as part of a slash invocation. Needs a pre-upload step that stages the file with the interaction token before dispatch, plus cleanup on expired / unused interactions.
- **Context-menu commands** — right-click a message or user to run a bot action. New command kinds `USER` and `MESSAGE`, new UI entry points in the message context menu and the user profile modal, and a different invocation payload shape (target ID rather than options).
- **Sub-command groups** — second nesting level (`/group sub command ...`). Currently only one level (`/foo bar`) is supported. Defer unless a real bot justifies it — it mostly bloats the command schema.
- **Three-layer permission model** — bot registers `default_member_permissions`; server admins override per-role, per-user, and per-channel via a settings UI. v1 is one layer (`PermUseBotCommands` role bit + existing bot-granted perms).
- **Per-channel command visibility** — scope registration to `allowed_channel_ids` / `blocked_channel_ids` so a bot only exposes a command in specific channels.
- **Global commands across servers** — an installed bot registering a command once that shows up on every server it's added to, rather than re-registering per-server. Only worth it if Parley grows to where bots are installed at scale.
- **Built-in `/help` command** — backend-generated, ephemeral by default, lists every command the invoker can use in the current channel. Deferred for v1: the `/` autocomplete dropdown with command descriptions already serves as the discovery UI; revisit if the dropdown becomes too noisy at scale.
- **Richer embed fields** — `video`, `provider` (read-only provider metadata for URL unfurls), native URL-unfurl pipeline that generates embeds for pasted links server-side.
- **Component `link` buttons** — no `custom_id`, just a URL; doesn't create an interaction. Trivial to add but intentionally deferred to keep v1 component types minimal (primary/secondary/success/danger only).
- **Component `premium` / monetization buttons** — not applicable unless Parley adds a monetization layer.
- **Interaction rate-limit tuning** — Discord does 5 req / 2s per interaction token, 30/60s globally. v1 inherits the existing `msgWriteLimiter`; revisit once we see real invocation patterns.
- **HTTP-webhook delivery transport** — fallback delivery mode for bots that would rather receive interactions at a registered URL than hold a WebSocket open. v1 delivers over the bot's existing WS connection.
- **Python SDK: cog-style slash command groups**, parity with the existing prefix `CommandBot` cog system.
- **TypeScript bot SDK** — the Python SDK is the only first-party SDK today. Once slash commands stabilize, mirror the API in TS for Node bots.

## Other

- **Per-channel message drafts** (auto-save) — localStorage-backed draft per (channel | DM) so typed-but-unsent text survives navigation. Small, frontend-only, high-delight. Slot into `MessageInput.tsx` alongside the existing attachment / reply state.
