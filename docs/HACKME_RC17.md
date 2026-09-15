# HackMe 0.1.0-rc17 — cutover channel

**Status:** LIVE · artifacts **0.1.0-rc17** published · [downloads](https://hackme.tech/downloads.html) · [GitHub](https://github.com/jokeez/hackme/releases/tag/0.1.0-rc17)

Related: [RC17_CUTOVER.md](RC17_CUTOVER.md) · prior [HACKME_RC16.md](HACKME_RC16.md) · [downloads](https://hackme.tech/downloads.html)

## What rc17 means

| Layer | State |
|-------|--------|
| Hub UI (`dashboard.html`) | `0.1.0-rc17` — Exchange tab, SUP wallet, Hunt/Dig copy |
| `main.go` / `CURRENT_VERSION` | `0.1.0-rc17` |
| Public downloads | **0.1.0-rc17** Win/Linux/deb/fuzz/ISO + SHA256SUMS |
| News / ticker | Honest cutover note — **no fake LIVE ISO SHA** |

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

Files under `dist/docs/` (`HMC_Listing_Pack.pdf`, `SUP_Companion_Overview.pdf`, `HackMe_Network_Pitch.pdf`, …) were last exported **Aug 2026** and may still say rc16 LIVE. Links on the site resolve; **regenerate after cutover publish** — see `DOCUMENTATION_EXPORT.md`.

## Operator note

Do **not** restart production hub early. Follow [RC17_CUTOVER.md](RC17_CUTOVER.md). After bundle publish, flip `PUBLISHED_ARTIFACT_VER` in `web/site/assets/app.js` to `0.1.0-rc17` and publish `latest.json`.
