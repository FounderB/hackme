#!/usr/bin/env bash
# HackMe Orders (B2B) — local node for Dig / Hunt campaigns (no mining worker).
#
#   bash start_hackme_orders.sh           # start node + open #orders
#   bash start_hackme_orders.sh --open    # ensure up, open dashboard
#   bash start_hackme_orders.sh --start   # start node only
#
# Env lives in XDG config (writable even when /opt/hackme is root-owned apt install).
# CLI (optional): keep hackme-fuzzing next to this script or on PATH.
set -euo pipefail

INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE=start-open
case "${1:-}" in
  --open|--open-only) MODE=open; shift || true ;;
  --start|--start-only) MODE=start; shift || true ;;
  -h|--help)
    echo "Usage: $0 [--open|--start]"
    echo "  B2B Orders profile — Dig/Hunt on 127.0.0.1:8080/#orders (no pool miner)."
    exit 0
    ;;
esac

BIN="${INSTALL_DIR}/hackme"
if [[ ! -x "$BIN" && -x /opt/hackme/hackme ]]; then
  BIN=/opt/hackme/hackme
fi
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
STATE_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/hackme"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/hackme"
LOG_DIR="${LOG_DIR:-$STATE_DIR/logs/orders}"
DATA_DIR="${HACKME_DATA_DIR:-$STATE_DIR/data}"
ENV_FILE="${ENV_FILE:-$CONFIG_DIR/env.orders}"
PID_FILE="${PID_FILE:-$LOG_DIR/hackme-orders.pid}"
NODE_LOG="${NODE_LOG:-$LOG_DIR/hackme-orders.log}"

notify() {
  local msg="$1"
  echo "[hackme-orders] $msg" >&2
  if [[ ! -t 1 ]] && command -v notify-send >/dev/null 2>&1; then
    notify-send -a "HackMe Orders" "HackMe Orders" "$msg" 2>/dev/null || true
  fi
}

die() {
  notify "$1"
  if [[ ! -t 1 ]] && command -v zenity >/dev/null 2>&1; then
    zenity --error --title="HackMe Orders" --width=420 --text="$1" 2>/dev/null || true
  fi
  exit 1
}

[[ -x "$BIN" ]] || die "hackme binary missing at $BIN — install: sudo apt install hackme-node  (or extract the Linux tarball)"

mkdir -p "$STATE_DIR" "$CONFIG_DIR" "$LOG_DIR" "$DATA_DIR"
chmod 700 "$DATA_DIR" 2>/dev/null || true

# Optional Dig from_code toolchains (portable under install or XDG).
ensure_toolchains() {
  if [[ "${SKIP_TOOLCHAINS:-0}" == "1" ]]; then
    return 0
  fi
  local tc=""
  for tc in \
    "${INSTALL_DIR}/install_from_code_toolchains.sh" \
    "${INSTALL_DIR}/../ops/install_from_code_toolchains.sh" \
    "/opt/hackme/install_from_code_toolchains.sh"; do
    if [[ -x "$tc" ]]; then
      APP_DIR="${INSTALL_DIR}" bash "$tc" >/dev/null 2>&1 || true
      break
    fi
  done
}

ensure_env() {
  local admin=""
  if [[ -f "$ENV_FILE" ]]; then
    admin="$(grep -E '^HACKME_ADMIN_TOKEN=' "$ENV_FILE" 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '\r' || true)"
  fi
  if [[ -z "$admin" ]]; then
    if command -v openssl >/dev/null 2>&1; then
      admin="$(openssl rand -hex 24)"
    else
      admin="$(python3 -c 'import secrets; print(secrets.token_hex(24))')"
    fi
  fi

  cat >"$ENV_FILE" <<EOF
HACKME_BIND_ADDR=127.0.0.1:8080
HACKME_ADMIN_TOKEN=${admin}
HACKME_REQUIRE_ADMIN_TOKEN=1
HACKME_DESKTOP_MODE=1
HACKME_DESKTOP_EXPOSE_ADMIN_TOKEN=1
HACKME_INTEGRATOR_SELF_REGISTER=1
HACKME_PUBLIC_AUTHORITY_BASE=https://hackme.tech
HACKME_CANONICAL_CHAIN_URL=https://hackme.tech
HACKME_DATA_DIR=${DATA_DIR}
HACKME_WORKING_DIR=${STATE_DIR}
HACKME_WORKER_LOCK_DIR=${STATE_DIR}/logs
LOG_DIR=${LOG_DIR}
DESKTOP_PROFILE=orders
HACKME_ORDERS_STARTER_HMC=0
WORKER_AUTOSTART=0
HACKME_WORKER_WATCHDOG=0
HACKME_DESKTOP_GPU_POOL=0
HACKME_FUZZ_SETTLE_PULL=1
EOF
  chmod 600 "$ENV_FILE" 2>/dev/null || true
  chmod 700 "$DATA_DIR" 2>/dev/null || true
  for f in "${DATA_DIR}/node_ed25519.seed" "${DATA_DIR}/miner_submit_ed25519_seed.hex"; do
    [[ -f "$f" ]] || continue
    chmod 600 "$f" 2>/dev/null || true
  done
}

node_up() {
  curl -fsS --max-time 2 "$BASE_URL/api/status?lite=1" >/dev/null 2>&1 \
    || curl -fsS --max-time 2 "$BASE_URL/api/health" >/dev/null 2>&1 \
    || curl -fsS --max-time 2 "${BASE_URL%/}/" >/dev/null 2>&1
}

stop_stale_pid() {
  if [[ -f "$PID_FILE" ]]; then
    local old
    old="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      if node_up; then
        return 0
      fi
      kill "$old" 2>/dev/null || true
      sleep 1
    fi
    rm -f "$PID_FILE"
  fi
}

print_cli_hint() {
  local cli=""
  for cli in \
    "${INSTALL_DIR}/hackme-fuzzing" \
    "${INSTALL_DIR}/bin/hackme-fuzzing" \
    "$(command -v hackme-fuzzing 2>/dev/null || true)"; do
    if [[ -n "$cli" && -x "$cli" ]]; then
      echo "[hackme-orders] CLI: $cli wizard --pack secrets --package audit" >&2
      echo "[hackme-orders] Hunt: $cli hunt packages" >&2
      return 0
    fi
  done
  echo "[hackme-orders] CLI tip: download hackme-fuzzing from https://hackme.tech/downloads.html#fuzzing-client" >&2
  echo "[hackme-orders]        or place it next to hackme in ${INSTALL_DIR}" >&2
}

start_node() {
  ensure_toolchains
  ensure_env
  stop_stale_pid
  if node_up; then
    notify "Node already running — ${BASE_URL}/#orders"
    return 0
  fi

  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
  export HACKME_DATA_DIR="$DATA_DIR"
  export HACKME_WORKING_DIR="$STATE_DIR"

  notify "Starting HackMe Orders node…"
  nohup "$BIN" >"$NODE_LOG" 2>&1 &
  echo $! >"$PID_FILE"

  local i
  for i in $(seq 1 40); do
    if node_up; then
      break
    fi
    sleep 0.25
  done
  if ! node_up; then
    tail -n 40 "$NODE_LOG" >&2 || true
    if grep -q 'address already in use' "$NODE_LOG" 2>/dev/null; then
      die "Port 8080 is in use. Stop Miner/desktop first, or set HACKME_BIND_ADDR in $ENV_FILE."
    fi
    die "Node did not start. See log: $NODE_LOG"
  fi
  notify "Orders ready — ${BASE_URL}/#orders"
  print_cli_hint
}

open_orders() {
  local url="${BASE_URL}/#orders"
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 || /usr/bin/xdg-open "$url" >/dev/null 2>&1 || true
  elif command -v gio >/dev/null 2>&1; then
    gio open "$url" >/dev/null 2>&1 || true
  else
    notify "Open in browser: $url"
  fi
}

case "$MODE" in
  start) start_node ;;
  open) start_node; open_orders ;;
  start-open) start_node; open_orders ;;
esac
exit 0
