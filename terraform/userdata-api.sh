#!/bin/bash
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

# Install required packages with retry
echo "=== Installing required packages ==="
run_with_retry "apt-get install -y git curl build-essential nginx certbot python3-certbot-nginx ufw software-properties-common redis-tools"

# Install Node.js (LTS)
echo "=== Installing Node.js ==="
curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
run_with_retry "apt-get install -y nodejs"

# Install Go from apt repository
echo "=== Installing Go ==="
run_with_retry "apt-get install -y golang-go"

# Verify Go installation
if ! command -v go &> /dev/null; then
    echo "ERROR: Go installation failed"
    exit 1
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
run_with_retry "GONOSUMDB=* go mod download"
run_with_retry "GONOSUMDB=* go build -mod=mod -o /usr/local/bin/parley-api ./cmd/api"

# Verify binary was created
if [ ! -f "/usr/local/bin/parley-api" ]; then
    echo "ERROR: API binary not created"
    exit 1
fi

# Build the frontend
echo "=== Building Parley frontend ==="
cd /parley/frontend
run_with_retry "npm ci"
run_with_retry "npm run build"
mkdir -p /var/www/parley
cp -r dist/* /var/www/parley/

# Create environment file
echo "=== Creating environment configuration ==="
mkdir -p /etc/parley

# URL-encode the DB password to handle special characters
DB_PASSWORD_ENCODED=$(python3 -c "import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=''))" "$DB_PASSWORD")

# Create environment file with PATH for Go
cat > /etc/parley/env <<EOF
DATABASE_URL=postgresql://${DB_USER}:$${DB_PASSWORD_ENCODED}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable
JWT_SECRET=${JWT_SECRET}
PORT=${PORT}
REDIS_URL=redis://${REDIS_HOST}:6379
PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
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
ExecStartPre=/bin/sh -c 'until redis-cli -h ${REDIS_HOST} ping 2>/dev/null | grep -q PONG; do echo "Waiting for Redis..."; sleep 2; done'
ExecStart=/usr/local/bin/parley-api
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

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
if ! nginx -t 2>&1 | grep -q "syntax is ok"; then
    echo "ERROR: Nginx configuration failed"
    nginx -t 2>&1 || true
    exit 1
fi

# Restart nginx with retry
run_with_retry "systemctl restart nginx"
run_with_retry "systemctl enable nginx"

# Configure firewall
echo "=== Configuring firewall ==="
ufw allow 22/tcp    # SSH
ufw allow 80/tcp    # HTTP
ufw allow 443/tcp   # HTTPS
ufw --force enable

# Start the API service with retry
echo "=== Starting Parley API service ==="
run_with_retry "systemctl start parley-api.service"

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
