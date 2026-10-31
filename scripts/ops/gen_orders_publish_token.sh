#!/usr/bin/env bash
# Generate a scoped Orders publish token (campaign register only; not admin).
#
#   bash scripts/ops/gen_orders_publish_token.sh
#   FORCE=1 bash scripts/ops/gen_orders_publish_token.sh
#
# Hub coordinator env:
#   HACKME_COORDINATOR_ORDERS_PUBLISH_TOKEN=<same>
# Release / Orders installer:
#   HACKME_RELEASE_ORDERS_PUBLISH_TOKEN=<same>  (or copy to .secrets/hackme_coordinator_orders_publish_token)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$ROOT/.secrets/hackme_coordinator_orders_publish_token}"
mkdir -p "$(dirname "$OUT")"
if [[ -f "$OUT" && "${FORCE:-0}" != "1" ]]; then
  echo "exists: $OUT (set FORCE=1 to overwrite)" >&2
  exit 1
fi
tok="$(openssl rand -hex 32)"
# Refuse equality with known worker/admin secrets if present.
for other in \
  "$ROOT/.secrets/hackme_coordinator_worker_token" \
  "$ROOT/.secrets/hackme_coordinator_admin_token"; do
  if [[ -f "$other" ]]; then
    o="$(tr -d '\r\n' <"$other")"
    if [[ -n "$o" && "$o" == "$tok" ]]; then
      echo "collision with $(basename "$other") — retry" >&2
      exit 1
    fi
  fi
done
printf '%s' "$tok" >"$OUT"
chmod 600 "$OUT"
echo "wrote $OUT (${#tok} hex chars)"
echo "Hub:     HACKME_COORDINATOR_ORDERS_PUBLISH_TOKEN=<value>"
echo "Release: HACKME_RELEASE_ORDERS_PUBLISH_TOKEN=<value>  (Orders bundle only; never admin)"
echo "Client:  pool.orders.token next to Orders install / ~/.config/hackme/"
