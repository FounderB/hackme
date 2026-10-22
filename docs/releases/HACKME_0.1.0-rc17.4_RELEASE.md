# HackMe 0.1.0-rc17.4

**Date:** 2026-10-22

## Highlights

- **Blake3 PoH gate** moved to height **485000** (was 500000). Update all `workerpoh` before tip reaches the gate.
- **Pseudo-shares:** coordinator verifies `share_nonces` at `M/1024`. **Soft mode (default) always attempt-pays**; shares are stats / rate-check only (`SHARE_RATE_REJECT` off until GPU full-batch harvest). `SHARE_STRICT=1` later enables share-pay.
- **Settlement:** HMC SQLite `export-json` merges SUP watermarks (no more `settled_sup` rollback); export fail-closed; SUP mint keeps pending on ambiguous curl; `set-sup` writes SQLite.
- **HackMe OS ISO** published (`HackMe-OS-0.1.0-rc17.4-amd64.iso` + `SHA256SUMS-iso.txt`) on GitHub release and `hackme.tech/dist`.
- **UX:** miner notices warn about Blake3 GH/s cliff (~100–200× drop vs algebraic) and required worker update.
- CUDA Blake3 device path parity tested on RTX 5060 Ti; NVRTC ≥12.8 recommended for `compute_120`.

## Operators

1. Deploy node + coordinator + `bin/workersettlectl` (soft attempt-pay is the live default).
2. Redeploy fleet workers from this release before tip **#485000**.
3. Confirm `SETTLE_EXPECTED_HOSTNAME` and settle healthcheck.
4. Read `/api/miner/notices` / site news.
5. Keep `HACKME_COORDINATOR_SHARE_STRICT` / `SHARE_RATE_REJECT` off until GPU share harvest lands.
