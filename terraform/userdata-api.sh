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

# Install Go
echo "=== Installing Go ==="
if ! command -v go &> /dev/null; then
    curl -fsSL https://go.dev/dl/go1.21.0.linux-amd64.tar.gz -o /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
fi

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
export PATH=$PATH:/usr/local/go/bin

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
go mod download
go build -o /usr/local/bin/parley-api ./cmd/api

# Create environment file
echo "=== Creating environment configuration ==="
mkdir -p /etc/parley
cat > /etc/parley/env <<EOF
DATABASE_URL=postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable
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

    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
    }

    # Main API proxy
    location / {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;

        # Timeouts for long-running connections
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
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