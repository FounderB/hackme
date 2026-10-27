# Paytest Dig — payout honesty (2026-10-27)

**Campaign:** `campaign-paytest-dig-20261027t090013z`  
**PoH order:** `order-paytest-20261027t090013z` (completed 4/4)  
**Origin node:** VPS `hackme-node` `:18080` · payer `HMC-dfc143732a1fc3bc`

## Verdict: **PASS — money path honest**

| Check | Expected | Observed |
|-------|----------|----------|
| Escrow open | 25 HMC, split Dig Deep **40/60** | runs 10 / bounty 15 · `escrow_split=40_60` |
| Runs complete | 64/64 | **64 paid** (`settle_run_status=paid`) |
| Runs pool spent | 10.0000 HMC | `runs_paid_hmc=10` · `per_run_units=15625000` (0.15625) |
| Bounty | refund if no crash-class | **15 HMC refunded** · `bounty_paid=0` |
| Report gate | PASS / noise not bounty | `verdict_card.gate=PASS` · `money_spent_hmc=10` · coverage_noise |
| Miner credits | Σ paid = runs × 0.15625 | e.g. `HMC-424fae…` **5.15625** = 33×0.15625 **exact** |
| Fresh miners | exact multiples | `HMC-77edfd…` / `HMC-0212af…` = **0.78125** (5 runs) |
| PoH attach | prepaid 4×0.05 | **0.2 HMC** · status **completed** · `refunded_hmc=0` |
| Customer wallet | ~100 − 10 − 0.2 | **89.67 HMC** spendable (≈89.8; ~0.13 residual/fees) |

## Who earned (Dig runs pool)

| Miner | Paid runs | HMC |
|-------|-----------|-----|
| HMC-424fae114b092cec | 33 | 5.15625 |
| HMC-da21ed76a7fb45f4 | 5 | 0.78125 |
| HMC-77edfd1440f868ee | 5 | 0.78125 |
| HMC-0212afba2e98403d | 5 | 0.78125 |
| HMC-48078e77a2a4f7ed | 4 | 0.625 |
| +7 others | 12 | 1.875 |
| **Total** | **64** | **10.000** |

Fleet mix: hub-fuzz-* + shannon/digga + others. Vast CUDA hybrid was mining PoH in parallel (share-PPS rail, separate from Dig escrow).

## Semantics note (not a bug)

Pool `progress.runs_ok` stayed low because Dig `result_ok=0` on detector hits. Those shards still **PayRun**. Customer report correctly classifies them as **coverage_noise** and does **not** pay bounty.

## Ops fix applied

- Set `HACKME_WASM_VALIDATE_BIN=/opt/hackme/hackme-wasm-check` in VPS `.env.coord` and restarted coordinator (was missing → earlier false “wasm-validate child” failures).

## Hunt

Node `/api/hunt/campaigns` still needs a clean `upstream_target_id` body (this pass focused Dig+PoH). Prior Vast Hunt campaign `hunt-vast-1793090094` already proved **24/24 runs_ok**.
