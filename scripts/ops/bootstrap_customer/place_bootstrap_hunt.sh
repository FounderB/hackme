#!/usr/bin/env bash
# Place one Bootstrap Hunt campaign (ASAN rail) onto the public pool.
#
#   bash place_bootstrap_hunt.sh jsmn [hunt_standard|hunt_lite] [shards]
#
# Env:
#   BOOTSTRAP_INSTALL  default /opt/hackme-bootstrap
#   BASE               default http://127.0.0.1:8080
#   COORD              default https://hackme.tech/pool/coordinator
#   STAMP              optional id stamp
set -euo pipefail
INSTALL="${BOOTSTRAP_INSTALL:-/opt/hackme-bootstrap}"
BASE="${BASE:-http://127.0.0.1:8080}"
COORD="${COORD:-https://hackme.tech/pool/coordinator}"
TARGET="${1:-jsmn}"
PKG="${2:-hunt_standard}"
SHARDS="${3:-32}"
STAMP="${STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
CID="hunt-bootstrap-${TARGET}-${STAMP}"
TITLE="Bootstrap Hunt · ${TARGET} · ${PKG}"
LOG_DIR="${LOG_DIR:-$INSTALL/logs/bootstrap/hunt}"
mkdir -p "$LOG_DIR"

# Hunt escrow enforces HuntMinShards=8 (internal/fuzzescrow/hunt.go).
if [[ "$SHARDS" -lt 8 ]]; then
  echo "[bootstrap-hunt] clamp shards $SHARDS -> 8 (HuntMinShards)" >&2
  SHARDS=8
fi

ADMIN="$(grep -m1 '^HACKME_ADMIN_TOKEN=' "$INSTALL/.env" | cut -d= -f2- | tr -d '\r\n')"
[[ -n "$ADMIN" ]] || { echo "[bootstrap-hunt] missing HACKME_ADMIN_TOKEN" >&2; exit 2; }
# shellcheck source=load_coord_token.sh
source "$(dirname "$0")/load_coord_token.sh"
load_bootstrap_coord_token

echo "[bootstrap-hunt $(date -u +%H:%M:%S)] PLACE $CID pkg=$PKG shards=$SHARDS"
resp="$(curl -sS --max-time 900 -X POST "$BASE/api/hunt/campaigns" \
  -H "Content-Type: application/json" \
  -H "X-Hackme-Admin-Token: $ADMIN" \
  -d "$(jq -nc \
    --arg id "$CID" \
    --arg title "$TITLE" \
    --arg pkg "$PKG" \
    --arg tid "$TARGET" \
    --argjson shards "$SHARDS" \
    '{
      id: $id,
      package: $pkg,
      title: $title,
      pool_distributed: true,
      budget_shards: $shards,
      status: "running",
      catalog: true,
      target_id: $tid
    }')")"
echo "$resp" | tee "$LOG_DIR/${CID}.json" >/dev/null
echo "$resp" | jq -c '{ok,id:(.campaign.id//.id),pool_sync,err:(.error//.code//.message)}' || echo "$resp" | head -c 800

cid_out="$(echo "$resp" | jq -r '.campaign.id // .id // empty')"
[[ -n "$cid_out" ]] || exit 1

# Best-effort resync + progress peek (non-fatal).
if [[ -x "$INSTALL/scripts/bootstrap_customer/bootstrap_resync_pool.sh" ]]; then
  CAMPAIGN_ID="$cid_out" bash "$INSTALL/scripts/bootstrap_customer/bootstrap_resync_pool.sh" \
    >>"$LOG_DIR/${CID}.resync.log" 2>&1 || true
fi
for i in 1 2 3 4 5; do
  prog="$(curl -fsS --max-time 20 -H "X-Hackme-Admin-Token: ${COORD_POLL_TOKEN}" "$COORD/api/fuzz/pool/campaigns/progress?id=${cid_out}" 2>/dev/null || echo '{}')"
  if echo "$prog" | jq -e '.ok==true' >/dev/null 2>&1; then
    echo "$prog" | jq -c '{ok,id,status,runs_done,budget_runs,title}'
    exit 0
  fi
  sleep "$i"
done
echo "[bootstrap-hunt] placed $cid_out (progress not yet visible)"
exit 0
