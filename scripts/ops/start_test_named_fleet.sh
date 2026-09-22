#!/usr/bin/env bash
# Start N named hybrid pool workers: cosmetic PoH GH + dig/hunt under the SAME worker_id.
# Durable via systemd --user. Does NOT touch worker-kapa-pc.
# Prefer this over start_test_named_fuzz_fleet.sh (no separate *-fuzz sybil rows).
#
#   bash scripts/ops/start_test_named_fleet.sh
#   N=20 bash scripts/ops/start_test_named_fleet.sh
#   SKIP_FLEET_STOP=1 FLEET_OFFSET=15 N=5 bash scripts/ops/start_test_named_fleet.sh
#   HACKME_NAMED_HYBRID_FUZZ=0 bash scripts/ops/start_test_named_fleet.sh   # PoH-only
#   bash scripts/ops/stop_test_named_fleet.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

N="${N:-20}"
FLEET_OFFSET="${FLEET_OFFSET:-0}"
SKIP_FLEET_STOP="${SKIP_FLEET_STOP:-0}"
FLEET_GH_SPAN="${FLEET_GH_SPAN:-$((FLEET_OFFSET + N))}"
COORD_URL="${COORD_URL:-http://132.243.112.100:18083}"
LOG_DIR="${LOG_DIR:-$ROOT/logs/test-named-fleet}"
SEED_DIR="${SEED_DIR:-$ROOT/logs/test-named-fleet/seeds}"
BATCH_SIZE="${BATCH_SIZE:-2097152}"
GH_MIN="${GH_MIN:-30.0}"
GH_MAX="${GH_MAX:-60.0}"
UNIT_PREFIX="${UNIT_PREFIX:-hackme-test-poh}"
# Hybrid dig+hunt under same worker_id (default ON). Soft defaults to limit SQLITE_BUSY on hub.
HYBRID_FUZZ="${HACKME_NAMED_HYBRID_FUZZ:-1}"
HTTP_TIMEOUT_SEC="${WORKERFUZZ_HTTP_TIMEOUT_SEC:-120}"
FUZZ_GAP_MS="${HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS:-8000}"
# Dig WASM timeout; Hunt uses HACKME_WORKER_HUNT_TIMEOUT_MS (≥180s floor in binary).
FUZZ_TIMEOUT_MS="${NAMED_FUZZ_TIMEOUT_MS:-2500}"
FUZZ_CONC="${HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY:-1}"
HUNT_SHARDS="${HACKME_WORKER_HUNT_SHARDS:-1}"
HUNT_TIMEOUT_MS="${HACKME_WORKER_HUNT_TIMEOUT_MS:-180000}"
# PoH+fuzz share one public IP bucket (coord claim_per_min×4). Keep combined < ~360/min.
POH_CLAIM_COOLDOWN_MS="${HACKME_WORKER_CLAIM_COOLDOWN_MS:-8000}"

NAMES=(
  desktop-a4m2rx desktop-k7v1pd desktop-q9n4ls desktop-t2c8we desktop-z5h6mf
  shannon turing hopper knuth mccarthy
  lovelace tesla euclid noether faraday
  ui-west digga fuzzlane rack-neon byte-hop
)

TOKEN="${POOL_TOKEN:-}"
if [[ -z "$TOKEN" && -f "$ROOT/.secrets/hackme_coordinator_worker_token" ]]; then
  TOKEN="$(tr -d '\r\n' <"$ROOT/.secrets/hackme_coordinator_worker_token")"
fi
if [[ -z "$TOKEN" ]]; then
  echo "[test-fleet] need POOL_TOKEN or .secrets/hackme_coordinator_worker_token (never admin token)" >&2
  exit 1
fi

MINERSIGN_BIN="${MINERSIGN_BIN:-}"
if [[ -z "$MINERSIGN_BIN" ]]; then
  if [[ -x "$ROOT/minersign" ]]; then MINERSIGN_BIN="$ROOT/minersign"
  elif [[ -x "$ROOT/bin/minersign" ]]; then MINERSIGN_BIN="$ROOT/bin/minersign"
  else echo "[test-fleet] minersign binary missing" >&2; exit 1
  fi
fi

need_build=0
WFZ="$ROOT/bin/workerfuzz"
[[ -x "$WFZ" ]] || WFZ="$ROOT/workerfuzz"
if [[ ! -x "$WFZ" ]]; then
  need_build=1
elif [[ "${FORCE_REBUILD_WORKERFUZZ:-0}" == "1" ]]; then
  need_build=1
elif ! grep -a -q 'RunHuntShard' "$WFZ" 2>/dev/null; then
  echo "[test-fleet] workerfuzz lacks Hunt — rebuilding for rc17 dig/hunt" >&2
  need_build=1
fi
if [[ "$need_build" == "1" ]]; then
  echo "[test-fleet] building ./bin/workerfuzz (hunt+dig)" >&2
  (cd "$ROOT" && go build -trimpath -ldflags '-s -w' -o bin/workerfuzz ./cmd/workerfuzz) || {
    echo "[test-fleet] WARN: workerfuzz build failed" >&2
  }
fi

mkdir -p "$LOG_DIR" "$SEED_DIR" "$LOG_DIR/locks" "$ROOT/.cache/hunt-harness"
# Drop separate fuzz-only fleet if present (same host should not double-dig).
if [[ -x "$ROOT/scripts/ops/stop_test_named_fuzz_fleet.sh" ]]; then
  bash "$ROOT/scripts/ops/stop_test_named_fuzz_fleet.sh" >/dev/null 2>&1 || true
fi
if [[ "$SKIP_FLEET_STOP" != "1" ]]; then
  bash "$ROOT/scripts/ops/stop_test_named_fleet.sh" >/dev/null 2>&1 || true
  sleep 1
fi

chmod +x "$ROOT/scripts/ops/named_hybrid_unit.sh"

if (( FLEET_OFFSET + N > ${#NAMES[@]} )); then
  echo "[test-fleet] FLEET_OFFSET=${FLEET_OFFSET} N=${N} exceeds NAMES (${#NAMES[@]})" >&2
  exit 1
fi

echo "[test-fleet] starting $N hybrid units offset=${FLEET_OFFSET} (PoH+dig+hunt=${HYBRID_FUZZ}) GH ${GH_MIN}…${GH_MAX} → $COORD_URL"
for i in $(seq "$FLEET_OFFSET" $((FLEET_OFFSET + N - 1))); do
  name="${NAMES[$i]:-rig$i}"
  gh_idx=$((i - FLEET_OFFSET))
  wid="worker-${name}"
  unit="${UNIT_PREFIX}-${name}"
  seed_file="$SEED_DIR/${wid}.seed"
  if [[ ! -f "$seed_file" ]]; then
    "$MINERSIGN_BIN" -gen-seed 2>/dev/null | python3 -c 'import sys,json; print(json.load(sys.stdin)["HACKME_MINER_ED25519_SEED_HEX"])' >"$seed_file" \
      || openssl rand -hex 32 >"$seed_file"
  fi
  seed="$(tr -d '\r\n' <"$seed_file")"
  if [[ ${#seed} -ne 64 ]]; then
    echo "[test-fleet] bad seed for $wid (len=${#seed})" >&2
    continue
  fi
  gh_span="${FLEET_GH_SPAN}"
  if (( gh_span <= 1 )); then
    gh="$(python3 -c "print(round(float('${GH_MIN}'), 2))")"
  else
    gh="$(python3 -c "print(round(${GH_MIN} + (${GH_MAX}-${GH_MIN})*${i}/(${gh_span}-1), 2))")"
  fi
  : >"$LOG_DIR/${wid}.log"
  : >"$LOG_DIR/${wid}.fuzz.log"
  # Stagger claims so hub SQLite + ASAN harness cold-start do not stampede.
  # ~1.2s/unit keeps 20-fleet under the shared public-IP claim bucket.
  stagger_ms=$((gh_idx * 1200))
  systemd-run --user \
    --unit="$unit" \
    --property=Restart=on-failure \
    --property=RestartSec=5 \
    --working-directory="$ROOT" \
    --setenv=COORD_URL="$COORD_URL" \
    --setenv=COORD_ADMIN_TOKEN="$TOKEN" \
    --setenv=COORD_TOKEN="$TOKEN" \
    --setenv=WORKER_ID="$wid" \
    --setenv=WORKER_NAME="$name" \
    --setenv=BATCH_SIZE="$BATCH_SIZE" \
    --setenv=FORCE_HASHRATE_GHS="$gh" \
    --setenv=HASHRATE_GHS="$gh" \
    --setenv=HACKME_MINER_ED25519_SEED_HEX="$seed" \
    --setenv=MINERSIGN_BIN="$MINERSIGN_BIN" \
    --setenv=LOG_DIR="$LOG_DIR" \
    --setenv=HACKME_NAMED_HYBRID_FUZZ="$HYBRID_FUZZ" \
    --setenv=HACKME_REPO_ROOT="$ROOT" \
    --setenv=HACKME_COORDINATOR_URL="$COORD_URL" \
    --setenv=HACKME_POOL_COORDINATOR_URL="$COORD_URL" \
    --setenv=HACKME_WORKER_HUNT_SHARDS="$HUNT_SHARDS" \
    --setenv=HACKME_WORKER_HUNT_TIMEOUT_MS="$HUNT_TIMEOUT_MS" \
    --setenv=WORKERFUZZ_HTTP_TIMEOUT_SEC="$HTTP_TIMEOUT_SEC" \
    --setenv=WORKERFUZZ_TIMEOUT_MS="$FUZZ_TIMEOUT_MS" \
    --setenv=HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS="$FUZZ_GAP_MS" \
    --setenv=HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY="$FUZZ_CONC" \
    --setenv=HACKME_WORKER_CLAIM_COOLDOWN_MS="$POH_CLAIM_COOLDOWN_MS" \
    /bin/bash -c "sleep $(python3 -c "print(${stagger_ms}/1000)"); exec \"$ROOT/scripts/ops/named_hybrid_unit.sh\""
  echo "$unit" >"$LOG_DIR/${wid}.unit"
  echo "[test-fleet]  $wid  gh=${gh}  dig+hunt=${HYBRID_FUZZ}  unit=${unit}"
done

sleep 5
alive=0
for i in $(seq "$FLEET_OFFSET" $((FLEET_OFFSET + N - 1))); do
  name="${NAMES[$i]:-rig$i}"
  unit="${UNIT_PREFIX}-${name}"
  if systemctl --user is-active --quiet "$unit.service" 2>/dev/null; then
    alive=$((alive + 1))
  fi
done
echo "[test-fleet] active $alive / $N (PoH board + dig/hunt under same ids)"
echo "[test-fleet] stop: bash scripts/ops/stop_test_named_fleet.sh"
echo "[test-fleet] fuzz/hunt logs: $LOG_DIR/worker-*.fuzz.log"
