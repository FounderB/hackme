#!/usr/bin/env bash
# Install persistent SSH tunnel so desktop settle-pull / pool-sync admin hits
# VPS coordinator loopback :18081 (not nginx :18083 which 403s admin routes).
#
# Usage:
#   bash scripts/ops/install_coord_admin_tunnel_user.sh
#   SSH_HOST=hackme-vps bash scripts/ops/install_coord_admin_tunnel_user.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${HACKME_REPO_ROOT:-$ROOT}"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
SSH_HOST="${SSH_HOST:-hackme-vps}"
mkdir -p "$UNIT_DIR"

sed -e "s|hackme-vps|$SSH_HOST|g" \
  "$ROOT/scripts/ops/systemd/hackme-coord-admin-tunnel.service" \
  >"$UNIT_DIR/hackme-coord-admin-tunnel.service"

# Free the port if a manual ssh -L is already bound.
if ss -ltn 2>/dev/null | grep -q '127.0.0.1:18081'; then
  if ! systemctl --user is-active --quiet hackme-coord-admin-tunnel.service 2>/dev/null; then
    echo "[install-coord-tunnel] freeing manual :18081 listeners"
    pkill -f 'ssh .* -L 127\.0\.0\.1:18081:127\.0\.0\.1:18081' 2>/dev/null || true
    sleep 1
  fi
fi

systemctl --user daemon-reload
systemctl --user enable --now hackme-coord-admin-tunnel.service
ok=0
for i in 1 2 3 4 5 6 7 8; do
  sleep 1
  if curl -fsS --max-time 5 http://127.0.0.1:18081/health >/dev/null 2>&1; then
    ok=1
    break
  fi
done
if [[ "$ok" == "1" ]]; then
  echo "[install-coord-tunnel] OK http://127.0.0.1:18081/health"
else
  echo "[install-coord-tunnel] WARN: tunnel unit started but /health not OK yet" >&2
  systemctl --user status hackme-coord-admin-tunnel.service --no-pager || true
  exit 1
fi
systemctl --user status hackme-coord-admin-tunnel.service --no-pager || true
