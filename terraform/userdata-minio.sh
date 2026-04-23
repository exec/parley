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

echo "=== MinIO setup complete ==="
