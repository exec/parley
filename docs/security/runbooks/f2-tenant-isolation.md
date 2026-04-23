# F2 — Tenant isolation on vmbr1 runbook

Fixes the HIGH F2 finding from `docs/security/2026-04-23-adversarial-audit.md`: `gitwise-db` (10.10.10.30) and `gitwise-app` (10.10.10.31) share `vmbr1` with every parley backend (db .10, api-1 .11, admin .15, minio .21, observability .60). A gitwise RCE reaches parley's DB, object store, API, and admin at L3 directly; F1 gates HTTP at the app layer but MinIO (:9000) and pgbouncer (:6432) remain L4-reachable from the gitwise IPs.

**Approach chosen.** Option B — keep the shared `vmbr1` bridge and enforce tenant isolation with Proxmox cluster-level firewall rules keyed on IPsets. Option A (per-tenant bridges `vmbr-parley` / `vmbr-gitwise`) is architecturally stronger but requires container IP changes, dmz-proxy route reconfig, and cert SAN churn — out of scope for this remediation window. Option B composes with F1 (app-layer allow/deny) for defence in depth.

**Scope of this runbook:** remediation against the already-running Proxmox host `eqr`. Proxmox firewall config lives outside this repo (`/etc/pve/firewall/*.fw` on the PVE host); those edits are applied by hand. See `terraform/proxmox/README.md` for how new deployments should reproduce this.

**Who runs this:** infra lead with root (or sudo) on `eqr` and the ability to `pct set` / restart containers briefly. No backend downtime is expected — `pct set net0 ...,firewall=1` applies live; guests keep their lease. Restart is only required if a guest was provisioned before the PVE firewall service was enabled on the node.

**The tenants.**
- **parley:** dmz-proxy (10.10.10.5), parley-db (10.10.10.10), parley-api-1..N (10.10.10.11..14), parley-admin (10.10.10.15), parley-minio (10.10.10.21), observability (10.10.10.60).
- **gitwise:** gitwise-db (10.10.10.30), gitwise-app (10.10.10.31).
- **Common gateway.** dmz-proxy (10.10.10.5) is the only host allowed to reach both tenants — it serves `parley.byexec.com` and `gitwise.byexec.com` from the same nginx.

---

## Pre-checks (confirm F2 is currently exploitable)

Run from **gitwise-app (CT 105, 10.10.10.31)** — these are the four L4 reaches the finding calls out. All four should currently succeed:

```
# pgbouncer (parley-db)
timeout 3 nc -zv 10.10.10.10 6432     # expect: succeeded
# MinIO (parley object store)
timeout 3 nc -zv 10.10.10.21 9000     # expect: succeeded
# parley-api nginx :80
timeout 3 nc -zv 10.10.10.11 80       # expect: succeeded
# parley-admin Go :8080 (if still exposed pre-F1) or nginx :80
timeout 3 nc -zv 10.10.10.15 80       # expect: succeeded
timeout 3 nc -zv 10.10.10.15 8080     # post-F1: connection refused (Go is now loopback-only); pre-F1: succeeded
```

Also confirm from **dmz-proxy (CT 106, 10.10.10.5)** the same hosts are reachable — they must stay reachable after apply:

```
timeout 3 nc -zv 10.10.10.10 6432     # expect: succeeded (and must still succeed after)
timeout 3 nc -zv 10.10.10.21 9000     # expect: succeeded
timeout 3 nc -zv 10.10.10.11 80       # expect: succeeded
timeout 3 nc -zv 10.10.10.15 80       # expect: succeeded
```

Record both sets of outputs. The gitwise set must invert after Step 3 (all four fail / timeout); the dmz-proxy set must remain unchanged.

---

## Step 1 — Extend `/etc/pve/firewall/cluster.fw` on the Proxmox host

On `eqr` as root (or sudo), edit `/etc/pve/firewall/cluster.fw`. The file already has an `internal` IPset (10.10.10.0/24) and a `no-internet` security group — leave those alone. Add the two tenant IPsets and the two tenant-isolation security groups (one per tenant).

The final file should contain the additions below. **Ordering matters** — the `[RULES]` block is evaluated top-to-bottom per packet direction, and PVE evaluates cluster-level rules before per-VM rules for matching guests.

```
[OPTIONS]

enable: 1

[ALIASES]

dmz-proxy 10.10.10.5

[IPSET parley]
# Every host that is part of the parley tenant on vmbr1.
# dmz-proxy is intentionally included — it is the shared ingress gateway and
# must reach both tenants. It is also in the gitwise ipset for the same reason.
10.10.10.5      # dmz-proxy (shared gateway)
10.10.10.10     # parley-db (pgbouncer 6432, postgres 5432, redis 6379)
10.10.10.11     # parley-api-1
10.10.10.12     # parley-api-2 (reserved)
10.10.10.13     # parley-api-3 (reserved)
10.10.10.14     # parley-api-4 (reserved)
10.10.10.15     # parley-admin
10.10.10.21     # parley-minio
10.10.10.60     # observability

[IPSET gitwise]
10.10.10.5      # dmz-proxy (shared gateway)
10.10.10.30     # gitwise-db
10.10.10.31     # gitwise-app

[IPSET internal]
# Pre-existing — do not remove. Used elsewhere in cluster.fw / per-VM rules.
10.10.10.0/24

[group no-internet]
# Pre-existing — do not remove.
# (leave current contents of this group as-is)

[group parley-tenant]
# Attached to every parley CT via its per-VM .fw file.
# First-match-wins. Allow dmz-proxy and intra-parley; drop everything
# else that came from a gitwise IP. Non-tenant sources (off-bridge or
# unknown) are not covered by this group — the per-VM default policy
# applies to them, which is ACCEPT unless you also set input_policy.
IN ACCEPT -source +dc/dmz-proxy
IN ACCEPT -source +dc/parley
IN DROP   -source +dc/gitwise -log warning
OUT ACCEPT -dest  +dc/dmz-proxy
OUT ACCEPT -dest  +dc/parley
OUT DROP   -dest  +dc/gitwise -log warning

[group gitwise-tenant]
# Attached to every gitwise CT via its per-VM .fw file. Symmetric to parley-tenant.
IN ACCEPT -source +dc/dmz-proxy
IN ACCEPT -source +dc/gitwise
IN DROP   -source +dc/parley -log warning
OUT ACCEPT -dest  +dc/dmz-proxy
OUT ACCEPT -dest  +dc/gitwise
OUT DROP   -dest  +dc/parley -log warning
```

**Note on IPset membership semantics.** PVE IPsets are source/destination *address lists*, not guest tags — `-source +dc/parley` matches solely on the packet's source IP. For the isolation to be correct the allow and drop rules MUST be split into two separate groups, one per tenant, each attached only to that tenant's CTs (Step 2). A single shared group that lists both `-source +parley ACCEPT` and `-source +gitwise ACCEPT` would fail open on a parley CT, because the gitwise-source accept would fire before the drop. That pitfall is the reason for the two-group design.

Validate the config before it takes effect:

```
pve-firewall compile              # no errors
pve-firewall status               # Status: enabled/running
```

---

## Step 2 — Attach the correct group to every tenant CT

For each CT, edit `/etc/pve/firewall/<vmid>.fw` on the Proxmox host. Create the file if it does not exist. Parley CTs reference `parley-tenant`; gitwise CTs reference `gitwise-tenant`.

Parley CT `.fw` content:

```
[OPTIONS]
enable: 1

[RULES]
GROUP parley-tenant
```

Gitwise CT `.fw` content:

```
[OPTIONS]
enable: 1

[RULES]
GROUP gitwise-tenant
```

The concrete CT IDs (confirm with `pct list`):

| CT ID | Hostname         | IP           | Tenant  | Group attached   |
|-------|------------------|--------------|---------|------------------|
| 100   | parley-admin     | 10.10.10.15  | parley  | `parley-tenant`  |
| 101   | parley-db        | 10.10.10.10  | parley  | `parley-tenant`  |
| 102   | parley-api-1     | 10.10.10.11  | parley  | `parley-tenant`  |
| 103   | parley-minio     | 10.10.10.21  | parley  | `parley-tenant`  |
| 104   | observability    | 10.10.10.60  | parley  | `parley-tenant`  |
| 105   | gitwise-app      | 10.10.10.31  | gitwise | `gitwise-tenant` |
| 108   | gitwise-db       | 10.10.10.30  | gitwise | `gitwise-tenant` |

*(CT IDs for gitwise are approximate — verify with `pct list | grep gitwise` before editing. dmz-proxy (106) is intentionally excluded — it must talk to both tenants and is exempted by the `+dc/dmz-proxy` ACCEPT rules at the top of each group.)*

Create the per-VM files:

```
for vmid in 100 101 102 103 104; do
  cat > /etc/pve/firewall/${vmid}.fw <<'EOF'
[OPTIONS]
enable: 1

[RULES]
GROUP parley-tenant
EOF
done

for vmid in 105 108; do
  cat > /etc/pve/firewall/${vmid}.fw <<'EOF'
[OPTIONS]
enable: 1

[RULES]
GROUP gitwise-tenant
EOF
done
```

---

## Step 3 — Enable the firewall flag on each CT's network interface

PVE rules on a guest only take effect when that guest's network interface has `firewall=1`. Check the current state:

```
pct config 100 | grep ^net0
# typical pre-state:
# net0: name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.15/24,type=veth
#   (no firewall= flag → firewall is OFF for this CT)
```

For each tenant CT, add `firewall=1` while preserving every other net0 option:

```
# parley CTs
pct set 100 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.15/24,type=veth,firewall=1
pct set 101 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.10/24,type=veth,firewall=1
pct set 102 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.11/24,type=veth,firewall=1
pct set 103 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.21/24,type=veth,firewall=1
pct set 104 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.60/24,type=veth,firewall=1

# gitwise CTs
pct set 105 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.31/24,type=veth,firewall=1
pct set 108 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.30/24,type=veth,firewall=1
```

**IMPORTANT** — the `pct set -net0` form replaces the whole NIC spec. Read each container's current `net0` line with `pct config <vmid>` first and append `,firewall=1` rather than pasting the examples above blindly. If a CT uses a different MAC, `tag`, `rate`, or similar, preserve it.

Changes apply without a reboot (the veth-pair's ebtables/iptables chains are reinstalled by `pve-firewall` within a few seconds). No downtime expected.

Confirm:

```
for vmid in 100 101 102 103 104 105 108; do
  echo "=== CT $vmid ==="
  pct config $vmid | grep ^net0
done
# every line must contain firewall=1
```

Apply:

```
systemctl reload pve-firewall      # usually unnecessary — changes are picked up
                                   # by the polling watcher in a few seconds.
iptables -L -n -v | head -40       # sanity: cluster rules are now emitted
```

---

## Verify after apply

**From gitwise-app (10.10.10.31) — all four MUST now fail / time out:**

```
timeout 3 nc -zv 10.10.10.10 6432     # expect: timeout (DROP doesn't reply)
timeout 3 nc -zv 10.10.10.21 9000     # expect: timeout
timeout 3 nc -zv 10.10.10.11 80       # expect: timeout
timeout 3 nc -zv 10.10.10.15 80       # expect: timeout
timeout 3 nc -zv 10.10.10.60 3000     # Grafana — expect: timeout
```

A REJECT would return immediately; with DROP you get an EAGAIN / timeout. Either way, no `succeeded`.

**From gitwise-app — same-tenant reach MUST still work:**

```
timeout 3 nc -zv 10.10.10.30 5432     # gitwise-db — expect: succeeded
```

**From dmz-proxy (10.10.10.5) — the happy path MUST be unchanged:**

```
timeout 3 nc -zv 10.10.10.10 6432     # expect: succeeded
timeout 3 nc -zv 10.10.10.21 9000     # expect: succeeded
timeout 3 nc -zv 10.10.10.11 80       # expect: succeeded
timeout 3 nc -zv 10.10.10.15 80       # expect: succeeded
timeout 3 nc -zv 10.10.10.31 80       # gitwise nginx — expect: succeeded (dmz routes both tenants)
```

**From parley-api (10.10.10.11) — intra-parley traffic MUST still work:**

```
timeout 3 nc -zv 10.10.10.10 6432     # expect: succeeded (api → pgbouncer)
timeout 3 nc -zv 10.10.10.21 9000     # expect: succeeded (api → minio)
timeout 3 nc -zv 10.10.10.30 5432     # expect: timeout (cross into gitwise — must fail)
```

**End-to-end app traffic:**

```
# From outside (Cloudflare path), both tenants still serve:
curl -sI https://parley.byexec.com/  | head -1   # expect: 200
curl -sI https://gitwise.byexec.com/ | head -1   # expect: 200
```

**Check the drop counter** (evidence the rule is actually matching). After a gitwise → parley probe attempt, on the Proxmox host:

```
journalctl -k -n 200 | grep -E 'parley-tenant|gitwise-tenant' | tail
# expect: kernel log lines for the dropped packets (because -log warning is set)
```

---

## Rollback

If anything breaks, reverse in the opposite order. The `pct set` step is the load-bearing one — clearing `firewall=1` on every CT immediately stops the rules from having any effect, even with the cluster.fw additions still in place.

```
# 1. Disable firewall on every CT (replace each net0 line with its original form).
pct set 100 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.15/24,type=veth
pct set 101 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.10/24,type=veth
pct set 102 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.11/24,type=veth
pct set 103 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.21/24,type=veth
pct set 104 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.60/24,type=veth
pct set 105 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.31/24,type=veth
pct set 108 -net0 name=eth0,bridge=vmbr1,gw=10.10.10.1,ip=10.10.10.30/24,type=veth

# 2. Remove per-VM .fw files (the GROUP reference).
rm -f /etc/pve/firewall/{100,101,102,103,104,105,108}.fw

# 3. Remove the tenant additions from cluster.fw (leave the pre-existing
#    `internal` ipset, `no-internet` group, and [ALIASES] block intact).
#    Delete the [IPSET parley], [IPSET gitwise], [group parley-tenant],
#    and [group gitwise-tenant] stanzas and the dmz-proxy alias we added.

# 4. Reload.
systemctl reload pve-firewall
```

No data migration, no service restart, no DNS change. Rollback is reversible in <60 seconds.

---

## Post-checks (leaving the fix in place)

Run daily for a week, then weekly, from `gitwise-app` (10.10.10.31):

```
for target in 10.10.10.10:6432 10.10.10.21:9000 10.10.10.11:80 10.10.10.15:80 10.10.10.60:3000; do
  host="${target%:*}"
  port="${target#*:}"
  result=$(timeout 3 nc -zv "$host" "$port" 2>&1)
  echo "$target -> $result"
done
# every line must say "timed out" / no route — any `succeeded` is a regression.
```

Alert if any probe starts succeeding — that's a regression, possibly from a CT being recreated without the `firewall=1` flag.

Also sample the kernel log for `tenant-isolation` drops on the Proxmox host:

```
journalctl -k --since "1 hour ago" | grep -cE 'parley-tenant|gitwise-tenant'
# steady state: 0 (no cross-tenant traffic)
# >0: investigate which source triggered it — could be legitimate misconfig
#     (e.g. new parley CT not added to the parley IPset) or an actual breach.
```

---

## Hand-off notes

- **Adding a new parley backend**: add its IP to `[IPSET parley]` in `cluster.fw`, write `/etc/pve/firewall/<newvmid>.fw` with `GROUP parley-tenant`, and `pct set <newvmid> -net0 ...,firewall=1`. Do *not* rely on the `internal` IPset to cover it — `internal` is a supernet for the `no-internet` group, not a tenant marker.
- **Adding a new gitwise CT**: symmetric — add IP to `[IPSET gitwise]`, write the per-VM .fw referencing `GROUP gitwise-tenant`, set `firewall=1`.
- **The `+dc/dmz-proxy` alias is load-bearing.** If dmz-proxy is ever renumbered, update the alias in `cluster.fw` first — otherwise both tenants lose external reachability simultaneously.
- **This runbook does not replace F1.** F1 (app-layer allow/deny on nginx + localhost-bind on admin Go) stays in place. F2 is a *layer below* — it stops L4 connects before they reach nginx. If F1 is removed, F2 alone is still enough to block gitwise→parley app traffic; if F2 is bypassed (e.g. new CT forgets `firewall=1`), F1 still blocks gitwise→parley HTTP. Defence in depth.
- **Related findings.** D5 (pgbouncer reachable from vmbr1) is closed by F2 — gitwise can no longer connect to :6432 at L4. D1 (MinIO public backups) keeps its own fix; F2 just narrows the blast radius.
- **Future-proofing.** A cleaner long-term design is Option A (`vmbr-parley` + `vmbr-gitwise`, no shared bridge). If the cluster is ever re-provisioned, adopt Option A at that time — see `terraform/proxmox/README.md` for the sketch.
