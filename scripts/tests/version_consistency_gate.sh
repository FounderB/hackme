#!/usr/bin/env bash
# Fail if main.go Version, app.js RELEASE_VER, and CURRENT_VERSION diverge.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_VER="$(grep -E '^\s*Version\s*=' "$ROOT/main.go" | sed -n 's/.*"\([^"]*\)".*/\1/p' | head -1)"
JS_VER="$(grep -oE 'RELEASE_VER = "[^"]+"' "$ROOT/web/site/assets/app.js" | sed 's/.*"\([^"]*\)".*/\1/')"
CUR_VER="$(tr -d ' \n\r' <"$ROOT/scripts/release/CURRENT_VERSION")"
fail=0
echo "[version-gate] main.go=$GO_VER app.js=$JS_VER CURRENT_VERSION=$CUR_VER"
if [[ "$GO_VER" != "$JS_VER" ]]; then
  echo "[version-gate] FAIL main.go != app.js" >&2
  fail=$((fail + 1))
fi
if [[ "$GO_VER" != "$CUR_VER" ]]; then
  echo "[version-gate] FAIL main.go != CURRENT_VERSION" >&2
  fail=$((fail + 1))
fi
# ISO channel may be a string literal or aliased to PUBLISHED_ARTIFACT_VER (honest until SHA).
JS_ISO="$(grep -oE 'ISO_CHANNEL = "[^"]+"' "$ROOT/web/site/assets/app.js" | sed 's/.*"\([^"]*\)".*/\1/' || true)"
if [[ -z "$JS_ISO" ]] && grep -qE 'ISO_CHANNEL = PUBLISHED_ARTIFACT_VER' "$ROOT/web/site/assets/app.js"; then
  JS_ISO="$(grep -oE 'PUBLISHED_ARTIFACT_VER = "[^"]+"' "$ROOT/web/site/assets/app.js" | sed 's/.*"\([^"]*\)".*/\1/')"
fi
CUR_ISO="$(tr -d ' \n\r' <"$ROOT/scripts/release/CURRENT_ISO_VERSION" 2>/dev/null || echo 0.1.0-rc11l)"
if [[ -n "$JS_ISO" && "$JS_ISO" != "$CUR_ISO" ]]; then
  echo "[version-gate] FAIL app.js ISO_CHANNEL ($JS_ISO) != CURRENT_ISO_VERSION ($CUR_ISO)" >&2
  fail=$((fail + 1))
fi
if [[ "$fail" -gt 0 ]]; then exit 1; fi
ISO_VER="$CUR_ISO"
echo "[version-gate] OK — Win/Linux channel $GO_VER · ISO channel $ISO_VER"
