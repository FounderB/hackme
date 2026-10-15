#!/usr/bin/env bash
# Settle accrued HMC for named-fleet workers onto their unique locked wallets.
#
#   bash scripts/ops/fleet_settle_named.sh           # live settle (allowlisted)
#   bash scripts/ops/fleet_settle_named.sh --dry-run # print accrued only
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/local/bin:${PATH:-}
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

LOG_DIR="${LOG_DIR:-$ROOT/logs/test-named-fleet}"
SEED_DIR="${SEED_DIR:-$LOG_DIR/seeds}"
COORD_URL="${COORD_URL:-http://132.243.112.100:18083}"
CHAIN_BASE="${CHAIN_BASE:-http://127.0.0.1:18080}"
DRY=0
[[ "${1:-}" == "--dry-run" ]] && DRY=1

NAMES=(
  desktop-a4m2rx desktop-k7v1pd desktop-q9n4ls desktop-t2c8we desktop-z5h6mf
  shannon turing hopper knuth mccarthy
  lovelace tesla euclid noether faraday
  ui-west digga fuzzlane rack-neon byte-hop
  desktop-p3r9kx-w desktop-m2w8qn-w desktop-h7c4vj-w
  desktop-y1d8qt-w desktop-q8n2kp-w desktop-x4m7vt-w
  anechka FroZenn
)
IDS=()
for n in "${NAMES[@]}"; do
  [[ -f "$SEED_DIR/worker-${n}.seed" ]] && IDS+=("worker-${n}")
done
(( ${#IDS[@]} > 0 )) || { echo "[fleet-settle] no seed workers in $SEED_DIR" >&2; exit 1; }

ALLOW="$(printf '%s,' "${IDS[@]}" | sed 's/,$//')"
echo "[fleet-settle] allowlist=${#IDS[@]} workers dry=$DRY coord=$COORD_URL"

TOKEN="${COORD_ADMIN_TOKEN:-${HACKME_COORDINATOR_ADMIN_TOKEN:-${COORD_TOKEN:-}}}"
if [[ -z "$TOKEN" && -f "$ROOT/.secrets/hackme_coordinator_admin_token" ]]; then
  TOKEN="$(tr -d '\r\n' <"$ROOT/.secrets/hackme_coordinator_admin_token")"
fi
if [[ -z "$TOKEN" && -f "$ROOT/.secrets/hackme_coordinator_worker_token" ]]; then
  TOKEN="$(tr -d '\r\n' <"$ROOT/.secrets/hackme_coordinator_worker_token")"
fi
[[ -n "$TOKEN" ]] || { echo "[fleet-settle] missing coordinator admin/worker token" >&2; exit 1; }

# Snapshot accrued for fleet (dry or pre-flight)
curl -fsS --max-time 25 -H "X-Hackme-Admin-Token: $TOKEN" \
  "${COORD_URL}/api/work/stats?details=1" -o /tmp/fleet-settle-stats.json || {
  echo "[fleet-settle] WARN: details=1 failed, trying public stats" >&2
  curl -fsS --max-time 20 -A 'Mozilla/5.0' \
    'https://hackme.tech/pool/coordinator/api/work/stats' -o /tmp/fleet-settle-stats.json
}

python3 - "$ALLOW" <<'PY'
import json,sys
allow=set(x for x in sys.argv[1].split(",") if x)
d=json.load(open("/tmp/fleet-settle-stats.json"))
w=d.get("workers") or {}
total=0.0; n=0
print("--- fleet accrued ---")
for wid in sorted(allow):
  x=w.get(wid) or {}
  pay=float(x.get("payout_hmc") or 0)
  addr=x.get("payout_address") or ""
  if pay<=0 and not addr: continue
  n+=1; total+=pay
  print(f"  {wid}\t{pay:.8f}\t{addr}")
print(f"[fleet-settle] rows={n} accrued_hmc={total:.8f}")
PY

if (( DRY )); then
  echo "[fleet-settle] dry-run only — no transfers"
  exit 0
fi

export COORD_URL
export COORD_ADMIN_TOKEN="$TOKEN"
export HACKME_COORDINATOR_ADMIN_TOKEN="$TOKEN"
export SETTLE_WORKER_ALLOWLIST="$ALLOW"
export HACKME_SETTLE_WORKER_ALLOWLIST="$ALLOW"
export STATE_FILE="${STATE_FILE:-$LOG_DIR/worker_settlement_state.json}"
export MIN_SETTLE_HMC="${MIN_SETTLE_HMC:-0.0001}"
export SETTLE_SEQUENTIAL="${SETTLE_SEQUENTIAL:-1}"
export SETTLE_PAYOUT_PAUSE_SEC="${SETTLE_PAYOUT_PAUSE_SEC:-12}"

if [[ -z "${ADMIN_TOKEN:-${HACKME_ADMIN_TOKEN:-}}" && -f "$ROOT/.secrets/hackme_admin_token" ]]; then
  export ADMIN_TOKEN="$(tr -d '\r\n' <"$ROOT/.secrets/hackme_admin_token")"
  export HACKME_ADMIN_TOKEN="$ADMIN_TOKEN"
fi

# Settle on VPS (treasury + chain). Sync allowlist patch so we never settle non-fleet.
if [[ "${LOCAL_SETTLE:-0}" != "1" && -n "${FAUCET_SSH:-hackme-vps}" ]]; then
  HOST="${FAUCET_SSH:-hackme-vps}"
  REMOTE_ROOT="$(ssh -o BatchMode=yes -o ConnectTimeout=15 "$HOST" \
    'for d in /home/kapa/Desktop/HackMe ~/HackMe /opt/hackme; do [[ -x $d/scripts/ops/settle_worker_payouts.sh ]] && echo $d && break; done')"
  [[ -n "$REMOTE_ROOT" ]] || { echo "[fleet-settle] remote HackMe settle script not found" >&2; exit 1; }
  # Ensure allowlist support exists remotely (idempotent patch)
  scp -o BatchMode=yes -o ConnectTimeout=15 \
    "$ROOT/scripts/ops/settle_worker_payouts.sh" \
    "$HOST:$REMOTE_ROOT/scripts/ops/settle_worker_payouts.sh"
  ssh -o BatchMode=yes -o ConnectTimeout=20 "$HOST" bash -s <<REMOTE
set -euo pipefail
cd "$REMOTE_ROOT"
export SETTLE_WORKER_ALLOWLIST='$ALLOW'
export HACKME_SETTLE_WORKER_ALLOWLIST='$ALLOW'
export COORD_URL='${COORD_URL}'
export MIN_SETTLE_HMC='${MIN_SETTLE_HMC:-0.0001}'
export STATE_FILE='data/worker_settlement_state.named-fleet.json'
export SETTLE_SEQUENTIAL=1
export SETTLE_PAYOUT_PAUSE_SEC=12
bash scripts/ops/settle_worker_payouts.sh
REMOTE
else
  export CHAIN_BASE
  export SETTLE_WORKER_ALLOWLIST="$ALLOW"
  export HACKME_SETTLE_WORKER_ALLOWLIST="$ALLOW"
  bash "$ROOT/scripts/ops/settle_worker_payouts.sh"
fi

echo "[fleet-settle] live settle finished"
