#!/usr/bin/env bash
# Ideal customer-repo Hunt path (boring, one script):
#   inventory → harness build → Hunt Standard pool → wait shards → token report
#
# Usage:
#   ADMIN_TOKEN=… NODE=http://127.0.0.1:8080 \
#     bash scripts/ops/place_customer_hunt.sh /path/to/repo fuzzing/target.c [shards]
#
# Env:
#   NODE / BASE          local HackMe node (default http://127.0.0.1:8080)
#   COORD                coordinator progress peek (optional)
#   ADMIN_TOKEN          X-Hackme-Admin-Token (or HACKME_ADMIN_TOKEN)
#   PACKAGE              hunt_standard|hunt_lite (default hunt_standard)
#   GIT_URL / GIT_REF    optional repo metadata for the campaign
#   MIN_DONE             shards to wait before fetching report (default 4)
#   WAIT_SEC             max wait for pool progress (default 900)
#   OUT                  report directory (default reports/customer-hunt/<stamp>)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

REPO="${1:-}"
SOURCE_REL="${2:-}"
SHARDS="${3:-16}"
[[ -n "$REPO" && -n "$SOURCE_REL" ]] || {
  echo "usage: $0 /path/to/repo relative/harness.c [shards]" >&2
  exit 2
}
REPO="$(cd "$REPO" && pwd)"
[[ -f "$REPO/$SOURCE_REL" ]] || {
  echo "missing harness file: $REPO/$SOURCE_REL" >&2
  exit 2
}

NODE="${NODE:-${BASE:-http://127.0.0.1:8080}}"
NODE="${NODE%/}"
COORD="${COORD:-}"
ADMIN_TOKEN="${ADMIN_TOKEN:-${HACKME_ADMIN_TOKEN:-}}"
PACKAGE="${PACKAGE:-hunt_standard}"
GIT_URL="${GIT_URL:-}"
GIT_REF="${GIT_REF:-}"
MIN_DONE="${MIN_DONE:-4}"
WAIT_SEC="${WAIT_SEC:-900}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${OUT:-$ROOT/reports/customer-hunt/$STAMP}"
mkdir -p "$OUT"

if [[ -z "$ADMIN_TOKEN" && -f .secrets/hackme_admin_token ]]; then
  ADMIN_TOKEN="$(tr -d '\r\n' <.secrets/hackme_admin_token)"
fi
[[ -n "$ADMIN_TOKEN" ]] || {
  echo "ADMIN_TOKEN / HACKME_ADMIN_TOKEN required" >&2
  exit 2
}

require_cmd() { command -v "$1" >/dev/null 2>&1 || {
  echo "need $1" >&2
  exit 2
}; }
require_cmd curl
require_cmd jq

hdr=(-H "X-Hackme-Admin-Token: ${ADMIN_TOKEN}" -H "Content-Type: application/json")
log() { echo "[customer-hunt $(date -u +%H:%M:%S)] $*" | tee -a "$OUT/run.log"; }

log "1/4 inventory $REPO"
INV="$(curl -fsS --max-time 120 -X POST "${NODE}/api/hunt/inventory" \
  "${hdr[@]}" \
  -d "$(jq -nc --arg p "$REPO" '{path:$p, max_files:400}')")"
echo "$INV" | jq . >"$OUT/inventory.json"
echo "$INV" | jq -e --arg rel "$SOURCE_REL" '
  .ok == true and (.inventory.targets | map(.path) | index($rel) != null)
' >/dev/null || {
  echo "$INV" | jq -c '{ok,n:(.inventory.targets|length),sample:(.inventory.targets|map(.path)|.[0:8])}' >&2
  echo "inventory missing $SOURCE_REL" >&2
  exit 1
}

log "2/4 harness build $SOURCE_REL"
repo_obj="$(jq -nc --arg p "$REPO" --arg u "$GIT_URL" --arg r "$GIT_REF" '
  {path:$p} + (if $u != "" then {git_url:$u} else {} end) + (if $r != "" then {ref:$r} else {} end)
')"
BUILD="$(curl -fsS --max-time 600 -X POST "${NODE}/api/hunt/harness/build" \
  "${hdr[@]}" \
  -d "$(jq -nc --argjson repo "$repo_obj" --arg s "$SOURCE_REL" \
    '{repo:$repo, source_rel:$s, template_accept:false}')")"
echo "$BUILD" | jq . >"$OUT/harness-build.json"
HARNESS_HASH="$(echo "$BUILD" | jq -r '.build.harness_hash // .harness_hash // empty')"
[[ -n "$HARNESS_HASH" && "$HARNESS_HASH" != "null" ]] || {
  echo "harness build failed:" >&2
  echo "$BUILD" | jq . >&2
  exit 1
}
log "harness_hash=$HARNESS_HASH"

CID="hunt-customer-$(basename "$REPO" | tr -c 'a-zA-Z0-9' '-' | tr '[:upper:]' '[:lower:]')-${STAMP}"
TITLE="Customer Hunt · $(basename "$REPO") · ${PACKAGE}"
log "3/4 create $PACKAGE pool campaign $CID (shards=$SHARDS)"
CREATE="$(curl -fsS --max-time 900 -X POST "${NODE}/api/hunt/campaigns" \
  "${hdr[@]}" \
  -d "$(jq -nc \
    --arg id "$CID" \
    --arg title "$TITLE" \
    --arg pkg "$PACKAGE" \
    --arg p "$REPO" \
    --arg rel "$SOURCE_REL" \
    --arg owner "customer:$(basename "$REPO" | tr -c 'a-zA-Z0-9' '-' | tr '[:upper:]' '[:lower:]')" \
    --argjson shards "$SHARDS" \
    --argjson repo "$repo_obj" \
    '{
      id: $id,
      package: $pkg,
      title: $title,
      owner_ref: $owner,
      pool_distributed: true,
      budget_shards: $shards,
      status: "running",
      catalog: false,
      repo: $repo,
      inventory_target: {path: $rel, title: ($rel), source: "inventory"},
      template_accept: false
    }')")"
echo "$CREATE" | jq . >"$OUT/create.json"
CID="$(echo "$CREATE" | jq -r '.campaign.id // empty')"
REPORT_TOKEN="$(echo "$CREATE" | jq -r '.customer_report_token // empty')"
[[ -n "$CID" && -n "$REPORT_TOKEN" ]] || {
  echo "create failed:" >&2
  echo "$CREATE" | jq . >&2
  exit 1
}
echo "$CREATE" | jq -e '.ok == true and (.pool_sync == "ok" or .pool_sync == "queued" or .pool_sync == "sync")' >/dev/null \
  || {
    echo "pool_sync failed: $(echo "$CREATE" | jq -c '{pool_sync,pool_sync_warning,error,code,message}')" >&2
    exit 1
  }
log "campaign=$CID pool_sync=$(echo "$CREATE" | jq -r '.pool_sync')"

deadline=$((SECONDS + WAIT_SEC))
DONE=0
while ((SECONDS < deadline)); do
  if [[ -n "$COORD" ]]; then
    PROG="$(curl -fsS --max-time 15 -H "X-Hackme-Admin-Token: ${HACKME_COORDINATOR_ADMIN_TOKEN:-${HACKME_POOL_COORDINATOR_TOKEN:-}}" "${COORD%/}/api/fuzz/pool/campaigns/progress?id=${CID}" 2>/dev/null || echo '{}')"
    DONE="$(echo "$PROG" | jq -r '.runs_done // .done // 0' 2>/dev/null || echo 0)"
  else
    DONE="$(curl -fsS --max-time 15 "${NODE}/api/fuzz/campaigns/${CID}" \
      -H "X-Hackme-Admin-Token: ${ADMIN_TOKEN}" \
      | jq -r '.campaign.summary.runs_done // 0' 2>/dev/null || echo 0)"
  fi
  if [[ "${DONE:-0}" -ge "$MIN_DONE" ]]; then
    break
  fi
  log "waiting shards_done=${DONE:-0}/$MIN_DONE …"
  sleep 10
done
[[ "${DONE:-0}" -ge "$MIN_DONE" ]] || {
  echo "timeout: shards_done=${DONE:-0} want >=$MIN_DONE (campaign still running)" >&2
  exit 1
}

log "4/4 customer report (done=$DONE)"
curl -fsS --max-time 60 \
  "${NODE}/api/fuzz/campaigns/${CID}/report?format=json&limit=50" \
  -H "X-Hackme-Report-Token: ${REPORT_TOKEN}" \
  | tee "$OUT/report.json" | jq -e '.ok == true' >/dev/null
curl -fsS --max-time 60 \
  "${NODE}/api/fuzz/campaigns/${CID}/report.html" \
  -H "X-Hackme-Report-Token: ${REPORT_TOKEN}" \
  >"$OUT/report.html"

jq -n \
  --arg stamp "$STAMP" \
  --arg cid "$CID" \
  --arg repo "$REPO" \
  --arg source "$SOURCE_REL" \
  --arg harness "$HARNESS_HASH" \
  --arg pkg "$PACKAGE" \
  --argjson done "$DONE" \
  --arg report_token "$REPORT_TOKEN" \
  '{stamp:$stamp, campaign_id:$cid, repo:$repo, source_rel:$source, harness_hash:$harness,
    package:$pkg, runs_done:$done, customer_report_token:$report_token,
    flow:"inventory→build→pool→report"}' >"$OUT/summary.json"

log "PASS → $OUT"
jq -c . "$OUT/summary.json"
