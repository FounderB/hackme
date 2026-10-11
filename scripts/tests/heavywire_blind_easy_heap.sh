#!/usr/bin/env bash
# Blind easy BUG-HEAP prove: public TLV seeds + dict only (NO seed_bug_heap.bin).
# Pass = ASAN heap-buffer-overflow (or libFuzzer crash artifact) within WALL_SEC.
#
#   bash scripts/tests/heavywire_blind_easy_heap.sh
#   HEAVYWIRE_ROOT=/path/to/heavywire WALL_SEC=180 bash scripts/tests/heavywire_blind_easy_heap.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
HW="${HEAVYWIRE_ROOT:-/home/kapa/Desktop/123/heavywire}"
WALL_SEC="${WALL_SEC:-180}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$ROOT/reports/customer-hunt/heavywire-blind-easy-$STAMP"
mkdir -p "$OUT"

log() { echo "[hw-easy $(date -u +%H:%M:%S)] $*" | tee -a "$OUT/run.log"; }

[[ -d "$HW" ]] || { echo "missing heavywire at $HW" >&2; exit 2; }

log "prepare public seeds (exclude private heap crasher)"
(cd "$HW" && make CC=clang seeds fuzz >/dev/null)
rm -f "$HW/corpus/seed_bug_heap.bin"
ls -la "$HW/corpus" | tee "$OUT/corpus-ls.txt"

log "lane A: native libFuzzer ${WALL_SEC}s (ground truth)"
mkdir -p "$OUT/native"
set +e
(cd "$HW" && ./build/fuzz_parse corpus \
  -max_total_time="$WALL_SEC" \
  -max_len=4096 \
  -dict=fuzz/heavywire.dict \
  -artifact_prefix="$OUT/native/crash-" \
  -print_final_stats=1) >"$OUT/native/fuzz.log" 2>&1
NATIVE_RC=$?
set -e
NATIVE_CRASHES=$(find "$OUT/native" -type f -name 'crash-*' ! -name '*.txt' 2>/dev/null | wc -l | tr -d ' ')
NATIVE_ASAN=0
rg -q 'heap-buffer-overflow|AddressSanitizer|SUMMARY: AddressSanitizer' "$OUT/native/fuzz.log" && NATIVE_ASAN=1 || true
log "native rc=$NATIVE_RC crashes=$NATIVE_CRASHES asan_hit=$NATIVE_ASAN"

log "lane B: Hunt RunEngineShard libfuzzer ${WALL_SEC}s with pack seeds+dict"
PIN="$OUT/pin"
rm -rf "$PIN"
mkdir -p "$PIN"
rsync -a --exclude '.git' --exclude 'build' --exclude 'artifacts' --exclude 'dist' \
  --exclude 'BUGS.md' --exclude 'HACKME.md' --exclude 'POOL_HANDOFF.md' \
  "$HW/" "$PIN/"
rm -f "$PIN/corpus/seed_bug_heap.bin"
go run ./scripts/tests/tools/heavywire_hunt_engine_prove.go \
  -pin "$PIN" \
  -wall "$WALL_SEC" \
  -out "$OUT/hunt-engine.json" \
  >"$OUT/hunt-engine.log" 2>&1 || true

python3 - "$OUT" "$NATIVE_ASAN" "$NATIVE_CRASHES" "$ROOT" <<'PY'
import json, os, sys, glob, re
out, native_asan, native_crashes, hackme_root = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), os.path.abspath(sys.argv[4])
hunt_asan = 0
hunt_crashes = 0
lf_dir = os.path.join(hackme_root, "reports", "oss-cve-libfuzzer", "inv_heavywire_blind_easy")
hunt_paths = [os.path.join(out, "hunt-engine.log")]
for name in ("session.stderr",):
    p = os.path.join(lf_dir, name)
    if os.path.isfile(p):
        hunt_paths.append(p)
hunt_paths += glob.glob(os.path.join(lf_dir, "crashes", "*"))
for p in hunt_paths:
    if os.path.isfile(p) and os.path.getsize(p) < 5_000_000:
        try:
            t = open(p, errors="replace").read()
        except Exception:
            continue
        if re.search(r"heap-buffer-overflow|AddressSanitizer|SUMMARY: AddressSanitizer", t):
            hunt_asan = 1
    if "/crashes/" in p.replace("\\", "/") and os.path.isfile(p) and not p.endswith(".txt"):
        hunt_crashes += 1
eng = {}
jp = os.path.join(out, "hunt-engine.json")
if os.path.isfile(jp):
    eng = json.load(open(jp))
    hunt_crashes = max(hunt_crashes, int(eng.get("crash_files") or 0))
    if eng.get("asan_hit"):
        hunt_asan = 1
report = {
  "native_asan": native_asan,
  "native_crashes": native_crashes,
  "hunt_asan": hunt_asan,
  "hunt_crashes": hunt_crashes,
  "hunt_engine": eng,
  "pass": bool(native_asan or native_crashes or hunt_asan or hunt_crashes),
}
json.dump(report, open(os.path.join(out, "SCORECARD.json"), "w"), indent=2)
print(json.dumps(report, indent=2))
if not report["pass"]:
    raise SystemExit(1)
print("PASS: easy BUG-HEAP visible without private seed")
PY

log "DONE → $OUT/SCORECARD.json"
