# Blake3 PoH GPU matrix (LIVE fv≥3)

**Status:** in progress — fill rows on Vast + desktop after soft-release CUDA path.  
**Do not compare Peak GH/s to the June 2026 algebraic table** — that was pre-cliff hybrid/v1 calib.

Historical (pre-Blake3) matrix: [`archive/vast/VAST_GPU_MATRIX_VERDICT_20260602.md`](archive/vast/VAST_GPU_MATRIX_VERDICT_20260602.md) — kept as archive only.

---

## How to read columns

| Column | Meaning |
|--------|---------|
| **Burn GH/s** | Pure SearchPoH wall (`blake3bench` / calib maxBatch) — kernel truth |
| **Pool GH/s** | **Only** from `GET /api/work/stats` → `active_rigs[].hashrate_gh_s` (coordinator credit). Never from worker `submit ok ghs=` logs. |
| **Util %** | nvidia-smi during mining (often low even when burn is healthy) |
| **HMC/h est** | Accrual Δ over ≥15 min from coordinator `total_payout_hmc` — not worker self-report |
| **Settle** | On-chain settle observed from treasury in the run window (Y/N + note) |
| **Backend** | `cuda` device / `opencl` device / `opencl→host` (Blake3 host fallback) |

Method: tip ≥#485000, `pool_hashrate_kind=blake3_pow_ghs`, lease up to 64M, KEEP+Lemire+OOB-fix. Official `0.1.0-rc17.4` still lags kernel.

**Economics (2026-10-25+):** Blake3 **share-PPS** (`SHARE_STRICT=1`): pay verified shares, adaptive leases ~`LEASE_WORK_SEC` (default 3s), pool GH/M from shares/wall — not client `hashrate_gh_s`. Soft attempt-pay remains only if `SHARE_STRICT=0`. Live `REWARD_AUTO=0` + `REWARD_PER_M=5e-5`, `found_bonus=0.0001`, `max_claim_batch` up to 64G.

---

## Results matrix (Blake3)

| Series | Config | Worker / rig | Burn GH/s | Pool GH/s | Util % | HMC/h est | Settle | Backend | Verdict | Notes |
|--------|--------|--------------|-----------|-----------|--------|-----------|--------|---------|---------|-------|
| **50xx** | 1× RTX 5060 Ti | `worker-kapa-pc` | **~5–7.8** | **~4–6** (`/api/work/stats`, post GH/s honesty) | ~15–30 | fleet ~1–3 HMC/h after rpm pin | Y | **cuda** | **PASS** | Desktop; OpenCL Blake3 = host ~0.25 |
| **50xx** | 1× RTX 5090 | `vast-rtx5090-blake3-01` | **~26.8** (2²⁶) | ~0.7–1.3 pre-fix (hist) | ~0 | — | — | **cuda** | **PASS burn** | first Vast; instance gone |
| **50xx** | **2× RTX 5090** | `vast-2x5090` (`-gpu0`+`-gpu1`) | **~26.3 + ~26.4** | **~29–38** merged (`/api/work/stats`) | ~0–2 | TBD ≥15m | — | **cuda** | **PASS** | direct `:35940`; Σ burn ~52.7; fleet merge strip `-gpuN`; RTT cheat clamp fixed |
| **40xx** | 1× RTX 4090 | `vast-rtx4090-blake3-01` | **~17.1** (2²⁶) | **~20–25** (`/api/work/stats`) | ~0 | TBD ≥15m | — | **cuda** | **PASS** | direct `:38782`; calib 17.15; CC 8.9 |
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

HMC accrual ≠ chain block subsidy. Soft attempt-pay is treasury-backed; keep `reward_per_m` / M-cap aligned with ~`econ_expected_empty_hmc_hour` (~1.2). Historical inflated accrual (~3k HMC stock) remains until operator revoke/settle policy.

---

## Next fills (order)

1. Re-rent Vast 5090 / 4090 under new GH/s + rpm.  
2. Multi-GPU: 2× then **4×**.  
3. AMD after OpenCL Blake3 device kernel.  
4. Windows CUDA/OpenCL smoke.
