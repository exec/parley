#!/usr/bin/env bash
# Cross-compile parley-api and ship it to the proxmox LXC.
#
# Usage:   scripts/deploy-api.sh
# Env:     VMID (default 102), SSH_HOST (default eqr), SERVICE (default parley-api)
# Rollback: ssh "$SSH_HOST" "sudo pct exec $VMID -- bash -c \
#            'mv /usr/local/bin/parley-api.bak /usr/local/bin/parley-api && \
#             systemctl restart $SERVICE'"
#
# Note: single-instance restart drops in-flight WebSocket connections briefly.
# For zero-downtime, run multiple api LXCs behind parley-lb and roll one at a time.

set -euo pipefail

REPO=$(git rev-parse --show-toplevel)
cd "$REPO"

VMID=${VMID:-102}
SSH_HOST=${SSH_HOST:-eqr}
SERVICE=${SERVICE:-parley-api}
DEST=/usr/local/bin/parley-api
LOCAL_BUILD=$(mktemp -t parley-api.XXXXXX)
trap 'rm -f "$LOCAL_BUILD"' EXIT

echo "==> cross-compiling parley-api (linux/amd64, static)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=readonly -trimpath -o "$LOCAL_BUILD" ./cmd/api

SIZE=$(wc -c <"$LOCAL_BUILD" | tr -d ' ')
echo "    built ${SIZE} bytes"

echo "==> staging on ${SSH_HOST}..."
scp -q "$LOCAL_BUILD" "${SSH_HOST}:/tmp/parley-api.new"

echo "==> swapping binary in LXC ${VMID} and restarting ${SERVICE}..."
ssh "$SSH_HOST" "set -e
sudo pct push ${VMID} /tmp/parley-api.new ${DEST}.new --perms 0755
sudo pct exec ${VMID} -- bash -c '
  set -e
  [ -f ${DEST} ] && cp -f ${DEST} ${DEST}.bak
  mv ${DEST}.new ${DEST}
  systemctl restart ${SERVICE}
  sleep 1
  systemctl is-active --quiet ${SERVICE}
'
rm -f /tmp/parley-api.new"

echo "==> done. ${SERVICE} restarted on LXC ${VMID}. Backup at ${DEST}.bak"
