#!/usr/bin/env bash
# One named hybrid unit: cosmetic PoH + dig/hunt under the SAME worker_id.
# Dig = WASM property claims; Hunt = ASAN hunt_shard claims (rc17).
# Used by start_test_named_fleet.sh. Does not use *-fuzz sybil ids.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COORD_URL="${COORD_URL:?}"
TOKEN="${COORD_TOKEN:-${COORD_ADMIN_TOKEN:?}}"
WORKER_ID="${WORKER_ID:?}"
WORKER_NAME="${WORKER_NAME:-$WORKER_ID}"
LOG_DIR="${LOG_DIR:?}"
SEED_HEX="${HACKME_MINER_ED25519_SEED_HEX:?}"
FORCE_GH="${FORCE_HASHRATE_GHS:?}"
BATCH_SIZE="${BATCH_SIZE:-2097152}"
MINERSIGN_BIN="${MINERSIGN_BIN:-$ROOT/bin/minersign}"
ENABLE_FUZZ="${HACKME_NAMED_HYBRID_FUZZ:-1}"
FUZZ_BIN="${WORKERFUZZ_BIN:-}"
if [[ -z "$FUZZ_BIN" ]]; then
  if [[ -x "$ROOT/bin/workerfuzz" ]]; then FUZZ_BIN="$ROOT/bin/workerfuzz"
  elif [[ -x "$ROOT/workerfuzz" ]]; then FUZZ_BIN="$ROOT/workerfuzz"
  fi
fi

POH_LOG="${LOG_DIR}/${WORKER_ID}.log"
FUZZ_LOG="${LOG_DIR}/${WORKER_ID}.fuzz.log"
LOCK_DIR="${LOG_DIR}/locks"
mkdir -p "$LOG_DIR" "$LOCK_DIR" "$ROOT/.cache/hunt-harness"
# Dig WASM timeout (short). Hunt uses HACKME_WORKER_HUNT_TIMEOUT_MS floor (≥180s in workerfuzzloop).
FUZZ_TIMEOUT_MS="${WORKERFUZZ_TIMEOUT_MS:-2500}"

# Only reclaim fuzz lock for this worker_id (PoH loop has no instance lock).
pkill -f "bin/workerfuzz .* -worker ${WORKER_ID}( |$)" 2>/dev/null || true
sleep 0.2

cleanup() {
  local p
  for p in "${PIDS[@]:-}"; do
    [[ -n "${p:-}" ]] || continue
    kill -TERM "$p" 2>/dev/null || true
    pkill -P "$p" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

PIDS=()

(
  export COORD_URL COORD_ADMIN_TOKEN="$TOKEN" COORD_TOKEN="$TOKEN"
  export WORKER_ID WORKER_NAME BATCH_SIZE
  export HASHRATE_GHS="$FORCE_GH" FORCE_HASHRATE_GHS="$FORCE_GH"
  export HACKME_WORKER_SIGN_SUBMITS=1
  export HACKME_MINER_ED25519_SEED_HEX="$SEED_HEX"
  export HACKME_MINER_NONCE_FILE="${LOG_DIR}/${WORKER_ID}.nonce"
  export MINERSIGN_BIN COORD_PUSH_WORK=1
  # Leave claim budget for dig/hunt under the same worker_id (coord per-worker cap).
  export HACKME_WORKER_CLAIM_COOLDOWN_MS="${HACKME_WORKER_CLAIM_COOLDOWN_MS:-8000}"
  exec bash "$ROOT/scripts/ops/worker_loop.sh"
) >>"$POH_LOG" 2>&1 &
PIDS+=($!)
echo "[hybrid-unit] PoH pid=${PIDS[0]} id=${WORKER_ID} gh=${FORCE_GH}"

if [[ "$ENABLE_FUZZ" == "1" || "$ENABLE_FUZZ" == "true" || "$ENABLE_FUZZ" == "yes" ]]; then
  if [[ -z "$FUZZ_BIN" || ! -x "$FUZZ_BIN" ]]; then
    echo "[hybrid-unit] WARN: workerfuzz missing — PoH-only for ${WORKER_ID}" >&2
  else
    (
      export COORD_URL COORD_TOKEN="$TOKEN"
      export WORKER_ID
      export HACKME_MINER_ED25519_SEED_HEX="$SEED_HEX"
      export HACKME_WORKER_LOCK_DIR="$LOCK_DIR"
      # Hunt/dig shared env — look like a real hybrid miner.
      export HACKME_REPO_ROOT="${HACKME_REPO_ROOT:-$ROOT}"
      export HACKME_COORDINATOR_URL="${HACKME_COORDINATOR_URL:-$COORD_URL}"
      export HACKME_POOL_COORDINATOR_URL="${HACKME_POOL_COORDINATOR_URL:-$COORD_URL}"
      export HACKME_WORKER_HUNT_SHARDS="${HACKME_WORKER_HUNT_SHARDS:-1}"
      export HACKME_WORKER_HUNT_TIMEOUT_MS="${HACKME_WORKER_HUNT_TIMEOUT_MS:-180000}"
      export WORKERFUZZ_HTTP_TIMEOUT_SEC="${WORKERFUZZ_HTTP_TIMEOUT_SEC:-120}"
      export HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY="${HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY:-1}"
      export HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS="${HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS:-8000}"
      exec "$FUZZ_BIN" -coord "$COORD_URL" -token "$TOKEN" -worker "$WORKER_ID" \
        -timeout-ms "$FUZZ_TIMEOUT_MS"
    ) >>"$FUZZ_LOG" 2>&1 &
    PIDS+=($!)
    echo "[hybrid-unit] dig/hunt pid=${PIDS[1]} id=${WORKER_ID} (hybrid, not *-fuzz)"
  fi
else
  echo "[hybrid-unit] fuzz/hunt off for ${WORKER_ID}"
fi

# Exit if any child dies (systemd Restart= will bring the unit back).
while true; do
  for p in "${PIDS[@]}"; do
    if ! kill -0 "$p" 2>/dev/null; then
      echo "[hybrid-unit] child $p exited — stopping unit ${WORKER_ID}"
      exit 1
    fi
  done
  sleep 2
done
