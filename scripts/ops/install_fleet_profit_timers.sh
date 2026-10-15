#!/usr/bin/env bash
# Install user systemd timers: dig/hunt boost watchdog + periodic settle + metrics.
# Local-only (scripts often gitignored from GitHub).
#
#   bash scripts/ops/install_fleet_profit_timers.sh
#   bash scripts/ops/install_fleet_profit_timers.sh uninstall
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/local/bin:${PATH:-}
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$UNIT_DIR"

chmod +x \
  "$ROOT/scripts/ops/fleet_profit_status.sh" \
  "$ROOT/scripts/ops/fleet_profit_watchdog.sh" \
  "$ROOT/scripts/ops/fleet_settle_named.sh" \
  "$ROOT/scripts/ops/start_test_named_fleet.sh" \
  2>/dev/null || true

if [[ "${1:-}" == "uninstall" ]]; then
  systemctl --user disable --now hackme-fleet-profit-watchdog.timer 2>/dev/null || true
  systemctl --user disable --now hackme-fleet-settle.timer 2>/dev/null || true
  systemctl --user disable --now hackme-fleet-metrics.timer 2>/dev/null || true
  rm -f "$UNIT_DIR"/hackme-fleet-{profit-watchdog,settle,metrics}.{service,timer}
  systemctl --user daemon-reload
  echo "[fleet-timers] uninstalled"
  exit 0
fi

cat >"$UNIT_DIR/hackme-fleet-profit-watchdog.service" <<EOF
[Unit]
Description=HackMe named fleet dig/hunt boost watchdog
[Service]
Type=oneshot
WorkingDirectory=$ROOT
Environment=PATH=/usr/bin:/bin:/usr/local/bin
ExecStart=/bin/bash $ROOT/scripts/ops/fleet_profit_watchdog.sh
EOF

cat >"$UNIT_DIR/hackme-fleet-profit-watchdog.timer" <<'EOF'
[Unit]
Description=Run dig/hunt boost watchdog every 3 min
[Timer]
OnBootSec=2min
OnUnitActiveSec=3min
AccuracySec=15s
Persistent=true
[Install]
WantedBy=timers.target
EOF

cat >"$UNIT_DIR/hackme-fleet-settle.service" <<EOF
[Unit]
Description=HackMe named fleet settle to unique wallets
[Service]
Type=oneshot
WorkingDirectory=$ROOT
Environment=PATH=/usr/bin:/bin:/usr/local/bin
Environment=FAUCET_SSH=hackme-vps
ExecStart=/bin/bash $ROOT/scripts/ops/fleet_settle_named.sh
EOF

cat >"$UNIT_DIR/hackme-fleet-settle.timer" <<'EOF'
[Unit]
Description=Settle named fleet every 6 hours
[Timer]
OnBootSec=10min
OnUnitActiveSec=6h
AccuracySec=5min
Persistent=true
[Install]
WantedBy=timers.target
EOF

cat >"$UNIT_DIR/hackme-fleet-metrics.service" <<EOF
[Unit]
Description=HackMe named fleet profit metrics snapshot
[Service]
Type=oneshot
WorkingDirectory=$ROOT
Environment=PATH=/usr/bin:/bin:/usr/local/bin
Environment=WATCH=1
Environment=FAUCET_SSH=hackme-vps
ExecStart=/bin/bash $ROOT/scripts/ops/fleet_profit_status.sh
EOF

cat >"$UNIT_DIR/hackme-fleet-metrics.timer" <<'EOF'
[Unit]
Description=Fleet profit metrics every 5 min
[Timer]
OnBootSec=1min
OnUnitActiveSec=5min
AccuracySec=20s
Persistent=true
[Install]
WantedBy=timers.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now hackme-fleet-profit-watchdog.timer
systemctl --user enable --now hackme-fleet-settle.timer
systemctl --user enable --now hackme-fleet-metrics.timer
echo "[fleet-timers] installed + enabled"
systemctl --user list-timers --all 'hackme-fleet-*' 2>/dev/null || true
