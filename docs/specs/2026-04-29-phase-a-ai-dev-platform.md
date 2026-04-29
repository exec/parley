# Phase A — Parley as an AI Dev Platform

**Date:** 2026-04-29
**Status:** Draft (pre-implementation)
**Supersedes:** the R1–R5 ordering in `2026-04-28-gitwise-devbox-vision.md`
**Companion:** `2026-04-28-github-integration-design.md` (the foundation Phase A builds on)

This spec captures the strategic pivot, scope, and decomposition for
Phase A. Phase B (the gitwise merge) is intentionally not specced here —
it is gated on Phase A signal and gets its own spec when/if Phase A
validates the pitch.

---

## 1. Strategic frame

Parley is **not** a Discord competitor. The chat + voice + selfbot
infrastructure parley already has is the *substrate* for a different
product: an opinionated AI dev platform aimed at the rising population of
people who want to build ambitious software but lack the technical
fundamentals to configure a dev environment from scratch.

The premise: 11-year-olds are shipping deployed dynamic websites and
Minecraft-likes via natural-language prompts to AI tools. They don't know
git, ssh, or command line. They DO know what they want to build. As this
becomes mainstream, the missing-product-shape is a place that:

1. Takes the user's project intent — in their own words, at their
   self-reported skill level — and produces an opinionated dev environment
   plus an agent/skill framework calibrated to that level.
2. Hosts collaborative dev sessions with AI as a first-class participant
   in voice channels (not as a webhook bot or a chat assistant in a
   separate window).
3. Manages software-development workflow primitives (branching, PRs,
   review) at an abstraction the user can actually engage with.

Why parley specifically: the social/voice substrate matters because
software development is social — but every existing tool fails this in
one direction or the other. Cursor and Claude Code in isolation are solo.
Discord with bot integrations is social but not dev-aware. GitHub
Copilot Workspace is dev-aware but not collaborative in real time.
Parley already has the social half (chat, voice, server model, selfbots,
themes, the GitHub embed we just shipped) — Phase A leans into the
dev-platform half until the product's identity flips.

**Phase A goal:** prove this pitch with the smallest deliverable that
exercises the differentiation. If Phase A lands, Phase B (merging gitwise
in for first-class repo + PR + issue support) is the natural next step.
If Phase A doesn't land, Phase B is killed and a lot of scope was saved.

## 2. Non-goals (Phase A)

- **No gitwise merge.** External GitHub repos via the existing gitprovider
  integration are the only repo source.
- **No multi-user write in the Dev Box.** Single-user terminal sessions
  with VC participants as read-only viewers. Multi-tenant write requires
  hardened sandbox isolation that gates Phase A.2.
- **No user-installable skills / no MCP-server marketplace.** Built-in
  skills only in V1 — community contributions wait until A4.2.
- **No private-repo support.** Waits for OAuth (P4 of the GitHub spec or
  the Phase B merge, whichever lands first).
- **No PM-shareable presets.** Phase A ships a fixed set of built-in
  presets; user-authored presets and a sharing model are A4.2.
- **No PR review / issue tracking / code search inside parley.** Those
  are Phase B features; until then users still review on GitHub.
- **No billing model.** Dev Box runs in closed beta or per-server quota in
  V1; pricing is a Phase A.5 concern, not a coding concern.

## 3. Decomposition

Four primitives. They compose into the user-visible feature; each can be
specced and shipped separately under Phase A.

### A1 — Projects as a parley primitive

A "parley project" is a new entity owned by a server with:

- Name + description (markdown, short)
- A linked external repo (provider + owner + repo, references the
  existing `gitprovider` abstraction)
- A `CLAUDE.md` (markdown, editable, the heart of the project's agent
  configuration)
- A preset reference (which template the project was created from)
- A self-reported skill level (`beginner` | `intermediate` | `expert` |
  `auto` | `custom`)
- An optional associated voice channel (where the Dev Workspace activity
  attaches when a session is active)

#### Schema (Migration #72)

```sql
CREATE TABLE projects (
  id              BIGSERIAL PRIMARY KEY,
  server_id       BIGINT      NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  name            VARCHAR(80) NOT NULL,
  description     TEXT        NOT NULL DEFAULT '',
  claude_md       TEXT        NOT NULL DEFAULT '',
  skill_level     VARCHAR(16) NOT NULL DEFAULT 'auto'
                    CHECK (skill_level IN ('beginner','intermediate','expert','auto','custom')),
  preset_id       INT         REFERENCES project_presets(id) ON DELETE SET NULL,
  vc_channel_id   BIGINT      REFERENCES channels(id) ON DELETE SET NULL,
  owner_user_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at      TIMESTAMP   NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMP   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_projects_server     ON projects(server_id);
CREATE INDEX idx_projects_owner      ON projects(owner_user_id);
CREATE INDEX idx_projects_vc_channel ON projects(vc_channel_id) WHERE vc_channel_id IS NOT NULL;

CREATE TABLE project_repos (
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  provider   VARCHAR(16) NOT NULL,                 -- 'github' for V1
  owner      VARCHAR(100) NOT NULL,
  repo       VARCHAR(100) NOT NULL,
  PRIMARY KEY (project_id, provider, owner, repo)
);

CREATE TABLE project_skills (
  project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  skill_id   VARCHAR(64) NOT NULL,                 -- matches built-in registry slugs
  PRIMARY KEY (project_id, skill_id)
);

CREATE TABLE project_presets (
  id          SERIAL PRIMARY KEY,
  slug        VARCHAR(48) UNIQUE NOT NULL,         -- 'web-app', 'discord-bot', etc.
  name        VARCHAR(80) NOT NULL,
  description TEXT        NOT NULL DEFAULT '',
  is_builtin  BOOLEAN     NOT NULL DEFAULT TRUE
);
-- Seed inserts for built-in presets happen in the same migration; see A4.
```

Same `'custom'`-as-CHECK pattern as `user_preferences.active_theme` —
keep the allowlist tight so freeform skill levels can't leak into the
column.

#### Permission model

| Action | Required permission |
|---|---|
| Create project | `MANAGE_CHANNELS` on the server (or server owner) |
| Update / delete project | project's `owner_user_id` OR `MANAGE_CHANNELS` |
| Edit CLAUDE.md | same as update |
| View project | `VIEW_CHANNEL` on any channel in the server (i.e. server members) |

`MANAGE_CHANNELS` is the closest existing perm — projects feel like a
channel-class primitive, not a user-class one. If demand emerges for a
finer-grained `MANAGE_PROJECTS` permission later, add the bit then.

#### API endpoints

All under `/api`, all behind the standard auth middleware.

| Method | Path | Scope | Purpose |
|---|---|---|---|
| POST   | `/projects`              | `profile:write` + perm check | Create project |
| GET    | `/servers/{id}/projects` | `servers:read`               | List server's projects |
| GET    | `/projects/{id}`         | `servers:read`               | Detail (incl. CLAUDE.md inline) |
| PATCH  | `/projects/{id}`         | `profile:write` + perm check | Update name/desc/skill/vc/repo |
| PATCH  | `/projects/{id}/claude-md` | `profile:write` + perm check | Edit CLAUDE.md (own endpoint to make WS broadcasting cleaner) |
| DELETE | `/projects/{id}`         | `profile:write` + perm check | Delete |

Bot scope mapping follows the same pattern as servers/channels in
`cmd/api/routes.go`. A `messages:read`-only bot can't see projects;
a `servers:read` bot can see metadata but not edit; a `profile:write`
bot can manage its own projects.

#### WebSocket events

All scoped to the `server:{id}` topic so members of the server receive
them; non-members never see project state.

| Event | Payload |
|---|---|
| `PROJECT_CREATE` | full project record |
| `PROJECT_UPDATE` | full project record (last-write-wins on the client) |
| `PROJECT_DELETE` | `{id, server_id}` |

The CLAUDE.md edit explicitly does NOT broadcast on every keystroke —
only on the PATCH commit. Live collaborative editing of CLAUDE.md is out
of scope for V1.

#### Frontend

- New "Projects" entry in the server-channel sidebar, between the channel
  list and the voice-channel section. Renders as a collapsible group
  (matches the existing channel-group visual).
- Create-project wizard (modal, 5 steps: preset → repo → skill → describe
  → review CLAUDE.md). Stays open with "Save draft" support so a slow
  synthesis call doesn't block the user.
- Project detail view: CLAUDE.md rendered as markdown via the existing
  `MarkdownRenderer`, with an "Edit" button that swaps to a textarea +
  "Save" / "Cancel". Linked repo card uses `GitHubRepoEmbed` to render
  the same embed as in chat. Linked VC link if set.
- Pages route: `/servers/:serverId/projects/:projectId` (deep-linkable
  same as the explorer).

Files (frontend new):
- `frontend/src/api/projects.ts`
- `frontend/src/components/projects/ProjectList.tsx` (sidebar group)
- `frontend/src/components/projects/CreateProjectWizard.tsx`
- `frontend/src/components/projects/ProjectView.tsx`
- `frontend/src/components/projects/*.css`

Files (backend new):
- `internal/projects/service.go`
- `internal/projects/repository.go`
- `internal/projects/handler.go`
- Migration #72 entry in `internal/db/migrations.go`

### A2 — Skill-level-aware setup + synthesis agent

The create-project wizard:

1. **Pick a preset** (Web App, Discord Bot, Python Script, Static Site,
   Backend API, or Custom — see A4)
2. **Link a repo** (GitHub via the existing integration; or "I'll link
   one later" — V1 supports a placeholder repo state)
3. **Skill slider:** Beginner / Intermediate / Expert / Auto / Custom
4. **Freeform:** "Describe what you want to build" (multi-line markdown,
   ~500-2000 char target)
5. **→ Synthesis agent** runs server-side: takes (preset, skill,
   freeform) → produces a tailored `CLAUDE.md`
6. **Preview the output**, edit if desired, save

Synthesis agent = an LLM call from the parley backend behind a provider
interface (Ollama for V1, Anthropic-ready — see open question 2 below)
with a carefully tuned system prompt that teaches the model to:

- Map skill levels to verbosity / hand-holding / risk-aversion
  (`beginner` → "always run tests, ask before destructive ops, explain
  unfamiliar terms inline"; `expert` → "skip the tutorials, prefer
  one-line answers, assume context")
- Honor preset-specific norms (Web App preset emphasizes Next.js+TS
  conventions, Discord Bot emphasizes the parley selfbot pattern)
- Integrate the user's freeform description into the result without
  letting it fight the preset's structural decisions

`custom` skill level is special: the user types their own one-shot
preamble, which gets synthesized into the standard CLAUDE.md shape via
the same agent (so it integrates with preset norms instead of replacing
them wholesale).

### A3 — Dev Workspace voice activity (external repos, single-user)

Voice channels gain an "Activity" panel — a third pane alongside the
existing voice + chat-sidebar layout. First activity: **Dev Workspace**.

Flow when a VC participant starts a Dev Workspace and the channel has a
linked project:

1. Backend allocates an ephemeral container (sandbox tech: gVisor for V1,
   see open question 1).
2. The container clones the project's linked repo + drops the project's
   CLAUDE.md at `/workspace/CLAUDE.md` + has `claude` preinstalled.
3. A terminal pane appears in the VC for that participant
   (xterm.js client + `node-pty` server, multiplexed over the existing
   parley WebSocket).
4. Other VC participants see "Alice is in the Dev Workspace" and can
   subscribe to view a **read-only stream** of her terminal.
5. Session ends on participant leave-VC OR explicit "end session" → the
   container is destroyed, any commits remain on her fork (no auto-PR
   in V1, see Non-goals).

Resource caps per session:
- 2 vCPU, 4 GiB RAM, 10 GiB disk
- 30-min idle reaper (no keystrokes for 30 min → kill)
- Egress whitelist: parley.byexec.com + GitHub + npm + pypi + crates.io +
  Anthropic API (for `claude`)
- Per-server concurrent quota (default 1 simultaneous Dev Box per server
  in V1; configurable later)

### A4 — Built-in skill / preset framework

V1 ships with:

- **~6-8 built-in presets:** "Web App (Next.js + TS)", "Discord-style
  Bot (Python)", "Python Script", "Static Site (Astro)", "Backend API
  (Go)", "Mobile App (Expo)", "Custom (no scaffold)" + 1 more TBD.
- **~10-15 built-in skills:** the Anthropic superpowers set (brainstorming,
  TDD, debugging, writing-plans, executing-plans, requesting-code-review,
  verification-before-completion, systematic-debugging, etc.) bundled into
  parley as project-attachable modules.

Presets are Go modules (or a structured JSON registry) under
`internal/projects/presets/` — not user-editable in V1. Each emits:
- A CLAUDE.md template with `{{user_description}}` + skill-level-aware
  sections
- A skill list to attach to the project by default
- A "scaffolding suggestion" payload (file list with bodies) the user
  reviews and optionally pushes to their repo

Custom presets where PMs save / share configurations: deferred to A4.2.

## 4. Phasing inside Phase A

| Slice | Scope | Estimate | Validates |
|---|---|---|---|
| **A1.0** | `projects` schema + CRUD endpoints, create wizard with manual CLAUDE.md (no synthesis yet), list + detail UI | 1-2 weeks | Data model is right; projects feel native to parley |
| **A2.0** | Synthesis agent + skill slider, presets data model, 4 built-in presets | 1-2 weeks | Skill-aware CLAUDE.md generation is actually useful |
| **A3.0** | Dev Workspace activity: container orchestration spike, single-user no-write terminal, viewer stream | 3-4 weeks | The voice-channel-shared-dev-env vision works in practice |
| **A4.0** | Remaining presets, ~10-15 built-in skills, skill-attach UI | 1 week (parallelizable with A3) | The framework feels like a real product, not a toy |
| **A0 — pitch** | One-sentence elevator pitch, demo video, landing-page rewrite | (concurrent, your job) | The differentiation is communicable |

Total Phase A: **~2 months calendar.**

**A1+A2 together is the smallest go/no-go gate.** If the synthesis flow
doesn't feel valuable in private-beta testing — i.e., the generated
CLAUDE.md doesn't materially outperform a user starting from scratch —
kill A3 and reconsider before sinking 3-4 weeks into container
orchestration.

## 5. Open questions (decide before coding)

1. **Sandbox tech for A3.** gVisor vs Firecracker vs rootless
   Docker-in-Docker. Recommendation: gVisor for V1 (fast cold-start, OK
   isolation for friend-and-family launch, runs Docker images directly).
   Firecracker if Phase A graduates to genuinely multi-tenant write.
2. **Synthesis-agent provider.** **Locked: Ollama for V1, Anthropic-ready
   abstraction.** Build `internal/synthesis/` with a `Provider` interface
   so the call site is provider-agnostic; ship the Ollama implementation
   first (matches the existing theme-generation stack), keep an Anthropic
   implementation behind the same interface as a flip-the-flag upgrade
   when funding/quality demands it. Why Ollama now: no `ANTHROPIC_API_KEY`
   yet (claude.ai account, not API), and dogfooding our own infra is a
   strength during private beta. The interface keeps the option open.
   Implication: V1 quality is bounded by the local model — accept that
   trade and re-evaluate at Phase A go/no-go.
3. **Project ↔ Server cardinality.** **Locked: 1 project = 1 server
   (1:N, no cross-server projects)** for V1. Revisit if cross-server
   collab becomes a real ask. Schema enforces this via the single
   `server_id` FK on `projects`.
4. **Project ↔ VC cardinality.** Does a project always have a VC, or can
   it exist without one? Recommendation: VC is optional; the Dev
   Workspace activity is a per-VC override. Same project can be linked
   from multiple VCs over its life.
5. **Pricing for Dev Box.** Containers cost real money. We need an
   approach to bill or hard-cap usage before A3 ships beyond closed
   beta. Phase A-internal note; does not block coding A1/A2.
6. **CLAUDE.md edit history.** **Locked: yes, versioned** using the same
   `version_history` pattern as bin posts. Add a `project_claude_md_versions`
   table (or equivalent — match the exact bin-posts shape) in Migration #72.
   Synthesis-generated original preserved as v1; every PATCH appends.

## 6. Files this spec touches

**New (backend):**
- `internal/projects/` — service, repository, handler
- `internal/synthesis/` — `Provider` interface + Ollama impl (V1) +
  Anthropic impl (built but unwired) + system prompt for CLAUDE.md
  synthesis
- `internal/devbox/` — container orchestration, terminal proxy, lifecycle
  (only when A3 lands)
- `internal/projects/presets/` — built-in preset registry (Go modules)
- `internal/skills/` — built-in skill registry

**New (frontend):**
- `frontend/src/components/projects/` — wizard, list, detail view
- `frontend/src/components/devbox/` — activity panel, xterm viewer (A3)
- `frontend/src/lib/skills/` — skill registry mirror (UI display only)
- `frontend/src/api/projects.ts` — typed client

**New (DB):**
- Migration #72 (or whatever's next): `projects`, `project_repos`,
  `project_skills`, `project_presets` schema + seed data

**Modified:**
- `cmd/api/routes.go` — mount `/api/projects/*` endpoints
- `cmd/api/main.go` — wire synthesis service + (later) devbox service
- Server-channel-sidebar UI — new "Projects" entry alongside text + VC
- Voice activity panel UI — Dev Workspace as the first activity
- `terraform/userdata-api.sh` — no env changes for V1 (Ollama is already
  reachable from api LXC); `ANTHROPIC_API_KEY` deferred until provider
  flip happens.

## 7. Out of scope but adjacent

These will come up; flagging now so they don't surprise us:

- **The vision doc's R3 (auth-linked providers)** still applies later for
  GitHub OAuth + private repos. Phase A doesn't need it.
- **Import to Gitwise** stays a Phase B+ concern (and may simplify if the
  merge model means "import to Gitwise" becomes "create a parley repo
  from this GitHub URL" — still future work).
- **Mobile.** Dev Workspace on phones is nonsense. A3 is desktop-first;
  mobile users will see a "Dev Workspace started" status and the chat
  side-panel, but no terminal. Same as today's voice UX trade-off.
- **TOS / abuse.** Running user-supplied code in our containers (even
  just `claude` shell sessions) opens TOS surface area. Lawyer pass
  required before A3 launches publicly.

## 8. What success looks like

End of Phase A, a friends-and-family user can:

1. Open a parley server.
2. Click "Create Project," pick a preset (e.g. "Discord-style Bot"),
   describe what they want to build in one paragraph, set their skill
   level to "Beginner," and link their GitHub repo (or skip the repo).
3. Read the generated CLAUDE.md, edit if needed, save.
4. Hop in the project's voice channel.
5. Start a Dev Workspace; have Claude Code waiting in the terminal,
   already on the right branch, with the right CLAUDE.md loaded.
6. Build something. Friends in the VC watch the terminal, chat about
   what's happening, suggest things in voice.

If that experience makes the user say "this is the first time
collaborative coding has actually felt natural," Phase A worked. If they
shrug or say "I'd just use Cursor and Discord separately," Phase A
didn't. That's the gate before Phase B.

---

## 9. Kickoff for A1.0 (cold-start checklist)

For the next session — what to do first, in order, to start A1
implementation. Each is roughly a session-block of work; check off as
they land.

**Decisions to lock first** (15 min, no code):
- [x] Question §5.2 (synthesis-agent provider): **Ollama for V1 behind
      a `Provider` interface; Anthropic impl built alongside but unwired
      until funding/quality flip.** No `ANTHROPIC_API_KEY` in terraform
      yet.
- [x] Question §5.3 (project ↔ server cardinality): **1:N (one server,
      many projects, no cross-server)** for V1. Single `server_id` FK
      on `projects` enforces.
- [x] Question §5.6 (CLAUDE.md edit history): **yes, versioned** —
      `project_claude_md_versions` table mirrors the bin-posts pattern.

**Backend (in order):**
- [x] Migration #72 — projects + project_repos + project_skills +
      project_presets + project_claude_md_versions + 7 seeded V1 presets.
- [x] `internal/db/project_repository.go` — Postgres CRUD + version
      snapshotting in tx (no separate per-package repo file; matches
      the parley convention of per-domain files in `internal/db/`).
- [x] `internal/projects/service.go` — perm-gated business layer with
      hub broadcasting (uses `permissions.HasPermission` with
      `PermManageChannels` for mutations; server-membership check for view).
- [x] `internal/projects/handler.go` — HTTP handlers per the API table.
- [x] Wired into `cmd/api/routes.go` under the protected group; scope
      gates per A1 §API endpoints (servers:read for reads, profile:write
      for mutations).
- [x] `PROJECT_CREATE/UPDATE/DELETE` event constants in
      `internal/websocket/events.go`; broadcast on `server:{id}` topic.
- [x] Tests: `internal/projects/service_test.go` — sentinel-error
      distinctness + validation paths (matches `internal/bin/service_test.go`
      shape; no DB integration tests at this layer).

**Frontend (in order):**
- [x] `frontend/src/api/projects.ts` typed client + types in
      `frontend/src/api/types.ts`.
- [x] WS handler in App.tsx — dispatches `parley:project_*` CustomEvents
      that ProjectsPage listens for (matches the call-events pattern).
- [x] Sidebar entry: small "Projects" button below the server header
      in `ChannelList.tsx` (full collapsible group deferred — single
      button is enough for V1 discoverability; tracks `projectsActive`).
- [x] `CreateProjectWizard.tsx` with manual CLAUDE.md textarea.
- [x] `ProjectView.tsx` with view + edit modes.
- [x] `ProjectsPage.tsx` is the page-level container; URL-driven via
      `/servers/:id/projects[/:pid]` (mirrors the explorer takeover
      pattern in App.tsx).

**Smoke test gate (manual):** ✅ **PASSED 2026-04-29.**
- [x] Create a project from the wizard with a manual CLAUDE.md.
- [x] Verify it appears for other server members in real time (WS broadcast).
- [x] Edit the CLAUDE.md, refresh — persists.
- [x] Delete the project — disappears for everyone.

A1.0 shipped to prod 2026-04-29 (commit `3be3cc1`). Theme-variable polish
on the projects UI deferred — uses hardcoded fallbacks today, not yet
calibrated against user themes. Acceptable for closed beta. Then A2.0 layers the
synthesis agent on top — the wizard's manual textarea gets replaced by
the skill slider + freeform input + synthesis call → review-and-edit
flow.

A2.0's first commit should be `internal/synthesis/` — the `Provider`
interface, the Ollama implementation (used by the wizard immediately),
and the Anthropic implementation alongside it (compiled, tested, but
unwired in `cmd/api/main.go` until funding/quality flip). See §6.

## 10. Pointers for cold context

If you're picking this up in a fresh session:

- This spec is the source of truth for Phase A. Read it whole; it's
  ~1.5k lines of markdown.
- Companion specs: `2026-04-28-github-integration-design.md` (the
  gitprovider abstraction Phase A leans on) and
  `2026-04-28-gitwise-devbox-vision.md` (background, marked superseded).
- The `internal/bin/` package is the closest existing pattern to project
  for backend shape (entity owned by a channel, with versioned content,
  WS broadcasting, handler tests). Use it as the template.
- The `frontend/src/components/bin/` directory is the closest pattern for
  frontend shape (sidebar group + create modal + detail view).
- Theme variables, the explorer, and the GitHub embed are all live in
  prod (commit `7f0a6a9` and after). Don't re-implement those.
- Existing `MarkdownRenderer` in `frontend/src/components/ui/` should be
  reused for CLAUDE.md preview — same component the chat uses for
  message bodies and bin descriptions.
