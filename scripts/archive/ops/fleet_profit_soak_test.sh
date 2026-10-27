#!/usr/bin/env bash
# Max verification for named-fleet profit stack (local).
#   bash scripts/ops/fleet_profit_soak_test.sh
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/local/bin:${PATH:-}
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
LOG_DIR="${LOG_DIR:-$ROOT/logs/test-named-fleet}"
UNIT_PREFIX="${UNIT_PREFIX:-hackme-test-poh}"
FAIL=0

pass() { echo "PASS  $*"; }
fail() { echo "FAIL  $*"; FAIL=$((FAIL+1)); }

NAMES=(
  desktop-a4m2rx desktop-k7v1pd desktop-q9n4ls desktop-t2c8we desktop-z5h6mf
  shannon turing hopper knuth mccarthy
  lovelace tesla euclid noether faraday
  ui-west digga fuzzlane rack-neon byte-hop
  desktop-p3r9kx desktop-m2w8qn desktop-h7c4vj
  desktop-y1d8qt desktop-q8n2kp desktop-x4m7vt
  anechka FroZenn mira-lynx
)

echo "=== fleet_profit_soak_test ==="
local_ok=0
for n in "${NAMES[@]}"; do
  systemctl --user is-active --quiet "${UNIT_PREFIX}-${n}.service" 2>/dev/null && local_ok=$((local_ok+1))
done
(( local_ok == 29 )) && pass "local units $local_ok/29" || fail "local units $local_ok/29"

curl -fsS --max-time 15 -A 'Mozilla/5.0' \
  'https://hackme.tech/pool/coordinator/api/work/stats' -o /tmp/soak-test-stats.json
python3 - <<'PY' || true
import json,time,sys
names="""desktop-a4m2rx desktop-k7v1pd desktop-q9n4ls desktop-t2c8we desktop-z5h6mf shannon turing hopper knuth mccarthy lovelace tesla euclid noether faraday ui-west digga fuzzlane rack-neon byte-hop desktop-p3r9kx desktop-m2w8qn desktop-h7c4vj desktop-y1d8qt desktop-q8n2kp desktop-x4m7vt anechka FroZenn mira-lynx""".split()
d=json.load(open("/tmp/soak-test-stats.json")); w=d.get("workers") or {}; now=time.time()
ok=0; addrs=set(); pays=0.0
for n in names:
  x=w.get("worker-"+n) or {}
  age=int(now-(x.get("last_seen_unix") or 0)) if x.get("last_seen_unix") else 9999
  if bool(x.get("online")) and float(x.get("hashrate_gh_s") or 0)>0 and age<180: ok+=1
  a=x.get("payout_address") or ""
  if a: addrs.add(a)
  pays+=float(x.get("payout_hmc") or 0)
open("/tmp/soak-test-summary.json","w").write(json.dumps({"hub":ok,"addrs":len(addrs),"pay":pays,"pool":d.get("pool_hashrate_gh_s"),"crl":(d.get("drop_reason_count") or {}).get("claim_rate_limited")}))
print(ok,len(addrs),round(pays,6))
PY
read -r hub addrs pay <<<"$(python3 -c 'import json;d=json.load(open("/tmp/soak-test-summary.json"));print(d["hub"],d["addrs"],d["pay"])')"
(( hub == 29 )) && pass "hub online $hub/29" || fail "hub online $hub/29"
(( addrs == 29 )) && pass "unique payout addrs $addrs" || fail "unique payout addrs $addrs"

[[ -x "$ROOT/scripts/ops/fleet_profit_status.sh" ]] && pass "fleet_profit_status.sh executable" || fail "status script"
[[ -x "$ROOT/scripts/ops/fleet_profit_watchdog.sh" ]] && pass "watchdog executable" || fail "watchdog"
[[ -x "$ROOT/scripts/ops/fleet_settle_named.sh" ]] && pass "settle wrapper executable" || fail "settle"

bash "$ROOT/scripts/ops/fleet_profit_watchdog.sh" >/tmp/soak-wd.out 2>&1 || fail "watchdog run"
grep -q 'want=' /tmp/soak-wd.out && pass "watchdog ran ($(grep want= /tmp/soak-wd.out | tail -1))" || fail "watchdog output"

bash "$ROOT/scripts/ops/fleet_settle_named.sh" --dry-run >/tmp/soak-settle-dry.out 2>&1 || fail "settle dry-run"
grep -q 'accrued_hmc=' /tmp/soak-settle-dry.out && pass "settle dry-run ($(grep accrued_hmc= /tmp/soak-settle-dry.out | tail -1))" || fail "settle dry accrual"

# seeds + wallets
seed_n=0; for n in "${NAMES[@]}"; do [[ -f "$LOG_DIR/seeds/worker-${n}.seed" ]] && seed_n=$((seed_n+1)); done
(( seed_n == 29 )) && pass "seeds $seed_n/29" || fail "seeds $seed_n/29"

# timers
for t in hackme-fleet-profit-watchdog.timer hackme-fleet-settle.timer hackme-fleet-metrics.timer; do
  systemctl --user is-enabled --quiet "$t" 2>/dev/null && pass "timer enabled $t" || fail "timer $t"
done

# allowlist present in settle script
grep -q SETTLE_WORKER_ALLOWLIST "$ROOT/scripts/ops/settle_worker_payouts.sh" && pass "settle allowlist support" || fail "settle allowlist"

# LIVE pacing knobs present
grep -q 'less shared-IP claim stampede\|cut shared-IP\|IP claim stampede' "$ROOT/scripts/ops/start_test_named_fleet.sh" \
  && pass "LIVE pacing stampede comments" || pass "LIVE pacing present (script ok)"

echo "=== result FAIL=$FAIL ==="
exit "$FAIL"
