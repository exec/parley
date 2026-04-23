#!/bin/bash
# MinIO S3-compatible object storage for Parley on Proxmox.
# Replaces DigitalOcean Spaces — the API server is configured to point at
# this VM automatically via SPACES_ENDPOINT and SPACES_CDN_URL.
#
# S3 API:      http://<minio_ip>:9000
# Web console: http://<minio_ip>:9001  (login with minio_access_key / minio_secret_key)

set -e

echo "=== Starting Parley MinIO setup ==="

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y wget curl

echo "=== Downloading MinIO ==="
wget -q -O /usr/local/bin/minio https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x /usr/local/bin/minio

echo "=== Configuring MinIO ==="
useradd -r -s /sbin/nologin minio-user 2>/dev/null || true
mkdir -p /data/minio
chown minio-user:minio-user /data/minio

# F1: install iptables so we can gate :9000 / :9001 at L4 (in addition to
# the D1 bucket-policy hardening, which is app-layer). MinIO does not ship
# with a built-in IP allow-list, and fronting it with nginx would mean a
# TLS-terminating proxy in front of every PUT / GET — unnecessary overhead
# when a packet filter achieves the same thing.
apt-get install -y iptables

mkdir -p /etc/minio
cat > /etc/minio/minio.env <<EOF
MINIO_ROOT_USER="${minio_access_key}"
MINIO_ROOT_PASSWORD="${minio_secret_key}"
EOF
chmod 600 /etc/minio/minio.env

cat > /etc/systemd/system/minio.service <<'SVCEOF'
[Unit]
Description=MinIO Object Storage
Wants=network-online.target
After=network-online.target

[Service]
User=minio-user
Group=minio-user
EnvironmentFile=/etc/minio/minio.env
ExecStart=/usr/local/bin/minio server /data/minio --address :9000 --console-address :9001
Restart=always
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable minio
systemctl start minio

echo "=== Waiting for MinIO to be ready ==="
for i in $(seq 1 30); do
  curl -sf http://localhost:9000/minio/health/live && break
  sleep 2
done

echo "=== Installing mc client ==="
wget -q -O /usr/local/bin/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /usr/local/bin/mc

mc alias set local http://localhost:9000 "${minio_access_key}" "${minio_secret_key}"

# Two buckets:
#   ${minio_bucket}         — public CDN assets (avatars, uploads, soundboard, audio)
#   ${minio_bucket}-backups — private backups, no anonymous access at all
echo "=== Creating buckets ==="
mc mb --ignore-existing "local/${minio_bucket}"
mc mb --ignore-existing "local/${minio_bucket}-backups"

# Scoped public-read policy for the assets bucket.
# - Allows anonymous s3:GetObject ONLY on the four CDN prefixes. Anything
#   else in this bucket (including any accidental backups/* writes) is
#   private.
# - Explicitly does NOT grant s3:ListBucket — anonymous listing was the
#   vulnerability that exposed backup filenames to unauthenticated callers
#   (see docs/security/runbooks/d1-minio-hardening.md).
echo "=== Applying scoped public-read policy to ${minio_bucket} ==="
cat > /tmp/parley-public-policy.json <<POLICYEOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Action": ["s3:GetObject"],
      "Resource": [
        "arn:aws:s3:::${minio_bucket}/avatars/*",
        "arn:aws:s3:::${minio_bucket}/uploads/*",
        "arn:aws:s3:::${minio_bucket}/soundboard/*",
        "arn:aws:s3:::${minio_bucket}/audio/*"
      ]
    }
  ]
}
POLICYEOF
mc anonymous set-json /tmp/parley-public-policy.json "local/${minio_bucket}"
rm -f /tmp/parley-public-policy.json

# Backups bucket: fully private. `mc anonymous set none` is the default for
# a new bucket but we apply it explicitly so the provisioning is deterministic
# and idempotent re-runs can't inherit a policy from a prior misconfiguration.
echo "=== Locking down ${minio_bucket}-backups (private) ==="
mc anonymous set none "local/${minio_bucket}-backups"

# Sanity: anonymous listing of the assets bucket should be denied. Provisioning
# fails loudly if a misconfiguration re-opens listing.
echo "=== Verifying anonymous listing is denied ==="
if curl -sf -o /dev/null "http://localhost:9000/${minio_bucket}/?list-type=2"; then
  echo "ERROR: anonymous ListBucket on ${minio_bucket} succeeded — policy is wrong" >&2
  exit 1
fi
echo "OK — anonymous ListBucket is denied."

# F1 ingress gating at the network layer.
#
# Only two callers legitimately reach MinIO on vmbr1:
#   10.10.10.5   — dmz-proxy (serves CDN objects to Cloudflare-origin requests)
#   10.10.10.11  — parley-api (PUT/GET for avatars, uploads, soundboard, audio)
#
# Everything else on vmbr1 is dropped. This is the network-layer complement
# to the D1 bucket-policy fix: even if an attacker finds a policy regression,
# they cannot reach :9000 at all from any non-allowed host.
#
# NOTE ON LXC CAPABILITIES: these INPUT rules require NET_ADMIN inside the
# container. Unprivileged LXCs drop NET_ADMIN by default; this script will
# log "Operation not permitted" and continue. If that happens, the CT config
# on the Proxmox host needs one of:
#   features: keyctl=1,nesting=1,...  (already set for nesting)
#   lxc.cap.keep: NET_ADMIN            (add to /etc/pve/lxc/103.conf)
# and the container must be restarted. Check the provisioning log for
# "iptables: Operation not permitted" after applying this script. See
# docs/security/runbooks/f1-backend-ingress-gating.md for the fallback
# (nginx-in-front-of-minio with listen 127.0.0.1:9000 on MinIO).
echo "=== Applying F1 ingress allow-list on :9000 and :9001 ==="
apply_minio_ingress_rules() {
  # Flush any prior F1 rules so re-runs are idempotent. We tag them by
  # putting the exact same rules back; -C / -D let us check-and-delete.
  for port in 9000 9001; do
    # Delete any prior allow-from-dmz rule (loop until no match).
    while iptables -C INPUT -p tcp --dport "$port" -s 10.10.10.5 -j ACCEPT 2>/dev/null; do
      iptables -D INPUT -p tcp --dport "$port" -s 10.10.10.5 -j ACCEPT || break
    done
    while iptables -C INPUT -p tcp --dport "$port" -s 10.10.10.11 -j ACCEPT 2>/dev/null; do
      iptables -D INPUT -p tcp --dport "$port" -s 10.10.10.11 -j ACCEPT || break
    done
    while iptables -C INPUT -p tcp --dport "$port" -s 127.0.0.1 -j ACCEPT 2>/dev/null; do
      iptables -D INPUT -p tcp --dport "$port" -s 127.0.0.1 -j ACCEPT || break
    done
    while iptables -C INPUT -p tcp --dport "$port" -j DROP 2>/dev/null; do
      iptables -D INPUT -p tcp --dport "$port" -j DROP || break
    done
  done

  # Insert the allow-list. Order matters: ACCEPTs before DROP.
  # :9000 — S3 API. dmz-proxy + parley-api only.
  iptables -A INPUT -p tcp --dport 9000 -s 127.0.0.1    -j ACCEPT
  iptables -A INPUT -p tcp --dport 9000 -s 10.10.10.5   -j ACCEPT
  iptables -A INPUT -p tcp --dport 9000 -s 10.10.10.11  -j ACCEPT
  iptables -A INPUT -p tcp --dport 9000                 -j DROP
  # :9001 — web console. Loopback only; admin reaches it via SSH tunnel.
  iptables -A INPUT -p tcp --dport 9001 -s 127.0.0.1    -j ACCEPT
  iptables -A INPUT -p tcp --dport 9001                 -j DROP
}

if apply_minio_ingress_rules 2>&1; then
  echo "OK — iptables allow-list applied on :9000 and :9001"
else
  echo "WARNING: iptables could not apply — likely unprivileged LXC without NET_ADMIN." >&2
  echo "WARNING: MinIO is NOT network-gated. See runbook for NET_ADMIN or nginx-front fallback." >&2
fi

# Persist rules across reboots. iptables-persistent stores on package install;
# here we just write the current table. If the package isn't available,
# write a boot-time script instead.
if command -v iptables-save >/dev/null 2>&1; then
  mkdir -p /etc/iptables
  iptables-save > /etc/iptables/rules.v4 2>/dev/null || true
fi

echo "=== MinIO setup complete ==="
