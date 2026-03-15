#!/bin/bash
# Cloud-init script for the Parley voice (LiveKit) droplet
# DNS prereq: A record  vc.parley.x86-64.com → this droplet's public IP

LIVEKIT_API_KEY="${LIVEKIT_API_KEY}"
LIVEKIT_API_SECRET="${LIVEKIT_API_SECRET}"
VC_DOMAIN="vc.parley.x86-64.com"

echo "=== Starting Parley VC (LiveKit) setup ==="

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get upgrade -y
apt-get install -y ca-certificates curl ufw nginx certbot python3-certbot-nginx

# ── Docker ────────────────────────────────────────────────────────────────────
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  > /etc/apt/sources.list.d/docker.list
apt-get update -y
apt-get install -y docker-ce docker-ce-cli containerd.io
systemctl enable docker
systemctl start docker

# ── Detect public IP ──────────────────────────────────────────────────────────
PUBLIC_IP=$(curl -sf http://169.254.169.254/metadata/v1/interfaces/public/0/ipv4/address || hostname -I | awk '{print $1}')
echo "Public IP: $PUBLIC_IP"

# ── LiveKit config ────────────────────────────────────────────────────────────
mkdir -p /etc/livekit
cat > /etc/livekit/livekit.yaml <<EOF
port: 7880
bind_addresses:
  - ""
rtc:
  tcp_port: 7881
  port_range_start: 50000
  port_range_end: 60000
  use_external_ip: true
  node_ip: $${PUBLIC_IP}
keys:
  ${LIVEKIT_API_KEY}: ${LIVEKIT_API_SECRET}
logging:
  level: info
  pion_level: error
EOF

# ── LiveKit systemd service ───────────────────────────────────────────────────
docker pull livekit/livekit-server:latest

cat > /etc/systemd/system/livekit.service <<EOF
[Unit]
Description=LiveKit SFU
After=docker.service
Requires=docker.service

[Service]
Restart=always
RestartSec=5
ExecStartPre=-/usr/bin/docker rm -f livekit
ExecStart=/usr/bin/docker run --rm --name livekit \
  --network host \
  -v /etc/livekit/livekit.yaml:/etc/livekit.yaml \
  livekit/livekit-server:latest \
  --config /etc/livekit.yaml
ExecStop=/usr/bin/docker stop livekit
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable livekit
systemctl start livekit

# ── Nginx: proxy port 80/443 → LiveKit 7880 (WebSocket-aware) ────────────────
cat > /etc/nginx/sites-available/livekit <<EOF
server {
    listen 80;
    server_name $${VC_DOMAIN};

    location / {
        proxy_pass http://127.0.0.1:7880;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
EOF

rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/livekit /etc/nginx/sites-enabled/
nginx -t && systemctl restart nginx && systemctl enable nginx

# ── TLS via certbot (non-fatal — DNS may not be ready yet) ───────────────────
certbot --nginx -d "$${VC_DOMAIN}" --non-interactive --agree-tos \
  --email noreply@parley.x86-64.com --redirect || \
  echo "WARNING: certbot failed (DNS not ready?) — run certbot manually after DNS propagates"

# ── Firewall ──────────────────────────────────────────────────────────────────
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 7880/tcp          # LiveKit direct (fallback)
ufw allow 7881/tcp          # LiveKit TCP ICE
ufw allow 3478/udp          # TURN UDP
ufw allow 50000:60000/udp   # WebRTC media
ufw --force enable

echo "=== LiveKit VC setup complete ==="
echo "LiveKit available at: http://$${PUBLIC_IP}:7880 (and https://$${VC_DOMAIN} once DNS is set)"
