# F1 — Backend ingress gating runbook

Fixes the HIGH F1 finding from `docs/security/2026-04-23-adversarial-audit.md`: every parley backend on `vmbr1` (api CT 102, admin CT 100, MinIO CT 103) accepts HTTP from any other vmbr1 host, bypassing Cloudflare, the WAF, fail2ban, and the CF-IP gate.

**Scope of this runbook:** remediation against the already-running containers on Proxmox. For fresh deployments, the terraform userdata scripts now provision the gate; this runbook is only for the existing prod.

**Who runs this:** infra lead with Proxmox host `pct enter` access (or root SSH to each CT) and the ability to restart CTs if NET_ADMIN needs to be added to CT 103.

**The allow-listed source IP** is `10.10.10.5` — `dmz-proxy` on `vmbr1`. All legitimate traffic (Cloudflare → DO cloud firewall → DMZ nginx) originates there. The api container (`10.10.10.11`) is additionally allow-listed on MinIO for object PUT/GET.

---

## Pre-checks (before any change)

Run these from `dmz-proxy` (CT 106) — they should all currently succeed, confirming the finding is live:

```
# API CT 102
curl -sI http://10.10.10.11/health         | head -1   # expect 200
curl -sI http://10.10.10.11/api/discover   | head -1   # expect 200

# Admin CT 100 — BOTH nginx :80 and Go :8080 are directly reachable
curl -sI http://10.10.10.15/              | head -1   # expect 200 (nginx SPA)
curl -sI http://10.10.10.15:8080/         | head -1   # expect 200 (Go SPA — the audit case)

# MinIO CT 103
curl -sI http://10.10.10.21:9000/minio/health/live | head -1   # expect 200
```

Then run the same four commands from **any non-allowed vmbr1 host** (e.g. `gitwise-app` at `10.10.10.31`, or if you're testing from the Proxmox host itself, from `vmbr1` by `ssh root@10.10.10.30`). They should also succeed right now — that's the bug.

Record the outputs. They must invert after applying this runbook (200s from dmz-proxy stay 200; 200s from non-dmz hosts become 403 / connection-refused).

---

## Step 1 — Gate parley-api (CT 102)

Target the container-local nginx SPA + /api proxy at `:80`. The Go API at `127.0.0.1:8080` is already loopback-only so no separate gate is needed there.

On `pct enter 102` (or `ssh root@10.10.10.11`):

```
# Edit the existing nginx site; insert `allow 10.10.10.5; deny all;`
# at the top of the `server { ... }` block (above `location /`).
sed -i '/^    root \/var\/www\/parley;$/a\
\
    # F1: only accept ingress from dmz-proxy.\
    allow 10.10.10.5;\
    deny all;' /etc/nginx/sites-available/parley-api

# Verify the insertion visually before reloading.
grep -A 2 'allow 10.10.10.5' /etc/nginx/sites-available/parley-api

nginx -t && systemctl reload nginx
```

Alternative: copy the updated terraform/userdata-api.sh nginx block by hand into `/etc/nginx/sites-available/parley-api` (the file already exists on the container — this is identical to re-running the userdata).

**Verify (from dmz-proxy 10.10.10.5):**
```
curl -sI http://10.10.10.11/health | head -1        # expect HTTP/1.1 200 OK
curl -sI http://10.10.10.11/api/discover | head -1  # expect 200
```

**Verify (from any other vmbr1 host, e.g. 10.10.10.31 or the Proxmox host):**
```
curl -sI http://10.10.10.11/health | head -1        # expect HTTP/1.1 403 Forbidden
```

**Rollback:** remove the `allow/deny` lines, `nginx -t && systemctl reload nginx`.

---

## Step 2 — Gate parley-admin (CT 100)

Two gates needed: the nginx SPA at `:80` AND the Go server at `:8080` (which the audit reached directly, bypassing nginx).

On `pct enter 100` (or `ssh root@10.10.10.15`):

```
# --- 2a. Add allow-list to nginx :80
sed -i '/^    server_name _;$/a\
\
    # F1: only accept ingress from dmz-proxy.\
    allow 10.10.10.5;\
    deny all;' /etc/nginx/sites-available/parley-admin

grep -A 2 'allow 10.10.10.5' /etc/nginx/sites-available/parley-admin
nginx -t && systemctl reload nginx

# --- 2b. Rebind the Go admin to 127.0.0.1 only.
# The updated server.go honors ADMIN_BIND_LOCAL=1. Set it in the env file
# and restart the service. Also need the new binary with the bind-local
# code path.
grep -q '^ADMIN_BIND_LOCAL=' /etc/parley/admin-env || echo 'ADMIN_BIND_LOCAL=1' >> /etc/parley/admin-env

# Deploy the new admin binary from main branch after merge.
cd /parley
git fetch origin main
git checkout main
git pull
go build -o /usr/local/bin/parley-admin ./cmd/admin
systemctl restart parley-admin

# Confirm the listening socket moved from *:8080 to 127.0.0.1:8080.
ss -ltnp | grep :8080
# expect: LISTEN 0 ... 127.0.0.1:8080 ... users:(("parley-admin",...))
# NOT:   LISTEN 0 ... 0.0.0.0:8080
```

**Verify (from dmz-proxy 10.10.10.5):**
```
curl -sI http://10.10.10.15/           | head -1   # expect 200 (nginx SPA)
curl -sI http://10.10.10.15:8080/      | head -1   # expect: connection refused
                                                    # (Go now loopback-only)
# API traffic still works because it flows through nginx:
curl -sI http://10.10.10.15/api/users  | head -1   # expect 401 (auth required) — NOT 403
```

**Verify (from any other vmbr1 host):**
```
curl -sI http://10.10.10.15/           | head -1   # expect 403 (nginx deny)
curl -sI http://10.10.10.15:8080/      | head -1   # expect: connection refused
```

**Rollback:**
- `sed -i '/allow 10.10.10.5;/,/deny all;/d' /etc/nginx/sites-available/parley-admin && nginx -t && systemctl reload nginx`
- `sed -i '/^ADMIN_BIND_LOCAL=/d' /etc/parley/admin-env && systemctl restart parley-admin`

---

## Step 3 — Gate parley-minio (CT 103)

MinIO does not speak source-IP allow-lists natively, so we gate at L4 via iptables. The parley-api container (10.10.10.11) must stay allow-listed because it PUTs uploaded objects; dmz-proxy (10.10.10.5) must stay allow-listed because it serves CDN reads to Cloudflare origins.

**Prerequisite — NET_ADMIN capability inside the LXC.** Unprivileged LXCs drop `NET_ADMIN` by default. Without it, `iptables -A INPUT ...` fails with `Operation not permitted` and no rule is applied. Check from a Proxmox shell:

```
# On the Proxmox host:
pct exec 103 -- iptables -L INPUT -n 2>&1 | head -5
```

If that outputs `iptables: Permission denied` or similar, add NET_ADMIN to the container config and restart it:

```
# On the Proxmox host:
grep -q '^lxc.cap.keep' /etc/pve/lxc/103.conf || echo 'lxc.cap.keep: NET_ADMIN' >> /etc/pve/lxc/103.conf
pct restart 103
```

Then on `pct enter 103` (or `ssh root@10.10.10.21`):

```
apt-get install -y iptables iptables-persistent

# Apply the F1 allow-list. dmz-proxy + parley-api + loopback on :9000.
# Loopback only on :9001 (web console — reach via SSH tunnel).
iptables -A INPUT -p tcp --dport 9000 -s 127.0.0.1   -j ACCEPT
iptables -A INPUT -p tcp --dport 9000 -s 10.10.10.5  -j ACCEPT
iptables -A INPUT -p tcp --dport 9000 -s 10.10.10.11 -j ACCEPT
iptables -A INPUT -p tcp --dport 9000                -j DROP

iptables -A INPUT -p tcp --dport 9001 -s 127.0.0.1   -j ACCEPT
iptables -A INPUT -p tcp --dport 9001                -j DROP

# Persist across reboot.
mkdir -p /etc/iptables
iptables-save > /etc/iptables/rules.v4
```

**Verify (from dmz-proxy 10.10.10.5):**
```
curl -sI http://10.10.10.21:9000/minio/health/live | head -1   # expect 200
```

**Verify (from parley-api 10.10.10.11):**
```
curl -sI http://10.10.10.21:9000/minio/health/live | head -1   # expect 200
```

**Verify (from any other vmbr1 host, e.g. 10.10.10.31 or gitwise):**
```
timeout 3 curl -sI http://10.10.10.21:9000/minio/health/live
# expect: exit 28 (curl operation timeout — DROP doesn't reply) OR exit 124
# should NOT print "HTTP/1.1 200 OK"
```

**Verify the console is not exposed:**
```
# From any non-loopback vmbr1 host:
timeout 3 curl -sI http://10.10.10.21:9001/      # expect timeout / no reply
```

**Fallback if NET_ADMIN cannot be granted** (e.g. policy forbids it on CT 103): stand up a local nginx on CT 103 and rebind MinIO to `--address 127.0.0.1:9000`. Terraform userdata has scaffold comments marking this path. Steps:

1. `apt-get install -y nginx`.
2. Edit `/etc/systemd/system/minio.service` — change `ExecStart=... --address :9000 ...` to `--address 127.0.0.1:9000`. `systemctl daemon-reload && systemctl restart minio`.
3. Create `/etc/nginx/sites-available/parley-minio` listening on `:9000` with `allow 10.10.10.5; allow 10.10.10.11; deny all;` and `proxy_pass http://127.0.0.1:9000;`. Enable + reload nginx.

**Rollback:**
```
iptables -F INPUT   # NOTE: flushes all INPUT rules — only safe because
                    # CT 103 has no other INPUT rules besides these.
rm -f /etc/iptables/rules.v4
```

---

## Step 4 — Out-of-scope check: dmz-proxy

The DMZ nginx configuration (`dmz.conf`) is not in this repo (per the audit). This runbook does not touch CT 106 / dmz-proxy — it's the thing we're allow-listing. After Steps 1–3 are applied, dmz-proxy continues to work because it's the only host reaching backends.

If `dmz.conf` is later checked in, add Cloudflare's source-IP ranges as an `allow`-list on the DMZ side too (not in scope for F1).

---

## Post-checks (leaving the fix in place)

Run daily for a week, then weekly, from `gitwise-app` (10.10.10.31) or any non-dmz vmbr1 host:

```
# All four must stay closed / denied.
for url in \
  http://10.10.10.11/health \
  http://10.10.10.15/ \
  http://10.10.10.15:8080/ \
  http://10.10.10.21:9000/minio/health/live
do
  code=$(timeout 3 curl -s -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || echo TIMEOUT)
  echo "$url -> $code"
done
# expect: 403, 403, 000 (conn refused), TIMEOUT
```

And from dmz-proxy (10.10.10.5), the same four must continue to work (health checks 200; admin 200; MinIO 200).

Alert if any of the non-dmz checks starts returning 200 — that's a regression.

---

## Hand-off notes

- Go side: `cmd/admin/server.go` now honors `ADMIN_BIND_LOCAL=1` to bind `127.0.0.1:<ADMIN_PORT>` only. Default behaviour (unset) is unchanged — all-interfaces bind — so non-LXC / dev deployments are unaffected.
- Terraform side: `userdata-api.sh`, `userdata-admin.sh`, `userdata-minio.sh` all carry the gate now, so a fresh `terraform apply` on a clean cluster produces a gated topology out of the box.
- If a new backend is ever added on vmbr1, apply the same pattern: nginx `allow 10.10.10.5; deny all;` at the server block for anything that serves HTTP, plus an iptables DROP for anything MinIO-like that doesn't front-end with nginx.
- Related findings: **F2** (tenant isolation via per-tenant bridges) supersedes F1 — once every tenant is on its own bridge, L3 cross-tenant reachability goes away and F1 becomes a second layer. Keep F1 in place regardless; defense in depth.
