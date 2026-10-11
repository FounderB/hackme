#!/usr/bin/env bash
# Cap Hunt miner on-disk growth: harness cache, AFL workdirs, old LF corpus tails, logs.
# Safe/idempotent — never touches .env / wallets / Dig WASM artifacts.
#
# Usage:
#   bash scripts/ops/prune_hunt_miner_caches.sh
#   bash scripts/ops/prune_hunt_miner_caches.sh --repo /opt/hackme
#   HACKME_HUNT_CACHE_MAX_AGE_DAYS=3 bash scripts/ops/prune_hunt_miner_caches.sh
set -euo pipefail

REPO=""
MAX_AGE_DAYS="${HACKME_HUNT_CACHE_MAX_AGE_DAYS:-3}"
MAX_HARNESS="${HACKME_HUNT_HARNESS_CACHE_MAX:-64}"
MAX_AFL_BIN="${HACKME_HUNT_AFL_BIN_CACHE_MAX:-32}"
MAX_CORPUS_FILES="${HACKME_HUNT_PERSIST_CORPUS_MAX:-4096}"
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo|-r) REPO="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h)
      sed -n '2,12p' "$0"
      exit 0
      ;;
    *) echo "[hunt-prune] unknown arg: $1" >&2; exit 2 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "$REPO" ]]; then
  if [[ -d "${SCRIPT_DIR}/../../internal/hunt" ]]; then
    REPO="$(cd "${SCRIPT_DIR}/../.." && pwd)"
  elif [[ -x "${SCRIPT_DIR}/hackme" || -d "${SCRIPT_DIR}/.cache" ]]; then
    REPO="$SCRIPT_DIR"
  else
    REPO="${HACKME_REPO_ROOT:-$SCRIPT_DIR}"
  fi
fi
REPO="$(cd "$REPO" 2>/dev/null && pwd)" || { echo "[hunt-prune] repo not found" >&2; exit 1; }

log() { echo "[hunt-prune] $*"; }

rm_path() {
  local p="$1"
  if [[ "$DRY_RUN" == "1" ]]; then
    log "DRY-RUN would remove $p"
    return 0
  fi
  rm -rf "$p"
}

# Keep newest N regular files in a directory (by mtime).
prune_dir_count() {
  local dir="$1" max="$2"
  [[ -d "$dir" ]] || return 0
  [[ "$max" =~ ^[0-9]+$ ]] || return 0
  local n
  n="$(find "$dir" -maxdepth 1 -type f ! -name '.*' 2>/dev/null | wc -l | tr -d ' ')"
  if [[ "${n:-0}" -le "$max" ]]; then
    return 0
  fi
  log "cap $dir: $n → $max"
  # shellcheck disable=SC2012
  ls -1t "$dir" 2>/dev/null | tail -n +"$((max + 1))" | while read -r f; do
    [[ -n "$f" ]] || continue
    rm_path "$dir/$f"
  done
}

# Age-prune AFL workdirs (out/ wiped per shard; whole target dir if stale).
prune_afl_work() {
  local base="$REPO/.cache/hunt-afl"
  [[ -d "$base" ]] || return 0
  find "$base" -mindepth 1 -maxdepth 1 -type d -mtime "+${MAX_AGE_DAYS}" 2>/dev/null | while read -r d; do
    log "stale AFL work $d (>${MAX_AGE_DAYS}d)"
    rm_path "$d"
  done
  # Always drop leftover out/ trees (queue re-imported each shard).
  find "$base" -type d -name out 2>/dev/null | while read -r d; do
    rm_path "$d"
  done
}

prune_persist_corpora() {
  local base="$REPO/reports/oss-cve-libfuzzer"
  [[ -d "$base" ]] || return 0
  find "$base" -type d -name corpus 2>/dev/null | while read -r d; do
    prune_dir_count "$d" "$MAX_CORPUS_FILES"
  done
}

prune_harness_cache() {
  prune_dir_count "$REPO/.cache/hunt-harness" "$MAX_HARNESS"
  prune_dir_count "$REPO/.cache/hunt-afl-bin" "$MAX_AFL_BIN"
}

prune_old_logs() {
  local logs="$REPO/logs"
  [[ -d "$logs" ]] || return 0
  find "$logs" -maxdepth 1 -type f \( -name '*libfuzzer*.log' -o -name '*hunt-afl*.log' -o -name 'workerfuzz-*.log' \) \
    -mtime "+${MAX_AGE_DAYS}" 2>/dev/null | while read -r f; do
    log "old log $f"
    rm_path "$f"
  done
}

log "repo=$REPO age_days=$MAX_AGE_DAYS dry_run=$DRY_RUN"
prune_afl_work
prune_harness_cache
prune_persist_corpora
prune_old_logs
log "done"
