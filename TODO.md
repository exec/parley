# TODO

## Slash commands — Phase 1 (shipped)

- Go backend: `internal/botcommands/` package (models, repository, service, handler, tests); migrations for `bot_commands`, `bot_interactions`, and `messages.kind`; `PermUseBotCommands` at bit 42; `INTERACTION_CREATE` WS event; in-process per-server command cache with write-invalidation; 50-upserts/hr rate limit on registration; background sweeper that expires pending interactions after 15min.
- HTTP API: bot CRUD (`GET/PUT/POST /api/bots/@me/servers/{id}/commands`, `DELETE .../{name}`), user list (`GET /api/servers/{id}/commands`, joined with user table so each command carries `bot_username/display_name/avatar_url`), user invoke (`POST /api/channels/{channelID}/interactions`), and bot respond (`POST /api/interactions/{token}/respond`) gated by an `InteractionTokenAuth` middleware.
- Frontend: `/` autocomplete in `MessageInput.tsx` modeled on the existing mention/channel/emoji pattern; new `SlashCommandDropdown`, `useSlashCommands` hook (module-scope cache, 30s stale-while-revalidate), `api/slashCommands.ts` client, in-place option picker supporting STRING/INTEGER/BOOLEAN (choices render as `<select>`, required options show an asterisk).
- Python SDK: `@bot.slash_command(name, description, options)` decorator + `SlashOption` + `InteractionContext.respond()` (blanks `Authorization` header so the token-auth route isn't overridden); auto-registers commands on `READY` per joined server; dispatches `INTERACTION_CREATE` to the registered handler with options spread as kwargs.

## Slash commands — Phase 2+ (not yet shipped)

- **Embeds** (Phase 2): `message_embeds` table + `<Embed>` renderer, bot API for attaching embeds, `attachment://name` scheme, tighter limits than Discord (2000 desc / 10 fields / 4 embeds / 3000 char sum).
- **Components + native user-lock + ephemeral** (Phase 3): `message_components` table with `locked_to_user_id`, button + string/user/role/channel select renderers, `COMPONENT_INTERACTION_CREATE` WS event, backend-enforced user-lock that rejects non-invoker clicks with a canned ephemeral response (the main developer-ergonomics win over Discord), `flags.ephemeral` on responses with invoker-only fanout.
- **Defer + follow-up** (Phase 2.5): `POST /api/interactions/:token/respond` currently only supports synchronous reply. Add `type:"defer"` ACK that marks state=`deferred`, a `PATCH /api/interactions/:token/response` for editing the deferred reply, and `POST/PATCH/DELETE /api/interactions/:token/followups` for additional messages. Discord's 3s deadline is the hard cap for the initial ACK.
- **Offline / degraded bot UX in the dropdown**: commands from an offline bot render disabled with "Bot is offline"; degraded bots render with an amber dot + tooltip but stay selectable. Hook into bot status + the existing `set_degraded(True)` SDK method.
- **Docs rewrite** (Phase 4): expand `docs/bots.md` with a slash-commands section covering registration, option types, the respond endpoint, and a full Python example. Mirror the contract table from the design doc.

## Slash commands — v2

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
