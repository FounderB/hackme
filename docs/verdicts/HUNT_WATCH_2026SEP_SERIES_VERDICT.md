# Hunt Watch 2026sep — series verdict

**Status:** CLOSED · 12/12 · public ledger  
**Engine:** Hunt Standard (ASAN+UBSan subprocess) — **not** libFuzzer  
**Public URL:** https://hackme.tech/reports/hunt-watch-2026sep/

## Numbers

| Metric | Value |
|--------|-------|
| Target runs | 50 |
| Iterations | 192,507,336 |
| CLEAN | 43 |
| CVE_CANDIDATE | 2 |
| INFORMATIONAL | 5 |
| Crash artifacts (series) | 1,437 |

## Interpretation

- Product-lane marathon for **Hunt**, separate from OSS CVE Watch (nghttp2 / libheif libFuzzer).
- **CLEAN** on mature parsers is a normal honest outcome.
- **CVE_CANDIDATE** / **INFORMATIONAL** = sanitizer class buckets for triage — not published CVE IDs.
- Hunt value = verified sanitizer audit + report, not exec/s vs libFuzzer.

## Artifacts

- Site: `web/site/reports/hunt-watch-2026sep/`
- Operator rollup: `reports/hunt-watch/2026sep/ROLLUP.{html,md,json}`
- Re-export: `SERIES=2026sep python3 scripts/ops/export_hunt_watch_rollup.py`
