# Gitwise + Dev Box — Vision Spec

**Date:** 2026-04-28
**Status:** ⚠️ **SUPERSEDED 2026-04-29** by `2026-04-29-phase-a-ai-dev-platform.md`. Kept for background.
**Companion:** `2026-04-28-github-integration-design.md` (the foundation this builds on)

> **Status update (2026-04-29):** This document captured the original
> federation-style vision (gitwise as a peer service surfaced through a
> `GitwiseProvider` implementing `gitprovider.Provider`). After a strategic
> re-evaluation it was superseded by the Phase A spec, which:
>
> - **Reframes parley's positioning** from "Discord clone with selfbot
>   support and a deeper GitHub embed" to "AI dev platform built on a
>   social/voice substrate." parley's plausible user acquisition is as the
>   latter, not the former.
> - **Reorders the work.** Phase A (Dev Box voice activity on EXTERNAL
>   GitHub repos + skill-level-aware project setup + agent/preset/skill
>   framework) ships first to validate the pitch. Phase B (gitwise merge)
>   is gated on Phase A signal, since it nearly doubles parley's scope and
>   should not be undertaken speculatively.
> - **Replaces the federation model with a merge model.** When/if Phase B
>   happens, gitwise will be cherry-picked into parley's `internal/`
>   packages, not federated as a separate service. The features that
>   matter most (Dev Box wiring, AI-native data, PR review inside parley)
>   all want a shared data layer; federation is fighting yourself.
>
> The R1–R5 phasing below is **obsolete**. The architectural sketches for
> Dev Box (§5), the threat model, and the AI-native data-model rationale
> remain useful as background, which is why this document survives in the
> repo rather than being deleted.

This spec describes the longer arc of parley's developer-collaboration story. It is **not** a green-lit implementation plan. It exists to:

1. Make sure the GitHub integration spec we ship near-term doesn't paint us into a corner.
2. Capture the vision so future work has a single reference.
3. Surface the hard problems early so they can be sized when their phase lands.

---

## 1. Vision (one paragraph)

parley becomes the social layer over Gitwise. A user posts a Gitwise repo link the same way they post a GitHub link — same embed shape, same Explore takeover. Then it goes further: voice channels can host a **Dev Box activity**, an ephemeral shared development environment scoped to a repo. The dev box owner picks who has write privileges; everyone else watches. Each writer gets a sandboxed terminal with a per-user fork (or branch) and a preinstalled toolkit — `git`, `claude`, `node`, etc. When a writer is done, parley auto-files a PR back to the maintainer's repo. The maintainer reviews PRs without leaving parley, in the same Explorer pane. Discord has Poker Night; parley has *the entire software-development workflow* as a voice activity.

---

## 2. Layers in this story

The integration unfolds in four expanding rings. Each ring is independently shippable.

| Ring | Capability | Built atop | Effort |
|---|---|---|---|
| **R1 — GitHub unfurl + Explore** | Public repo embeds, read-only code browser | (foundation) | covered in `github-integration-design.md` |
| **R2 — Gitwise unfurl** | Gitwise repos work identically via `GitwiseProvider` | R1 | small |
| **R3 — Auth-linked providers** | OAuth-linked GitHub + Gitwise per-user; private repos; "Import to Gitwise" action | R2 | medium |
| **R4 — Dev Box voice activity** | Shared sandboxed terminal env in VC, owner-controlled write privileges, auto-PR | R3 | large |
| **R5 — Claude Code as VC activity** | Dev Box preset where every writer gets `claude` already attached to a per-user worktree | R4 | small once R4 ships |

R1 is in flight. R2 is essentially "implement `GitwiseProvider`." R3–R5 are the substantive new work and the rest of this spec covers them.

---

## 3. R2 — Gitwise as a provider

### 3.1 Why this is cheap

Gitwise's API surface (from `internal/api/handlers/`):

| Gitwise endpoint | parley `Provider` method |
|---|---|
| `GET /api/v1/users/{owner}/{repo}` | `GetRepo` |
| `GET /api/v1/repos/{owner}/{repo}/tree?ref=&path=` | `GetTree` |
| `GET /api/v1/repos/{owner}/{repo}/blob?ref=&path=` | `GetBlob` |
| `GET /api/v1/repos/{owner}/{repo}/releases` | `ListReleases` |

The mapping is 1:1. `GitwiseProvider` is a sibling of `GitHubProvider` in `internal/gitprovider/gitwise/`, configured via `GITWISE_BASE_URL` env (`https://gitwise.app/api/v1` for prod, `http://localhost:8080/api/v1` for dev).

### 3.2 What the frontend changes

Almost nothing. The link matcher gains a second pattern for `gitwise.app/{owner}/{repo}`. The embed component reuses `EmbedCard` with the Gitwise logo. The Explorer route gains `/explore/gitwise/{owner}/{repo}`. The `provider` URL parameter already plumbs through.

### 3.3 What the backend changes

A new file (`gitwise/client.go`) plus one switch in the provider registry. Caching is provider-keyed (`git:gw:...`) so it doesn't collide with GitHub.

### 3.4 Done when

A user pastes a Gitwise URL in chat → embed → Explore → reads code with Shiki highlighting. Identical UX to GitHub.

---

## 4. R3 — Auth-linked providers

### 4.1 Per-user OAuth tokens

Each parley user can link a GitHub identity and/or a Gitwise identity. parley stores the OAuth token encrypted at rest (existing `internal/auth` patterns). The provider client uses the linked token when the user makes a request — so private repos render for that user, and the rate-limit budget is the user's, not parley's egress IP.

Storage:

```sql
CREATE TABLE provider_links (
  user_id     BIGINT REFERENCES users(id),
  provider    TEXT NOT NULL,   -- 'github' | 'gitwise'
  provider_uid TEXT NOT NULL,  -- their login on the provider
  access_token BYTEA NOT NULL, -- encrypted
  refresh_token BYTEA,         -- encrypted, optional
  scopes      TEXT[] NOT NULL,
  expires_at  TIMESTAMPTZ,
  linked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, provider)
);
```

Cache keys for authed requests get the user_id mixed in: `git:gh:repo:{user_id}:{owner}:{repo}` — separate buckets so user A's view of a private repo doesn't leak to user B.

### 4.2 "Import to Gitwise"

A button on the GitHub embed (visible only when the user has linked Gitwise). Calls `POST /api/git/import` with `{from: github://owner/repo, to_org: "myorg"}`. Backend uses the user's GitHub token to clone, the user's Gitwise token to create the repo and push. Polled-status modal until done.

The backend code that does this is straightforward — the hard part is the UX (where does the imported repo land? what name? what does the destination user see?). Specced separately when R3 starts.

### 4.3 Permissions

The link is per-user. Servers cannot share linked accounts. When a user posts a private GitHub link, the embed renders only for users who have linked GitHub themselves with access to that repo (everyone else sees a placeholder "private — link your GitHub to view"). This is mildly annoying UX but it's the only correct answer.

---

## 5. R4 — Dev Box voice activity

This is the hard ring. It needs its own design doc when it starts; this section is scope and risk only.

### 5.1 Concept

A voice channel (server-scope or DM) gains a third panel: the **Activity** panel. Existing parley voice infra already has the slot for this (LiveKit + parley control plane). Dev Box is one activity; future activities (Poker, drawing apps, whatever) reuse the slot.

When a voice channel owner starts a Dev Box, they pick:
- A repo (any provider — GitHub or Gitwise the user has access to)
- Which VC participants get **write** access (default: only owner; everyone else is read-only viewer)
- Optional preset: blank, Claude Code, custom Dockerfile

Each writer gets:
- An ephemeral container with their fork/branch checked out
- A terminal in the parley UI (xterm.js → WebSocket → backend → `node-pty` or similar)
- Preinstalled tools (`git`, `gh`/`gwise` CLI configured with their linked token, optionally `claude`)

Read-only viewers see a **stream** of selected writers' terminals — pick a writer from a dropdown, see what they're typing.

When the writer's session ends (idle, kicked, voice channel closes), parley:
- Pushes their branch to their fork
- Auto-opens a PR back to the maintainer's repo if there are commits
- Posts a system message in the voice channel: "Alice's Dev Box session ended → PR #42 opened"

### 5.2 Architecture sketch

```
┌──────────────────────────────────────────────────────────────┐
│  parley API (Go)                                              │
│   ↓                                                            │
│  Dev Box service                                               │
│   ├─ session lifecycle (create / kick / end / idle reaper)    │
│   ├─ terminal proxy WS endpoint                                │
│   └─ PR-on-end worker                                          │
│   ↓                                                            │
│  Container orchestrator (Firecracker | gVisor | Docker)       │
│   ├─ per-user rootless workspace                               │
│   ├─ per-user clone of fork                                    │
│   ├─ resource caps (CPU, mem, disk, net)                       │
│   └─ network egress whitelist (provider domain + npm/pypi)    │
└──────────────────────────────────────────────────────────────┘
```

The orchestrator choice is the central question. Options ranked by isolation/complexity tradeoff:

| Option | Isolation | Cold start | Complexity | Notes |
|---|---|---|---|---|
| Docker-in-Docker rootless | Medium | ~3s | Low | Easiest; OK for trusted users only |
| gVisor | High | ~3s | Medium | User-space syscall sandbox; runs Docker images |
| Firecracker microVMs | Very high | ~150ms | High | What Fly.io uses; multi-tenant safe |
| Nested KVM | Highest | ~5s | Very high | Overkill |

Firecracker is the right answer if Dev Box ever runs untrusted code. For a friends-and-family launch, gVisor is fine.

### 5.3 Permissions / threat model

| Actor | Default | Risks |
|---|---|---|
| Owner | Read+write+admin | Trusted by definition |
| Writer | Read+write to their own session | Sandbox escape, network exfil, crypto-mining |
| Viewer | Read-only stream of one writer | Could screenshot |

Mitigations:
- Egress whitelist (provider API + package registries only — no random internet)
- Resource caps per session
- Idle reaper (30 min no activity → kill)
- Per-server quotas (max N concurrent dev boxes)
- Maintainer audit log of every Dev Box session that produced a PR

### 5.4 Cost

Containers cost money. If a server runs 10 Dev Box sessions for an hour, that's real compute. parley-the-product will need a billing model (per-server seat with included Dev Box hours? metered overage?). Not designed here. The spec just acknowledges that Dev Box is the first parley feature with non-trivial marginal cost — every prior feature is bandwidth-bound, this one is CPU/memory-bound.

### 5.5 Done when

A maintainer can: open a VC, start a Dev Box on their repo, grant a friend write access, watch the friend type, see a PR appear in the maintainer's GitHub/Gitwise queue when the friend leaves, review and merge it without leaving parley.

---

## 6. R5 — Claude Code as a VC activity

Once R4 is done, this is a Dev Box preset:

- Image: existing Dev Box base + `claude` CLI + per-user API key (or pooled team key)
- On session start: `claude --resume` in the terminal, with the repo already cloned + on a fresh branch
- Owner can broadcast a single shared `claude` session ("everyone watches one Claude work") OR give each writer their own
- Voice transcript optionally piped in as additional context (separate spec — needs careful UX and consent)

The point of calling this out: it's not a separate product. It's a preset on R4. The "Claude Code voice activity" framing is a marketing surface, not an engineering surface.

---

## 7. Risks and unknowns

1. **Sandbox security.** Single biggest risk. If R4 ships with weak isolation and a writer uses Dev Box to mine crypto on parley's hardware, the trust hit is permanent. Decide isolation tech *before* building R4 UI.
2. **Cost runaway.** Idle reaper + per-server quotas are mandatory, not optional.
3. **Provider rate limits at R3.** If 1000 users link GitHub and the embeds all fan out to per-user GitHub calls, parley becomes a friendly DDoS source against itself. Mitigation: cache aggressively per-user; let public-repo cache be shared as in R1.
4. **PR spam to maintainers.** Auto-opening PRs on session end is a footgun if a writer pushes garbage. Default to a "draft" PR; the writer must promote to ready manually. Maintainer can also disable auto-PR per Dev Box.
5. **Gitwise feature parity.** Gitwise is yours, but it has to actually support every feature parley wants to expose. R2 is small only because Gitwise's API is already tree/blob/commits-shaped. R3+ may require Gitwise-side work first (OAuth flow, fine-grained tokens).
6. **Mobile.** Dev Box on phones is nonsense. R4 is desktop-first; mobile users see "joined a Dev Box session" but can't interact. Accept this.
7. **Legal.** A user running their own code in parley's containers raises questions (TOS, abuse handling, DMCA on cached repo content, export controls on cryptographic code). Lawyer pass before R4 launch.

---

## 8. What this spec changes about R1

The R1 spec already accounts for everything this spec needs. Specifically:

- `Provider` interface is provider-agnostic by design. R2 drops in.
- Cache keys are provider-namespaced (`git:gh:` vs `git:gw:`). No collision later.
- The frontend `provider` parameter is already in the URL space.
- The Explorer route already supports deep links — R4's "open this repo in the dev box" is a navigation, not a redesign.

The only adjustment R1 should make for the future: **don't hardcode `github` anywhere user-facing**. The embed component should read its title/icon from the provider's metadata (so `GitwiseRepoEmbed` is a one-line variant, not a copy). The link matcher should be a registry, not an `if (host === 'github.com')`.

---

## 9. Phasing summary

| When | What | Gate |
|---|---|---|
| Now | R1 (github-integration-design.md) | none |
| R1 + 1 month | R2 (Gitwise unfurl) | R1 in production, no major regressions |
| R1 + 2-3 months | R3 (OAuth-linked + Import) | R2 stable; design pass on private-repo UX |
| R1 + 6+ months | R4 (Dev Box) | R3 shipped; isolation tech chosen; billing model decided |
| R1 + 7+ months | R5 (Claude Code preset) | R4 stable |

These are not commitments — they're the right *order*. Calendar slips when reality intrudes.
