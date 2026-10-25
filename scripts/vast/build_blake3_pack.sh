#!/usr/bin/env bash
# Build a self-contained Blake3 CUDA worker pack for Vast (or any remote NVIDIA box).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
OUT="${1:-$ROOT/dist/vast-blake3-pack}"
mkdir -p "$OUT/bin" "$OUT/scripts" "$OUT/examples"

if [[ -f "$ROOT/scripts/ops/cuda_env.sh" ]]; then
  # shellcheck source=/dev/null
  source "$ROOT/scripts/ops/cuda_env.sh" >/dev/null 2>&1 || true
fi

echo "[vast-pack] building workerpoh-cuda + blake3bench + minersign → $OUT"
go build -trimpath -tags 'cuda opencl' -o "$OUT/bin/workerpoh-cuda" ./cmd/workerpoh
go build -trimpath -tags 'cuda opencl' -o "$OUT/bin/blake3bench" ./cmd/blake3bench
go build -trimpath -o "$OUT/bin/minersign" ./cmd/minersign
go build -trimpath -o "$OUT/bin/workerfuzz" ./cmd/workerfuzz 2>/dev/null || true

cp -a "$ROOT/scripts/vast/"*.sh "$OUT/scripts/"
cp -a "$ROOT/examples/env.vast.example" "$OUT/examples/"
cat >"$OUT/README.md" <<'EOF'
# HackMe Blake3 Vast pack

1. `cp examples/env.vast.example env.vast` and fill `COORD_TOKEN` + `HACKME_MINER_ED25519_SEED_HEX`
   (`./bin/minersign -gen-seed`).
2. Burn calib: `./bin/blake3bench -backend cuda -batch $((1<<26)) -samples 3`
3. Pool: `ENV_FILE=./env.vast bash scripts/01_run_pool_worker.sh`
4. Record Pool GH/s only from coordinator `/api/work/stats` (never worker self-log).

SHARE_STRICT PPS: shares are collected in-kernel during the main Blake3 pass.
EOF

chmod 755 "$OUT/bin/"* "$OUT/scripts/"*.sh
(cd "$(dirname "$OUT")" && tar -czf "$(basename "$OUT").tar.gz" "$(basename "$OUT")")
echo "[vast-pack] ready: $OUT and ${OUT}.tar.gz"
ls -lah "$OUT/bin"
