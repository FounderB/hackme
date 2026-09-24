#!/usr/bin/env bash
# Local AFL++ soak → Hunt L2 seed cache (.cache/hunt-lf-seeds/{target}).
# Internal calibration lane — not a pool SKU.
#
#   TARGET=cjson WALL_SEC=300 bash scripts/ops/run_oss_afl_session.sh
#   TARGET=libucl WALL_SEC=600 HARNESS=/path/to/asan_harness bash scripts/ops/run_oss_afl_session.sh
#
# Requires: afl-fuzz (afl++), clang (to build harness if HARNESS unset).
# Seeds land in the same cache as libFuzzer imports for RankLibFuzzerSeeds merge.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export HACKME_REPO_ROOT="$ROOT"

TARGET="${TARGET:-cjson}"
WALL_SEC="${WALL_SEC:-300}"
OUT="${OUT:-$ROOT/.cache/hunt-afl/$TARGET}"
SEED_CACHE="${SEED_CACHE:-$ROOT/.cache/hunt-lf-seeds/$TARGET}"
HARNESS="${HARNESS:-}"

log() { echo "[hunt-afl $(date -u +%H:%M:%S)] $*" >&2; }

command -v afl-fuzz >/dev/null 2>&1 || {
  echo "[hunt-afl] need afl-fuzz (afl++)" >&2
  exit 2
}

mkdir -p "$OUT/in" "$OUT/out" "$SEED_CACHE"
if [[ -z "$(ls -A "$OUT/in" 2>/dev/null || true)" ]]; then
  printf '{"a":1}\n' >"$OUT/in/seed1.json"
  printf '[]\n' >"$OUT/in/seed2.json"
  printf 'x\n' >"$OUT/in/seed3.bin"
fi

if [[ -z "$HARNESS" ]]; then
  log "ensure upstream clone TARGET=$TARGET"
  TARGETS="$TARGET" bash "$ROOT/scripts/ops/build_oss_cve_pack.sh" >/dev/null
  # Prefer an already-built Hunt ASAN harness if present in cache.
  HARNESS="$(find "$ROOT/.cache/hunt-harness" -type f -name '*.bin' 2>/dev/null | head -1 || true)"
  if [[ -z "$HARNESS" || ! -x "$HARNESS" ]]; then
    echo "[hunt-afl] set HARNESS=/path/to/asan_stdin_harness (built Hunt harness)" >&2
    exit 2
  fi
fi
[[ -x "$HARNESS" ]] || {
  echo "[hunt-afl] HARNESS not executable: $HARNESS" >&2
  exit 2
}

log "afl-fuzz TARGET=$TARGET wall=${WALL_SEC}s harness=$HARNESS"
export AFL_SKIP_CPUFREQ="${AFL_SKIP_CPUFREQ:-1}"
export AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES="${AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES:-1}"
export ASAN_OPTIONS="${ASAN_OPTIONS:-detect_leaks=0:abort_on_error=1:allocator_may_return_null=1}"
timeout --signal=INT "${WALL_SEC}s" afl-fuzz -i "$OUT/in" -o "$OUT/out" -V "$WALL_SEC" -- "$HARNESS" @@ \
  >"$OUT/afl.log" 2>&1 || true

# Prefer queue / crashes as L2 seeds (same dir as LF import).
copied=0
for d in "$OUT/out/default/queue" "$OUT/out/queue" "$OUT/out/default/crashes" "$OUT/out/crashes"; do
  [[ -d "$d" ]] || continue
  while IFS= read -r -d '' f; do
    base="$(basename "$f")"
    [[ "$base" == README.txt ]] && continue
    cp -n "$f" "$SEED_CACHE/afl-${base}" 2>/dev/null || cp "$f" "$SEED_CACHE/afl-${base}.$$"
    copied=$((copied + 1))
  done < <(find "$d" -type f -print0 2>/dev/null)
done
log "copied $copied AFL artifacts → $SEED_CACHE"
echo "$copied"
