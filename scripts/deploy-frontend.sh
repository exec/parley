#!/usr/bin/env bash
# Build the frontend and atomically swap it into /var/www/parley on the API LXC.
#
# Usage:  scripts/deploy-frontend.sh
# Env:    VMID (default 102), SSH_HOST (default eqr)
# Rollback: ssh "$SSH_HOST" "sudo pct exec $VMID -- bash -c \
#            'rm -rf /var/www/parley && mv /var/www/parley.old /var/www/parley'"

set -euo pipefail

REPO=$(git rev-parse --show-toplevel)
cd "$REPO/frontend"

VMID=${VMID:-102}
SSH_HOST=${SSH_HOST:-eqr}
DEST=/var/www/parley
TAR=$(mktemp -t parley-frontend.XXXXXX).tar.gz
trap 'rm -f "$TAR"' EXIT

echo "==> building frontend (vite)..."
npm run build >/dev/null

echo "==> packing dist..."
tar -czf "$TAR" -C dist .
echo "    $(wc -c <"$TAR" | tr -d ' ') bytes"

echo "==> staging on ${SSH_HOST}..."
scp -q "$TAR" "${SSH_HOST}:/tmp/parley-frontend.tar.gz"

echo "==> swapping into LXC ${VMID}..."
ssh "$SSH_HOST" "set -e
sudo pct push ${VMID} /tmp/parley-frontend.tar.gz /tmp/parley-frontend.tar.gz
sudo pct exec ${VMID} -- bash -c '
  set -e
  rm -rf /var/www/parley.new
  mkdir -p /var/www/parley.new
  tar -xzf /tmp/parley-frontend.tar.gz -C /var/www/parley.new
  rm -rf /var/www/parley.old
  [ -d ${DEST} ] && mv ${DEST} /var/www/parley.old
  mv /var/www/parley.new ${DEST}
  rm -f /tmp/parley-frontend.tar.gz
'
rm -f /tmp/parley-frontend.tar.gz"

echo "==> done. /var/www/parley swapped; previous tree at /var/www/parley.old"
