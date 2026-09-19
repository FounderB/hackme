#!/usr/bin/env bash
# Measure Hunt harness publish WAL / latency: JSON+b64 vs octet-stream+object store.
# Usage (on VPS or with NODE_SSH):
#   NODE_SSH=hackme-vps bash scripts/ops/measure_harness_objectstore.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COORD="${COORD:-http://127.0.0.1:18081}"
SIZE_MB="${SIZE_MB:-8}"
HASH="${HASH:-$(openssl rand -hex 16)}"

run_remote() {
  if [[ -n "${NODE_SSH:-}" ]]; then
    ssh -o BatchMode=yes "$NODE_SSH" "$@"
  else
    bash -lc "$*"
  fi
}

ADMIN="$(run_remote 'grep -m1 "^HACKME_COORDINATOR_ADMIN_TOKEN=" /opt/hackme/.env.coord 2>/dev/null | cut -d= -f2- | tr -d "\r\n"')"
if [[ -z "$ADMIN" ]]; then
  ADMIN="$(run_remote 'set -a; source /opt/hackme/.env.vps; set +a; echo -n "$HACKME_COORDINATOR_ADMIN_TOKEN"')"
fi
[[ -n "$ADMIN" ]] || { echo "missing admin token" >&2; exit 1; }

echo "=== measure harness publish SIZE_MB=$SIZE_MB HASH=$HASH ==="
run_remote "python3 - <<'PY'
import base64,json,os,subprocess,time,urllib.request
coord='${COORD}'
admin='''${ADMIN}'''
size=int(${SIZE_MB})*1024*1024
hash='${HASH}'
data=os.urandom(size)
fuzz_db='/opt/hackme/data/coordinator_fuzz.db'
wal=fuzz_db+'-wal'

def wal_size():
    try: return os.path.getsize(wal)
    except FileNotFoundError: return 0

def db_blob_mb():
    out=subprocess.check_output(['sqlite3',fuzz_db,\"SELECT COALESCE(SUM(LENGTH(binary_blob)),0)/1024.0/1024 FROM hunt_harness_artifacts\"],text=True)
    return float(out.strip() or 0)

def harness_files_mb():
    root='/opt/hackme/data/harness'
    total=0
    if os.path.isdir(root):
        for dp,_,fs in os.walk(root):
            for f in fs:
                total+=os.path.getsize(os.path.join(dp,f))
    return total/1024/1024

before_wal=wal_size(); before_blob=db_blob_mb(); before_files=harness_files_mb()
# octet-stream upload
req=urllib.request.Request(coord+'/api/fuzz/pool/hunt/harness', data=data, method='POST')
req.add_header('Content-Type','application/octet-stream')
req.add_header('X-Hackme-Harness-Hash', hash)
req.add_header('X-Hackme-Source-Rel','measure.bin')
req.add_header('X-Hackme-Admin-Token', admin)
t0=time.time()
try:
    with urllib.request.urlopen(req, timeout=120) as r:
        body=r.read().decode()
        code=r.status
except Exception as e:
    print('UPLOAD_FAIL', e); raise
dt=time.time()-t0
after_wal=wal_size(); after_blob=db_blob_mb(); after_files=harness_files_mb()
print(json.dumps({
  'ok': True,
  'http': code,
  'resp': body[:200],
  'bytes': size,
  'upload_sec': round(dt,3),
  'wal_before': before_wal,
  'wal_after': after_wal,
  'wal_delta': after_wal-before_wal,
  'blob_mb_before': round(before_blob,3),
  'blob_mb_after': round(after_blob,3),
  'files_mb_before': round(before_files,3),
  'files_mb_after': round(after_files,3),
  'files_delta_mb': round(after_files-before_files,3),
}, indent=2))
# GET + 304
import ssl
ctx=ssl.create_default_context()
get=urllib.request.Request(coord+'/api/fuzz/pool/hunt/harness/'+hash)
get.add_header('Authorization','Bearer '+admin)
t1=time.time()
with urllib.request.urlopen(get, timeout=60) as r:
    got=r.read(); etag=r.headers.get('ETag'); code1=r.status
t_get=time.time()-t1
get2=urllib.request.Request(coord+'/api/fuzz/pool/hunt/harness/'+hash)
get2.add_header('Authorization','Bearer '+admin)
get2.add_header('If-None-Match', etag or ('\"'+hash+'\"'))
try:
    with urllib.request.urlopen(get2, timeout=30) as r:
        code304=r.status
except urllib.error.HTTPError as e:
    code304=e.code
print(json.dumps({'get_sec': round(t_get,3), 'get_bytes': len(got), 'etag': etag, 'second_status': code304}, indent=2))
assert len(got)==size
assert code304==304
print('MEASURE_OK')
PY"
