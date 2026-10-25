#!/usr/bin/env bash
# Rent Vast GPU(s) for Blake3 matrix fills. Requires VAST_API_KEY.
#
# Examples:
#   VAST_API_KEY=... bash scripts/vast/rent_blake3_matrix.sh search "RTX 4070 Ti Super" 2
#   VAST_API_KEY=... bash scripts/vast/rent_blake3_matrix.sh rent <offer_id> [offer_id2]
#   VAST_API_KEY=... bash scripts/vast/rent_blake3_matrix.sh list
#   VAST_API_KEY=... bash scripts/vast/rent_blake3_matrix.sh destroy <instance_id>
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KEY_FILE="${VAST_API_KEY_FILE:-$ROOT/.secrets/vast/vast_api_key}"

load_key() {
  if [[ -n "${VAST_API_KEY:-}" ]]; then
    return 0
  fi
  if [[ -f "$KEY_FILE" ]]; then
    VAST_API_KEY="$(tr -d ' \n\r' <"$KEY_FILE")"
    export VAST_API_KEY
  fi
  if [[ -z "${VAST_API_KEY:-}" ]]; then
    echo "Missing VAST_API_KEY. Put it in $KEY_FILE or export VAST_API_KEY." >&2
    echo "Create key at https://cloud.vast.ai/account/ → API. chmod 600 the file." >&2
    exit 2
  fi
}

ensure_cli() {
  if command -v vastai >/dev/null 2>&1; then
    return 0
  fi
  echo "[vast] installing vastai CLI (pip --user)..."
  python3 -m pip install --user -q vastai
  export PATH="$HOME/.local/bin:$PATH"
  command -v vastai >/dev/null 2>&1 || {
    echo "vastai CLI still missing after pip install" >&2
    exit 1
  }
}

load_key
ensure_cli
# vastai reads ~/.config/vastai/vast_api_key
mkdir -p "$HOME/.config/vastai"
umask 077
printf '%s\n' "$VAST_API_KEY" >"$HOME/.config/vastai/vast_api_key"
chmod 600 "$HOME/.config/vastai/vast_api_key"
# Keep repo secret copy in sync when provided via env
mkdir -p "$(dirname "$KEY_FILE")"
if [[ ! -f "$KEY_FILE" ]]; then
  printf '%s\n' "$VAST_API_KEY" >"$KEY_FILE"
  chmod 600 "$KEY_FILE"
fi

cmd="${1:-}"
shift || true
case "$cmd" in
  search)
    name="${1:-RTX 4070 Ti Super}"
    num="${2:-2}"
    echo "[vast] search gpu_name=$name num_gpus>=$num (verified, rentable)"
    vastai search offers "gpu_name=${name} num_gpus>=${num} rentable=true verified=true" \
      -o 'dph_total+' --raw 2>/dev/null | python3 - <<'PY' || vastai search offers "gpu_name=${name}" -o 'dph_total+' | head -40
import json,sys
try:
  data=json.load(sys.stdin)
except Exception:
  sys.exit(0)
rows=data if isinstance(data,list) else data.get('offers') or data.get('results') or []
for o in rows[:15]:
  oid=o.get('id') or o.get('ask_contract_id') or o.get('offer_id')
  print(f"id={oid}  gpus={o.get('num_gpus')}  {o.get('gpu_name')}  ${o.get('dph_total')}/h  {o.get('geolocation') or o.get('country')}")
PY
    ;;
  rent)
    if [[ $# -lt 1 ]]; then
      echo "usage: $0 rent <offer_id> [offer_id...]" >&2
      exit 2
    fi
    for oid in "$@"; do
      echo "[vast] create instance from offer $oid"
      vastai create instance "$oid" --image nvidia/cuda:12.4.1-devel-ubuntu22.04 \
        --disk 40 --ssh --direct --onstart-cmd 'sleep infinity' || true
    done
    vastai show instances
    ;;
  list)
    vastai show instances
    ;;
  destroy|stop)
    for id in "$@"; do
      echo "[vast] destroy $id"
      vastai destroy instance "$id" || true
    done
    ;;
  *)
    cat <<EOF
usage:
  $0 search [gpu_name] [min_gpus]   # default: "RTX 4070 Ti Super" 2
  $0 rent <offer_id> [...]
  $0 list
  $0 destroy <instance_id> [...]
EOF
    exit 2
    ;;
esac
