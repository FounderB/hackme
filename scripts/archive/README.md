# Archived scripts

One-shot / spent operator helpers moved out of `scripts/ops/` so the live tree stays small.

| Path | What lived here |
|------|-----------------|
| [ops/](ops/) | Perf patches, Hunt Watch 12d, obscure OSS daemons, fleet soak, old nginx harness patch, spent bootstrap cycle helpers |
| [ops/bootstrap_customer/](ops/bootstrap_customer/) | One-off fluxtap / finalize / payout-track helpers (live bots stay under `scripts/ops/bootstrap_customer/`) |
| [ops/telegram/](ops/telegram/) | Preview / news publish helpers |

Do **not** wire these into systemd or CI without re-reading the header — most assume old paths or a finished ops window.
