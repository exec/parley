#!/bin/bash
set -euo pipefail

REPO_URL="${REPO_URL}"
DATABASE_URL="${DATABASE_URL}"
ADMIN_JWT_SECRET="${ADMIN_JWT_SECRET}"
PARLEY_JWT_SECRET="${PARLEY_JWT_SECRET}"
ADMIN_IMPERSONATE_SECRET="${ADMIN_IMPERSONATE_SECRET}"
ADMIN_PORT="${ADMIN_PORT}"

exec > >(tee -a /var/log/cloud-init-output.log) 2>&1
echo "=== Starting Parley Admin setup ==="

# Install deps
apt-get update -y
apt-get install -y git curl nginx

# Install Go
if ! command -v go &>/dev/null; then
  curl -sLO https://go.dev/dl/go1.23.4.linux-amd64.tar.gz
  tar -C /usr/local -xzf go1.23.4.linux-amd64.tar.gz
  rm go1.23.4.linux-amd64.tar.gz
fi
export PATH=$PATH:/usr/local/go/bin
export HOME=/root
export GOPATH=/root/go
export GOCACHE=/root/.cache/go-build

# Install Node
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

# Clone repo
if [ -d "/parley" ]; then
  cd /parley && git pull origin main 2>/dev/null || true
else
  git clone --depth 1 $${REPO_URL} /parley
fi

# Build admin binary
cd /parley
go mod download
go build -o /usr/local/bin/parley-admin ./cmd/admin

# Build admin frontend
cd /parley/admin-frontend
npm ci
npm run build
mkdir -p /var/www/parley-admin
cp -r dist/* /var/www/parley-admin/

# Environment file
mkdir -p /etc/parley
cat > /etc/parley/admin-env <<EOF
DATABASE_URL=$${DATABASE_URL}
ADMIN_JWT_SECRET=$${ADMIN_JWT_SECRET}
PARLEY_JWT_SECRET=$${PARLEY_JWT_SECRET}
ADMIN_IMPERSONATE_SECRET=$${ADMIN_IMPERSONATE_SECRET}
ADMIN_PORT=$${ADMIN_PORT}
EOF
chmod 600 /etc/parley/admin-env

# Systemd service
cat > /etc/systemd/system/parley-admin.service <<'SVCEOF'
[Unit]
Description=Parley Admin Panel
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/parley
EnvironmentFile=/etc/parley/admin-env
ExecStart=/usr/local/bin/parley-admin serve
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable parley-admin
systemctl start parley-admin

# Nginx — proxy /api to Go, serve frontend for everything else
cat > /etc/nginx/sites-available/parley-admin <<'NGINXEOF'
server {
    listen 80;
    server_name _;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location / {
        root /var/www/parley-admin;
        try_files $uri $uri/ /index.html;
    }
}
NGINXEOF

ln -sf /etc/nginx/sites-available/parley-admin /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
systemctl reload nginx

echo "=== Parley Admin setup complete ==="
