#!/usr/bin/env bash
# Baseline + gate for GHS ↔ Dig/Hunt coupling.
# Usage: bash scripts/tests/ghs_dig_coupling_bench.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "== unit: ScheduleDig (boost / backpressure) =="
go test ./internal/workerfuzzloop/ -run 'TestScheduleDig|TestTruthy|TestEnvInt' -count=1 -timeout 60s

echo
echo "== unit: fuzz claim GHS priority + fleet ETA =="
go test ./cmd/coordinator/ -run 'TestAllowFuzzClaimByGHS|TestFuzzFleetCapacity|TestWorkerRateLimit|TestEffectiveWorkerHashrate' -count=1 -timeout 90s

echo
echo "== micro A/B: claim-gap scale under healthy vs collapsed GHS =="
go test ./internal/workerfuzzloop/ -run TestScheduleDigBackpressureAndBoost -v -count=1

echo
echo "== JSON summary (T0 logic gates) =="
python3 - <<'PY'
import json, time
summary = {
  "bench": "ghs_dig_coupling",
  "ts": int(time.time()),
  "gates": {
    "schedule_dig_boost_gap_scale": 0.5,
    "schedule_dig_boost_floor_pct": 70,
    "claim_ghs_priority_default": True,
    "dig_only_admit_pct_when_hybrid": 25,
    "fleet_eta_heuristic": "hybrid*90 + dig_only*180 shards/h",
  },
  "honesty": "GPU GHS steers Dig/Hunt claim priority + ETA; ASAN/WASM remain CPU",
  "compare_hint": "Measure pool: est_shards_per_hour + eta_sec_fleet before/after deploy; dig-pause rate vs dig_boosted on hybrid logs",
}
print(json.dumps(summary, indent=2))
PY

echo "OK — GHS↔Dig coupling gates passed"
