# Hunt / Dig engine — v2.7

**Engine:** `fuzz_engine_v2.7`

## vs v2.6

| | v2.6 | v2.7 |
|--|------|------|
| Havoc ops | 48 | **64** |
| Stack depth | ≤24 | **≤32** |
| Pre-havoc crossover | every 9th | **every 7th** |
| Extra ops | footguns / endian | nest braces, UTF-16LE, protobuf tags, float NaN bits, length frames, RTL marks, corpus mask |

## Local stress

```bash
go test ./internal/fuzzengine/ -run 'TestEngineAB|TestLocalStress|TestHavoc' -v -count=1
bash scripts/tests/hunt_engine_depth_bench.sh
bash scripts/tests/fuzz_engine_local_stress.sh
```
