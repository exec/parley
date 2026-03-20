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

echo "=== Creating bucket ==="
wget -q -O /usr/local/bin/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /usr/local/bin/mc

mc alias set local http://localhost:9000 "${minio_access_key}" "${minio_secret_key}"
mc mb --ignore-existing "local/${minio_bucket}"
# Allow unauthenticated GET so SPACES_CDN_URL links work without pre-signed URLs
mc anonymous set download "local/${minio_bucket}"

echo "=== MinIO setup complete ==="
