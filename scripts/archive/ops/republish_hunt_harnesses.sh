#!/usr/bin/env bash
# Upload Hunt harness blobs to coordinator + release stuck leases.
# Usage (on hub or via ssh):
#   bash scripts/ops/republish_hunt_harnesses.sh
#   COORD=http://127.0.0.1:18081 HASHES="abc… def…" bash scripts/ops/republish_hunt_harnesses.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

COORD="${COORD:-http://127.0.0.1:18081}"
ADMIN_TOKEN="${HACKME_COORDINATOR_ADMIN_TOKEN:-${HACKME_POOL_COORDINATOR_ADMIN_TOKEN:-}}"
if [[ -z "$ADMIN_TOKEN" && -f .secrets/hackme_coordinator_admin_token ]]; then
  ADMIN_TOKEN="$(tr -d '\r\n' <.secrets/hackme_coordinator_admin_token)"
fi
if [[ -z "$ADMIN_TOKEN" ]]; then
  echo "admin token required" >&2
  exit 1
fi

CACHE="${HUNT_HARNESS_CACHE:-$ROOT/.cache/hunt-harness}"
mapfile -t HASHES < <(
  if [[ -n "${HASHES:-}" ]]; then
    # shellcheck disable=SC2086
    printf '%s\n' $HASHES
  else
    # default: hashes from running hunt campaigns on coordinator fuzz db
    DB="${COORD_FUZZ_DB:-$ROOT/data/coordinator_fuzz.db}"
    if [[ -f "$DB" ]]; then
      sqlite3 "$DB" "select distinct json_extract(config_json,'$.harness_hash') from fuzz_campaigns where status='running' and campaign_type='hunt' and json_extract(config_json,'$.harness_hash') is not null;"
    fi
  fi
)

if [[ ${#HASHES[@]} -eq 0 ]]; then
  echo "no harness hashes to publish" >&2
  exit 1
fi

ok=0
for hash in "${HASHES[@]}"; do
  hash="$(echo "$hash" | tr -d '[:space:]')"
  [[ -n "$hash" ]] || continue
  bin="$CACHE/${hash}.bin"
  if [[ ! -f "$bin" ]]; then
    echo "MISSING $bin — try: go test ./internal/hunt -run TestEnsureHarnessBinaryCached  or EnsureHarnessBinary for target" >&2
    continue
  fi
  b64_file="$(mktemp)"
  json_file="$(mktemp)"
  base64 -w0 "$bin" >"$b64_file"
  echo "POST harness $hash ($(stat -c%s "$bin") bytes) → $COORD"
  jq -nc --arg h "$hash" --rawfile b "$b64_file" --arg s "republish:$hash" \
    '{harness_hash:$h, source_rel:$s, binary_b64:$b}' >"$json_file"
  curl -fsS -X POST "$COORD/api/fuzz/pool/hunt/harness" \
    -H "Content-Type: application/json" \
    -H "X-Hackme-Admin-Token: $ADMIN_TOKEN" \
    --data-binary @"$json_file" \
    | jq -e '.ok == true' >/dev/null
  rm -f "$b64_file" "$json_file"
  # fetch roundtrip with worker or admin
  tmp="$(mktemp)"
  curl -fsS -H "X-Hackme-Admin-Token: $ADMIN_TOKEN" "$COORD/api/fuzz/pool/hunt/harness/${hash}" -o "$tmp"
  if ! cmp -s "$bin" "$tmp"; then
    echo "FETCH MISMATCH $hash" >&2
    rm -f "$tmp"
    exit 1
  fi
  rm -f "$tmp"
  echo "OK $hash"
  ok=$((ok + 1))
done

# Release stuck Hunt leases (attempts=0, no error) so workers re-claim with harness available.
DB="${COORD_FUZZ_DB:-$ROOT/data/coordinator_fuzz.db}"
if [[ -f "$DB" ]]; then
  echo "releasing leased hunt shards with attempts=0…"
  sqlite3 "$DB" <<'SQL'
PRAGMA busy_timeout=30000;
BEGIN IMMEDIATE;
UPDATE fuzz_work_items
SET status='pending', lease_owner='', lease_until=0, updated_at=strftime('%s','now')
WHERE status='leased'
  AND attempts=0
  AND (last_error IS NULL OR last_error='')
  AND campaign_id IN (SELECT id FROM fuzz_campaigns WHERE campaign_type='hunt' AND status='running');
COMMIT;
SQL
  sqlite3 "$DB" "select campaign_id, status, count(*) from fuzz_work_items where campaign_id in (select id from fuzz_campaigns where campaign_type='hunt' and status='running') group by 1,2;"
fi

echo "republished=$ok"
[[ "$ok" -gt 0 ]]
