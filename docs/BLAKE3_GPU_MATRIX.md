# Blake3 PoH GPU matrix (LIVE fv≥3)

**Status:** in progress — fill rows on Vast + desktop after soft-release CUDA path.  
**Do not compare Peak GH/s to the June 2026 algebraic table** — that was pre-cliff hybrid/v1 calib.

Historical (pre-Blake3) matrix: [`archive/vast/VAST_GPU_MATRIX_VERDICT_20260602.md`](archive/vast/VAST_GPU_MATRIX_VERDICT_20260602.md) — kept as archive only.

---

## How to read columns

| Column | Meaning |
|--------|---------|
| **Burn GH/s** | Pure SearchPoH wall (`blake3bench` / calib maxBatch) — kernel truth |
| **Pool GH/s** | Coordinator credit for that worker/rig (lease RTT–aware) |
| **Util %** | nvidia-smi during mining (often low even when burn is healthy) |
| **HMC/h est** | Accrual Δ over ≥15 min window (attempt-pay + found_bonus); not a promise |
| **Settle** | On-chain settle observed from treasury in the run window (Y/N + note) |
| **Backend** | `cuda` device / `opencl` device / `opencl→host` (Blake3 host fallback) |

Method: tip ≥#485000, `pool_hashrate_kind=blake3_pow_ghs`, lease up to 64M, build `bde3081+` / `5e2f8d9+`.

---

## Results matrix (Blake3)

| Series | Config | Worker / rig | Burn GH/s | Pool GH/s | Util % | HMC/h est | Settle | Backend | Verdict | Notes |
|--------|--------|--------------|-----------|-----------|--------|-----------|--------|---------|---------|-------|
| **50xx** | 1× RTX 5060 Ti | `worker-kapa-pc` | **~5.1–6.5** | **~0.9–1.8** | ~15–30 | TBD window | Y (treasury settle live) | **cuda** | **PASS** | Desktop; OpenCL Blake3 = host ~0.25 |
| **50xx** | 1× RTX 5090 | — | TBD | TBD | TBD | TBD | — | cuda | — | Vast |
| **40xx** | 1× RTX 4090 | — | TBD | TBD | TBD | TBD | — | cuda | — | Vast |
| **40xx** | 2× RTX 4070 Ti S | — | TBD | TBD | TBD | TBD | — | cuda | — | multi-GPU agg |
| **30xx** | 2× RTX 3060 Ti | — | TBD | TBD | TBD | TBD | — | cuda | — | multi-GPU |
| **20xx** | 4× RTX 2080 Ti | — | TBD | TBD | TBD | TBD | — | cuda | — | **4-GPU rig** target |
| **10xx** | 2× GTX 1080 Ti | — | TBD | TBD | TBD | TBD | — | opencl→host? | — | needs OpenCL Blake3 device |
| **AMD** | RX 6800 / 7800 | — | TBD | TBD | TBD | TBD | — | opencl | — | blocked until `.cl` Blake3 |

---

## Multi-GPU / rig checklist

For **2× / 4×** configs also record:

1. One UI row vs `-gpu0…N` split (fleet aggregate).  
2. Σ burn ≈ sum of per-GPU burns (±10%).  
3. Pool credit scales with GPUs (not stuck on 1×).  
4. PSU / PCIe / thermals note (throttle → util drop).  
5. Hybrid Dig/Hunt: `hybrid_workers_online` + `no_fuzz_work` when campaigns idle.

---

## Income note

HMC accrual ≠ “mining exchange rate.” Soft attempt-pay + `found_bonus` dominate when large leases find often. Always pair **HMC/h est** with tip height, `reward_per_m`, and whether settle drained treasury float.

---

## Next fills (order)

1. Vast single-GPU: 5090 → 4090 → 3090/3060 Ti.  
2. Multi-GPU: 2× then **4×** (20xx or 30xx).  
3. AMD after OpenCL Blake3 device kernel.  
4. Windows CUDA/OpenCL smoke (separate column later).
