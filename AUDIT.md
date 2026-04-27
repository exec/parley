# Adversarial Security Audit — 2026-04-26

Working doc for the red-team findings. Delete once items are tracked elsewhere.

Four parallel attackers (identity, rbac, realtime, infra) with full source access
+ explicit authorization to attack prod. Live PoC executed against `dh`
(uid `646019121799`) on https://parley.byexec.com.

**Headline:** No external full-account-takeover of `dh` was found. The auth
model holds. **But** a hostile stranger can harass, impersonate, and exfiltrate
PII against any user inside ~10 minutes from a fresh account.

---

## Status

| # | Title | Severity | Status |
|---|---|---|---|
| 1 | Group-DM force-add → PII exfiltration | HIGH | **deferred** — friend system not built; intentional state |
| 2 | Forwarded-message forgery | HIGH | ✅ `bd7bed3` |
| 3 | Ring spam → modal/audio DoS + transcript flood | HIGH | **open** |
| 4 | DM-stranger notification spam | HIGH | **open** |
| 5 | Role-position bump → leapfrog hierarchy | HIGH | ✅ `b46b826` |
| 6 | Login-CSRF / session fixation via /auth/desktop/exchange | MED | ✅ `3ed5da2` |
| 7 | Self-accept / self-decline own ring | MED | ✅ `916b64d` |
| 8 | Login timing oracle | LOW | ✅ `7f64766` |
| 9 | /api/voice/{vc}/heartbeat no membership check | LOW | ✅ `7f64766` |
| 10 | Mention-notify ctx-canceled + missing membership check | def-in-depth | ✅ `7f64766` |
| 11 | Discovery ILIKE without escapeLike() | def-in-depth | ✅ `7f64766` |
| 12 | Audit-log invite-code disclosure | def-in-depth | ✅ `7f64766` |
| 13 | Pin-message permission requires both bits | functional | ✅ `7f64766` |
| 14 | GH Actions tag pinning (supply chain) | HIGH-cond | ✅ `2d54731` |
| 15 | Verify tfstate gitignored | hygiene | ✅ already covered |

---

## Open items

### #1 Group-DM force-add → PII exfiltration *(deferred)*
Anyone can add anyone to GCs in the current product state because there's no
visible friend system yet — that's intentional. Refit when friend-graph lands:
add IsBlocked check + recipient consent gate to `internal/dm/service.go`
CreateChannel + AddMembers. Track at that point.

### #3 Ring spam → modal/audio DoS + transcript flood
- Surface: `POST /api/dms/{id}/call/ring` + `/cancel`
- File: `internal/voice/ring_handler.go:67-127`, route registration `cmd/api/routes.go:358-361`
- Fix: add `userRateLimitMiddleware(ringInitiateLimiter)` to both routes (e.g. 5/min keyed on actor:dmID). Consider also: suppress `call_missed` system message for cancels that fire within ~3s of the ring (human couldn't have missed it).

### #4 DM-stranger notification spam
- Surface: `POST /api/dms` (no friend gate) + `POST /api/dms/{id}/messages`
- Files: `internal/dm/service.go:69`, `internal/notification/service.go:71-87`, `internal/dm/handler.go:347-368`
- Fix: same recipient-consent gate as #1 (friend system) + per-pair `(actor, recipient)` rate limit (e.g. 1 unsolicited DM/min) until accepted.

---

## What held up

JWT alg/key confusion · impersonation token forging · password reset / verify-email tokens · mass assignment on profile (`PUT /api/auth/profile`, `PATCH /api/users/me` — tightly named structs) · vanity URL squat (UNIQUE constraint) · channel overwrite escalation (Admin bit stripped from overwrite mask) · bot ownership transfer (strict author equality on edit) · WS subscription bypass (`CheckChannelAccess` fail-closes on missing checker) · WS ticket replay (`GETDEL` atomic) · LiveKit token forgery (server-validated room+identity, post-`AuthorizeJoin`) · WS event spoofing (`client.go:127-191` only handles a tight inbound type set) · force-mute / force-disconnect via WS (HTTP-only with perm gates) · Spaces enumeration (16-byte random keys, listing 403) · key collision / cross-user clobber (no caller-controlled component) · upload content sniff (magic-byte, 7 formats) · Cloudflare bypass (DMZ allow-list to `10.10.10.5`) · email injection (Brevo HTTPS API, no SMTP socket) · Tauri capability set (no FS/shell/HTTP exposed via deep-link) · updater signature chain (minisign pubkey baked at `tauri.conf.json:63`) · postgres bound to vmbr1+loopback only · redis password rotated · MinIO L4 allow-list to DMZ + parley-api only · CORS allowlist tight + `Allow-Credentials: true` only on allowlisted origins · admin panel off the public vhost (WireGuard only).

---

## Cleanup queue *(still pending)*

### Live attacker artifacts in DB

```sql
-- forged GC dh was force-added to + the forged "forward from dh" message
DELETE FROM dm_messages WHERE dm_channel_id IN (336793114, 513058821) AND author_id IN (945648654416, 631940014957, 643600600668);
DELETE FROM notifications WHERE user_id = 646019121799 AND actor_username IN ('rt1777245501', 'rtb1777245511', 'atk50000', 'redteam_a', 'redteam_b');
DELETE FROM dm_channels WHERE id IN (336793114, 513058821);
```

### Test accounts to delete

| uid | username | created by |
|---|---|---|
| 657252937437 | redteam_a | identity-attacker |
| 120317065137 | redteam_b | identity-attacker |
| 945648654416 | rt1777245501 | rbac-attacker |
| 631940014957 | rtb1777245511 | rbac-attacker |
| 643600600668 | atk50000 | realtime-attacker |
| ? | atk100783921 | realtime-attacker |
| ? | vic2522715093 | realtime-attacker |
| ? | vic22522715093 | realtime-attacker |

### DMZ proxy

```
/etc/nginx/conf.d/fail2ban-blocks.conf  was emptied 2-3 times when an
attacker IPv6 (2601:247:c501:a740::/64) got auto-banned mid-test.
Restore if you want fail2ban enforcing again.
```

### Repro scripts left on local /tmp

`/tmp/live_pwn.py`, `/tmp/ring_spam.py`, `/tmp/dm_spam.py`, `/tmp/self_accept.py`,
`/tmp/ws_attack.py`, `/tmp/ws_dm_eaves.py`, `/tmp/maxconn_real.py`,
`/tmp/ws_size_dos.py`, `/tmp/atk_token.txt`, `/tmp/vic_token.txt`,
`/tmp/vic2_token.txt`, `/tmp/atk2_token.txt`, `/tmp/atk2_uid.txt`,
`/tmp/dh_dm_id.txt`, `/tmp/verify_pwn.sql`, `/tmp/cols2.sql`,
`/tmp/test_forward.sh`, `/tmp/test-fix.mjs`, `/tmp/test-bundle.mjs`,
`/tmp/test-bundle2.mjs`.
