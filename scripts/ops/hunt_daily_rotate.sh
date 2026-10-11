#!/usr/bin/env bash
# Local Hunt daily rotator — one OSS catalog target per slot, 24h coverage.
#
# Usage (one slot now):
#   bash scripts/ops/hunt_daily_rotate.sh
#   SLOT_WALL_SEC=1800 bash scripts/ops/hunt_daily_rotate.sh
#   FORCE_TARGET=libucl bash scripts/ops/hunt_daily_rotate.sh
#   DRY_RUN=1 bash scripts/ops/hunt_daily_rotate.sh
#
# Install user timer (hourly):
#   bash scripts/ops/install_hunt_daily_rotate_user.sh
#
# Env:
#   SLOT_WALL_SEC   wall seconds per library (default 3600)
#   HUNT_PKG        hunt_lite|hunt_standard|hunt_heavy (default hunt_standard)
#   FORCE_TARGET    pin one target (skip rotation)
#   DRY_RUN=1       print plan only
#   NICE_LEVEL      default 10
#
# Slot outcomes always exit 0 after writing meta/rollup so systemd --user
# hourly timers stay healthy. Ops-hard failures (no clang/go) still exit 1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export HACKME_REPO_ROOT="$ROOT"

SLOT_WALL_SEC="${SLOT_WALL_SEC:-3600}"
HUNT_PKG="${HUNT_PKG:-hunt_standard}"
HUNT_ITER="${HUNT_ITER:-500000}"
NICE_LEVEL="${NICE_LEVEL:-10}"
DAY_UTC="${HUNT_DAILY_DAY:-$(date -u +%Y%m%d)}"
HOUR_UTC="${HUNT_DAILY_HOUR:-$(date -u +%H)}"
STAMP_UTC="$(date -u +%Y%m%dT%H%M%SZ)"
BASE_OUT="$ROOT/reports/hunt-daily/$DAY_UTC"
LOCK="$ROOT/reports/hunt-daily/.rotate.lock"
QUEUE_PY="$ROOT/scripts/ops/hunt_daily_queue.py"
mkdir -p "$BASE_OUT" "$ROOT/reports/hunt-daily"

# Disk hygiene before each slot (harness/AFL workdirs/corpus caps).
if [[ -x "$ROOT/scripts/ops/prune_hunt_miner_caches.sh" ]]; then
  bash "$ROOT/scripts/ops/prune_hunt_miner_caches.sh" --repo "$ROOT" || true
fi

log() { echo "[hunt-daily $(date -u +%H:%M:%S)] $*" | tee -a "$BASE_OUT/rotate.log"; }

finish_slot() {
  # $1 = process exit to report in logs (always returns 0 from script)
  local rc="${1:-0}"
  python3 "$ROOT/scripts/ops/export_hunt_daily_rollup.py" --day "$DAY_UTC" || true
  log "rollup → $BASE_OUT/ROLLUP.md"
  exit 0
}

write_error_local() {
  # $1=verdict $2=error code
  python3 - "$SLOT_OUT/hunt-local.json" "$TARGET" "$1" "$2" <<'PY'
import json, sys
path, target, verdict, err = sys.argv[1:5]
json.dump(
    {
        "ok": False,
        "target": target,
        "verdict": verdict,
        "error": err,
        "iterations": 0,
        "crashes": 0,
        "exec_per_sec": 0,
        "unique_signatures": 0,
        "finding_families": {"family_count": 0},
        "sanitizer_signatures": {},
    },
    open(path, "w"),
    indent=2,
)
print(path)
PY
}

write_meta() {
  local rc="$1"
  python3 - "$SLOT_OUT" "$TARGET" "$HUNT_PKG" "$SLOT_WALL_SEC" "$STAMP_UTC" "$rc" <<'PY'
import json, sys, os
slot, target, pkg, wall, stamp, rc = sys.argv[1:7]
meta = {
  "stamp": stamp,
  "target": target,
  "package": pkg,
  "wall_sec": int(wall),
  "rc": int(rc),
  "ok": int(rc) == 0 and os.path.isfile(f"{slot}/hunt-local.json"),
}
p = f"{slot}/hunt-local.json"
if os.path.isfile(p):
  hunt = json.load(open(p))
  meta["verdict"] = hunt.get("verdict")
  meta["iterations"] = hunt.get("iterations")
  meta["crashes"] = hunt.get("crashes")
  meta["exec_per_sec"] = hunt.get("exec_per_sec")
  meta["unique_signatures"] = hunt.get("unique_signatures")
  fam = hunt.get("finding_families") or {}
  meta["family_count"] = fam.get("family_count")
  if hunt.get("error"):
    meta["error"] = hunt.get("error")
    meta["ok"] = False
json.dump(meta, open(f"{slot}/meta.json", "w"), indent=2)
print(json.dumps(meta))
PY
}

exec 9>"$LOCK"
if ! flock -n 9; then
  log "SKIP: another hunt-daily slot holds $LOCK"
  exit 0
fi

if ! command -v clang >/dev/null 2>&1; then
  log "FAIL: clang required"
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  log "FAIL: go required"
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  log "FAIL: python3 required"
  exit 1
fi

pick_target() {
  if [[ -n "${FORCE_TARGET:-}" ]]; then
    echo "$FORCE_TARGET"
    return
  fi
  python3 "$QUEUE_PY" --repo "$ROOT" pick --day "$DAY_UTC" --hour "$HOUR_UTC"
}

TARGET="$(pick_target)"
SLOT_OUT="$BASE_OUT/${HOUR_UTC}00-${TARGET}"
mkdir -p "$SLOT_OUT/crashes"

ENG_LINE="$(grep -E 'const Version[[:space:]]*=' internal/fuzzengine/engine.go | head -1 | sed 's/^[[:space:]]*//')"
log "=== slot day=$DAY_UTC hour=$HOUR_UTC target=$TARGET pkg=$HUNT_PKG wall=${SLOT_WALL_SEC}s ==="
log "engine=${ENG_LINE:-unknown}"
log "out=$SLOT_OUT"

python3 "$QUEUE_PY" --repo "$ROOT" schedule --day "$DAY_UTC" --out "$BASE_OUT/schedule.json"

if [[ "${DRY_RUN:-0}" == "1" ]]; then
  log "DRY_RUN — would build+hunt $TARGET"
  exit 0
fi

# Preflight: driver must exist (FORCE_TARGET can still point at aspirational IDs).
DRIVER_OK="$(python3 - "$ROOT" "$TARGET" <<'PY'
import json, sys
from pathlib import Path
root, tid = Path(sys.argv[1]), sys.argv[2]
sys.path.insert(0, str(root / "scripts" / "ops"))
from hunt_daily_queue import load_catalog, driver_path
m = load_catalog(root)
by = {t["id"]: t for t in (m.get("targets") or []) if t.get("id")}
tgt = by.get(tid) or {"id": tid, "driver": f"{tid}_stdin"}
print("1" if driver_path(root, tgt).is_file() else "0")
PY
)"
if [[ "$DRIVER_OK" != "1" ]]; then
  log "SKIP missing driver for $TARGET — see tasks/sources/fuzz/oss/"
  write_error_local "SKIP" "missing_driver"
  write_meta 0
  finish_slot 0
fi

log "prebuild OSS pack for $TARGET"
if ! TARGETS="$TARGET" bash "$ROOT/scripts/ops/build_oss_cve_pack.sh" >>"$SLOT_OUT/build.log" 2>&1; then
  log "ERROR build — see $SLOT_OUT/build.log"
  write_error_local "ERROR" "build_failed"
  write_meta 1
  finish_slot 1
fi

log "START Hunt local"
set +e
nice -n "$NICE_LEVEL" go run ./scripts/tests/tools/hunt_bench_local.go \
  -target "$TARGET" \
  -package "$HUNT_PKG" \
  -iter "$HUNT_ITER" \
  -wall "$SLOT_WALL_SEC" \
  -out "$SLOT_OUT/hunt-local.json" \
  -report "$SLOT_OUT/hunt-report.json" \
  -crashes-dir "$SLOT_OUT/crashes" \
  >"$SLOT_OUT/hunt.log" 2>&1
rc=$?
set -e
log "DONE rc=$rc"

if [[ "$rc" -ne 0 ]]; then
  if [[ ! -f "$SLOT_OUT/hunt-local.json" ]]; then
    write_error_local "ERROR" "hunt_failed"
  else
    # Ensure failed runs surface a verdict for rollup (not blank → NONE).
    python3 - "$SLOT_OUT/hunt-local.json" <<'PY'
import json, sys
p = sys.argv[1]
doc = json.load(open(p))
if not doc.get("verdict"):
    doc["verdict"] = "ERROR"
    doc["ok"] = False
    doc.setdefault("error", "hunt_failed")
    json.dump(doc, open(p, "w"), indent=2)
PY
  fi
fi

write_meta "$rc"
finish_slot "$rc"
