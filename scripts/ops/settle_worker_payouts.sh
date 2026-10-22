#!/usr/bin/env bash
set -euo pipefail

# Settle coordinator off-chain worker accruals to on-chain transfers.
#
# Inputs:
# - COORD_URL: coordinator base (default http://127.0.0.1:18081)
# - CHAIN_BASE: canonical node base (default http://127.0.0.1:18080)
# - ADMIN_TOKEN: node admin token used for /api/tx/send
# - COORD_ADMIN_TOKEN / HACKME_COORDINATOR_ADMIN_TOKEN: coordinator admin (required for ?details=1 after hardening)
# - MIN_SETTLE_HMC: minimum delta per worker to settle (default 0.01)
# - DAILY_FORCE_INTERVAL_SEC: force-settle cycle cadence (default 86400 = once/day)
# - DAILY_MIN_SETTLE_HMC: minimal delta used during force-settle cycle (default 0.0001)
# - FORCE_SETTLE_ALL: set 1 to force daily branch immediately for this run
# - STATE_FILE: local settlement state file (default data/worker_settlement_state.json)
# - WORKER_PAYOUT_MAP: optional csv "worker_id=HMC-...,worker2=HMC-..."
#
# Notes:
# - Primary payout address source: workers[*].payout_address from hybrid signed submits.
# - Fallback source: WORKER_PAYOUT_MAP mapping.

require_cmd() {
  # VPS safety: broken /dev/null (regular file) breaks `command -v … >/dev/null`.
  if command -v "$1" 2>&1 | grep -q .; then
    return 0
  fi
  echo "[settle-workers] missing command: $1 (PATH=${PATH:-})" >&2
  exit 1
}

require_cmd curl
require_cmd jq
require_cmd python3

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COORD_SECRET_FILE="${COORD_SECRET_FILE:-${ROOT_DIR}/.secrets/hackme_coordinator_admin_token}"
ADMIN_SECRET_FILE="${ADMIN_SECRET_FILE:-${ROOT_DIR}/.secrets/hackme_admin_token}"

COORD_URL="${COORD_URL:-http://127.0.0.1:18081}"
CHAIN_BASE="${CHAIN_BASE:-http://127.0.0.1:18080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-${HACKME_ADMIN_TOKEN:-}}"
COORD_ADMIN_TOKEN="${COORD_ADMIN_TOKEN:-${HACKME_COORDINATOR_ADMIN_TOKEN:-${COORD_TOKEN:-}}}"
MIN_SETTLE_HMC="${MIN_SETTLE_HMC:-0.0001}"
DAILY_FORCE_INTERVAL_SEC="${DAILY_FORCE_INTERVAL_SEC:-86400}"
DAILY_MIN_SETTLE_HMC="${DAILY_MIN_SETTLE_HMC:-0.0001}"
FORCE_SETTLE_ALL="${FORCE_SETTLE_ALL:-0}"
STATE_FILE="${STATE_FILE:-data/worker_settlement_state.json}"
# SQLite ledger (Wave C): default beside JSON. Set SETTLE_SQLITE=0 to disable.
STATE_DB="${STATE_DB:-${HACKME_WORKER_SETTLEMENT_DB:-}}"
if [[ -z "$STATE_DB" && "${SETTLE_SQLITE:-1}" != "0" ]]; then
  if [[ "$STATE_FILE" == *.json ]]; then
    STATE_DB="${STATE_FILE%.json}.db"
  else
    STATE_DB="${STATE_FILE}.db"
  fi
fi
WORKER_PAYOUT_MAP="${WORKER_PAYOUT_MAP:-}"
# Comma-separated worker_id allowlist (e.g. named fleet only). Empty = all workers.
SETTLE_WORKER_ALLOWLIST="${SETTLE_WORKER_ALLOWLIST:-${HACKME_SETTLE_WORKER_ALLOWLIST:-}}"
SETTLECTL="${SETTLECTL:-}"
if [[ -z "$SETTLECTL" ]]; then
  for c in "$ROOT_DIR/bin/workersettlectl" "$ROOT_DIR/workersettlectl"; do
    if [[ -x "$c" ]]; then
      SETTLECTL="$c"
      break
    fi
  done
fi
SETTLE_TX_WAIT_SEC="${SETTLE_TX_WAIT_SEC:-90}"
SETTLE_SEQUENTIAL="${SETTLE_SEQUENTIAL:-1}"
SETTLE_PAYOUT_PAUSE_SEC="${SETTLE_PAYOUT_PAUSE_SEC:-10}"
SETTLE_NONCE_RETRIES="${SETTLE_NONCE_RETRIES:-6}"
CURL_MAX_TIME="${CURL_MAX_TIME:-60}"
TREASURY_RESERVE_HMC="${TREASURY_RESERVE_HMC:-0.05}"
SETTLE_MAX_CHUNK_HMC="${SETTLE_MAX_CHUNK_HMC:-40}"
MIN_FEE_UNITS=1000
UNITS_PER_HMC=100000000

curl_json() {
  local url="$1"
  shift
  local attempt=0
  while (( attempt < 5 )); do
    if out="$(curl -fsS --max-time "$CURL_MAX_TIME" "$@" "$url" 2>/dev/null)"; then
      printf '%s' "$out"
      return 0
    fi
    attempt=$((attempt + 1))
    sleep $((attempt * 2))
  done
  return 1
}

payer_balance_hmc() {
  local addr="$1"
  curl_json "${CHAIN_BASE}/api/address/${addr}" | jq -r '(.balance_units // 0) / 100000000'
}

payer_nonce() {
  local addr="$1"
  curl_json "${CHAIN_BASE}/api/address/${addr}" | jq -r '.next_nonce // 0'
}

if [[ -z "$ADMIN_TOKEN" && -r "$ADMIN_SECRET_FILE" ]]; then
  ADMIN_TOKEN="$(tr -d '\r\n' <"$ADMIN_SECRET_FILE")"
fi
if [[ -z "$COORD_ADMIN_TOKEN" && -r "$COORD_SECRET_FILE" ]]; then
  COORD_ADMIN_TOKEN="$(tr -d '\r\n' <"$COORD_SECRET_FILE")"
fi
if [[ -z "$ADMIN_TOKEN" ]]; then
  echo "[settle-workers] ADMIN_TOKEN (or HACKME_ADMIN_TOKEN or ${ADMIN_SECRET_FILE}) is required" >&2
  exit 1
fi
if [[ -z "$COORD_ADMIN_TOKEN" ]]; then
  echo "[settle-workers] COORD_ADMIN_TOKEN required for coordinator /api/work/stats?details=1 (set env or ${COORD_SECRET_FILE})" >&2
  exit 1
fi

mkdir -p "$(dirname "$STATE_FILE")"
if [[ ! -f "$STATE_FILE" ]]; then
  echo '{"workers":{},"meta":{"last_force_unix":0}}' >"$STATE_FILE"
fi
# systemd runs as User=hackme; state must not be root-only.
if [[ "$(id -u)" -eq 0 ]]; then
  chown hackme:hackme "$(dirname "$STATE_FILE")" "$STATE_FILE" 2>/dev/null || true
  chmod 700 "$(dirname "$STATE_FILE")" 2>/dev/null || true
  chmod 600 "$STATE_FILE" 2>/dev/null || true
fi
if [[ ! -r "$STATE_FILE" || ! -w "$STATE_FILE" ]]; then
  echo "[settle-workers] cannot read/write STATE_FILE=${STATE_FILE} (run: sudo chown hackme:hackme ${STATE_FILE})" >&2
  exit 1
fi

# Cross-host single-payer guard (Wave C): only one machine may settle the same
# payer wallet. Set SETTLE_EXPECTED_HOSTNAME to this host's short hostname
# (or FQDN). Empty = disabled (local flock still applies on this host).
if [[ -n "${SETTLE_EXPECTED_HOSTNAME:-}" ]]; then
  _hn="$(hostname -s 2>/dev/null || hostname 2>/dev/null || echo "")"
  _hnf="$(hostname -f 2>/dev/null || echo "")"
  if [[ "$_hn" != "$SETTLE_EXPECTED_HOSTNAME" && "$_hnf" != "$SETTLE_EXPECTED_HOSTNAME" ]]; then
    echo "[settle-workers] refuse: hostname=${_hn}/${_hnf} != SETTLE_EXPECTED_HOSTNAME=${SETTLE_EXPECTED_HOSTNAME}" >&2
    exit 1
  fi
fi

settle_sqlite_ready() {
  [[ -n "$STATE_DB" && "${SETTLE_SQLITE:-1}" != "0" && -n "$SETTLECTL" && -x "$SETTLECTL" ]]
}

settle_sqlite_sync_json() {
  if settle_sqlite_ready; then
    # Fail-closed: a silent export miss leaves JSON stale → double-pay (audit 5b).
    if ! "$SETTLECTL" export-json "$STATE_DB" "$STATE_FILE" >/dev/null; then
      echo "[settle-workers] FATAL: export-json failed (refusing to continue with stale JSON)" >&2
      return 1
    fi
  fi
  return 0
}

# Prefer SQLite watermark when ledger is active (audit 5b).
settle_read_settled_hmc() {
  local wid="$1"
  if settle_sqlite_ready; then
    local row
    if row="$("$SETTLECTL" get-json "$STATE_DB" "$wid" 2>/dev/null)"; then
      jq -r '.settled_hmc // 0' <<<"$row"
      return 0
    fi
  fi
  jq -r --arg wid "$wid" '.workers[$wid].settled_hmc // 0' "$STATE_FILE"
}

settle_read_session_anchor() {
  local wid="$1"
  if settle_sqlite_ready; then
    local row
    if row="$("$SETTLECTL" get-json "$STATE_DB" "$wid" 2>/dev/null)"; then
      local a
      a="$(jq -r '.session_anchor_hmc // empty' <<<"$row")"
      if [[ -n "$a" && "$a" != "0" && "$a" != "0.0" ]]; then
        printf '%s' "$a"
        return 0
      fi
    fi
  fi
  jq -r --arg wid "$wid" '.workers[$wid].session_anchor_hmc // empty' "$STATE_FILE"
}

# Single-writer: avoid two cron/systemd instances racing on nonce + state (see docs/POOL_SECURITY_THREATS_VERDICT.md).
LOCK_FILE="${STATE_FILE}.flock"
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  echo "[settle-workers] skip: another instance holds ${LOCK_FILE}" >&2
  exit 0
fi

if settle_sqlite_ready; then
  echo "[settle-workers] sqlite ledger: $STATE_DB (ctl=$SETTLECTL)"
  # migrate may be noop; missing JSON must fail closed (audit 5c).
  if ! "$SETTLECTL" migrate "$STATE_DB" "$STATE_FILE" >/dev/null; then
    echo "[settle-workers] FATAL: migrate failed" >&2
    exit 1
  fi
  if ! settle_sqlite_sync_json; then
    exit 1
  fi
elif [[ -n "$STATE_DB" && "${SETTLE_SQLITE:-1}" != "0" ]]; then
  echo "[settle-workers] WARN: STATE_DB set but workersettlectl missing — JSON-only path (build: go build -o bin/workersettlectl ./cmd/workersettlectl)" >&2
fi

tx_response_ok() {
  local resp="$1"
  [[ -n "$resp" ]] || return 1
  local ok status tx_hash
  ok="$(jq -r '(.ok // false) | tostring' <<<"$resp" 2>/dev/null | tr -d '\r\n' || echo "false")"
  status="$(jq -r '.status // ""' <<<"$resp" 2>/dev/null | tr -d '\r\n' || echo "")"
  tx_hash="$(jq -r '.tx_hash // ""' <<<"$resp" 2>/dev/null | tr -d '\r\n' || echo "")"
  [[ "$tx_hash" == "nonce-advanced" || "$tx_hash" == "ok" ]] && return 1
  [[ "$ok" == "true" && -n "$tx_hash" ]] && return 0
  [[ -n "$tx_hash" && "$status" =~ ^(pending|accepted|included)$ ]] && return 0
  return 1
}

# Confirm tx is included on chain before advancing settled_hmc bookkeeping.
# Poll up to SETTLE_TX_WAIT_SEC (same budget as wait_tx_pool_clear) — an 8s cap
# left pending_settle stuck while transfers were already included under load.
verify_settlement_tx() {
  local tx_hash="$1"
  local wait_sec="${2:-$SETTLE_TX_WAIT_SEC}"
  local deadline status=""
  [[ -n "$tx_hash" ]] || return 1
  [[ "$tx_hash" == "nonce-advanced" || "$tx_hash" == "ok" ]] && return 1
  [[ "$wait_sec" =~ ^[0-9]+$ ]] || wait_sec=90
  deadline=$((SECONDS + wait_sec))
  while (( SECONDS < deadline )); do
    status="$(curl -fsS --max-time 10 "${CHAIN_BASE}/api/tx/${tx_hash}" 2>/dev/null | jq -r '.status // .tx.status // ""' 2>/dev/null | tr -d '\r\n' || echo "")"
    case "$status" in
      included) return 0 ;;
      rejected) return 1 ;;
      pending|accepted|"") ;;
      *) ;;
    esac
    sleep 1
  done
  return 1
}

normalize_tx_resp() {
  local r="${1:-}"
  r="${r//$'\r'/}"
  r="${r//$'\n'/}"
  while [[ "$r" == *'}}' ]]; do r="${r%?}"; done
  if ! jq -e . <<<"$r" >/dev/null 2>&1; then
    r="{}"
  fi
  printf '%s' "$r"
}

tx_pool_pending_count() {
  curl -fsS "${CHAIN_BASE}/api/tx/pool" | jq '.txs | if . == null then 0 else length end' 2>/dev/null || echo 0
}

wait_tx_pool_clear() {
  local deadline=$((SECONDS + SETTLE_TX_WAIT_SEC))
  while (( SECONDS < deadline )); do
    if [[ "$(tx_pool_pending_count)" == "0" ]]; then
      return 0
    fi
    sleep 5
  done
  echo "[settle-workers] warn: tx pool still pending after ${SETTLE_TX_WAIT_SEC}s" >&2
  return 1
}

send_settlement_tx() {
  local from_addr="$1" to_addr="$2" amount_units="$3" ts="$4" nonce="$5"
  local tx_body resp attempt=0
  tx_body="$(jq -nc \
    --arg from "$from_addr" \
    --arg to "$to_addr" \
    --argjson amount "$amount_units" \
    --argjson fee "$MIN_FEE_UNITS" \
    --argjson nonce "$nonce" \
    --argjson ts "$ts" \
    '{tx_type:"transfer_v1",from:$from,to:$to,amount_units:$amount,fee_units:$fee,nonce:$nonce,timestamp_unix:$ts,memo:"worker_settlement"}')"
  while (( attempt < 4 )); do
    if resp="$(curl -sS --max-time "$CURL_MAX_TIME" -X POST "${CHAIN_BASE}/api/tx/send" \
      -H "Content-Type: application/json" \
      -H "X-Hackme-Admin-Token: ${ADMIN_TOKEN}" \
      -d "$tx_body" 2>/dev/null)"; then
      printf '%s' "$resp"
      return 0
    fi
    attempt=$((attempt + 1))
    sleep $((attempt * 2))
  done
  return 1
}

if ! jq -e '.workers | type=="object"' "$STATE_FILE" >/dev/null 2>&1; then
  tmp="$(mktemp)"
  jq -c '{workers:(.workers // {}),meta:(.meta // {})}' "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
fi

now_unix="$(date +%s)"
force_settle="$FORCE_SETTLE_ALL"
if [[ "$force_settle" != "1" ]]; then
  last_force_unix="$(jq -r '.meta.last_force_unix // 0' "$STATE_FILE" 2>/dev/null || echo 0)"
  if ! [[ "$last_force_unix" =~ ^[0-9]+$ ]]; then
    last_force_unix=0
  fi
  elapsed="$((now_unix - last_force_unix))"
  if [[ "$elapsed" -ge "$DAILY_FORCE_INTERVAL_SEC" ]]; then
    force_settle=1
  fi
fi
if [[ "$force_settle" == "1" ]]; then
  echo "[settle-workers] force-settle mode ON (interval=${DAILY_FORCE_INTERVAL_SEC}s daily_min=${DAILY_MIN_SETTLE_HMC} HMC)"
fi

stats_json="$(curl -fsS -H "X-Hackme-Admin-Token: ${COORD_ADMIN_TOKEN}" "${COORD_URL}/api/work/stats?details=1")"
payer_addr="$(curl -fsS -H "X-Hackme-Admin-Token: ${ADMIN_TOKEN}" "${CHAIN_BASE}/api/wallet" | jq -r '.address // ""')"
if [[ -z "$payer_addr" ]]; then
  echo "[settle-workers] failed to resolve payer wallet address from ${CHAIN_BASE}/api/wallet" >&2
  exit 1
fi

map_json='{}'
if [[ -n "$WORKER_PAYOUT_MAP" ]]; then
  map_json="$(python3 - "$WORKER_PAYOUT_MAP" <<'PY'
import json,sys
raw=sys.argv[1].strip()
out={}
for part in raw.split(','):
    p=part.strip()
    if not p or '=' not in p:
        continue
    k,v=p.split('=',1)
    k=k.strip(); v=v.strip()
    if k and v:
        out[k]=v
print(json.dumps(out))
PY
)"
fi

# Public coordinators often omit workers{} even with ?details=1; without rows the script used to exit and never pay.
workers_json="$(echo "$stats_json" | jq -c '.workers // {} | if type == "object" then . else {} end')"
if [[ "$workers_json" == "{}" ]]; then
  wcount="$(echo "$stats_json" | jq '(.workers_count // 0) | floor')"
  tp="$(echo "$stats_json" | jq -r '.total_payout_hmc // 0')"
  nmap="$(jq 'length' <<<"$map_json")"
  tp_pos="$(echo "$stats_json" | jq '((.total_payout_hmc // 0) | tonumber) > 0')"
  if [[ "$wcount" -gt 0 ]] && [[ "$tp_pos" == "true" ]]; then
    if [[ "$nmap" -eq 1 ]]; then
      wid="$(jq -r 'keys[0]' <<<"$map_json")"
      workers_json="$(echo "$stats_json" | jq -c --arg wid "$wid" '{($wid): {payout_hmc: .total_payout_hmc}}')"
      echo "[settle-workers] coordinator omitted workers{} — synthetic row for ${wid} using pool total_payout_hmc=${tp} HMC (single WORKER_PAYOUT_MAP entry)" >&2
    elif [[ "$nmap" -gt 1 ]]; then
      echo "[settle-workers] ERROR: workers{} empty but WORKER_PAYOUT_MAP has ${nmap} keys; cannot split pool total safely. Set one worker id, upgrade coordinator to expose workers{}, or run separate pools." >&2
      exit 2
    else
      echo "[settle-workers] no workers in coordinator stats — set WORKER_PAYOUT_MAP=worker-id=HMC-... (one entry) to settle when API omits workers{}" >&2
      exit 0
    fi
  else
    echo "[settle-workers] no workers in coordinator stats"
    exit 0
  fi
fi

workers_list="$(jq -r 'to_entries[] | @base64' <<<"$workers_json")"
if [[ -z "$workers_list" ]]; then
  echo "[settle-workers] no worker entries"
  exit 0
fi

settled_any=0
payouts_sent=0
while IFS= read -r item; do
  [[ -z "$item" ]] && continue
  row="$(printf '%s' "$item" | base64 -d)"
  worker_id="$(jq -r '.key' <<<"$row")"
  if [[ -n "$SETTLE_WORKER_ALLOWLIST" ]]; then
    case ",${SETTLE_WORKER_ALLOWLIST}," in
      *",${worker_id},"*) ;;
      *) continue ;;
    esac
  fi
  if [[ "$(jq -r '.value.address_conflict // false' <<<"$row")" == "true" ]]; then
    echo "[settle-workers] skip ${worker_id}: address_conflict — refuse payout (fleet merge)" >&2
    continue
  fi
  payout_hmc="$(jq -r '.value.payout_hmc // 0' <<<"$row")"
  signed_addr="$(jq -r '.value.payout_address // ""' <<<"$row")"
  map_addr="$(jq -r --arg wid "$worker_id" '.[$wid] // ""' <<<"$map_json")"
  # WORKER_PAYOUT_MAP overrides hybrid signed address when set (operator payout routing).
  if [[ -n "$map_addr" ]]; then
    to_addr="$map_addr"
  else
    to_addr="$signed_addr"
  fi
  if [[ -z "$to_addr" ]]; then
    echo "[settle-workers] skip ${worker_id}: no payout address (signed/map)"
    continue
  fi
  if [[ "$to_addr" == "$payer_addr" ]]; then
    echo "[settle-workers] skip ${worker_id}: payout address equals payer address (${payer_addr})"
    continue
  fi

  already_hmc="$(settle_read_settled_hmc "$worker_id")"
  # Coordinator payout counters are in-memory: a restart resets payout_hmc while
  # settlement state keeps a high settled_hmc watermark → naive delta stays 0 forever.
  #
  # NEVER lower settled_hmc (that opened a double-pay window if accrual later
  # looked "fresh" against a zeroed watermark). On reset: keep lifetime settled,
  # anchor a session floor at current accrued, and pay only session growth;
  # on success settled_hmc += delta (lifetime watermark only moves up).
  session_anchor="$(settle_read_session_anchor "$worker_id")"
  rebase="$(python3 - "$payout_hmc" "$already_hmc" <<'PY'
import sys
p=float(sys.argv[1]); s=float(sys.argv[2])
print("1" if (s - p) > 1e-9 else "0")
PY
)"
  if [[ "$rebase" == "1" ]]; then
    if [[ -z "$session_anchor" ]]; then
      session_anchor="$payout_hmc"
      echo "[settle-workers] session-anchor ${worker_id}: settled_hmc ${already_hmc} kept; anchor=${session_anchor} (coordinator counter reset)"
      if ! jq --arg wid "$worker_id" --arg anchor "$session_anchor" --argjson ts "$(date +%s)" \
        '(.workers[$wid] = ((.workers[$wid] // {}) + {session_anchor_hmc:($anchor|tonumber),rebased_unix:$ts}))' \
        "$STATE_FILE" >"${STATE_FILE}.tmp"; then
        echo "[settle-workers] session-anchor write failed for ${worker_id}" >&2
        continue
      fi
      mv "${STATE_FILE}.tmp" "$STATE_FILE"
    fi
    baseline_hmc="$session_anchor"
  else
    # Accrual caught up / normal path — drop stale session anchor if present.
    if [[ -n "$session_anchor" ]]; then
      jq --arg wid "$worker_id" 'del(.workers[$wid].session_anchor_hmc)' "$STATE_FILE" >"${STATE_FILE}.tmp" \
        && mv "${STATE_FILE}.tmp" "$STATE_FILE" || true
    fi
    baseline_hmc="$already_hmc"
  fi
  delta_hmc="$(python3 - "$payout_hmc" "$baseline_hmc" <<'PY'
import sys
p=float(sys.argv[1]); s=float(sys.argv[2]); d=p-s
if d < 0: d = 0.0
print(f"{d:.12f}")
PY
)"
  should_settle="$(python3 - "$delta_hmc" "$MIN_SETTLE_HMC" "$force_settle" "$DAILY_MIN_SETTLE_HMC" <<'PY'
import sys
d=float(sys.argv[1]); m=float(sys.argv[2]); force=(sys.argv[3]=="1"); dm=float(sys.argv[4])
ok = (d >= m) or (force and d >= dm)
print("1" if ok else "0")
PY
)"
  if [[ "$should_settle" != "1" ]]; then
    if [[ "$force_settle" == "1" ]]; then
      echo "[settle-workers] skip ${worker_id}: delta ${delta_hmc} HMC < force-daily min ${DAILY_MIN_SETTLE_HMC}"
    else
      echo "[settle-workers] skip ${worker_id}: delta ${delta_hmc} HMC < min ${MIN_SETTLE_HMC}"
    fi
    continue
  fi

  amount_units="$(python3 - "$delta_hmc" <<'PY'
import sys
u=int(float(sys.argv[1])*100000000+0.5)
print(max(0,u))
PY
)"
  if [[ "$amount_units" -le 0 ]]; then
    echo "[settle-workers] skip ${worker_id}: computed amount_units=0"
    continue
  fi

  # Crash recovery: durable pending_settle before transfer (anti double-pay).
  pending_tx="$(jq -r --arg wid "$worker_id" '.workers[$wid].pending_settle.tx_hash // empty' "$STATE_FILE")"
  if [[ -n "$pending_tx" ]]; then
    if verify_settlement_tx "$pending_tx"; then
      pend_delta="$(jq -r --arg wid "$worker_id" '.workers[$wid].pending_settle.delta_hmc // 0' "$STATE_FILE")"
      pend_addr="$(jq -r --arg wid "$worker_id" '.workers[$wid].pending_settle.payout_address // ""' "$STATE_FILE")"
      new_settled_hmc="$(python3 - "$already_hmc" "$pend_delta" <<'PY'
import sys
print(f"{float(sys.argv[1])+float(sys.argv[2]):.12f}")
PY
)"
      tmp="$(mktemp)"
      jq --arg wid "$worker_id" --arg addr "$pend_addr" --arg settled "$new_settled_hmc" --arg tx "$pending_tx" --argjson ts "$(date +%s)" \
        '(.workers[$wid] = ((.workers[$wid] // {}) + {settled_hmc:($settled|tonumber),payout_address:$addr,last_tx_hash:$tx,last_settle_unix:$ts}))
         | del(.workers[$wid].pending_settle)' \
        "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
      echo "[settle-workers] recovered pending ${worker_id} tx=${pending_tx} settled=${new_settled_hmc}"
      settled_any=1
      continue
    fi
    echo "[settle-workers] skip ${worker_id}: pending_settle tx=${pending_tx} not confirmed — not re-sending" >&2
    echo "[settle-workers] tip: CLEAR_PENDING_HMC=1 drops pending without advancing settled (only if tx rejected/never sent; double-pay risk if included)" >&2
    if [[ "${CLEAR_PENDING_HMC:-${CLEAR_PENDING_SETTLE:-0}}" == "1" ]]; then
      tmp="$(mktemp)"
      jq --arg wid "$worker_id" 'del(.workers[$wid].pending_settle)' "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
      echo "[settle-workers] CLEAR_PENDING_HMC cleared pending_settle for ${worker_id} (settled_hmc unchanged)"
    fi
    continue
  fi
  if jq -e --arg wid "$worker_id" '.workers[$wid].pending_settle != null' "$STATE_FILE" >/dev/null 2>&1; then
    echo "[settle-workers] skip ${worker_id}: pending_settle without tx_hash — not re-sending" >&2
    if [[ "${CLEAR_PENDING_HMC:-${CLEAR_PENDING_SETTLE:-0}}" == "1" ]]; then
      tmp="$(mktemp)"
      jq --arg wid "$worker_id" 'del(.workers[$wid].pending_settle)' "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
      echo "[settle-workers] CLEAR_PENDING_HMC cleared empty pending_settle for ${worker_id}"
    fi
    continue
  fi

  pbal="$(payer_balance_hmc "$payer_addr" 2>/dev/null || echo 0)"
  max_pay_hmc="$(python3 - "$pbal" "$TREASURY_RESERVE_HMC" "$SETTLE_MAX_CHUNK_HMC" "$delta_hmc" <<'PY'
import sys
bal, res, chunk, delta = map(float, sys.argv[1:])
avail = max(0.0, bal - res)
cap = min(chunk, avail, delta)
print(f"{cap:.12f}")
PY
)"
  if awk -v m="$max_pay_hmc" 'BEGIN{exit !(m>0)}'; then
    if awk -v a="$max_pay_hmc" -v d="$delta_hmc" 'BEGIN{exit !(a+0.0000001<d)}'; then
      echo "[settle-workers] chunk ${worker_id}: treasury=${pbal} HMC — paying ${max_pay_hmc} of ${delta_hmc} HMC now"
      delta_hmc="$max_pay_hmc"
      amount_units="$(python3 - "$delta_hmc" <<'PY'
import sys
u=int(float(sys.argv[1])*100000000+0.5)
print(max(0,u))
PY
)"
    fi
  else
    echo "[settle-workers] ERROR settle ${worker_id}: treasury balance ${pbal} HMC too low (need topup)" >&2
    continue
  fi

  ts="$(date +%s)"
  if [[ "$SETTLE_SEQUENTIAL" == "1" && "$payouts_sent" -gt 0 ]]; then
    wait_tx_pool_clear || true
    if [[ "$SETTLE_PAYOUT_PAUSE_SEC" -gt 0 ]]; then
      sleep "$SETTLE_PAYOUT_PAUSE_SEC"
    fi
  fi
  nonce="$(payer_nonce "$payer_addr" 2>/dev/null || echo 0)"
  nonce_before="$nonce"

  # Durable intent before mint/transfer (cleared on hard failure; retained on ambiguous crash).
  if settle_sqlite_ready; then
    if ! "$SETTLECTL" set-pending "$STATE_DB" "$worker_id" "$to_addr" "$delta_hmc" "$nonce" >/dev/null; then
      echo "[settle-workers] ERROR pending intent ${worker_id}: sqlite set-pending failed" >&2
      continue
    fi
    settle_sqlite_sync_json || { echo "[settle-workers] FATAL: export-json after settle failed" >&2; exit 1; }
  else
    tmp="$(mktemp)"
    if ! jq --arg wid "$worker_id" --arg addr "$to_addr" --arg delta "$delta_hmc" --argjson ts "$ts" --argjson nonce "$nonce" \
      '.workers[$wid].pending_settle = {delta_hmc:($delta|tonumber),payout_address:$addr,started_unix:$ts,nonce:$nonce}' \
      "$STATE_FILE" >"$tmp"; then
      rm -f "$tmp"
      echo "[settle-workers] ERROR pending intent ${worker_id}: jq failed" >&2
      continue
    fi
    mv "$tmp" "$STATE_FILE"
  fi

  resp=""
  if ! resp="$(send_settlement_tx "$payer_addr" "$to_addr" "$amount_units" "$ts" "$nonce")"; then
    resp=""
  fi
  resp="$(normalize_tx_resp "${resp:-{}}")"
  ok="$(jq -r '(.ok // false) | tostring' <<<"${resp:-{}}" 2>/dev/null | tr -d '\r\n' || echo "false")"
  code="$(jq -r '.code // ""' <<<"${resp:-{}}" 2>/dev/null | tr -d '\r\n' || echo "")"
  attempt=1
  while ! tx_response_ok "${resp:-{}}" && [[ "$attempt" -lt "$SETTLE_NONCE_RETRIES" ]]; do
    case "$code" in
      pending_nonce_conflict|invalid_nonce) ;;
      insufficient_balance)
        echo "[settle-workers] ERROR settle ${worker_id}: treasury insufficient_balance — run ensure_settlement_treasury_float.sh" >&2
        break
      ;;
      *) break ;;
    esac
    wait_tx_pool_clear || true
    sleep $((SETTLE_PAYOUT_PAUSE_SEC + attempt * 2))
    nonce="$(payer_nonce "$payer_addr" 2>/dev/null || echo 0)"
    if ! resp="$(send_settlement_tx "$payer_addr" "$to_addr" "$amount_units" "$ts" "$nonce")"; then
      resp=""
    fi
    resp="$(normalize_tx_resp "${resp:-{}}")"
    ok="$(jq -r '(.ok // false) | tostring' <<<"${resp:-{}}" 2>/dev/null | tr -d '\r\n' || echo "false")"
    code="$(jq -r '.code // ""' <<<"${resp:-{}}" 2>/dev/null | tr -d '\r\n' || echo "")"
    attempt=$((attempt + 1))
  done
  if ! tx_response_ok "${resp:-{}}"; then
    nonce_after="$(payer_nonce "$payer_addr" 2>/dev/null || echo "$nonce_before")"
    if [[ "$nonce_after" =~ ^[0-9]+$ && "$nonce_before" =~ ^[0-9]+$ && "$nonce_after" -gt "$nonce_before" ]]; then
      echo "[settle-workers] WARN ${worker_id}: ambiguous send (nonce ${nonce_before}->${nonce_after}) without verifiable tx_hash — keeping pending_settle, not re-sending next run" >&2
    else
      echo "[settle-workers] ERROR settle ${worker_id} -> ${to_addr}: ${resp:-curl_failed}" >&2
      if settle_sqlite_ready; then
        "$SETTLECTL" clear-pending "$STATE_DB" "$worker_id" >/dev/null || true
        settle_sqlite_sync_json || { echo "[settle-workers] FATAL: export-json after settle failed" >&2; exit 1; }
      else
        tmp="$(mktemp)"
        jq --arg wid "$worker_id" 'del(.workers[$wid].pending_settle)' "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
      fi
    fi
    continue
  fi
  tx_hash="$(jq -r '.tx_hash // ""' <<<"${resp:-{}}" 2>/dev/null || echo "")"
  if [[ -n "$tx_hash" ]]; then
    if settle_sqlite_ready; then
      "$SETTLECTL" attach-tx "$STATE_DB" "$worker_id" "$tx_hash" >/dev/null || true
      settle_sqlite_sync_json || { echo "[settle-workers] FATAL: export-json after settle failed" >&2; exit 1; }
    else
      tmp="$(mktemp)"
      jq --arg wid "$worker_id" --arg tx "$tx_hash" \
        '.workers[$wid].pending_settle.tx_hash = $tx' \
        "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
    fi
  fi
  if [[ -z "$tx_hash" ]] || ! verify_settlement_tx "$tx_hash"; then
    echo "[settle-workers] ERROR settle ${worker_id}: tx_hash missing or not confirmed on chain (${tx_hash:-empty}) — pending_settle retained" >&2
    continue
  fi
  settled_any=1
  payouts_sent=$((payouts_sent + 1))
  echo "[settle-workers] settled ${worker_id} -> ${to_addr} delta=${delta_hmc} HMC tx=${tx_hash}"
  new_settled_hmc="$(python3 - "$already_hmc" "$delta_hmc" <<'PY'
import sys
already=float(sys.argv[1]); delta=float(sys.argv[2])
print(f"{already + delta:.12f}")
PY
)"
  # Advance session anchor with paid accrual so the next delta is only new growth.
  new_anchor_hmc="$(python3 - "$payout_hmc" <<'PY'
import sys
print(f"{float(sys.argv[1]):.12f}")
PY
)"
  if settle_sqlite_ready; then
    if [[ "$rebase" == "1" ]]; then
      if ! "$SETTLECTL" apply "$STATE_DB" "$worker_id" "$to_addr" "$tx_hash" "$new_settled_hmc" "$new_anchor_hmc" >/dev/null; then
        echo "[settle-workers] ERROR state update ${worker_id}: sqlite apply failed" >&2
        continue
      fi
    else
      if ! "$SETTLECTL" apply "$STATE_DB" "$worker_id" "$to_addr" "$tx_hash" "$new_settled_hmc" clear >/dev/null; then
        echo "[settle-workers] ERROR state update ${worker_id}: sqlite apply failed" >&2
        continue
      fi
    fi
    settle_sqlite_sync_json || { echo "[settle-workers] FATAL: export-json after settle failed" >&2; exit 1; }
  else
    tmp="$(mktemp)"
    if [[ "$rebase" == "1" ]]; then
      jq_expr='(.workers[$wid] = ((.workers[$wid] // {}) + {settled_hmc:($settled|tonumber),session_anchor_hmc:($anchor|tonumber),payout_address:$addr,last_tx_hash:$tx,last_settle_unix:$ts})) | del(.workers[$wid].pending_settle)'
    else
      jq_expr='(.workers[$wid] = ((.workers[$wid] // {}) + {settled_hmc:($settled|tonumber),payout_address:$addr,last_tx_hash:$tx,last_settle_unix:$ts})) | del(.workers[$wid].pending_settle) | del(.workers[$wid].session_anchor_hmc)'
    fi
    if ! jq --arg wid "$worker_id" --arg addr "$to_addr" --arg settled "$new_settled_hmc" --arg anchor "$new_anchor_hmc" --arg tx "$tx_hash" --argjson ts "$ts" \
      "$jq_expr" \
      "$STATE_FILE" >"$tmp"; then
      rm -f "$tmp"
      echo "[settle-workers] ERROR state update ${worker_id}: jq failed" >&2
      continue
    fi
    mv "$tmp" "$STATE_FILE"
  fi
  if [[ "$(id -u)" -eq 0 ]]; then
    chown hackme:hackme "$STATE_FILE" "$STATE_DB" 2>/dev/null || true
    chmod 600 "$STATE_FILE" 2>/dev/null || true
    chmod 600 "$STATE_DB" 2>/dev/null || true
  fi
done <<<"$workers_list"

if [[ "$settled_any" == "1" ]]; then
  echo "[settle-workers] done: payouts submitted"
else
  echo "[settle-workers] done: nothing to settle"
fi
if [[ "$force_settle" == "1" && "$settled_any" == "1" ]]; then
  if settle_sqlite_ready; then
    "$SETTLECTL" set-force "$STATE_DB" "$now_unix" >/dev/null || true
    settle_sqlite_sync_json || { echo "[settle-workers] FATAL: export-json after settle failed" >&2; exit 1; }
  else
    tmp="$(mktemp)"
    jq --argjson ts "$now_unix" '.meta.last_force_unix = $ts' "$STATE_FILE" >"$tmp" && mv "$tmp" "$STATE_FILE"
  fi
elif [[ "$force_settle" == "1" ]]; then
  echo "[settle-workers] force window not advanced (no successful settle this run)" >&2
fi

if [[ -x "$ROOT_DIR/scripts/ops/publish_settlement_state.sh" ]]; then
  STATE_FILE="$STATE_FILE" \
    SETTLEMENT_CANONICAL_JSON="$(dirname "$STATE_FILE")/settlement_canonical_public.json" \
    bash "$ROOT_DIR/scripts/ops/publish_settlement_state.sh" 2>/dev/null || true
fi
