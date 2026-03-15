#!/bin/bash
set -euo pipefail
for IP in 174.138.51.177 159.203.111.52 167.71.186.109; do
  ssh -o StrictHostKeyChecking=no root@$IP "
    cd /parley && git pull origin main
    cd frontend && npm run build
    cp -r dist/* /var/www/parley/
    echo 'OK $IP'
  " &
done
wait
