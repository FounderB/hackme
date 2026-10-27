#!/usr/bin/env bash
set -euo pipefail

# HTTP watchdog for hackme-node.service (systemd active != responsive).
# Intended for timer-based execution (every minute).

SERVICE_NAME="${SERVICE_NAME:-hackme-node.service}"
CHAIN_BASE="${CHAIN_BASE:-http://127.0.0.1:18080}"
STATUS_URL="${STATUS_URL:-${CHAIN_BASE}/api/status}"
CURL_MAX_SEC="${CURL_MAX_SEC:-8}"
LOG_FILE="${LOG_FILE:-/opt/hackme/logs/node-watchdog.log}"

mkdir -p "$(dirname "$LOG_FILE")"

ts() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }

# Tip-stall alert: HTTP can be healthy while no block is appended (e.g. every PoH
# candidate rejected by a broken wasm gate). Alert only — a restart does not fix that.
STALL_SEC="${STALL_SEC:-600}"
STATE_FILE="${STATE_FILE:-/opt/hackme/logs/node-watchdog.tip}"

check_tip_stall() {
  local tip now prev_tip prev_since
  tip="$1"
  now="$(date +%s)"
  prev_tip=""
  prev_since="$now"
  if [[ -f "$STATE_FILE" ]]; then
    read -r prev_tip prev_since <"$STATE_FILE" || true
  fi
  if [[ "$tip" != "$prev_tip" || -z "$prev_since" ]]; then
    echo "$tip $now" >"$STATE_FILE"
    return 0
  fi
  if (( now - prev_since >= STALL_SEC )); then
    echo "$(ts) [node-watchdog] ALERT chain_stall tip=${tip} unchanged for $((now - prev_since))s (>= ${STALL_SEC}s)" | tee -a "$LOG_FILE" >&2
    return 2
  fi
  return 0
}

if status_json="$(curl -fsS --max-time "$CURL_MAX_SEC" "$STATUS_URL" 2>/dev/null)" &&
  jq -e '.tip_height >= 0' >/dev/null 2>&1 <<<"$status_json"; then
  echo "$(ts) [node-watchdog] ${STATUS_URL} OK" >>"$LOG_FILE"
  check_tip_stall "$(jq -r '.tip_height' <<<"$status_json")" || exit 2
  exit 0
fi

echo "$(ts) [node-watchdog] ${STATUS_URL} failed or hung -> restart ${SERVICE_NAME}" >>"$LOG_FILE"
systemctl restart "$SERVICE_NAME"
sleep 2

if curl -fsS --max-time "$CURL_MAX_SEC" "$STATUS_URL" | jq -e '.tip_height >= 0' >/dev/null 2>&1; then
  echo "$(ts) [node-watchdog] restart success" >>"$LOG_FILE"
  exit 0
fi

echo "$(ts) [node-watchdog] restart failed — still unreachable" >>"$LOG_FILE"
exit 1
