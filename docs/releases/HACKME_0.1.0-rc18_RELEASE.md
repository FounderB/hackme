# HackMe 0.1.0-rc18

## Highlights
- **Share-PPS LIVE** — `SHARE_STRICT` pays verified Blake3 `share_nonces` (work % (M/1024) == 0). Soft attempt-pay copy retired from public surfaces.
- **Hashrate honesty** — public/dashboard prefer `pool_hashrate_kind=blake3_pow_ghs` (shares/wall).
- **Pool-direct TLS** — origin `:18443` (allowlist) alongside cleartext `:18083` for fleet. Prefer TLS when clients trust the origin cert.
- **News / notices / RSS** — `news.xml` generated from `news-display.json`; miner-notices + dashboard What's new on **rc18**.
- **Downloads** — NVIDIA CUDA / AMD OpenCL requirements + Blake3 GPU matrix table; Hunt = Linux + bubblewrap.
- **HackMe OS** — terminal/GRUB neon aligned to dashboard cyan `#00d1ff` + matrix green; status panel shows share-PPS.

## Security
- Private GHSA draft for wazero compile-time LEB OOM: `docs/security/WAZERO_GHSA_DRAFT.md` (no public bomb recipes).
- CF-only `:443` guard script exists but **not enabled** (geo/`real_ip` false-403; rolled back).

## Install
Verify `SHA256SUMS.txt` / `SHA256SUMS-iso.txt` before run.
