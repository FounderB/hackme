#!/usr/bin/env bash
# Recover premature-finalize settle backlog on bootstrap customer node.
# Re-locks unpaid run pool, replays coordinator outbox, pulls settle, then
# marks the campaign completed once run pool is paid or legitimately empty.
#
#   CAMPAIGN_ID=campaign-bootstrap-yyjson-... \
#     bash scripts/ops/bootstrap_customer/bootstrap_recover_stuck_settle.sh
#
# Env: BASE=http://127.0.0.1:8080 INSTALL=/opt/hackme-bootstrap
#      COORD=https://hackme.tech/pool/coordinator
set -euo pipefail
INSTALL="${BOOTSTRAP_INSTALL:-${INSTALL:-/opt/hackme-bootstrap}}"
BASE="${BASE:-http://127.0.0.1:8080}"
COORD="${COORD:-https://hackme.tech/pool/coordinator}"
CAMPAIGN_ID="${CAMPAIGN_ID:?CAMPAIGN_ID required}"
ADMIN="$(grep -m1 '^HACKME_ADMIN_TOKEN=' "$INSTALL/.env" | cut -d= -f2- | tr -d '\r\n')"
[[ -n "$ADMIN" ]] || { echo "missing HACKME_ADMIN_TOKEN" >&2; exit 2; }

# shellcheck source=load_coord_token.sh
source "$(dirname "$0")/load_coord_token.sh"
load_bootstrap_coord_token

auth=(-H "Content-Type: application/json" -H "X-Hackme-Admin-Token: $ADMIN" -H "Authorization: Bearer $ADMIN")

echo "[recover] coordinator replay outbox campaign=$CAMPAIGN_ID"
if [[ -n "${COORD_POLL_TOKEN:-}" ]]; then
  curl -sS --max-time 120 -X POST "${COORD%/}/api/fuzz/pool/settle/replay" \
    -H "Content-Type: application/json" \
    -H "X-Hackme-Admin-Token: ${COORD_POLL_TOKEN}" \
    -d "{\"campaign_id\":\"$CAMPAIGN_ID\"}" | tee /tmp/hackme-coord-replay.json || true
  echo
else
  echo "[recover] WARN skip coordinator replay (no COORD_POLL_TOKEN)" >&2
fi

echo "[recover] reopen+pull campaign=$CAMPAIGN_ID"
curl -sS --max-time 180 "${auth[@]}" \
  -d "{\"campaign_id\":\"$CAMPAIGN_ID\",\"pull\":true}" \
  "$BASE/api/fuzz/escrow/reopen-settle-drain" | tee /tmp/hackme-reopen-settle.json
echo

echo "[recover] drain loop (up to ~3 min)"
for i in $(seq 1 36); do
  curl -sS --max-time 180 "${auth[@]}" \
    -d "{\"campaign_id\":\"$CAMPAIGN_ID\",\"pull\":true}" \
    "$BASE/api/fuzz/escrow/reopen-settle-drain" >/tmp/hackme-reopen-tick.json || true
  code=$(curl -sS -o /tmp/hackme-escrow.json -w '%{http_code}' --max-time 30 "${auth[@]}" \
    "$BASE/api/fuzz/campaigns/$CAMPAIGN_ID/escrow" || true)
  if [[ "$code" != "200" ]]; then
    echo "  tick=$i escrow_http=$code"
    sleep 5
    continue
  fi
  python3 - <<'PY'
import json
d=json.load(open("/tmp/hackme-escrow.json"))
e=d.get("escrow") or d
pool=d.get("pool_runs_done")
print(f"  status={e.get('status')} runs_paid_hmc={e.get('runs_paid_hmc')} runs_done={e.get('runs_done')} pool_runs_done={pool}")
open("/tmp/hackme-escrow-status","w").write(str(e.get("status") or ""))
open("/tmp/hackme-escrow-runs-paid","w").write(str(e.get("runs_paid_hmc") or "0"))
PY
  st="$(cat /tmp/hackme-escrow-status)"
  paid="$(cat /tmp/hackme-escrow-runs-paid)"
  if [[ "$st" == "closed" && "$paid" != "0" && "$paid" != "0.0" ]]; then
    echo "[recover] DONE (runs paid)"
    exit 0
  fi
  if [[ "$st" == "closed" ]]; then
    # Hunt floor / no run outbox: still sync campaign status from pool progress.
    if [[ -n "${COORD_POLL_TOKEN:-}" ]]; then
      prog="$(curl -fsS --max-time 20 -H "X-Hackme-Admin-Token: ${COORD_POLL_TOKEN}" \
        "${COORD%/}/api/fuzz/pool/campaigns/progress?id=${CAMPAIGN_ID}" 2>/dev/null || echo '{}')"
      runs_ok="$(echo "$prog" | jq -r '.runs_ok // .runs_done // 0' 2>/dev/null || echo 0)"
      if [[ "${runs_ok:-0}" -gt 0 ]]; then
        echo "[recover] DONE closed (pool runs_ok=$runs_ok; hunt floor may have zero run payouts)"
        exit 0
      fi
    fi
  fi
  sleep 5
done
echo "[recover] timed out — check hub outbox + node journals" >&2
exit 1
