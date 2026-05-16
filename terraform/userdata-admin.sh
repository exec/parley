#!/bin/bash
set -euo pipefail

REPO_URL="${REPO_URL}"
DB_HOST="${DB_HOST}"
DB_PASSWORD="${DB_PASSWORD}"
REDIS_HOST="${REDIS_HOST}"
ADMIN_JWT_SECRET="${ADMIN_JWT_SECRET}"
# F-admin-jwt-secret: admin container no longer holds the api's user-session
# JWT_SECRET. It signs impersonation tokens with a dedicated key that the api
# verifies separately. An admin-container compromise therefore can no longer
# mint arbitrary user JWTs — only impersonation tokens, which the api's
# denyImpersonation middleware blocks from sensitive routes.
IMPERSONATION_JWT_SECRET="${IMPERSONATION_JWT_SECRET}"
ADMIN_PORT="${ADMIN_PORT}"
# F-admin-origin-fallback: explicit admin frontend origin. The Go admin server
# fails to start if ADMIN_ORIGIN is unset, so this must always be provisioned
# by terraform — no default IP fallback.
ADMIN_ORIGIN="${ADMIN_ORIGIN}"

exec > >(tee -a /var/log/cloud-init-output.log) 2>&1
echo "=== Starting Parley Admin setup ==="

# Install deps
apt-get update -y
apt-get install -y git curl nginx

# Install Go (must match go.mod toolchain version)
GO_VERSION="1.25.0"
if ! go version 2>/dev/null | grep -q "go$${GO_VERSION}"; then
  rm -rf /usr/local/go
  curl -sLO "https://go.dev/dl/go$${GO_VERSION}.linux-amd64.tar.gz"
  tar -C /usr/local -xzf "go$${GO_VERSION}.linux-amd64.tar.gz"
  rm "go$${GO_VERSION}.linux-amd64.tar.gz"
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

# URL-encode the DB password so special chars don't break the connection string
DB_PASSWORD_ENCODED=$(python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$${DB_PASSWORD}")
DATABASE_URL="postgres://parley:$${DB_PASSWORD_ENCODED}@$${DB_HOST}:5432/parley?sslmode=disable"

# Create service user (idempotent)
id -u parley >/dev/null 2>&1 || useradd -r -s /bin/false parley

# Environment file
mkdir -p /etc/parley
cat > /etc/parley/admin-env <<EOF
DATABASE_URL=$${DATABASE_URL}
REDIS_HOST=$${REDIS_HOST}
ADMIN_JWT_SECRET=$${ADMIN_JWT_SECRET}
IMPERSONATION_JWT_SECRET=$${IMPERSONATION_JWT_SECRET}
ADMIN_PORT=$${ADMIN_PORT}
ADMIN_ORIGIN=$${ADMIN_ORIGIN}
# F1: bind the admin HTTP server to 127.0.0.1 so the Go process is not
# reachable directly on :8080 from any other vmbr1 host. The container-local
# nginx reverse-proxies /api/ to 127.0.0.1:8080 and enforces the source-IP
# allow-list (only 10.10.10.5 / dmz-proxy).
ADMIN_BIND_LOCAL=1
EOF
chown parley:parley /etc/parley/admin-env
chmod 600 /etc/parley/admin-env

# Systemd service
cat > /etc/systemd/system/parley-admin.service <<'SVCEOF'
[Unit]
Description=Parley Admin Panel
After=network.target

[Service]
Type=simple
User=parley
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

    # F1 ingress gate — only accept connections from the DMZ proxy (10.10.10.5)
    # plus the proxmox host's vmbr1 IP (10.10.10.1). The host IP is the source
    # for SSH-forwarded tunnels (operator → proxmox → 10.10.10.15:80), used to
    # reach Grafana embedded at /grafana/ and the admin SPA. The admin surface
    # (SPA + /api/) must never be reachable from any other vmbr1 host. Admin Go
    # is separately bound to 127.0.0.1:8080 via ADMIN_BIND_LOCAL=1 so direct
    # :8080 access from vmbr1 is also blocked at L4.
    allow 10.10.10.5;
    allow 10.10.10.1;
    deny all;

    # Allow large request bodies so uploaded images / bulk API calls aren't truncated
    client_max_body_size 50M;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # Reverse-proxy the observability LXC's Grafana under /grafana/ so the
    # admin UI can iframe embed it without a separate SSH tunnel. Grafana
    # itself is configured with serve_from_sub_path + root_url=.../grafana/
    # so asset + API URLs it generates resolve back through here.
    # WebSocket upgrade headers are required for Grafana Live (log tailing).
    location /grafana/ {
        proxy_pass http://10.10.10.60:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;
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
