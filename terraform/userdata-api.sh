#!/bin/bash
set -e
# Works as a cloud-init script on DO droplets and as a re-runnable provisioner
# script inside LXC containers. Non-idempotent steps are guarded; ufw is skipped
# inside LXC because the Proxmox host firewall handles inter-container isolation.

IS_LXC=$(systemd-detect-virt --container 2>/dev/null || echo none)
case "$IS_LXC" in lxc|systemd-nspawn) IS_LXC=yes ;; *) IS_LXC=no ;; esac

# Configuration variables (passed from Terraform)
DB_HOST="${DB_HOST}"
DB_PORT="${DB_PORT}"
DB_NAME="${DB_NAME}"
DB_USER="${DB_USER}"
DB_PASSWORD="${DB_PASSWORD}"
JWT_SECRET="${JWT_SECRET}"
# F-admin-jwt-secret: api holds IMPERSONATION_JWT_SECRET alongside JWT_SECRET.
# It never signs with this key — it only verifies admin-minted impersonation
# tokens. Admin holds the same key for signing. Keeping the keys split means
# a compromise of either container can only produce one kind of token.
IMPERSONATION_JWT_SECRET="${IMPERSONATION_JWT_SECRET}"
PORT="${PORT}"
REPO_URL="${REPO_URL}"
LIVEKIT_API_KEY="${LIVEKIT_API_KEY}"
LIVEKIT_API_SECRET="${LIVEKIT_API_SECRET}"
LIVEKIT_URL="${LIVEKIT_URL}"
OLLAMA_API_URL="${OLLAMA_API_URL}"
OLLAMA_API_KEY="${OLLAMA_API_KEY}"
OLLAMA_MODEL="${OLLAMA_MODEL}"
BOT_KEY_SECRET="${BOT_KEY_SECRET}"
REDIS_PASSWORD="${REDIS_PASSWORD}"

echo "=== Starting Parley API setup ==="

# Function to run commands with retry logic
run_with_retry() {
    local max_retries=3
    local retry_delay=5
    local attempt=1
    local cmd="$@"

    while [ $attempt -le $max_retries ]; do
        echo "Attempt $attempt of $max_retries: $cmd"
        if eval "$cmd"; then
            return 0
        fi
        echo "Failed, retrying in $${retry_delay}s..."
        sleep $retry_delay
        attempt=$((attempt + 1))
    done

    echo "Command failed after $max_retries attempts: $cmd"
    return 1
}

# Update system with retry
echo "=== Updating system packages ==="
export DEBIAN_FRONTEND=noninteractive
run_with_retry "apt-get update -y" || true
run_with_retry "apt-get upgrade -y" || true

echo "=== Applying kernel tuning for high-connection workloads ==="
# || true on each: some sysctls are read-only in unprivileged LXC (e.g.
# fs.file-max); those are silently ignored and the persistent values below
# still apply on DO droplets where they're writable.

sysctl -w fs.file-max=2097152 || true

# Per-process FD limit (idempotent — only append once)
if ! grep -q "^# parley-api limits" /etc/security/limits.conf; then
cat >> /etc/security/limits.conf <<'LIMITS'
# parley-api limits
* soft nofile 1048576
* hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
LIMITS
fi

sysctl -w net.ipv4.ip_local_port_range="1024 65535" || true
sysctl -w net.ipv4.tcp_tw_reuse=1 || true
sysctl -w net.core.somaxconn=65535 || true
sysctl -w net.ipv4.tcp_max_syn_backlog=65535 || true
sysctl -w net.core.rmem_max=16777216 || true
sysctl -w net.core.wmem_max=16777216 || true
sysctl -w net.ipv4.tcp_keepalive_time=60 || true
sysctl -w net.ipv4.tcp_keepalive_intvl=10 || true
sysctl -w net.ipv4.tcp_keepalive_probes=6 || true

# Persist sysctl settings across reboots (idempotent: only append once)
if ! grep -q "^# parley-api sysctls" /etc/sysctl.conf; then
cat >> /etc/sysctl.conf << 'SYSCTL'
# parley-api sysctls
fs.file-max=2097152
net.ipv4.ip_local_port_range=1024 65535
net.ipv4.tcp_tw_reuse=1
net.core.somaxconn=65535
net.ipv4.tcp_max_syn_backlog=65535
net.core.rmem_max=16777216
net.core.wmem_max=16777216
net.ipv4.tcp_keepalive_time=60
net.ipv4.tcp_keepalive_intvl=10
net.ipv4.tcp_keepalive_probes=6
SYSCTL
fi

# Install required packages with retry
echo "=== Installing required packages ==="
run_with_retry "apt-get install -y git curl build-essential nginx certbot python3-certbot-nginx ufw redis-tools"

# Install Node.js (LTS)
echo "=== Installing Node.js ==="
curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
run_with_retry "apt-get install -y nodejs"

# Install Go from go.dev (must match go.mod toolchain version)
echo "=== Installing Go ==="
GO_VERSION="1.25.0"
if ! go version 2>/dev/null | grep -q "go$${GO_VERSION}"; then
    rm -rf /usr/local/go
    curl -sLO "https://go.dev/dl/go$${GO_VERSION}.linux-amd64.tar.gz"
    tar -C /usr/local -xzf "go$${GO_VERSION}.linux-amd64.tar.gz"
    rm "go$${GO_VERSION}.linux-amd64.tar.gz"
fi

# Redis runs on the DB node - skip local install
echo "=== Redis configured on DB node ==="

# Add Go to PATH for all shells
echo "=== Configuring Go PATH ==="
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh

# Set Go environment variables
export PATH=$PATH:/usr/local/go/bin
export HOME=/root
export GOPATH=/root/go
export GOMODCACHE=/root/go/pkg/mod
export GOCACHE=/root/.cache/go-build

# Clone or update Parley repository with retry
echo "=== Cloning Parley repository ==="
if [ -d "/parley" ]; then
    cd /parley
    echo "Updating existing repository..."
    run_with_retry "git pull origin main 2>/dev/null || git pull origin master 2>/dev/null || true"
else
    echo "Cloning repository..."
    run_with_retry "git clone --depth 1 ${REPO_URL} /parley"
    cd /parley
fi

# Build the Go application
echo "=== Building Parley API ==="
cd /parley
run_with_retry "go mod download"
run_with_retry "go build -mod=readonly -o /usr/local/bin/parley-api ./cmd/api"

# Verify binary was created
if [ ! -f "/usr/local/bin/parley-api" ]; then
    echo "ERROR: API binary not created"
    exit 1
fi

# Build the frontend
echo "=== Building Parley frontend ==="
cd /parley/frontend
CDN_HOST=$(python3 -c "from urllib.parse import urlparse; print(urlparse('${SPACES_CDN_URL}').hostname or '')")
# Unquoted EOF allows shell to write the resolved values into the file
cat > .env << EOF
VITE_CDN_HOST=$$CDN_HOST
VITE_SITE_URL=${SITE_URL}
EOF
run_with_retry "npm ci"
run_with_retry "npm run build"
mkdir -p /var/www/parley
cp -r dist/* /var/www/parley/

# NSFW moderation sidecar is disabled — moved to dedicated box (TODO)

# Create service user (idempotent)
echo "=== Creating parley service user ==="
id -u parley >/dev/null 2>&1 || useradd -r -s /bin/false parley

# Create environment file
echo "=== Creating environment configuration ==="
mkdir -p /etc/parley

# URL-encode the DB password to handle special characters
DB_PASSWORD_ENCODED=$(python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$DB_PASSWORD")

# Create environment file with PATH for Go
cat > /etc/parley/env <<EOF
DATABASE_URL=postgresql://${DB_USER}:$${DB_PASSWORD_ENCODED}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable
JWT_SECRET=${JWT_SECRET}
IMPERSONATION_JWT_SECRET=${IMPERSONATION_JWT_SECRET}
PORT=${PORT}
REDIS_URL=redis://:${REDIS_PASSWORD}@${REDIS_HOST}:6379
PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SPACES_ACCESS_KEY=${SPACES_ACCESS_KEY}
SPACES_SECRET_KEY=${SPACES_SECRET_KEY}
SPACES_BUCKET=${SPACES_BUCKET}
SPACES_REGION=${SPACES_REGION}
SPACES_ENDPOINT=${SPACES_ENDPOINT}
SPACES_CDN_URL=${SPACES_CDN_URL}
BREVO_API_KEY=${BREVO_API_KEY}
BREVO_FROM_EMAIL=${BREVO_FROM_EMAIL}
SITE_URL=${SITE_URL}
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
LIVEKIT_URL=${LIVEKIT_URL}
OLLAMA_API_URL=${OLLAMA_API_URL}
OLLAMA_API_KEY=${OLLAMA_API_KEY}
OLLAMA_MODEL=${OLLAMA_MODEL}
BOT_KEY_SECRET=${BOT_KEY_SECRET}
GIPHY_API_KEY=${GIPHY_API_KEY}
EOF

# Set proper permissions
chown parley:parley /etc/parley/env
chmod 600 /etc/parley/env

# Create systemd service
echo "=== Creating systemd service ==="
cat > /etc/systemd/system/parley-api.service <<EOF
[Unit]
Description=Parley API Service
After=network.target

[Service]
Type=simple
User=parley
WorkingDirectory=/parley
EnvironmentFile=/etc/parley/env
ExecStartPre=/bin/sh -c 'until redis-cli -h ${REDIS_HOST} -a ${REDIS_PASSWORD} ping 2>/dev/null | grep -q PONG; do echo "Waiting for Redis..."; sleep 2; done'
ExecStart=/usr/local/bin/parley-api
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable parley-api.service

# Configure Nginx as reverse proxy
echo "=== Configuring Nginx ==="

# Write a map to extract the real client IP from Cloudflare's CF-Connecting-IP header.
# All production traffic arrives through Cloudflare (enforced by DO cloud firewall), so
# CF-Connecting-IP is always the genuine client IP. Fall back to $remote_addr for
# direct/health-check traffic that bypasses Cloudflare (e.g. LB probes).
cat > /etc/nginx/conf.d/cloudflare-real-ip.conf << 'NGXEOF'
map $http_cf_connecting_ip $real_client_ip {
    default     $remote_addr;
    ~.+         $http_cf_connecting_ip;
}
NGXEOF

cat > /etc/nginx/sites-available/parley-api <<EOF
server {
    listen 80;
    server_name _;

    root /var/www/parley;
    index index.html;

    # F1 ingress gate — only accept connections from the DMZ proxy (10.10.10.5).
    # All legitimate traffic arrives via Cloudflare -> DO cloud firewall -> DMZ
    # nginx -> this backend. Any other vmbr1 host reaching :80 directly is a
    # lateral-movement attempt and must be dropped before it hits the SPA or /api.
    # Placed at the server level so every location inherits it.
    allow 10.10.10.5;
    deny all;

    # Security headers (server-level so they apply to every location). nginx
    # only inherits add_header into a location if that location declares NO
    # add_header itself — so we keep all headers here and avoid per-location
    # overrides.
    #
    # CSP keeps 'unsafe-inline' for styles because React's style={...} prop
    # produces inline style attributes; switching to nonce/hash CSP would
    # require migrating every dynamic-style call site. 'wasm-unsafe-eval' is
    # required by syntax-highlighter / KaTeX wasm pipelines.
    # Permissions-Policy allows mic+camera on this origin (LiveKit voice/video)
    # while denying everything else by default.
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;
    add_header X-Permitted-Cross-Domain-Policies none always;
    add_header Permissions-Policy "camera=(self), microphone=(self), geolocation=(), payment=(), usb=(), accelerometer=(), gyroscope=(), magnetometer=(), midi=(), serial=()" always;
    add_header Content-Security-Policy "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'; connect-src 'self' wss: https:; img-src 'self' data: https:; media-src 'self' https: data:; font-src 'self' data: https://fonts.gstatic.com; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com;" always;

    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$real_client_ip;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    # WebSocket proxy
    location /ws {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$real_client_ip;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    # API proxy
    location /api {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$real_client_ip;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # File upload — allow up to 50 MB
    location /api/upload {
        client_max_body_size 50m;
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$real_client_ip;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 120s;
        proxy_read_timeout 120s;
    }

    # Docs site (VitePress) — served at both /docs/ and /docs/developer/
    location /docs/developer/ {
        alias /parley/docs/.vitepress/dist/;
        try_files \$uri \$uri/ \$uri.html =404;
    }

    location /docs/ {
        alias /parley/docs/.vitepress/dist/;
        try_files \$uri \$uri/ \$uri.html =404;
    }

    # Hashed bundle assets must NOT fall back to index.html — otherwise a
    # missing chunk (stale tab references a hash that's been replaced by a
    # redeploy) gets served as text/html and the browser refuses the module
    # with a MIME-type error. Return a real 404 so the lazy-import promise
    # rejects cleanly.
    location ^~ /assets/ {
        try_files \$uri =404;
    }

    # Serve frontend - fallback to index.html for SPA routing.
    # Security headers are at the server level above; do not declare any
    # add_header here or nginx will stop inheriting the server-level set.
    location / {
        try_files \$uri \$uri/ /index.html;
    }
}
EOF

# Enable the site
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/parley-api /etc/nginx/sites-enabled/

# Test nginx configuration
if ! nginx -t 2>&1 | grep -q "syntax is ok"; then
    echo "ERROR: Nginx configuration failed"
    nginx -t 2>&1 || true
    exit 1
fi

# Restart nginx with retry
run_with_retry "systemctl restart nginx"
run_with_retry "systemctl enable nginx"

# Configure firewall (skipped in LXC — Proxmox firewall handles isolation)
if [ "$IS_LXC" = "no" ] && command -v ufw >/dev/null 2>&1; then
  echo "=== Configuring firewall ==="
  ufw allow 22/tcp    # SSH
  ufw allow 80/tcp    # HTTP
  ufw allow 443/tcp   # HTTPS
  ufw --force enable
else
  echo "=== Skipping ufw config (LXC — Proxmox firewall handles isolation) ==="
fi

# Restart (not start) so re-apply with a rewritten /etc/parley/env or updated
# binary actually picks up the changes — `start` is a no-op on an already-running
# service, which previously meant env changes silently failed to take effect.
echo "=== (Re)starting Parley API service ==="
run_with_retry "systemctl restart parley-api.service"

# Wait a moment for the service to start
sleep 3

# Verify the API service is running
if systemctl is-active --quiet parley-api.service; then
    echo "=== Parley API service started successfully ==="
else
    echo "=== Warning: Parley API service may have failed to start ==="
    systemctl status parley-api.service --no-pager || true
fi

# Verify nginx is running
systemctl status nginx --no-pager || true

# Final health check
echo "=== Verifying health endpoint ==="
sleep 2
if curl -sf http://localhost:${PORT}/health > /dev/null 2>&1; then
    echo "=== API health check passed ==="
else
    echo "=== Warning: Health check failed ==="
fi

echo "=== API droplet setup complete ==="
