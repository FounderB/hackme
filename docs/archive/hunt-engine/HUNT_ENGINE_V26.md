# Hunt / Dig engine — v2.6

**Engine:** `fuzz_engine_v2.6`

## Focus

Deeper **havoc mutation** for Hunt/Dig byte corpora — more parser footguns, longer stacks, more crossover — while staying **deterministic** for coordinator replay.

## What landed vs v2.5

| Feature | Why |
|---------|-----|
| **48 havoc ops** (was 32) | reverse, endian swap, sieve, format/URL/HTTP/gzip/zip footguns, case flip, chunk length mismatch, corpus splice |
| **Stack depth ≤24** (was 16) | rarer deep stacks when stage+salt align |
| **Crossover denser** | pre-havoc every 9th salt (was 13th) + double crossover op |
| **A/B gate vs frozen T0** | must not regress v2.5 unique/lens on the 5000-sample grid |

## Honesty

Mutation uniqueness ≠ CVE. Gains are **input diversity** for ASAN fleet depth, not “more bugs guaranteed”.

## Bench

```bash
go test ./internal/fuzzengine/ -run 'TestMeasureMutationDepth|TestEngineAB|TestHavoc' -v -count=1
bash scripts/tests/hunt_engine_depth_bench.sh
```
