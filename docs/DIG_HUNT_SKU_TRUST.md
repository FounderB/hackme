# Dig/Hunt SKU honesty + CLEAN trust

## Product modes

| Mode | Dig packages | Hunt packages | Seeds | Verify |
|------|--------------|---------------|-------|--------|
| **smoke** | scan, audit | hunt_lite, hunt_standard | merge OFF by default | floor + canary + sampled replay |
| **deep** | deep | hunt_heavy | external/research merge allowed | 40% sample + full-segment Dig hygiene prove (auto), longer budgets |

Promise language on reports: attested distributed smoke/depth — **not** OSS-Fuzz/CVE replacement.

## Dig money (Deep vs Smoke)

| SKU | Budget HMC | Escrow split | Est. miner per-run | PoH reward | Seeds |
|-----|------------|--------------|--------------------|------------|-------|
| Audit (smoke) | 5 | 20/80 | ~0.0039 | 0.05 | OFF |
| Deep | 40 | **40/60** | ~0.0078 | 0.25 | ON |

API/CLI expose `dig_billing` (`billing_model` = `smoke_runs` | `deep_depth`). Deep min escrow **25 HMC**.

## Env knobs (hub)

See [POOL_FUZZ_DISTRIBUTED.md](POOL_FUZZ_DISTRIBUTED.md). Floor soak:

- `HACKME_POOL_CLEAN_EDGES_REQUIRE=auto` (default): ON when ≥90% of last 64 Dig CLEAN samples report `edges_touched` (n≥32). Force with `=1`, disable with `=0`.
- `HACKME_POOL_CLEAN_MIN_MS_PER_EXEC` default **1**; deep min edges default **2** (`HACKME_POOL_CLEAN_MIN_EDGES_DEEP`).
- `HACKME_POOL_REPLAY_SAMPLE_PCT_DEEP` default **40**; `HACKME_POOL_HYGIENE_SEGMENT_PROVE=auto` full-segment proves Dig hygiene for deep or escrow multi-exec (Hunt untouched).
- Pool stats: `clean_floor.edges_report_rate` / `edges_require_active`.

## Worker

Dig submit must include `edges_touched` on `wasm_edge_bitmap` once auto-require flips or `REQUIRE=1`. Honest workers already send it.
