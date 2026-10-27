# Hunt daily rotate (local lab / contributor)

Hourly ASAN Hunt on one OSS catalog target from `upstream/oss_cve_targets.json` rotation queue.
Queue lists **only targets with harness drivers on disk** (`tasks/sources/fuzz/oss/<driver>.{c,rs}`).
Aspirational catalog IDs without drivers live under `rotation.deferred_until_driver` until harnesses land.
`scripts/ops/hunt_daily_queue.py` also filters by driver presence at runtime (defense in depth).

Covers up to 24 libraries/day (queue wraps). Watch `reports/hunt-daily/YYYYMMDD/ROLLUP.md`.

Weekly honesty ledger (families/signatures across days, not raw crashes):

```bash
python3 scripts/ops/export_hunt_weekly_rollup.py
# → reports/hunt-weekly/YYYYMMDD-YYYYMMDD/WEEKLY.md
```

## Quick start

```bash
# preview today's schedule + this hour's target
DRY_RUN=1 bash scripts/ops/hunt_daily_rotate.sh

# runnable queue / missing drivers
python3 scripts/ops/hunt_daily_queue.py list
python3 scripts/ops/hunt_daily_queue.py missing

# run one slot now (~1h wall, or shorter)
SLOT_WALL_SEC=900 bash scripts/ops/hunt_daily_rotate.sh

# install systemd --user hourly timer
bash scripts/ops/install_hunt_daily_rotate_user.sh
```

## What to watch

| Signal | Meaning |
|--------|---------|
| `family_count` / signature union | Depth that matters (honesty 2.0) |
| `CLEAN` streak | Honest quiet outcomes |
| `INFORMATIONAL` + signatures | Recurring UBSan/ASAN families (not raw crash counts) |
| `SKIP` / `ERROR` | Missing driver or build/hunt failure — slot recorded, timer stays green |
| `exec_per_sec` drift | Engine / machine regression |

Slot script **exits 0** after writing meta/rollup for ordinary slot failures so systemd `--user` timers do not flap red. Only missing clang/go/python3 fails the unit hard.

## Layout

```
reports/hunt-daily/YYYYMMDD/
  schedule.json          # 24h plan (runnable queue only)
  rotate.log
  ROLLUP.md / ROLLUP.json
  HH00-<target>/
    hunt-local.json
    meta.json
    crashes/
```

`csonh` is skipped (disclosure hold). Single-flight via flock — won't stack soaks.
