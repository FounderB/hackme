# HackMe 0.1.0-rc17.4

**Date:** 2026-10-22

## Highlights

- **Blake3 PoH gate** moved to height **485000** (was 500000). Update all `workerpoh` before tip reaches the gate.
- **Pseudo-shares:** coordinator verifies `share_nonces` at `M/1024`; soft attempt-pay fallback remains.
- **Settlement:** HMC SQLite `export-json` merges SUP watermarks (no more `settled_sup` rollback); export fail-closed; SUP mint keeps pending on ambiguous curl; `set-sup` writes SQLite.
- **UX:** miner notices warn about Blake3 GH/s cliff (~100–200× drop vs algebraic) and required worker update.
- CUDA Blake3 device path parity tested on RTX 5060 Ti; NVRTC ≥12.8 recommended for `compute_120`.

## Operators

1. Deploy node + coordinator + `bin/workersettlectl`.
2. Redeploy fleet workers from this release.
3. Confirm `SETTLE_EXPECTED_HOSTNAME` and settle healthcheck.
4. Read `/api/miner/notices` / site news.
