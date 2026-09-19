#!/usr/bin/env bash
# Archive Dig detector noise (security_violation) out of coordinator_fuzz.db,
# keep crashes / consensus / a small recent window, compress, DELETE, VACUUM.
#
# Remote:
#   NODE_SSH=hackme-vps bash scripts/ops/archive_fuzz_findings.sh
#
# Env:
#   KEEP_SECURITY_PER_CAMPAIGN  newest security_violation rows to keep (default 50)
#   COORD_SQL_DB                path to fuzz sqlite (default /opt/hackme/data/coordinator_fuzz.db)
#   ARCHIVE_DIR                 output dir (default /opt/hackme/data/archives)
#   SKIP_VACUUM=1               export+delete only
#   DRY_RUN=1                   print counts, do not write/delete
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

COORD_DB="${COORD_SQL_DB:-/opt/hackme/data/coordinator_fuzz.db}"
ARCHIVE_DIR="${ARCHIVE_DIR:-/opt/hackme/data/archives}"
KEEP_SECURITY_PER_CAMPAIGN="${KEEP_SECURITY_PER_CAMPAIGN:-50}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DRY_RUN="${DRY_RUN:-0}"
SKIP_VACUUM="${SKIP_VACUUM:-0}"

log() { echo "[archive-fuzz-findings] $*"; }

run_remote() {
  if [[ -n "${NODE_SSH:-}" ]]; then
    ssh -o BatchMode=yes "$NODE_SSH" "$@"
  else
    bash -lc "$*"
  fi
}

run_sql() {
  local sql="$1"
  if [[ -n "${NODE_SSH:-}" ]]; then
    ssh -o BatchMode=yes "$NODE_SSH" "sqlite3 -cmd '.timeout 120000' \"${COORD_DB}\" \"${sql}\""
  else
    sqlite3 -cmd '.timeout 120000' "$COORD_DB" "$sql"
  fi
}

log "db=${COORD_DB} keep_security_per_campaign=${KEEP_SECURITY_PER_CAMPAIGN} dry_run=${DRY_RUN}"

BEFORE="$(run_sql "SELECT finding_type, COUNT(*) FROM fuzz_findings GROUP BY 1 ORDER BY 2 DESC; SELECT 'TOTAL', COUNT(*) FROM fuzz_findings; SELECT 'DB_BYTES', page_count*page_size FROM pragma_page_count(), pragma_page_size();")"
log "before:"
echo "$BEFORE" | sed 's/^/  /'

ARCHIVE_CANDIDATES="$(run_sql "
SELECT COUNT(*) FROM fuzz_findings f
WHERE f.finding_type='security_violation'
  AND f.id NOT IN (
    SELECT id FROM (
      SELECT id,
             ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY created_at DESC, id DESC) AS rn
      FROM fuzz_findings
      WHERE finding_type='security_violation'
    ) ranked
    WHERE rn <= ${KEEP_SECURITY_PER_CAMPAIGN}
  );
")"
log "security_violation rows to archive: ${ARCHIVE_CANDIDATES}"

if [[ "${ARCHIVE_CANDIDATES}" == "0" ]]; then
  log "nothing to archive"
  exit 0
fi

if [[ "${DRY_RUN}" == "1" ]]; then
  log "DRY_RUN=1 — stopping before export/delete"
  exit 0
fi

run_remote "mkdir -p '${ARCHIVE_DIR}' && chown hackme:hackme '${ARCHIVE_DIR}' 2>/dev/null || true"

ARCHIVE_JSONL="${ARCHIVE_DIR}/fuzz_findings_security_violation_${STAMP}.jsonl"
ARCHIVE_ZST="${ARCHIVE_JSONL}.zst"
MANIFEST="${ARCHIVE_DIR}/fuzz_findings_security_violation_${STAMP}.manifest.txt"

log "export → ${ARCHIVE_ZST}"
# Export via remote shell so we stream compress on-box (no scp of 1GB+ JSONL).
run_remote "set -euo pipefail
sqlite3 -cmd '.timeout 120000' -cmd '.mode list' '${COORD_DB}' \"
SELECT json_object(
  'id', id,
  'campaign_id', campaign_id,
  'finding_type', finding_type,
  'severity', severity,
  'title', title,
  'input_sha256', input_sha256,
  'artifact_path', artifact_path,
  'repro_cmd', repro_cmd,
  'detail_json', detail_json,
  'created_at', created_at
)
FROM fuzz_findings f
WHERE f.finding_type='security_violation'
  AND f.id NOT IN (
    SELECT id FROM (
      SELECT id,
             ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY created_at DESC, id DESC) AS rn
      FROM fuzz_findings
      WHERE finding_type='security_violation'
    ) ranked
    WHERE rn <= ${KEEP_SECURITY_PER_CAMPAIGN}
  );
\" | zstd -T0 -19 -o '${ARCHIVE_ZST}'
ls -lh '${ARCHIVE_ZST}'
"

EXPORTED="$(run_remote "zstd -dc '${ARCHIVE_ZST}' | wc -l")"
log "exported_lines=${EXPORTED} (expect ~${ARCHIVE_CANDIDATES})"

run_remote "cat > '${MANIFEST}' <<EOF
stamp=${STAMP}
db=${COORD_DB}
keep_security_per_campaign=${KEEP_SECURITY_PER_CAMPAIGN}
archive_candidates=${ARCHIVE_CANDIDATES}
exported_lines=${EXPORTED}
archive=${ARCHIVE_ZST}
keep_types=crash,consensus_script_push,+newest ${KEEP_SECURITY_PER_CAMPAIGN} security_violation per campaign
EOF
chown hackme:hackme '${ARCHIVE_ZST}' '${MANIFEST}' 2>/dev/null || true
cat '${MANIFEST}'
"

if [[ "${EXPORTED}" -lt 1 ]]; then
  log "export empty — abort delete" >&2
  exit 1
fi

# Safety: refuse delete if export line count is wildly below candidate count.
MIN_OK=$(( ARCHIVE_CANDIDATES * 95 / 100 ))
if [[ "${EXPORTED}" -lt "${MIN_OK}" ]]; then
  log "export ${EXPORTED} << candidates ${ARCHIVE_CANDIDATES} — abort delete" >&2
  exit 1
fi

log "DELETE archived security_violation rows"
DELETED="$(run_sql "
DELETE FROM fuzz_findings
WHERE finding_type='security_violation'
  AND id NOT IN (
    SELECT id FROM (
      SELECT id,
             ROW_NUMBER() OVER (PARTITION BY campaign_id ORDER BY created_at DESC, id DESC) AS rn
      FROM fuzz_findings
      WHERE finding_type='security_violation'
    ) ranked
    WHERE rn <= ${KEEP_SECURITY_PER_CAMPAIGN}
  );
SELECT changes();
")"
log "deleted=${DELETED}"

AFTER_DEL="$(run_sql "SELECT finding_type, COUNT(*) FROM fuzz_findings GROUP BY 1 ORDER BY 2 DESC; SELECT 'TOTAL', COUNT(*) FROM fuzz_findings;")"
log "after delete (pre-vacuum):"
echo "$AFTER_DEL" | sed 's/^/  /'

if [[ "${SKIP_VACUUM}" == "1" ]]; then
  log "SKIP_VACUUM=1 — done"
  exit 0
fi

log "stop coordinator → VACUUM → start (exclusive shrink)"
# Note: COMPACT path is expanded locally; remote uses unquoted $COMPACT (set on remote).
run_remote "set -euo pipefail
systemctl stop hackme-coordinator
COMPACT='${COORD_DB}.compact.${STAMP}'
sqlite3 -cmd '.timeout 120000' '${COORD_DB}' \"VACUUM INTO \\\"\$COMPACT\\\";\"
chown --reference='${COORD_DB}' \"\$COMPACT\"
chmod --reference='${COORD_DB}' \"\$COMPACT\"
mv -f '${COORD_DB}' '${COORD_DB}.pre_vacuum_${STAMP}'
mv -f \"\$COMPACT\" '${COORD_DB}'
systemctl start hackme-coordinator
swapoff -a 2>/dev/null || true
swapon -a 2>/dev/null || true
sleep 2
systemctl is-active hackme-coordinator
ls -lh '${COORD_DB}' '${COORD_DB}.pre_vacuum_${STAMP}' '${ARCHIVE_ZST}'
df -h /opt/hackme/data
free -h
"

AFTER="$(run_sql "SELECT finding_type, COUNT(*) FROM fuzz_findings GROUP BY 1 ORDER BY 2 DESC; SELECT 'TOTAL', COUNT(*) FROM fuzz_findings; SELECT 'DB_BYTES', page_count*page_size FROM pragma_page_count(), pragma_page_size();")"
log "after vacuum:"
echo "$AFTER" | sed 's/^/  /'
log "done"
)
