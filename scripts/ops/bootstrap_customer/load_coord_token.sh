#!/usr/bin/env bash
# Load coordinator admin/worker token for pool progress polls.
# Never use the local node HACKME_ADMIN_TOKEN — it is rejected by the public pool.
#
# Usage (from place_*/snapshot scripts):
#   # shellcheck source=load_coord_token.sh
#   source "$(dirname "$0")/load_coord_token.sh"
#   load_bootstrap_coord_token
#   # sets COORD_POLL_TOKEN
load_bootstrap_coord_token() {
  local install="${BOOTSTRAP_INSTALL:-/opt/hackme-bootstrap}"
  COORD_POLL_TOKEN=""
  # Prefer live env / .env over .secrets — token rotates land in .env first; a stale
  # coordinator_admin.token caused pool_sync 401 + progress runs_done=0 soft-launch misses.
  # Never use the local node HACKME_ADMIN_TOKEN (rejected by the public pool).
  if [[ -n "${HACKME_COORDINATOR_ADMIN_TOKEN:-}" ]]; then
    COORD_POLL_TOKEN="$(printf '%s' "$HACKME_COORDINATOR_ADMIN_TOKEN" | tr -d '\r\n')"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -n "${HACKME_POOL_COORDINATOR_ADMIN_TOKEN:-}" ]]; then
    COORD_POLL_TOKEN="$(printf '%s' "$HACKME_POOL_COORDINATOR_ADMIN_TOKEN" | tr -d '\r\n')"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -f "$install/.env" ]]; then
    COORD_POLL_TOKEN="$(grep -m1 '^HACKME_COORDINATOR_ADMIN_TOKEN=' "$install/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r\n' || true)"
    if [[ -z "$COORD_POLL_TOKEN" ]]; then
      COORD_POLL_TOKEN="$(grep -m1 '^HACKME_POOL_COORDINATOR_ADMIN_TOKEN=' "$install/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r\n' || true)"
    fi
  fi
  # Explicit COORD_ADMIN_FILE wins when set; otherwise secrets file is last-resort fallback.
  if [[ -z "$COORD_POLL_TOKEN" && -n "${COORD_ADMIN_FILE:-}" && -f "$COORD_ADMIN_FILE" ]]; then
    COORD_POLL_TOKEN="$(tr -d '\r\n' <"$COORD_ADMIN_FILE")"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -f "$install/.secrets/coordinator_admin.token" ]]; then
    COORD_POLL_TOKEN="$(tr -d '\r\n' <"$install/.secrets/coordinator_admin.token")"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -n "${HACKME_POOL_COORDINATOR_TOKEN:-}" ]]; then
    COORD_POLL_TOKEN="$(printf '%s' "$HACKME_POOL_COORDINATOR_TOKEN" | tr -d '\r\n')"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -f "$install/.env" ]]; then
    COORD_POLL_TOKEN="$(grep -m1 '^HACKME_POOL_COORDINATOR_TOKEN=' "$install/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r\n' || true)"
  fi
  if [[ -z "$COORD_POLL_TOKEN" ]]; then
    echo "[coord-token] WARN missing coordinator token (progress polls will return unauthorized/{})" >&2
  fi
}
