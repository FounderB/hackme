#!/usr/bin/env bash
# Hunt Watch honesty 2.0 gate — public ledger cites families, not raw crashes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
SERIES="${SERIES:-2026sep}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export SERIES
export WATCH_BASE="$ROOT/reports/hunt-watch/$SERIES"
export SITE_OUT="$TMP/site"
export ROLLUP_HTML="$TMP/ROLLUP.html"
export OUT_MD="$TMP/ROLLUP.md"
export OUT_JSON="$TMP/ROLLUP.json"

if [[ ! -d "$WATCH_BASE" ]]; then
  echo "[FAIL] missing watch base $WATCH_BASE" >&2
  exit 1
fi

python3 "$ROOT/scripts/ops/export_hunt_watch_rollup.py"

python3 - "$TMP" "$SERIES" <<'PY'
import json, sys, re
from pathlib import Path
tmp, series = Path(sys.argv[1]), sys.argv[2]
rollup = json.loads((tmp / "ROLLUP.json").read_text())
html = (tmp / "site" / "index.html").read_text()
site_json = json.loads((tmp / "site" / "rollup.json").read_text())

def fail(msg):
    print("[FAIL]", msg, file=sys.stderr)
    sys.exit(1)

if rollup.get("honesty_version") != "2.0":
    fail(f"honesty_version={rollup.get('honesty_version')}")
if "finding_families" not in rollup or "corpus_health" not in rollup:
    fail("missing series finding_families / corpus_health")
fam = int(rollup.get("total_finding_families") or 0)
arts = int(rollup.get("total_crash_artifacts") or rollup.get("total_crashes") or 0)
if fam <= 0:
    fail(f"total_finding_families={fam}")
if arts > 0 and fam >= arts:
    # Collapse should exist for this series (libucl-style).
    fail(f"expected family_count << artifacts; fam={fam} arts={arts}")
note = str(rollup.get("honesty_note") or "")
if "family" not in note.lower():
    fail("honesty_note missing family guidance")

# Per-row stamps
rows = rollup.get("rows") or []
if not rows:
    fail("empty rows")
libucl = [r for r in rows if str(r.get("target") or "") == "libucl"]
if libucl:
    r = max(libucl, key=lambda x: int(x.get("crashes") or 0))
    fc = int((r.get("finding_families") or {}).get("family_count") or r.get("family_count") or 0)
    cr = int(r.get("crashes") or 0)
    if cr >= 50 and fc > 10:
        fail(f"libucl honesty: crashes={cr} families={fc} (expected collapse)")
    if "finding_families" not in r or "corpus_health" not in r:
        fail("libucl row missing finding_families/corpus_health")

# HTML: families primary in hero stats; crash artifacts not sole primary label
if "Finding families" not in html:
    fail("HTML missing Finding families primary stat")
if "honesty 2.0" not in html.lower() and "Honesty 2.0" not in html:
    fail("HTML missing Honesty 2.0 policy")
# Must not use old sole primary 'Crash artifacts' as the only crash-related hero
# (variant inputs secondary is OK)
if re.search(r"<b>Crash artifacts</b>", html):
    fail("HTML still has primary Crash artifacts hero (use Finding families)")
if "cite this" not in html.lower() and "cite this" not in html:
    # hint text
    if "not raw crashes" not in html.lower():
        fail("HTML missing cite-families guidance")
# Signals cite families
if "finding family" not in html.lower() and "finding families" not in html.lower():
    fail("HTML signals missing family language")

# Site JSON mirrors honesty fields
if site_json.get("honesty_version") != "2.0":
    fail("site rollup.json honesty_version")
if int(site_json.get("total_finding_families") or 0) != fam:
    fail("site/operator family mismatch")

print(f"[PASS] hunt_watch_honesty_gate series={series} families={fam} artifacts={arts} rows={len(rows)}")
PY

# Existing family renderer still green
go test . -run 'FindingFamily|CollapseCrash|RenderFuzzFamily' -count=1
go test ./internal/fuzzengine/ -run 'TestFindingFamily' -count=1
