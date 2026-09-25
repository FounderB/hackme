# HackMe 0.1.0-rc17 — cutover channel

**Status:** LIVE · full channel **0.1.0-rc17.2** (runtime + installers + ISO) · [downloads](https://hackme.tech/downloads.html) · [GitHub](https://github.com/jokeez/hackme/releases/tag/0.1.0-rc17.2)

Related: [RC17_CUTOVER.md](RC17_CUTOVER.md) · release notes [HACKME_0.1.0-rc17.2_RELEASE.md](releases/HACKME_0.1.0-rc17.2_RELEASE.md) · prior [HACKME_RC16.md](HACKME_RC16.md) · [BUG_BOUNTY.md](BUG_BOUNTY.md)

## What rc17.2 means

| Layer | State |
|-------|--------|
| Hub UI (`dashboard.html`) | `0.1.0-rc17.2` — Exchange · SUP · Hunt/Dig · deep-havoc v2.8 |
| `main.go` / `CURRENT_VERSION` / `app.js` `RELEASE_VER` | `0.1.0-rc17.2` |
| Public downloads (`PUBLISHED_ARTIFACT_VER`) | **0.1.0-rc17.2** Win/Linux/deb/fuzz/ISO + SHA256SUMS |
| News / ticker | rc17.2 LIVE full bundle |

## Ships since rc17 cut

- Paper Exchange desk (`exchange.hackme.tech` iframe)
- SUP public send/activity (nginx allowlist)
- Hunt ASAN + escrow pool hardening (Lite / Standard / Heavy · 50/50)
- Dig customer-first fleet (Scan / Audit / Deep · 20/80)
- **rc17.1** hotpatch: worker lifecycle + claim-identity
- **rc17.2** bundle: Hunt deep-havoc v2.8, fuzz release/capability harden, workerpoh env honor, pathsafe/CodeQL

## Product paths (honest)

| Product | CLI | Escrow |
|---------|-----|--------|
| **Dig** | `hackme-fuzzing wizard --package scan\|audit\|deep` | 20/80 |
| **Hunt** | `hackme-fuzzing hunt … --package hunt_lite\|hunt_standard\|hunt_heavy` | 50/50 |

## PDFs / listing packs

Listing PDFs under `dist/docs/` were regenerated for **rc17** (2026-09-15). Site links use `?v=20260915-rc17` — verify `SHA256SUMS-docs.txt` on [docs.html](https://hackme.tech/docs.html).

## Operator note

Published artifacts and `latest.json` point at **0.1.0-rc17.2**. Ops history: [RC17_CUTOVER.md](RC17_CUTOVER.md).
