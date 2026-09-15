#!/usr/bin/env bash
# Systemd digger entry for hub named fuzz workers.
set -euo pipefail
ROOT="${HACKME_REPO_ROOT:-/opt/hackme}"
cd "$ROOT"
export HACKME_REPO_ROOT="$ROOT"
export COORD_URL="${COORD_URL:-http://127.0.0.1:18081}"
if [[ -z "${COORD_TOKEN:-}" && -f .secrets/hackme_coordinator_worker_token ]]; then
  COORD_TOKEN="$(tr -d '\r\n' < .secrets/hackme_coordinator_worker_token)"
fi
export COORD_TOKEN
export HACKME_WORKER_HUNT_TIMEOUT_MS="${HACKME_WORKER_HUNT_TIMEOUT_MS:-300000}"
exec "$ROOT/bin/workerfuzz" -coord "$COORD_URL" -token "$COORD_TOKEN" -worker "${WORKER_ID:?}" -timeout-ms "${WORKERFUZZ_TIMEOUT_MS:-2000}"
