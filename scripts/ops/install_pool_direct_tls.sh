#!/usr/bin/env bash
# Install pool-direct TLS (:18443) + shared location snippet on hub.
# Does NOT change fleet COORD_URL (keeps :18083).
#
#   NODE_SSH=hackme-vps bash scripts/ops/install_pool_direct_tls.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
NODE_SSH="${NODE_SSH:-hackme-vps}"

echo "[pool-tls] syncing nginx assets to ${NODE_SSH}"
ssh -o BatchMode=yes "$NODE_SSH" 'sudo mkdir -p /etc/nginx/snippets /etc/nginx/sites-available'
scp -o BatchMode=yes \
  "$ROOT/deploy/nginx/snippets/hackme-pool-direct-locations.conf" \
  "$NODE_SSH:/tmp/hackme-pool-direct-locations.conf"
scp -o BatchMode=yes \
  "$ROOT/deploy/nginx/hackme-pool-direct.conf" \
  "$NODE_SSH:/tmp/hackme-pool-direct.conf"
scp -o BatchMode=yes \
  "$ROOT/deploy/nginx/hackme-pool-direct-tls.conf" \
  "$NODE_SSH:/tmp/hackme-pool-direct-tls.conf"

ssh -o BatchMode=yes "$NODE_SSH" bash -s <<'REMOTE'
set -euo pipefail
sudo cp /tmp/hackme-pool-direct-locations.conf /etc/nginx/snippets/hackme-pool-direct-locations.conf
sudo cp /tmp/hackme-pool-direct.conf /etc/nginx/sites-available/hackme-pool-direct.conf
sudo cp /tmp/hackme-pool-direct-tls.conf /etc/nginx/sites-available/hackme-pool-direct-tls.conf
sudo ln -sfn /etc/nginx/sites-available/hackme-pool-direct.conf /etc/nginx/sites-enabled/hackme-pool-direct.conf
sudo ln -sfn /etc/nginx/sites-available/hackme-pool-direct-tls.conf /etc/nginx/sites-enabled/hackme-pool-direct-tls.conf
# ufw: allow TLS direct without touching mining fleet rules
if command -v ufw >/dev/null 2>&1; then
  sudo ufw status | grep -q '18443/tcp' || sudo ufw allow 18443/tcp comment 'HackMe pool direct TLS'
fi
sudo nginx -t
sudo systemctl reload nginx
echo "[pool-tls] testing local listeners"
curl -fsS --max-time 5 http://127.0.0.1:18083/health
curl -fkS --max-time 5 https://127.0.0.1:18443/health
ss -lntp | grep -E ':18083|:18443' || true
REMOTE

echo "[pool-tls] done — fleet still on :18083; TLS ready on :18443"
