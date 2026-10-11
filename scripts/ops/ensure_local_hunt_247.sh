#!/usr/bin/env bash
# Ensure continuous local Hunt (obscure loop + hourly daily-rotate) and
# refresh GPU hybrid worker env for current Dig/Hunt + research-slot stack.
#
#   bash scripts/ops/ensure_local_hunt_247.sh           # rebuild workerfuzz, wire env, restart units
#   bash scripts/ops/ensure_local_hunt_247.sh status    # health snapshot only
#   bash scripts/ops/ensure_local_hunt_247.sh stop      # stop local Hunt units (not GPU PoH)
#   bash scripts/ops/ensure_local_hunt_247.sh restart   # restart local Hunt + kapa hybrid
#
# Local power = ASAN Hunt on-box (obscure + daily rotate). Pool Dig/Hunt is hybrid
# via worker-kapa-pc (workerpoh-cuda + workerfuzz process child).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export HACKME_REPO_ROOT="$ROOT"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
ENV_FILE="$ROOT/logs/systemd/hackme-kapa-worker.env"
ACTION="${1:-ensure}"

log() { echo "[local-hunt-247 $(date -u +%H:%M:%S)] $*"; }

write_kapa_env() {
  mkdir -p "$(dirname "$ENV_FILE")"
  # Preserve COORD_TOKEN if already present; else pull from .env.desktop / secrets.
  local token=""
  if [[ -f "$ENV_FILE" ]]; then
    token="$(grep -E '^COORD_TOKEN=' "$ENV_FILE" | head -1 | cut -d= -f2- || true)"
  fi
  if [[ -z "$token" && -f "$ROOT/.env.desktop" ]]; then
    token="$(grep -E '^HACKME_POOL_COORDINATOR_TOKEN=' "$ROOT/.env.desktop" | head -1 | cut -d= -f2- || true)"
  fi
  if [[ -z "$token" && -f "$ROOT/.secrets/hackme_coordinator_worker_token" ]]; then
    token="$(tr -d '\r\n' <"$ROOT/.secrets/hackme_coordinator_worker_token")"
  fi
  local coord="${COORD_URL:-http://132.243.112.100:18083}"
  if [[ -f "$ROOT/.env.desktop" ]]; then
    local c
    c="$(grep -E '^HACKME_POOL_COORDINATOR_URL=' "$ROOT/.env.desktop" | head -1 | cut -d= -f2- || true)"
    [[ -n "$c" ]] && coord="$c"
  fi
  cat >"$ENV_FILE" <<EOF
COORD_URL=$coord
COORD_TOKEN=$token
HACKME_COORDINATOR_URL=$coord
HACKME_POOL_COORDINATOR_URL=$coord
LD_LIBRARY_PATH=$ROOT/.deps/cuda-lib
HACKME_REPO_ROOT=$ROOT
HACKME_WORKER_LOCK_DIR=$ROOT/logs
HACKME_MINER_SEED_FILE=$ROOT/logs/desktop/data/node_ed25519.seed
HACKME_WORKERFUZZ_BIN=$ROOT/bin/workerfuzz
# Hybrid Dig/Hunt under same worker_id (process → rebuilt workerfuzz + research slot)
HACKME_WORKER_HYBRID_FUZZ=1
HACKME_WORKER_HYBRID_FUZZ_DIG=1
HACKME_WORKER_HYBRID_FUZZ_MODE=process
HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY=1
HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS=200
HACKME_WORKER_HYBRID_FUZZ_TIMEOUT_MS=2000
HACKME_WORKER_HYBRID_FUZZ_BACKPRESSURE_PCT=35
HACKME_WORKER_HYBRID_FUZZ_NICE=10
WORKERFUZZ_HTTP_TIMEOUT_SEC=30
HACKME_WORKER_HUNT_TIMEOUT_MS=300000
HACKME_WORKER_HUNT_SHARDS=1
# Coverage-guided Hunt (libFuzzer fleet default); audit defaults: no byte soft-fallback, bwrap required
HACKME_HUNT_COVERAGE_ENGINE=1
HACKME_HUNT_ENGINE_BYTE_FALLBACK=0
HACKME_HUNT_ISOLATE_REQUIRE=1
HACKME_POOL_SEED_FROM_RESEARCH=0
# Research slot (LF persist → corpus_delta) for byte Hunt only; soft-skip without clang/LF
HACKME_WORKER_RESEARCH_SLOT=1
HACKME_WORKER_RESEARCH_WINDOW_SEC=30
HACKME_WORKER_RESEARCH_MAX_DELTA=32
EOF
  chmod 600 "$ENV_FILE"
  log "wrote $ENV_FILE"
}

write_kapa_dropin() {
  local d="$UNIT_DIR/hackme-kapa-worker.service.d"
  mkdir -p "$d"
  cat >"$d/hybrid-research.conf" <<EOF
[Service]
# Current Hunt stack: process-mode hybrid + research slot (env file is source of truth).
EnvironmentFile=-$ENV_FILE
Environment=HACKME_REPO_ROOT=$ROOT
Environment=HACKME_WORKERFUZZ_BIN=$ROOT/bin/workerfuzz
EOF
  log "wrote $d/hybrid-research.conf"
}

ensure_obscure_unit() {
  # Keep existing user unit + day5-deep drop-in; only ensure WantedBy + daemon present.
  if [[ ! -f "$UNIT_DIR/hackme-obscure-hunt.service" ]]; then
    log "WARN: $UNIT_DIR/hackme-obscure-hunt.service missing — copy from a prior desktop install"
    return 1
  fi
  systemctl --user enable hackme-obscure-hunt.service hackme-obscure-watchdog.timer 2>/dev/null || true
}

ensure_daily_rotate() {
  if [[ ! -f "$UNIT_DIR/hackme-hunt-daily-rotate.timer" ]]; then
    bash "$ROOT/scripts/ops/install_hunt_daily_rotate_user.sh"
  else
    systemctl --user enable hackme-hunt-daily-rotate.timer 2>/dev/null || true
  fi
}

rebuild_bins() {
  log "rebuild bin/workerfuzz"
  go build -trimpath -ldflags "-s -w" -o "$ROOT/bin/workerfuzz" ./cmd/workerfuzz
  log "rebuild bin/oss_cve_hunt (local continuous Hunt)"
  go build -trimpath -o "$ROOT/bin/oss_cve_hunt" ./tools/oss_cve_hunt/
  if [[ ! -x "$ROOT/bin/workerpoh-cuda" ]]; then
    log "WARN: bin/workerpoh-cuda missing (nvcc may be absent) — GPU hybrid needs existing CUDA binary"
  else
    log "keep existing bin/workerpoh-cuda (nvcc not required)"
  fi
}

show_status() {
  echo "=== units ==="
  systemctl --user is-active hackme-obscure-hunt.service hackme-hunt-daily-rotate.timer hackme-hunt-daily-rotate.service hackme-kapa-worker.service hackme-desktop-node.service 2>&1 || true
  echo
  echo "=== local Hunt processes ==="
  pgrep -af 'run_oss_cve_obscure_loop|oss_cve_hunt|hunt_daily_rotate|hunt_bench_local' 2>/dev/null | head -20 || echo "(none)"
  echo
  echo "=== hybrid GPU ==="
  pgrep -af 'workerpoh-cuda|workerfuzz .*worker-kapa-pc' 2>/dev/null | head -10 || echo "(none)"
  echo
  if command -v nvidia-smi >/dev/null 2>&1; then
    nvidia-smi --query-gpu=utilization.gpu,memory.used,power.draw,temperature.gpu --format=csv
  fi
  echo
  echo "=== recent hybrid fuzz lines ==="
  local logf
  logf="$(ls -t "$ROOT"/logs/workerpoh-worker-kapa-pc-*.log 2>/dev/null | head -1 || true)"
  if [[ -n "$logf" ]]; then
    tail -n 15 "$logf" | grep -E 'submit ok|workerpoh-fuzz|hybrid fuzz|research_slot|no_fuzz_work|ghs_priority' || tail -n 8 "$logf"
  fi
  echo
  echo "Stop local Hunt:  bash $ROOT/scripts/ops/ensure_local_hunt_247.sh stop"
  echo "Restart all:      bash $ROOT/scripts/ops/ensure_local_hunt_247.sh restart"
}

do_stop_local() {
  log "stopping local Hunt units (GPU PoH left running)"
  systemctl --user stop hackme-obscure-hunt.service 2>/dev/null || true
  systemctl --user stop hackme-hunt-daily-rotate.service 2>/dev/null || true
  # Do not disable timers unless explicitly asked — stop is temporary.
}

do_ensure() {
  write_kapa_env
  write_kapa_dropin
  # Align .env.desktop hybrid dig + research with process-mode stack (idempotent keys).
  if [[ -f "$ROOT/.env.desktop" ]]; then
    ROOT="$ROOT" python3 - <<'PY'
import os
from pathlib import Path
p = Path(os.environ["ROOT"]) / ".env.desktop"
text = p.read_text()
want = {
    "HACKME_WORKER_HYBRID_FUZZ": "1",
    "HACKME_WORKER_HYBRID_FUZZ_DIG": "1",
    "HACKME_WORKER_HYBRID_FUZZ_MODE": "process",
    "HACKME_WORKER_HYBRID_FUZZ_CONCURRENCY": "1",
    "HACKME_WORKER_HYBRID_FUZZ_CLAIM_GAP_MS": "200",
    "HACKME_WORKER_HYBRID_FUZZ_TIMEOUT_MS": "2000",
    "HACKME_WORKER_HYBRID_FUZZ_BACKPRESSURE_PCT": "35",
    "HACKME_WORKER_RESEARCH_SLOT": "1",
    "HACKME_WORKER_RESEARCH_WINDOW_SEC": "30",
    "HACKME_WORKER_RESEARCH_MAX_DELTA": "32",
    "HACKME_WORKERFUZZ_BIN": "bin/workerfuzz",
}
lines = text.splitlines()
keys_seen = set()
out = []
for line in lines:
    if "=" in line and not line.strip().startswith("#"):
        k = line.split("=", 1)[0].strip()
        if k in want:
            out.append(f"{k}={want[k]}")
            keys_seen.add(k)
            continue
    out.append(line)
missing = [k for k in want if k not in keys_seen]
if missing:
    out.append("")
    out.append("# Local 24/7 Hunt hybrid / research (ensure_local_hunt_247)")
    for k in missing:
        out.append(f"{k}={want[k]}")
p.write_text("\n".join(out) + "\n")
print("updated .env.desktop hybrid/research keys")
PY
  fi
  rebuild_bins
  ensure_obscure_unit || true
  ensure_daily_rotate
  systemctl --user daemon-reload
  # Clear stale workerfuzz lock for kapa so process-mode can spawn.
  rm -f "$ROOT/logs/workerlock-workerfuzz-worker-kapa-pc.pid" \
        "$ROOT/logs/workerlock-workerpoh-worker-kapa-pc.pid" 2>/dev/null || true
  log "restart hackme-kapa-worker (hybrid process + research)"
  systemctl --user restart hackme-kapa-worker.service
  if ! systemctl --user is-active --quiet hackme-obscure-hunt.service; then
    log "start hackme-obscure-hunt"
    systemctl --user start hackme-obscure-hunt.service
  else
    log "obscure-hunt already active"
  fi
  if ! systemctl --user is-active --quiet hackme-hunt-daily-rotate.timer; then
    systemctl --user start hackme-hunt-daily-rotate.timer
  fi
  # If hourly slot not running this hour, kick one (flock prevents double).
  if ! pgrep -f 'hunt_daily_rotate\.sh' >/dev/null 2>&1; then
    log "start hunt-daily-rotate slot"
    systemctl --user start hackme-hunt-daily-rotate.service || true
  else
    log "hunt-daily-rotate slot already running"
  fi
  sleep 3
  show_status
}

case "$ACTION" in
  status) show_status ;;
  stop) do_stop_local; show_status ;;
  restart)
    write_kapa_env
    write_kapa_dropin
    rebuild_bins
    systemctl --user daemon-reload
    rm -f "$ROOT/logs/workerlock-workerfuzz-worker-kapa-pc.pid" \
          "$ROOT/logs/workerlock-workerpoh-worker-kapa-pc.pid" 2>/dev/null || true
    systemctl --user restart hackme-obscure-hunt.service hackme-kapa-worker.service
    systemctl --user start hackme-hunt-daily-rotate.service || true
    sleep 3
    show_status
    ;;
  ensure|start|*) do_ensure ;;
esac
