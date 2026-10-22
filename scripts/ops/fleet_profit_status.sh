#!/usr/bin/env bash
# Named-fleet profit + popularity snapshot (local only).
#   bash scripts/ops/fleet_profit_status.sh
#   WATCH=1 bash scripts/ops/fleet_profit_status.sh   # append jsonl
set -euo pipefail
export PATH=/usr/bin:/bin:/usr/local/bin:${PATH:-}
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

LOG_DIR="${LOG_DIR:-$ROOT/logs/test-named-fleet}"
SEED_DIR="${SEED_DIR:-$LOG_DIR/seeds}"
WALLET_DIR="${WALLET_DIR:-$LOG_DIR/wallets}"
METRICS_LOG="${METRICS_LOG:-$LOG_DIR/profit-metrics.jsonl}"
STATE_FILE="${STATE_FILE:-$LOG_DIR/profit-metrics-state.json}"
COORD_STATS_URL="${COORD_STATS_URL:-https://hackme.tech/pool/coordinator/api/work/stats}"
CHAIN_BASE="${CHAIN_BASE:-}"
FAUCET_SSH="${FAUCET_SSH:-hackme-vps}"
UNIT_PREFIX="${UNIT_PREFIX:-hackme-test-poh}"
WATCH="${WATCH:-0}"
MINERSIGN_BIN="${MINERSIGN_BIN:-$ROOT/bin/minersign}"

NAMES=(
  desktop-a4m2rx desktop-k7v1pd desktop-q9n4ls desktop-t2c8we desktop-z5h6mf
  shannon turing hopper knuth mccarthy
  lovelace tesla euclid noether faraday
  ui-west digga fuzzlane rack-neon byte-hop
  desktop-p3r9kx desktop-m2w8qn desktop-h7c4vj
  desktop-y1d8qt desktop-q8n2kp desktop-x4m7vt
  anechka FroZenn mira-lynx
)

mkdir -p "$LOG_DIR" "$WALLET_DIR"

addr_of_seed() {
  local seed="$1"
  printf '{}' | HACKME_MINER_ED25519_SEED_HEX="$seed" "$MINERSIGN_BIN" --nonce-file "/tmp/fps-ms-$$" \
    | python3 -c 'import sys,json; print(json.load(sys.stdin).get("miner_address",""))'
}

sync_wallet() {
  local wid="$1" seed
  local sf="$SEED_DIR/${wid}.seed"
  [[ -f "$sf" ]] || return 1
  seed="$(tr -d '\r\n' <"$sf")"
  [[ ${#seed} -eq 64 ]] || return 1
  local dest="$WALLET_DIR/$wid"
  mkdir -p "$dest"
  printf '%s' "$seed" >"$dest/node_ed25519.seed"
  chmod 600 "$dest/node_ed25519.seed"
  local addr
  addr="$(addr_of_seed "$seed")"
  printf '%s' "$addr" >"$dest/address.txt"
  echo "$addr"
}

balance_hmc() {
  local addr="$1"
  if [[ -n "${FAUCET_SSH:-}" ]]; then
    ssh -o BatchMode=yes -o ConnectTimeout=10 "$FAUCET_SSH" \
      "curl -fsS --max-time 6 http://127.0.0.1:18080/api/address/$addr" \
      | python3 -c 'import sys,json; print(float(json.load(sys.stdin).get("balance_hmc") or 0))' 2>/dev/null || echo 0
    return
  fi
  local base="${CHAIN_BASE:-https://hackme.tech}"
  curl -fsS --max-time 8 "$base/api/address/$addr" \
    | python3 -c 'import sys,json; print(float(json.load(sys.stdin).get("balance_hmc") or 0))' 2>/dev/null || echo 0
}

local_online=0
for name in "${NAMES[@]}"; do
  if systemctl --user is-active --quiet "${UNIT_PREFIX}-${name}.service" 2>/dev/null; then
    local_online=$((local_online + 1))
  fi
done

curl -fsS --max-time 15 -A 'Mozilla/5.0' "$COORD_STATS_URL" -o /tmp/fleet-profit-stats.json

python3 - <<'PY' "$STATE_FILE" "$METRICS_LOG" "$WATCH" "$local_online" "${NAMES[@]}"
import json,sys,time
state_path, metrics_path, watch, local_online = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
names=sys.argv[5:]
d=json.load(open("/tmp/fleet-profit-stats.json"))
w=d.get("workers") or {}
now=time.time()
rows=[]
hub_online=0
total_pay=0.0
total_att=0
for n in names:
  wid="worker-"+n
  x=w.get(wid) or {}
  ghs=float(x.get("hashrate_gh_s") or 0)
  att=int(x.get("accepted_attempts") or x.get("accepted_att") or 0)
  pay=float(x.get("payout_hmc") or 0)
  ls=x.get("last_seen_unix") or 0
  age=int(now-ls) if ls else None
  online=bool(x.get("online")) and ghs>0 and (age is None or age<180)
  if online: hub_online+=1
  total_pay+=pay; total_att+=att
  rows.append({"id":wid,"online":online,"ghs":round(ghs,2),"att":att,"pay":round(pay,8),"age":age,"addr":x.get("payout_address")})

try: prev=json.load(open(state_path))
except Exception: prev={"ts":now,"total_pay":total_pay,"total_att":total_att}
dt=max(1.0, now-float(prev.get("ts") or now))
d_pay=total_pay-float(prev.get("total_pay") or total_pay)
d_att=total_att-int(prev.get("total_att") or total_att)
hmc_per_h=(d_pay/dt)*3600.0
att_per_min=(d_att/dt)*60.0
drops=d.get("drop_reason_count") or {}
out={
  "ts": now,
  "pool_ghs": round(float(d.get("pool_hashrate_gh_s") or 0),2),
  "hub_workers_online": d.get("workers_online"),
  "fleet_local_online": local_online,
  "fleet_hub_online": hub_online,
  "fleet_n": len(names),
  "fleet_payout_hmc": round(total_pay,6),
  "fleet_att": total_att,
  "hmc_per_h": round(hmc_per_h,6),
  "att_per_min": int(att_per_min),
  "no_fuzz_work": drops.get("no_fuzz_work"),
  "claim_rate_limited": drops.get("claim_rate_limited"),
  "ghs_priority_defer": drops.get("ghs_priority_defer"),
  "workers": rows,
}
json.dump({"ts":now,"total_pay":total_pay,"total_att":total_att}, open(state_path,"w"))
print(json.dumps({k:out[k] for k in out if k!="workers"}, indent=2))
print("--- workers ---")
for r in sorted(rows, key=lambda z: -z["pay"]):
  mark="OK" if r["online"] else "DOWN"
  print(f"{mark:4} {r['id']}: ghs={r['ghs']} att={r['att']} pay={r['pay']} age={r['age']} addr={r['addr']}")
if watch=="1":
  open(metrics_path,"a").write(json.dumps({k:v for k,v in out.items() if k!="workers"})+"\n")
  print(f"[metrics] appended {metrics_path}")
PY

echo '--- on-chain balances ---'
total_bal=0
for name in "${NAMES[@]}"; do
  wid="worker-${name}"
  addr="$(sync_wallet "$wid" 2>/dev/null || true)"
  [[ -n "${addr:-}" ]] || continue
  bal="$(balance_hmc "$addr")"
  total_bal="$(python3 -c "print(round(float('$total_bal')+float('$bal'),8))")"
  printf '%s %s bal=%s\n' "$wid" "$addr" "$bal"
done
echo "fleet_onchain_hmc=$total_bal"
