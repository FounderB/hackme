# HackMe 0.1.0-rc18

## Highlights
- **Share-PPS LIVE** — `SHARE_STRICT` pays verified Blake3 `share_nonces` (work % (M/1024) == 0). Soft attempt-pay copy retired from public surfaces.
- **Hashrate honesty** — public/dashboard prefer `pool_hashrate_kind=blake3_pow_ghs` (shares/wall).
- **OpenCL / GPU workers fresh** — `make_release_bundle.sh` fail-closes on OpenCL: `rm -f` stale bins, no `|| true` on linux OpenCL build, `assert_worker_fresh` requires `-hackme-wasm-validate`, `share_scan_max`, and matching `vcs.revision`.
- **Pool-direct** — **use cleartext `http://132.243.112.100:18083` today.** IP HTTPS `:18443` has no IP SAN (stock Go clients reject). Target: DNS-only `pool-direct.hackme.tech` + Let's Encrypt.
- **News / notices / RSS** — `news.xml` from `news-display.json`; miner-notices + dashboard What's new on **rc18**.
- **Downloads** — NVIDIA CUDA / AMD OpenCL + Blake3 GPU matrix; Hunt = Linux + bubblewrap.
- **HackMe OS** — terminal/GRUB neon aligned to dashboard cyan `#00d1ff` + matrix green.

## Known pre-1.0
- **Self-update is sha256-only** (not signed). Verify `SHA256SUMS` / release assets before install.

## Security
- Wazero compile-time LEB OOM advisory draft lives **private** (`hackme-private/security/`); public tree has `docs/security/README.md` stub only. Embargo clock already running (mitigations + bomb tests public under `internal/sandbox/*bomb*_test.go`).
- CF-only `:443` guard script exists but **not enabled** (geo/`real_ip` false-403; rolled back).

## Install
Verify `SHA256SUMS.txt` / `SHA256SUMS-iso.txt` before run.
