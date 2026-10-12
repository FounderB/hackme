#!/usr/bin/env bash
# Recover premature-finalize settle backlog on bootstrap customer node.
# Re-locks unpaid run pool, pulls coordinator outbox (pays miners + ACKs), then
# marks the campaign completed so tryClose can finalize once rows are gone.
#
#   CAMPAIGN_ID=campaign-bootstrap-yyjson-... \
#     bash scripts/ops/bootstrap_customer/bootstrap_recover_stuck_settle.sh
#
# Env: BASE=http://127.0.0.1:8080 INSTALL=/opt/hackme-bootstrap
set -euo pipefail
INSTALL="${BOOTSTRAP_INSTALL:-${INSTALL:-/opt/hackme-bootstrap}}"
BASE="${BASE:-http://127.0.0.1:8080}"
CAMPAIGN_ID="${CAMPAIGN_ID:?CAMPAIGN_ID required}"
ADMIN="$(grep -m1 '^HACKME_ADMIN_TOKEN=' "$INSTALL/.env" | cut -d= -f2- | tr -d '\r\n')"
[[ -n "$ADMIN" ]] || { echo "missing HACKME_ADMIN_TOKEN" >&2; exit 2; }

auth=(-H "Content-Type: application/json" -H "X-Hackme-Admin-Token: $ADMIN" -H "Authorization: Bearer $ADMIN")

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
  curl -sS --max-time 180 "${auth[@]}" \
    -d '{"status":"completed"}' \
    "$BASE/api/fuzz/campaigns/$CAMPAIGN_ID/status" >/tmp/hackme-camp-status.json || true
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
print(f"  status={e.get('status')} runs_paid_hmc={e.get('runs_paid_hmc')} runs_done={e.get('runs_done')}")
open("/tmp/hackme-escrow-status","w").write(str(e.get("status") or ""))
PY
  if [[ "$(cat /tmp/hackme-escrow-status)" == "closed" ]]; then
    echo "[recover] DONE"
    exit 0
  fi
  sleep 5
done
echo "[recover] timed out — check hub outbox + node journals" >&2
exit 1
