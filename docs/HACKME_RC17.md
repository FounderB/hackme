# HackMe 0.1.0-rc17 — cutover channel

**Status:** LIVE · runtime/UI channel **0.1.0-rc17.1** (hotpatch) · installers still **0.1.0-rc17** · [downloads](https://hackme.tech/downloads.html) · [GitHub](https://github.com/jokeez/hackme/releases/tag/0.1.0-rc17)

Related: [RC17_CUTOVER.md](RC17_CUTOVER.md) · prior [HACKME_RC16.md](HACKME_RC16.md) · [downloads](https://hackme.tech/downloads.html) · [BUG_BOUNTY.md](BUG_BOUNTY.md)

## What rc17 means

| Layer | State |
|-------|--------|
| Hub UI (`dashboard.html`) | `0.1.0-rc17.1` — Exchange tab, SUP wallet, Hunt/Dig + worker lifecycle hotpatch |
| `main.go` / `CURRENT_VERSION` / `app.js` `RELEASE_VER` | `0.1.0-rc17.1` |
| Public downloads (`PUBLISHED_ARTIFACT_VER`) | **0.1.0-rc17** Win/Linux/deb/fuzz/ISO + SHA256SUMS (until rc17.1 bundle cut) |
| News / ticker | rc17 LIVE installers · rc17.1 hotpatch channel |

## Ships in the cut window

- Paper Exchange desk (`exchange.hackme.tech` iframe)
- SUP public send/activity (nginx allowlist)
- Hunt ASAN + escrow pool hardening (Lite / Standard / Heavy · 50/50)
- Dig customer-first fleet (Scan / Audit / Deep · 20/80)

## Product paths (honest)

| Product | CLI | Escrow |
|---------|-----|--------|
| **Dig** | `hackme-fuzzing wizard --package scan\|audit\|deep` | 20/80 |
| **Hunt** | `hackme-fuzzing hunt … --package hunt_lite\|hunt_standard\|hunt_heavy` | 50/50 |

## PDFs / listing packs

Listing PDFs under `dist/docs/` were regenerated for **rc17** (2026-09-15). Site links use `?v=20260915-rc17` — verify `SHA256SUMS-docs.txt` on [docs.html](https://hackme.tech/docs.html).

## Operator note

Cutover complete for published artifacts: `PUBLISHED_ARTIFACT_VER` is `0.1.0-rc17` and `latest.json` points at the rc17 bundle. Ops detail: [RC17_CUTOVER.md](RC17_CUTOVER.md).
