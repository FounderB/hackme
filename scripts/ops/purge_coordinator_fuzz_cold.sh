#!/usr/bin/env bash
# Purge cold rows from coordinator_fuzz.db to stop SQLITE_IOERR_WRITE (522) under load.
# Safe to run while coordinator is up (batched DELETEs + busy_timeout). Optional
# offline VACUUM INTO compact swap when PURGE_VACUUM=1 (brief coordinator stop).
#
#   NODE_SSH=hackme-vps bash scripts/ops/purge_coordinator_fuzz_cold.sh
#   NODE_SSH=hackme-vps PURGE_VACUUM=1 bash scripts/ops/purge_coordinator_fuzz_cold.sh
#
# Env:
#   COORD_SQL_DB     default /opt/hackme/data/coordinator_fuzz.db
#   KEEP_DAYS        retain recent closed work (default 5)
#   BATCH            delete batch size (default 25000)
#   PURGE_VACUUM=1   stop coord → VACUUM INTO → atomic swap → start
#   DRY_RUN=1
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

NODE_SSH="${NODE_SSH:-}"
COORD_DB="${COORD_SQL_DB:-/opt/hackme/data/coordinator_fuzz.db}"
KEEP_DAYS="${KEEP_DAYS:-5}"
BATCH="${BATCH:-25000}"
DRY_RUN="${DRY_RUN:-0}"
PURGE_VACUUM="${PURGE_VACUUM:-0}"
COORD_SERVICE="${COORD_SERVICE:-hackme-coordinator}"

log() { echo "[purge-fuzz-cold $(date -u +%H:%M:%S)] $*"; }

remote() {
  if [[ -n "$NODE_SSH" ]]; then
    ssh -o BatchMode=yes -o ConnectTimeout=30 "$NODE_SSH" "$@"
  else
    bash -lc "$*"
  fi
}

cutoff_sql="strftime('%s','now') - (${KEEP_DAYS} * 86400)"

snapshot() {
  remote "python3 - <<'PY'
import os, sqlite3, json
db=os.environ.get('COORD_DB','$COORD_DB')
con=sqlite3.connect(f'file:{db}?mode=ro', uri=True, timeout=60)
c=con.cursor()
def n(q):
  try: return c.execute(q).fetchone()[0]
  except Exception as e: return str(e)
print(json.dumps({
  'db_bytes': os.path.getsize(db),
  'wal_bytes': os.path.getsize(db+'-wal') if os.path.exists(db+'-wal') else 0,
  'work_done': n(\"SELECT COUNT(*) FROM fuzz_work_items WHERE status IN ('done','cancelled')\"),
  'work_open': n(\"SELECT COUNT(*) FROM fuzz_work_items WHERE status IN ('pending','leased','replay_pending')\"),
  'native_cold': n(\"SELECT COUNT(*) FROM fuzz_native_queue WHERE status IN ('confirmed','rejected')\"),
  'native_pending': n(\"SELECT COUNT(*) FROM fuzz_native_queue WHERE status='pending'\"),
  'outbox_applied': n(\"SELECT COUNT(*) FROM fuzz_settle_outbox WHERE status='applied'\"),
  'outbox_pending': n(\"SELECT COUNT(*) FROM fuzz_settle_outbox WHERE status='pending'\"),
  'corpus': n('SELECT COUNT(*) FROM fuzz_corpus'),
  'findings': n('SELECT COUNT(*) FROM fuzz_findings'),
}, indent=2))
PY"
}

log "db=$COORD_DB keep_days=$KEEP_DAYS batch=$BATCH dry_run=$DRY_RUN vacuum=$PURGE_VACUUM"
log "before:"
snapshot || true

if [[ "$DRY_RUN" == "1" ]]; then
  log "DRY_RUN — exit"
  exit 0
fi

# Python batch deleter on the hub (avoids ssh sqlite3 quoting hell).
remote "COORD_DB='$COORD_DB' KEEP_DAYS='$KEEP_DAYS' BATCH='$BATCH' python3 - <<'PY'
import os, sqlite3, time
db=os.environ['COORD_DB']
keep=int(os.environ.get('KEEP_DAYS','5'))
batch=int(os.environ.get('BATCH','25000'))
cutoff=int(time.time()) - keep*86400
con=sqlite3.connect(db, timeout=180)
con.execute('PRAGMA busy_timeout=180000')
con.execute('PRAGMA journal_mode=WAL')
c=con.cursor()

def drain(label, sql_select_ids, sql_delete):
    total=0
    while True:
        ids=[r[0] for r in c.execute(sql_select_ids, (cutoff, batch)).fetchall()]
        if not ids:
            break
        c.executemany(sql_delete, [(i,) for i in ids])
        con.commit()
        total += len(ids)
        print(f'{label}: +{len(ids)} total={total}', flush=True)
        if len(ids) < batch:
            break
    print(f'{label}: done total={total}', flush=True)

# work items closed + old
drain(
  'work_items_cold',
  \"\"\"SELECT id FROM fuzz_work_items
     WHERE status IN ('done','cancelled')
       AND COALESCE(updated_at, created_at, 0) < ?
     LIMIT ?\"\"\",
  'DELETE FROM fuzz_work_items WHERE id=?',
)

# native queue confirmed/rejected (keep pending for replay)
# table may use created_at or id ordering — prefer created_at when present
cols={r[1] for r in c.execute('PRAGMA table_info(fuzz_native_queue)')}
ts='created_at' if 'created_at' in cols else 'id'
if ts=='created_at':
    drain(
      'native_queue_cold',
      f\"\"\"SELECT id FROM fuzz_native_queue
         WHERE status IN ('confirmed','rejected') AND COALESCE({ts},0) < ?
         LIMIT ?\"\"\",
      'DELETE FROM fuzz_native_queue WHERE id=?',
    )
else:
    # no timestamp: keep newest N pending handled elsewhere; delete confirmed in id batches under soft cap
    total=0
    while True:
        ids=[r[0] for r in c.execute(
            \"SELECT id FROM fuzz_native_queue WHERE status IN ('confirmed','rejected') ORDER BY id LIMIT ?\",
            (batch,)).fetchall()]
        if not ids: break
        # stop if fewer than ~50k remain cold? we purge all confirmed/rejected
        c.executemany('DELETE FROM fuzz_native_queue WHERE id=?', [(i,) for i in ids])
        con.commit()
        total += len(ids)
        print(f'native_queue_cold: +{len(ids)} total={total}', flush=True)
        if len(ids) < batch: break
    print(f'native_queue_cold: done total={total}', flush=True)

# settle outbox applied
ob_cols={r[1] for r in c.execute('PRAGMA table_info(fuzz_settle_outbox)')}
ob_ts='applied_at' if 'applied_at' in ob_cols else ('updated_at' if 'updated_at' in ob_cols else 'created_at')
if ob_ts in ob_cols:
    drain(
      'settle_outbox_applied',
      f\"\"\"SELECT id FROM fuzz_settle_outbox
         WHERE status='applied' AND COALESCE({ob_ts},0) < ?
         LIMIT ?\"\"\",
      'DELETE FROM fuzz_settle_outbox WHERE id=?',
    )

# corpus rows for completed/cancelled campaigns (composite PK)
total=0
while True:
    rows=c.execute(
        \"\"\"SELECT campaign_id, input_sha256 FROM fuzz_corpus
           WHERE campaign_id IN (SELECT id FROM fuzz_campaigns WHERE status IN ('completed','cancelled','paused'))
             AND COALESCE(last_seen_at, first_seen_at, 0) < ?
           LIMIT ?\"\"\",
        (cutoff, batch),
    ).fetchall()
    if not rows:
        break
    c.executemany('DELETE FROM fuzz_corpus WHERE campaign_id=? AND input_sha256=?', rows)
    con.commit()
    total += len(rows)
    print(f'corpus_closed_campaigns: +{len(rows)} total={total}', flush=True)
    if len(rows) < batch:
        break
print(f'corpus_closed_campaigns: done total={total}', flush=True)

# checkpoint so WAL shrinks after mass deletes
try:
    c.execute('PRAGMA wal_checkpoint(TRUNCATE)')
    print('wal_checkpoint: ok', flush=True)
except Exception as e:
    print(f'wal_checkpoint: {e}', flush=True)
con.close()
print('purge batches complete', flush=True)
PY"

log "after delete:"
snapshot || true

if [[ "$PURGE_VACUUM" == "1" ]]; then
  log "VACUUM INTO compact swap (coordinator stop)"
  remote "set -euo pipefail
COORD_DB='$COORD_DB'
COORD_SERVICE='$COORD_SERVICE'
NEW=\${COORD_DB}.compact.\$\$
BACKUP=\${COORD_DB}.pre_vacuum.\$(date -u +%Y%m%dT%H%M%SZ)
systemctl stop \"\$COORD_SERVICE\"
sleep 2
# ensure no lockers
if fuser \"\$COORD_DB\" >/dev/null 2>&1; then
  echo \"db still busy\"; fuser -v \"\$COORD_DB\" || true
  systemctl start \"\$COORD_SERVICE\"
  exit 1
fi
sqlite3 \"\$COORD_DB\" \"PRAGMA busy_timeout=600000; PRAGMA wal_checkpoint(TRUNCATE);\"
sqlite3 \"\$COORD_DB\" \"VACUUM INTO '\$NEW';\"
ls -lah \"\$COORD_DB\" \"\$NEW\"
cp -a \"\$COORD_DB\" \"\$BACKUP\"
cp -a \"\${COORD_DB}-wal\" \"\${BACKUP}-wal\" 2>/dev/null || true
mv -f \"\$NEW\" \"\$COORD_DB\"
rm -f \"\${COORD_DB}-wal\" \"\${COORD_DB}-shm\"
chown hackme:hackme \"\$COORD_DB\" 2>/dev/null || true
chmod 644 \"\$COORD_DB\" 2>/dev/null || true
systemctl start \"\$COORD_SERVICE\"
sleep 8
systemctl is-active \"\$COORD_SERVICE\"
curl -fsS --max-time 20 http://127.0.0.1:18081/api/fuzz/pool/stats >/dev/null && echo fuzz_stats_ok || echo fuzz_stats_warming
ls -lah \"\$COORD_DB\" \"\$BACKUP\"
echo VACUUM_SWAP_OK backup=\$BACKUP
"
fi

log "done"
