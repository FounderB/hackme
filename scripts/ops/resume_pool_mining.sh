#!/usr/bin/env bash
# Clear mining pause and restore watchdog/autostart flags (inverse of stop_pool_workers.sh).
#
#   bash scripts/ops/resume_pool_mining.sh
#   ROOT_DIR=/path/to/linux bash scripts/ops/resume_pool_mining.sh
#   Then: bash start_hackme_miner.sh  (or POST /api/worker/start)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Flat release layout: …/linux/resume_pool_mining.sh → ROOT = linux/
# Checkout layout: …/scripts/ops/resume_pool_mining.sh → ROOT = repo
if [[ -x "${SCRIPT_DIR}/hackme" || -x "${SCRIPT_DIR}/bin/workerpoh" || -x "${SCRIPT_DIR}/workerpoh" ]]; then
  ROOT_DIR="$(cd "${ROOT_DIR:-${HACKME_ROOT:-$SCRIPT_DIR}}" && pwd)"
else
  ROOT_DIR="$(cd "${ROOT_DIR:-${HACKME_ROOT:-$SCRIPT_DIR/../..}}" && pwd)"
fi
LOG_DIR="${LOG_DIR:-$ROOT_DIR/logs}"
DESKTOP_ENV_FILE="${DESKTOP_ENV_FILE:-$ROOT_DIR/.env.desktop}"
ENV_FILE="${ENV_FILE:-$ROOT_DIR/.env}"
PAUSE_FILE="${HACKME_MINING_PAUSED_FILE:-$LOG_DIR/mining_paused}"

mkdir -p "$LOG_DIR"
rm -f "$PAUSE_FILE" \
  "$ROOT_DIR/logs/mining_paused" \
  "$ROOT_DIR/logs/desktop/mining_paused" 2>/dev/null || true

for ef in "$DESKTOP_ENV_FILE" "$ENV_FILE"; do
  [[ -f "$ef" ]] || continue
  for key in HACKME_WORKER_WATCHDOG WORKER_AUTOSTART; do
    if grep -q "^${key}=" "$ef" 2>/dev/null; then
      sed -i "s/^${key}=.*/${key}=1/" "$ef"
    else
      echo "${key}=1" >>"$ef"
    fi
  done
done

echo "[resume-mining] cleared pause; watchdog/autostart restored in env"
echo "[resume-mining] start workers: bash ${ROOT_DIR}/start_hackme_miner.sh   OR   POST /api/worker/start"
