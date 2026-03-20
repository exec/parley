#!/bin/bash
# Nginx load balancer for Parley API nodes.
# Created when api_count > 1; skipped for single-node deployments.
#
# Uses ip_hash for sticky sessions so WebSocket connections from the same
# client always land on the same API node — mirrors the DO LB cookie-based
# sticky sessions and avoids unnecessary Redis pub/sub round-trips for
# message echo. WS idle timeout matches the DO LB (1800s).

set -e

echo "=== Starting Parley LB setup ==="

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y nginx

echo "=== Writing nginx configuration ==="

# UPSTREAM_SERVERS is rendered by Terraform:
#   "    server 192.168.1.11:80;\n    server 192.168.1.12:80;"
cat > /etc/nginx/sites-available/parley-lb <<NGXEOF
upstream parley_api {
    ip_hash;
${UPSTREAM_SERVERS}
}

server {
    listen 80;
    server_name _;

    location /health {
        proxy_pass http://parley_api;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
    }

    location /ws {
        proxy_pass http://parley_api;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
    }

    location / {
        proxy_pass http://parley_api;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
NGXEOF

rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/parley-lb /etc/nginx/sites-enabled/

nginx -t
systemctl restart nginx
systemctl enable nginx

echo "=== LB setup complete ==="
