# TODO

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
- **Built-in `/help` command** — backend-generated, ephemeral by default, lists every command the invoker can use in the current channel. (Decide in v1 whether to ship this or leave it to bots; if deferred, land it here.)
- **Raise the 25-commands-per-bot-per-server cap toward 100** — once we have real usage data showing bots bumping the cap.
- **Richer embed fields** — `video`, `provider` (read-only provider metadata for URL unfurls), native URL-unfurl pipeline that generates embeds for pasted links server-side.
- **Component `link` buttons** — no `custom_id`, just a URL; doesn't create an interaction. Trivial to add but intentionally deferred to keep v1 component types minimal (primary/secondary/success/danger only).
- **Component `premium` / monetization buttons** — not applicable unless Parley adds a monetization layer.
- **Interaction rate-limit tuning** — Discord does 5 req / 2s per interaction token, 30/60s globally. v1 inherits the existing `msgWriteLimiter`; revisit once we see real invocation patterns.
- **HTTP-webhook delivery transport** — fallback delivery mode for bots that would rather receive interactions at a registered URL than hold a WebSocket open. v1 delivers over the bot's existing WS connection.
- **Python SDK: cog-style slash command groups**, parity with the existing prefix `CommandBot` cog system.
- **TypeScript bot SDK** — the Python SDK is the only first-party SDK today. Once slash commands stabilize, mirror the API in TS for Node bots.

## Other

- **Per-channel message drafts** (auto-save) — localStorage-backed draft per (channel | DM) so typed-but-unsent text survives navigation. Small, frontend-only, high-delight. Slot into `MessageInput.tsx` alongside the existing attachment / reply state.
