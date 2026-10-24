#!/usr/bin/env bash
# Local Blake3 burn comparison: CUDA device vs OpenCL SearchPoH vs CPU host.
#   bash scripts/local/bench_blake3_backends.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/ops/cuda_env.sh
source "$ROOT/scripts/ops/cuda_env.sh"
export LD_LIBRARY_PATH="${LD_LIBRARY_PATH:-}:$ROOT/.deps/cuda-lib"
BATCH="${BENCH_BATCH:-67108864}"
SAMPLES="${BENCH_SAMPLES:-3}"
OUT_DIR="${BENCH_OUT:-$ROOT/docs/local}"
mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
REPORT="$OUT_DIR/blake3-backend-bench-$STAMP.txt"

{
  echo "Blake3 backend bench $STAMP"
  echo "host=$(hostname) batch=$BATCH samples=$SAMPLES"
  nvidia-smi --query-gpu=name,driver_version,utilization.gpu,power.draw --format=csv,noheader 2>/dev/null || true
  echo
} | tee "$REPORT"

run() {
  local be="$1"
  echo "=== backend=$be ===" | tee -a "$REPORT"
  go run -tags cuda,opencl ./cmd/blake3bench -backend "$be" -batch "$BATCH" -samples "$SAMPLES" 2>&1 | tee -a "$REPORT"
  echo | tee -a "$REPORT"
}

run cuda
run opencl
run cpu

echo "report: $REPORT"
rg -n 'BEST_AVG_GHS|OpenCL Blake3|host Blake3' "$REPORT" || true
