#!/usr/bin/env bash
# Live Hunt jsmn smoke against public coordinator (no secret echo).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:${HOME}/go/bin:${PATH:-}"

ADMIN="$(tr -d '\r\n' < .secrets/hackme_coordinator_admin_token)"
COORD="${COORD:-https://hackme.tech/pool/coordinator}"

echo "[live-hunt] ensure jsmn harness"
go test -count=1 ./internal/hunt -run 'TestEnsureHarnessBinaryCached' -timeout 5m >/dev/null

HASH="$(python3 - <<'PY'
import hashlib, json
data = json.load(open("upstream/oss_cve_targets.json"))
t = next(x for x in data["targets"] if x["id"] == "jsmn")
parts = t["id"] + t["repo"] + t["ref"] + t["driver"]
parts += ",".join(t.get("upstream_src", [])) + ",".join(t.get("build_flags", []))
print(hashlib.sha256(parts.encode()).hexdigest()[:32])
PY
)"
BIN=".cache/hunt-harness/${HASH}.bin"
[[ -f "$BIN" ]] || { echo "missing $BIN" >&2; exit 1; }
echo "[live-hunt] hash=$HASH size=$(stat -c%s "$BIN")"

python3 - "$COORD" "$ADMIN" "$HASH" "$BIN" <<'PY'
import base64, json, sys, time, urllib.error, urllib.request
coord, admin, h, path = sys.argv[1:5]

def req(method, url, body=None):
    data = None if body is None else json.dumps(body).encode()
    r = urllib.request.Request(
        url, data=data, method=method,
        headers={
            "Content-Type": "application/json",
            "X-Hackme-Admin-Token": admin,
            "User-Agent": "Mozilla/5.0",
        },
    )
    with urllib.request.urlopen(r, timeout=180) as resp:
        return resp.status, json.loads(resp.read().decode() or "{}")

print("[live-hunt] publish harness…")
st, out = req("POST", coord + "/api/fuzz/pool/hunt/harness", {
    "harness_hash": h,
    "source_rel": "live-smoke:jsmn",
    "binary_b64": base64.b64encode(open(path, "rb").read()).decode(),
})
print("[live-hunt] publish", st, {k: out.get(k) for k in ("ok", "harness_hash", "error", "bytes") if k in out or True})

cid = f"hunt-live-jsmn-{int(time.time())}"
st, out = req("POST", coord + "/api/fuzz/pool/campaigns", {
    "id": cid,
    "campaign_type": "hunt",
    "title": "Live Hunt jsmn verify",
    "status": "running",
    "budget_runs": 16,
    "budget_seconds": 600,
    "config": {
        "pool_distributed": True,
        "work_kind": "hunt_shard",
        "campaign_type": "hunt",
        "hunt_package": "hunt_lite",
        "hunt_engine": "libfuzzer",
        "engine_lease_sec": 45,
        "upstream_target_id": "jsmn",
        "harness_hash": h,
        "check_semantics": "native_crash",
        "depth_tier": "oss_cve",
        "input_mode": "bytes",
        "iterations_per_shard": 2,
        "max_input_bytes": 256,
        "escrow_split": "50_50",
        "bounty_requires_native": True,
        "native_repro_mode": "oss_upstream",
    },
})
print("[live-hunt] campaign", st, out)
print("[live-hunt] CID", cid)

# Coverage Hunt leases are ~45–60s; requiring 4 ok shards in ~160s was a false FAIL.
# Pass bar: >=2 clean coverage shards within ~12 min.
min_ok = int(__import__("os").environ.get("LIVE_HUNT_MIN_OK", "2"))
best_ok = 0
for i in range(48):
    time.sleep(15)
    try:
        st, d = req("GET", f"{coord}/api/fuzz/pool/campaigns/progress?id={cid}")
    except Exception as e:
        print(f"[live-hunt] progress[{i}] err {e}")
        continue
    # Prefer runs_ok (result_ok=1). runs_done alone includes failed Hunt replays.
    if "runs_ok" not in d and "failed_checks" not in d:
        print(f"[live-hunt] progress[{i}] missing runs_ok/failed_checks — coordinator too old; refuse false pass")
        raise SystemExit(2)
    try:
        ok = int(d.get("runs_ok") if d.get("runs_ok") is not None else max(0, int(d.get("runs_done") or 0) - int(d.get("failed_checks") or 0)))
        failed = int(d.get("failed_checks") or 0)
    except Exception:
        ok, failed = 0, 0
    best_ok = max(best_ok, ok)
    print(f"[live-hunt] progress[{i}] runs_ok={ok} failed={failed} done={d.get('runs_done')} status={d.get('status')}")
    if ok >= min_ok:
        print(f"[live-hunt] PASS clean_shards>={min_ok}")
        raise SystemExit(0)

print(f"[live-hunt] FAIL best_runs_ok={best_ok} want>={min_ok}")
raise SystemExit(1)
PY
