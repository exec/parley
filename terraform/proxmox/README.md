# Parley Proxmox deployment

Terraform module that provisions the parley stack as LXC containers on a Proxmox host.

Containers provisioned:

| CT (role)         | Bridge  | IP (default)    |
|-------------------|---------|-----------------|
| `parley-db`       | `vmbr1` | `10.10.10.10`   |
| `parley-api-1..N` | `vmbr1` | `10.10.10.11..` |
| `parley-admin`    | `vmbr1` | `10.10.10.15`   |
| `parley-minio`    | `vmbr1` | `10.10.10.21`   |
| `parley-lb`       | `vmbr1` | `10.10.10.20`   |

Out of scope for this module: `dmz-proxy` (CT 106, 10.10.10.5), `observability` (CT 104, 10.10.10.60), and any co-tenant (e.g. `gitwise-*`). Those are provisioned separately but share `vmbr1`.

## Quick start

```
cp terraform.tfvars.example terraform.tfvars
# fill in proxmox endpoint, API token, SSH key paths, secrets
terraform init
terraform apply
```

`terraform apply` will:

1. Create each CT with cloud-init (hostname, static IP, SSH key).
2. `pct push` the matching `userdata-*.sh` into the CT and execute it (installs packages, renders configs, starts services).
3. The userdata scripts codify the F1 remediation — nginx `allow 10.10.10.5; deny all;` on every backend, admin Go bound to `127.0.0.1:8080`, MinIO L4 allow-list via iptables.

## Post-apply — PVE firewall rules for tenant isolation (F2)

**Terraform does NOT configure `/etc/pve/firewall/*.fw`.** The bpg/proxmox provider does not expose cluster-firewall / IPset resources, and — more importantly — these files live at the Proxmox node scope, not per-guest. They have to be applied out-of-band by root on the PVE host.

After `terraform apply`, apply the F2 runbook **before** exposing the stack to any co-tenant:

- Runbook: [`docs/security/runbooks/f2-tenant-isolation.md`](../../docs/security/runbooks/f2-tenant-isolation.md)
- Related finding: F2 in `docs/security/2026-04-23-adversarial-audit.md`

The runbook adds `parley` / `gitwise` IPsets, a `tenant-isolation` security group, per-CT `.fw` files, and `pct set ... firewall=1` on every CT NIC.

If a fresh deployment is single-tenant (no `gitwise-*` or other co-tenants on `vmbr1`), the F2 firewall is not immediately required, but:

- Apply it anyway. It is the only durable fence against future tenant sprawl on the shared bridge.
- Running `firewall=1` on every CT with an empty-but-enabled group is effectively a no-op and gives you a ready slot to extend later without a fresh `pct set` pass (which takes a brief NIC flap).

## Re-provisioning on fresh hardware

If the cluster is ever rebuilt on hardware where tenants can be isolated at the bridge layer, prefer **Option A**: give each tenant its own bridge (`vmbr-parley` 10.10.20.0/24, `vmbr-gitwise` 10.10.30.0/24). That removes the shared-L2 problem entirely and makes F2 unnecessary. Changes required:

- Add a new variable `proxmox_bridge` per tenant in `variables.tf`.
- Renumber every CT into the tenant's subnet; regenerate `*.tfvars` and origin cert SANs if any service IP is surfaced externally.
- Route dmz-proxy to both bridges (add a second veth on CT 106).
- Drop the F2 runbook — per-bridge isolation is enforced by the kernel, not by `pve-firewall`.

This is a bigger lift (container IP changes cascade into nginx upstreams, TLS cert SANs, and pgbouncer `userlist.txt` host pins) so it is not the default. Option B (F2 runbook) gives equivalent L3/L4 protection on the current topology.

## Files

- `main.tf` — CT resources + `null_resource.*_provision` steps (SSH to Proxmox host, `pct push`, `pct exec` the userdata script).
- `variables.tf` — connection info, CT sizing, IP assignments.
- `outputs.tf` — CT IDs and IPs.
- `../userdata-*.sh` — per-role provisioning scripts.
