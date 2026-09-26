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
  # Prefer secrets file (same as bootstrap_resync_pool) — never local HACKME_ADMIN_TOKEN.
  if [[ -f "${COORD_ADMIN_FILE:-$install/.secrets/coordinator_admin.token}" ]]; then
    COORD_POLL_TOKEN="$(tr -d '\r\n' <"${COORD_ADMIN_FILE:-$install/.secrets/coordinator_admin.token}")"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -n "${HACKME_COORDINATOR_ADMIN_TOKEN:-}" ]]; then
    COORD_POLL_TOKEN="$(printf '%s' "$HACKME_COORDINATOR_ADMIN_TOKEN" | tr -d '\r\n')"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -n "${HACKME_POOL_COORDINATOR_TOKEN:-}" ]]; then
    COORD_POLL_TOKEN="$(printf '%s' "$HACKME_POOL_COORDINATOR_TOKEN" | tr -d '\r\n')"
  fi
  if [[ -z "$COORD_POLL_TOKEN" && -f "$install/.env" ]]; then
    COORD_POLL_TOKEN="$(grep -m1 '^HACKME_COORDINATOR_ADMIN_TOKEN=' "$install/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r\n' || true)"
    if [[ -z "$COORD_POLL_TOKEN" ]]; then
      COORD_POLL_TOKEN="$(grep -m1 '^HACKME_POOL_COORDINATOR_TOKEN=' "$install/.env" 2>/dev/null | cut -d= -f2- | tr -d '\r\n' || true)"
    fi
  fi
  if [[ -z "$COORD_POLL_TOKEN" ]]; then
    echo "[coord-token] WARN missing coordinator token (progress polls will return unauthorized/{})" >&2
  fi
}
