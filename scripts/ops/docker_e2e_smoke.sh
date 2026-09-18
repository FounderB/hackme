#!/usr/bin/env bash
# Full docker e2e: build stack → status → seed-perm repair → worker/start → security-audit.
#
#   export E2E_TOKEN="$(openssl rand -hex 24)"
#   bash scripts/ops/docker_e2e_smoke.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${PATH:-}"

COMPOSE=(docker compose -f deploy/docker-compose.e2e.yml)
E2E_TOKEN="${E2E_TOKEN:-$(openssl rand -hex 24)}"
export E2E_TOKEN
NODE_URL="${NODE_URL:-http://127.0.0.1:${NODE_PUBLISH_PORT:-18080}}"
COORD_URL="${COORD_URL:-http://127.0.0.1:${COORD_PUBLISH_PORT:-18081}}"
ADMIN="$E2E_TOKEN"

cleanup() {
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e] token=${E2E_TOKEN:0:8}… node=$NODE_URL"
cleanup
if [[ "${HACKME_E2E_NO_BUILD:-0}" == "1" ]]; then
  "${COMPOSE[@]}" up -d
else
  "${COMPOSE[@]}" up --build -d
fi

echo "[e2e] wait node+coordinator"
ok=0
for i in $(seq 1 90); do
  if curl -fsS "$NODE_URL/api/status?lite=1" >/dev/null 2>&1 \
    && curl -fsS "$COORD_URL/api/network/stats" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 2
done
if [[ "$ok" != "1" ]]; then
  echo "[e2e] FAIL: services did not become healthy" >&2
  "${COMPOSE[@]}" ps -a >&2 || true
  "${COMPOSE[@]}" logs --tail 80 node >&2 || true
  "${COMPOSE[@]}" logs --tail 40 coordinator >&2 || true
  exit 1
fi
curl -fsS "$NODE_URL/api/status?lite=1" >/dev/null
curl -fsS "$COORD_URL/api/network/stats" >/dev/null
echo "[e2e] status OK"

# Make the live node seed world-readable (classic installer footgun: permissions too open 0644/0666).
echo "[e2e] chmod 0644 existing node_ed25519.seed"
cid="$("${COMPOSE[@]}" ps -q node)"
docker exec "$cid" bash -lc '
  set -e
  f=/data/node_ed25519.seed
  test -f "$f"
  chmod 644 "$f"
  ls -l "$f"
'

echo "[e2e] POST /api/worker/start (must auto-repair seed perms)"
resp="$(mktemp)"
code="$(curl -sS -o "$resp" -w '%{http_code}' -X POST "$NODE_URL/api/worker/start" \
  -H "Content-Type: application/json" \
  -H "X-Hackme-Admin-Token: $ADMIN" \
  -d "{\"coord_url\":\"http://coordinator:8081\",\"coord_token\":\"$ADMIN\"}")"
echo "[e2e] worker/start HTTP $code body=$(head -c 400 "$resp")"
[[ "$code" == "200" ]] || { cat "$resp"; exit 1; }
if grep -qi 'permissions too open' "$resp"; then
  echo "[e2e] FAIL: seed permission error still returned" >&2
  exit 1
fi
docker exec "$cid" bash -lc 'stat -c "%a %n" /data/node_ed25519.seed' | tee /tmp/hackme-e2e-seed-mode.txt
mode="$(awk '{print $1}' /tmp/hackme-e2e-seed-mode.txt)"
[[ "$mode" == "600" ]] || echo "[e2e] WARN: seed mode is $mode (expected 600 after repair)"

echo "[e2e] wallet / local-auth"
curl -fsS -H "X-Hackme-Admin-Token: $ADMIN" "$NODE_URL/api/wallet" | head -c 300; echo
# local-auth is loopback-only inside the container (host→publish is not loopback).
docker exec "$cid" curl -fsS "http://127.0.0.1:8080/api/desktop/local-auth" | head -c 240; echo

echo "[e2e] POST /api/genesis + fund wallet for orders/audit escrow"
code="$(curl -sS -o "$resp" -w '%{http_code}' -X POST "$NODE_URL/api/genesis" \
  -H "X-Hackme-Admin-Token: $ADMIN")"
echo "[e2e] genesis HTTP $code $(head -c 200 "$resp")"
[[ "$code" == "200" ]] || { cat "$resp"; exit 1; }
docker exec -i "$cid" python3 -c '
import sqlite3
con = sqlite3.connect("/data/hackme.db")
cur = con.cursor()
# 100 HMC @ 1e8 units/HMC
units = 100 * 100_000_000
cur.execute("UPDATE wallet SET balance_hmc=100, balance_units=? WHERE id=1", (units,))
row = cur.execute("SELECT address FROM wallet WHERE id=1").fetchone()
if not row:
    raise SystemExit("wallet row missing after genesis")
cur.execute("UPDATE accounts SET balance_units=? WHERE address=?", (units, row[0]))
con.commit()
print("wallet funded", row[0], "units", units)
'
wbal="$(curl -fsS -H "X-Hackme-Admin-Token: $ADMIN" "$NODE_URL/api/wallet" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("balance_orders_spendable_hmc") or d.get("balance_hmc") or d.get("balance_local_mirror_hmc"))')"
echo "[e2e] spendable_hmc=$wbal"

echo "[e2e] POST /api/tasks (developer/admin)"
task_body='{"id":"e2e-order-'"$(date +%s)"'","kind":"synthetic_poh_v1","reward_hmc":0.05,"difficulty_score":1,"target_solves":1,"wasm_check_hex":"0061736d0100000001060160017e017f0302010007090105636865636b00000a0601040041010b","payer_ref":"e2e"}'
code="$(curl -sS -o "$resp" -w '%{http_code}' -X POST "$NODE_URL/api/tasks" \
  -H "Content-Type: application/json" \
  -H "X-Hackme-Admin-Token: $ADMIN" \
  -d "$task_body")"
echo "[e2e] tasks HTTP $code $(head -c 240 "$resp")"
[[ "$code" == "200" || "$code" == "201" ]] || { cat "$resp"; exit 1; }

echo "[e2e] POST /api/security-audit"
# Re-fund after PoH order spend
docker exec -i "$cid" python3 -c '
import sqlite3
con = sqlite3.connect("/data/hackme.db")
cur = con.cursor()
units = 100 * 100_000_000
cur.execute("UPDATE wallet SET balance_hmc=100, balance_units=? WHERE id=1", (units,))
row = cur.execute("SELECT address FROM wallet WHERE id=1").fetchone()
cur.execute("UPDATE accounts SET balance_units=? WHERE address=?", (units, row[0]))
con.commit()
'
audit_body='{"title":"e2e-audit","payer_ref":"e2e","budget_hmc":1,"budget_runs":16,"budget_seconds":600,"create_poh_order":true,"pool_distributed":false,"depth_tier":"wasm_only","wasm_check_hex":"0061736d0100000001060160017e017f0302010007090105636865636b00000a0601040041010b"}'
code="$(curl -sS -o "$resp" -w '%{http_code}' -X POST "$NODE_URL/api/security-audit" \
  -H "Content-Type: application/json" \
  -H "X-Hackme-Admin-Token: $ADMIN" \
  -d "$audit_body")"
echo "[e2e] security-audit HTTP $code $(head -c 400 "$resp")"
[[ "$code" == "200" ]] || { cat "$resp"; exit 1; }

echo "[e2e] PASS — install/mine/orders cycle OK"
rm -f "$resp"
