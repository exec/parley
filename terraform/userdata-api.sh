#!/bin/bash
set -e

# Cloud-init script for Parley API droplets
# This script sets up the environment and runs the Go API service

# Configuration variables (passed from Terraform)
DB_HOST="${DB_HOST}"
DB_PORT="${DB_PORT}"
DB_NAME="${DB_NAME}"
DB_USER="${DB_USER}"
DB_PASSWORD="${DB_PASSWORD}"
JWT_SECRET="${JWT_SECRET}"
PORT="${PORT}"
REPO_URL="${REPO_URL}"

echo "=== Starting Parley API setup ==="

# Update system
echo "=== Updating system packages ==="
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get upgrade -y

# Install required packages
echo "=== Installing required packages ==="
apt-get install -y \
    git \
    curl \
    build-essential \
    nginx \
    certbot \
    python3-certbot-nginx \
    ufw \
    software-properties-common

# Install Node.js (LTS)
echo "=== Installing Node.js ==="
curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
apt-get install -y nodejs

# Install Go
echo "=== Installing Go ==="
if ! command -v go &> /dev/null; then
    curl -fsSL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -o /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
fi

# Add Go to PATH and set required env vars
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
export PATH=$PATH:/usr/local/go/bin
export HOME=/root
export GOPATH=/root/go
export GOMODCACHE=/root/go/pkg/mod
export GOCACHE=/root/.cache/go-build

# Clone or update Parley repository
echo "=== Cloning Parley repository ==="
if [ -d "/parley" ]; then
    cd /parley
    git pull origin main 2>/dev/null || git pull origin master 2>/dev/null || true
else
    git clone --depth 1 "${REPO_URL}" /parley
fi

# Build the Go application
echo "=== Building Parley API ==="
cd /parley
GONOSUMDB=* go mod download
GONOSUMDB=* go build -mod=mod -o /usr/local/bin/parley-api ./cmd/api

# Build the frontend
echo "=== Building Parley frontend ==="
cd /parley/frontend
npm ci
npm run build
mkdir -p /var/www/parley
cp -r dist/* /var/www/parley/

# Create environment file
echo "=== Creating environment configuration ==="
mkdir -p /etc/parley

# URL-encode the DB password to handle special characters (/, =, +, etc.)
DB_PASSWORD_ENCODED=$(python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "${DB_PASSWORD}")

cat > /etc/parley/env <<EOF
DATABASE_URL=postgresql://${DB_USER}:$${DB_PASSWORD_ENCODED}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable
JWT_SECRET=${JWT_SECRET}
PORT=${PORT}
EOF

# Set proper permissions
chmod 600 /etc/parley/env

# Create systemd service
echo "=== Creating systemd service ==="
cat > /etc/systemd/system/parley-api.service <<EOF
[Unit]
Description=Parley API Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/parley
EnvironmentFile=/etc/parley/env
ExecStart=/usr/local/bin/parley-api
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable parley-api.service

# Configure Nginx as reverse proxy
echo "=== Configuring Nginx ==="
cat > /etc/nginx/sites-available/parley-api <<EOF
server {
    listen 80;
    server_name _;

    root /var/www/parley;
    index index.html;

    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
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
        proxy_set_header X-Real-IP \$remote_addr;
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
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Serve frontend - fallback to index.html for SPA routing
    location / {
        try_files \$uri \$uri/ /index.html;
    }
}
EOF

# Enable the site
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/parley-api /etc/nginx/sites-enabled/

# Test nginx configuration
nginx -t

# Restart nginx
systemctl restart nginx
systemctl enable nginx

# Configure firewall
echo "=== Configuring firewall ==="
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw --force enable

# Start the API service
echo "=== Starting Parley API service ==="
systemctl start parley-api.service

# Wait a moment for the service to start
sleep 3

# Check service status
if systemctl is-active --quiet parley-api.service; then
    echo "=== Parley API service started successfully ==="
else
    echo "=== Warning: Parley API service may have failed to start ==="
    systemctl status parley-api.service || true
fi

# Verify nginx is running
systemctl status nginx --no-pager || true

echo "=== API droplet setup complete ==="