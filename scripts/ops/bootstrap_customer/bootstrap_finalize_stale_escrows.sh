#!/usr/bin/env bash
# Close stale open fuzz escrows on bootstrap customer node (free local mirror HMC).
# Marks aged still-open campaigns cancelled, then POST /api/fuzz/escrow/cleanup-stale.
#
#   bash /opt/hackme-bootstrap/scripts/bootstrap_customer/bootstrap_finalize_stale_escrows.sh
#
# Env: MIN_AGE_SEC=86400 LIMIT=120 DRY_RUN=1
set -euo pipefail
INSTALL="${BOOTSTRAP_INSTALL:-/opt/hackme-bootstrap}"
BASE="${BASE:-http://127.0.0.1:8080}"
MIN_AGE_SEC="${MIN_AGE_SEC:-86400}"
LIMIT="${LIMIT:-120}"
DRY_RUN="${DRY_RUN:-0}"
DB="${INSTALL}/data/hackme.db"
ADMIN="$(grep -m1 '^HACKME_ADMIN_TOKEN=' "$INSTALL/.env" | cut -d= -f2- | tr -d '\r\n')"
[[ -n "$ADMIN" ]] || { echo "missing HACKME_ADMIN_TOKEN" >&2; exit 2; }
[[ -f "$DB" ]] || { echo "missing $DB" >&2; exit 2; }

export INSTALL BASE MIN_AGE_SEC LIMIT DRY_RUN DB ADMIN
python3 - <<'PY'
import json, os, sqlite3, time, urllib.request
db=os.environ["DB"]
base=os.environ["BASE"].rstrip("/")
admin=os.environ["ADMIN"]
min_age=int(os.environ["MIN_AGE_SEC"])
limit=int(os.environ["LIMIT"])
dry=os.environ.get("DRY_RUN","0")=="1"
now=int(time.time())
cutoff=now-min_age
con=sqlite3.connect(db, timeout=120)
con.execute("PRAGMA busy_timeout=120000")
rows=con.execute(
    """SELECT e.campaign_id, e.created_at, e.budget_units, COALESCE(c.status,'')
       FROM fuzz_campaign_escrow e
       LEFT JOIN fuzz_campaigns c ON c.id=e.campaign_id
       WHERE e.status='open' AND e.created_at < ?
       ORDER BY e.created_at ASC LIMIT ?""",
    (cutoff, limit),
).fetchall()
print(f"stale_open={len(rows)} cutoff_age_sec={min_age}")
marked=0
for cid, created, budget, cstatus in rows:
    age=now-int(created or 0)
    print(f"  {cid} camp={cstatus or '?'} age_h={age/3600:.1f} budget_hmc={(budget or 0)/1e8:.4f}")
    if dry:
        continue
    if (cstatus or "").lower() not in ("cancelled", "completed"):
        con.execute(
            "UPDATE fuzz_campaigns SET status='cancelled', completed_at=? WHERE id=?",
            (now, cid),
        )
        marked += 1
con.commit()
print(f"marked_cancelled={marked}")
if dry:
    raise SystemExit(0)
req=urllib.request.Request(
    f"{base}/api/fuzz/escrow/cleanup-stale",
    data=b"{}",
    headers={"Content-Type":"application/json","X-Hackme-Admin-Token":admin},
    method="POST",
)
with urllib.request.urlopen(req, timeout=180) as resp:
    body=resp.read().decode()
print("cleanup-stale:", body[:500])
# wallet snapshot
try:
    with urllib.request.urlopen(urllib.request.Request(f"{base}/api/wallet", headers={"X-Hackme-Admin-Token":admin}), timeout=30) as r:
        w=json.loads(r.read().decode())
    print("spendable_hmc", w.get("balance_orders_spendable_hmc"), "onchain", w.get("balance_on_chain_hmc"))
except Exception as e:
    print("wallet_snap", e)
open_left=con.execute("SELECT COUNT(*), COALESCE(SUM(budget_units),0) FROM fuzz_campaign_escrow WHERE status='open'").fetchone()
print(f"open_left={open_left[0]} locked_hmc={(open_left[1] or 0)/1e8:.2f}")
PY
