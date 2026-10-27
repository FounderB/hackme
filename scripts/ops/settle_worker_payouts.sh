#!/usr/bin/env bash
# Thin wrapper: prefer Go settler (cmd/workersettle). Legacy bash archived at
# scripts/archive/ops/settle_worker_payouts.sh — set SETTLE_USE_BASH=1 to force it.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SETTLE_BIN="${SETTLE_BIN:-}"
if [[ -z "$SETTLE_BIN" ]]; then
  for c in "$ROOT_DIR/bin/workersettle" "$ROOT_DIR/workersettle"; do
    if [[ -x "$c" ]]; then
      SETTLE_BIN="$c"
      break
    fi
  done
fi

if [[ "${SETTLE_USE_BASH:-0}" != "1" && -n "$SETTLE_BIN" && -x "$SETTLE_BIN" ]]; then
  exec "$SETTLE_BIN" "$@"
fi

ARCHIVED="$ROOT_DIR/scripts/archive/ops/settle_worker_payouts.sh"
if [[ -x "$ARCHIVED" || -f "$ARCHIVED" ]]; then
  echo "[settle-workers] WARN: falling back to archived bash settler (build: go build -o bin/workersettle ./cmd/workersettle)" >&2
  exec bash "$ARCHIVED" "$@"
fi

echo "[settle-workers] FATAL: no bin/workersettle and no archived bash at $ARCHIVED" >&2
exit 1
