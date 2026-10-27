# HackMe 0.1.0-rc17 — cutover channel

**Status:** HISTORICAL (superseded by rc18.x) · full channel **0.1.0-rc17.4** (runtime + installers + ISO) · [downloads](https://hackme.tech/downloads.html) · [GitHub](https://github.com/jokeez/hackme/releases/tag/0.1.0-rc17.4)

Related: [RC17_CUTOVER.md](../../internal/RC17_CUTOVER.md) · release notes [HACKME_0.1.0-rc17.4_RELEASE.md](../../releases/HACKME_0.1.0-rc17.4_RELEASE.md) · prior [HACKME_RC16.md](HACKME_RC16.md) · [BUG_BOUNTY.md](../../BUG_BOUNTY.md)

## What rc17.4 means

| Layer | State |
|-------|--------|
| Hub UI (`dashboard.html`) | `0.1.0-rc17.4` — Exchange · SUP · Hunt/Dig · Blake3 **LIVE** |
| `main.go` / `CURRENT_VERSION` / `app.js` `RELEASE_VER` | `0.1.0-rc17.4` |
| Public downloads (`PUBLISHED_ARTIFACT_VER`) | **0.1.0-rc17.4** Win/Linux/ISO + SHA256SUMS (`.deb` optional / may 404) |
| News / ticker | **Historical rc17.4** — Blake3 @ 485k activated · soft attempt-pay (superseded by rc18+ **share-PPS**) · ISO |
| Blake3 (2026-10-24) | Tip ≥ **#485000** — `blake3_v3` LIVE; pool GH/s = wall-rate (~few GH/s fleet class) |

## Ships since rc17 cut

- Paper Exchange desk (`exchange.hackme.tech` iframe)
- SUP public send/activity (nginx allowlist)
- Hunt ASAN + escrow pool hardening (Lite / Standard / Heavy · 50/50)
- Dig customer-first fleet (Scan / Audit smoke · 20/80 · Deep · 40/60)
- **rc17.1** hotpatch: worker lifecycle + claim-identity
- **rc17.2** bundle: Hunt deep-havoc v2.8, fuzz release/capability harden, workerpoh env honor, pathsafe/CodeQL
- **rc17.3**: Coverage Hunt engines auto-install on Linux miner install/update (`install_hunt_engines.sh`)
- **rc17.4** (historical): Blake3 PoH @ **#485000** (gate LIVE), soft attempt-pay default at that cut — **current channel is rc18.x share-PPS (`SHARE_STRICT`)**, settle SUP/HMC export fix, HackMe OS ISO

## Product paths (honest)

| Product | CLI | Escrow |
|---------|-----|--------|
| **Dig** | `hackme-fuzzing wizard --package scan\|audit\|deep` | smoke 20/80 · Deep 40/60 |
| **Hunt** | `hackme-fuzzing hunt … --package hunt_lite\|hunt_standard\|hunt_heavy` | 50/50 |

## PDFs / listing packs

Listing PDFs under `dist/docs/` — rebuild with `bash scripts/release/build_listing_pdfs.sh` for the active channel; verify `SHA256SUMS-docs.txt` on [docs.html](https://hackme.tech/docs.html).

## Operator note

Published channel **0.1.0-rc17.4** (rebuild/publish bundle, then verify SHA). Ops history: [RC17_CUTOVER.md](../../internal/RC17_CUTOVER.md).
