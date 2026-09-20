# GHS ↔ Dig/Hunt coupling

**Status:** shipped on branch `feat/ghs-dig-coupling`  
**Honesty:** GPU hashrate (GH/s) still pays **PoH HMC/SUP**. Dig/Hunt still pays **customer escrow**. ASAN/WASM stay on **CPU**.

## What changed

| Layer | Behavior |
|-------|----------|
| **Hybrid dig boost** | When live PoH GH/s ≥ ~70% of calib, claim gap halves → more Dig/Hunt shards without killing GPU |
| **Hybrid backpressure** | Unchanged idea: GH/s ≪ calib → pause dig (protect mining) |
| **Claim GHS priority** | When hybrid workers (≥1 GH/s) are online, dig-only (~0 GH) get ~25% admit slots; hybrids always claim |
| **Fleet capacity / ETA** | `GET /api/fuzz/pool/stats` + campaign progress expose `fleet_capacity`, `est_shards_per_hour`, `eta_sec_fleet` |

## Escape hatches

```bash
HACKME_FUZZ_CLAIM_GHS_PRIORITY=0          # disable claim soft-priority
HACKME_FUZZ_CLAIM_GHS_DIG_ONLY_PCT=25     # dig-only admit % when hybrids online
HACKME_WORKER_HYBRID_FUZZ_DIG_BOOST_PCT=70
HACKME_WORKER_HYBRID_FUZZ_BACKPRESSURE_PCT=10  # dig profile default
```

## Bench

```bash
bash scripts/tests/ghs_dig_coupling_bench.sh
```

## Why this helps

- **Customers:** ETA reflects real pool GPU+dig capacity; orders prefer live hybrid rigs.
- **Miners:** Healthy GHS digs faster (more escrow) without fake “GPU ASAN”.
- **Pool:** Dig-only Sybils no longer monopolize leases while GPUs are online.
