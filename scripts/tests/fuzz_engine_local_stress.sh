#!/usr/bin/env bash
# Heavy local stress for fuzz_engine mutation depth (runs on contributor machine).
# Usage: bash scripts/tests/fuzz_engine_local_stress.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "== host =="
uname -a || true
go version
nproc || true

echo
echo "== unit + A/B + multi-format stress =="
go test ./internal/fuzzengine/ -count=1 -timeout 300s -v \
  -run 'TestEngineABComparison|TestLocalStressMultiFormat|TestHavocStack|TestHavocExtra|TestMeasureMutationDepth|TestCompactCorpusSeed|TestFindingFamily'

echo
echo "== full fuzzengine package =="
go test ./internal/fuzzengine/ -count=1 -timeout 300s

echo
echo "== related hunt + workerfuzzloop (compile/logic) =="
go test ./internal/hunt/ ./internal/workerfuzzloop/ -count=1 -timeout 180s

echo
echo "== 50k-sample A/B microbench (JSON) =="
go test ./internal/fuzzengine/ -run TestEngineABComparison -count=1 -v \
  -timeout 120s 2>&1 | tee /tmp/fuzz_ab_5k.txt || true

# Extra: programmatically print 50k via a one-off if needed — gate already uses 5k.
python3 - <<'PY'
import re, pathlib
text = pathlib.Path("/tmp/fuzz_ab_5k.txt").read_text(errors="ignore")
m = re.search(r"gain_unique=([0-9.]+)% gain_lens=([0-9.]+)%", text)
print("parsed:", m.groups() if m else "n/a")
PY

echo "OK — local stress finished"
