#!/usr/bin/env bash
# Coordinator + node perf deploy: local build → VPS binaries, nginx, settlement.
#   NODE_SSH=hackme-vps bash scripts/ops/apply_coordinator_perf_vps.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NODE_SSH="${NODE_SSH:-hackme-vps}"
DEPLOY="${NODE_DEPLOY_DIR:-/opt/hackme}"

echo "[coord-perf] build coordinator + node"
(cd "$ROOT" && go build -o /tmp/hackme-coordinator ./cmd/coordinator/ && go build -o /tmp/hackme-node .)

echo "[coord-perf] rsync binaries + ops scripts"
rsync -avz /tmp/hackme-coordinator /tmp/hackme-node \
  "$ROOT/scripts/ops/vps_patch_coordinator_nginx_query.sh" \
  "$ROOT/scripts/ops/settle_worker_payouts.sh" \
  "$ROOT/scripts/ops/sync_settlement_admin_token.sh" \
  "$ROOT/scripts/ops/repair_worker_settlement_state.sh" \
  "$ROOT/scripts/ops/systemd/hackme-worker-settlement.service" \
  "$NODE_SSH:$DEPLOY/"
rsync -avz "$ROOT/scripts/ops/systemd/hackme-worker-settlement.service" \
  "$NODE_SSH:/tmp/hackme-worker-settlement.service"

ssh -o BatchMode=yes "$NODE_SSH" "bash -s" <<REMOTE
set -euo pipefail
DEPLOY='$DEPLOY'
# systemd uses /opt/hackme/coordinator (not hackme-coordinator)
sudo systemctl stop hackme-coordinator
sudo cp "\$DEPLOY/hackme-coordinator" "\$DEPLOY/coordinator"
sudo chmod 755 "\$DEPLOY/coordinator" "\$DEPLOY/hackme-node"
sudo systemctl daemon-reload
# Nginx query pass-through + reload
if [[ -f "\$DEPLOY/scripts/ops/vps_patch_coordinator_nginx_query.sh" ]]; then
  sudo bash "\$DEPLOY/scripts/ops/vps_patch_coordinator_nginx_query.sh" || true
fi
COORD_UNIT=/etc/systemd/system/hackme-coordinator.service
if [[ -f "\$COORD_UNIT" ]]; then
  sudo grep -q 'HACKME_COORDINATOR_WRITE_TIMEOUT_SEC' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_COORDINATOR_WRITE_TIMEOUT_SEC=480' "\$COORD_UNIT" || true
  # Cap Hunt ASAN on small VPS — dual workers + 250ms idle poll pegged one core.
  if sudo grep -q 'HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=3' "\$COORD_UNIT"; then
    sudo sed -i 's/HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=3/HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=1/' "\$COORD_UNIT" || true
  fi
  if sudo grep -q 'HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=2' "\$COORD_UNIT"; then
    sudo sed -i 's/HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=2/HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=1/' "\$COORD_UNIT" || true
  fi
  sudo grep -q 'HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_POOL_HUNT_REPLAY_MAX_PARALLEL=1' "\$COORD_UNIT" || true
  if sudo grep -q 'HACKME_POOL_HUNT_REPLAY_WORKERS=2' "\$COORD_UNIT"; then
    sudo sed -i 's/HACKME_POOL_HUNT_REPLAY_WORKERS=2/HACKME_POOL_HUNT_REPLAY_WORKERS=1/' "\$COORD_UNIT" || true
  fi
  sudo grep -q 'HACKME_POOL_HUNT_REPLAY_WORKERS' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_POOL_HUNT_REPLAY_WORKERS=1' "\$COORD_UNIT" || true
  sudo grep -q 'HACKME_COORDINATOR_READ_TIMEOUT_SEC' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_COORDINATOR_READ_TIMEOUT_SEC=60' "\$COORD_UNIT" || true
  if sudo grep -q 'HACKME_POOL_TICK_SEC=5' "\$COORD_UNIT"; then
    sudo sed -i 's/HACKME_POOL_TICK_SEC=5/HACKME_POOL_TICK_SEC=10/' "\$COORD_UNIT" || true
  fi
  sudo grep -q 'HACKME_POOL_TICK_SEC' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_POOL_TICK_SEC=10' "\$COORD_UNIT" || true
  sudo grep -q 'HACKME_COORDINATOR_PEER_FLUSH_SEC' "\$COORD_UNIT" || \
    sudo sed -i '/^EnvironmentFile=/a Environment=HACKME_COORDINATOR_PEER_FLUSH_SEC=5' "\$COORD_UNIT" || true
  # Below GPU per-worker floor (120): global>=120 forced lim>=120 and pegged the VPS core.
  if [[ -f "\$DEPLOY/.env.coord" ]]; then
    sudo sed -i 's/^HACKME_COORDINATOR_CLAIM_PER_MIN=.*/HACKME_COORDINATOR_CLAIM_PER_MIN=90/' "\$DEPLOY/.env.coord" || true
    sudo sed -i 's/^HACKME_COORDINATOR_SUBMIT_PER_MIN=.*/HACKME_COORDINATOR_SUBMIT_PER_MIN=480/' "\$DEPLOY/.env.coord" || true
    grep -q '^HACKME_COORDINATOR_CLAIM_PER_MIN=' "\$DEPLOY/.env.coord" || echo 'HACKME_COORDINATOR_CLAIM_PER_MIN=90' | sudo tee -a "\$DEPLOY/.env.coord" >/dev/null
    grep -q '^HACKME_COORDINATOR_SUBMIT_PER_MIN=' "\$DEPLOY/.env.coord" || echo 'HACKME_COORDINATOR_SUBMIT_PER_MIN=480' | sudo tee -a "\$DEPLOY/.env.coord" >/dev/null
  fi
  sudo systemctl daemon-reload
fi
sudo cp /tmp/hackme-worker-settlement.service /etc/systemd/system/hackme-worker-settlement.service
sudo systemctl restart hackme-coordinator hackme-node
sudo chown hackme:hackme "\$DEPLOY/data/worker_settlement_state.json" 2>/dev/null || true
bash "\$DEPLOY/scripts/ops/sync_settlement_admin_token.sh" 2>/dev/null || true
bash "\$DEPLOY/scripts/ops/repair_worker_settlement_state.sh" 2>/dev/null || true
systemctl start hackme-worker-settlement.service || true
echo "[coord-perf] coordinator: \$(systemctl is-active hackme-coordinator) node: \$(systemctl is-active hackme-node)"
curl -fsS http://127.0.0.1:18081/api/pool/stats | head -c 160
echo
REMOTE

echo "[coord-perf] done"
