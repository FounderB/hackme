#!/usr/bin/env bash
# Daily external checkpoint: create (signed, read-only DB) → OpenTimestamps → push to the
# public checkpoints repo. Anyone can then verify history with `checkpoint verify`.
#
# Layout (hub):
#   /opt/hackme/bin/checkpoint                  built by deploy_hackme_node.sh
#   /opt/hackme-checkpoints                     git clone of jokeez/hackme-checkpoints (outside
#                                               /opt/hackme so deploy rsync --delete never touches it)
#   /opt/hackme-checkpoints-venv/bin/ots        opentimestamps-client
#   ~hackme/.ssh/hackme_checkpoints_deploy      deploy key with write access to that repo only
set -euo pipefail

REPO="${CHECKPOINT_REPO:-/opt/hackme-checkpoints}"
BIN="${CHECKPOINT_BIN:-/opt/hackme/bin/checkpoint}"
DB="${CHECKPOINT_DB:-/opt/hackme/data/hackme.db}"
DATA_DIR="${CHECKPOINT_DATA_DIR:-/opt/hackme/data}"
OTS="${CHECKPOINT_OTS:-/opt/hackme-checkpoints-venv/bin/ots}"
KEY="${CHECKPOINT_DEPLOY_KEY:-$HOME/.ssh/hackme_checkpoints_deploy}"
MIN_BLOCKS="${CHECKPOINT_MIN_BLOCKS:-100}"

log() { echo "[checkpoint] $*"; }

export GIT_SSH_COMMAND="ssh -i ${KEY} -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=accept-new"
git_bot() { git -c user.name="HackMe checkpoint bot" -c user.email="checkpoints@hackme.tech" "$@"; }

cd "$REPO"
git pull --ff-only -q

# Refuses (non-zero) when the existing chain of checkpoints does not verify, when the node key
# differs from the published signer, or when the tip has not advanced (stalled chain → alert).
file="$("$BIN" create -db "$DB" -data-dir "$DATA_DIR" -dir "$REPO/checkpoints" -min-blocks "$MIN_BLOCKS")"
log "created ${file#"$REPO"/}"

if [[ -x "$OTS" ]]; then
  "$OTS" stamp "$file"
  log "ots stamped ${file#"$REPO"/}.ots"
  # Earlier stamps become Bitcoin-attested a few hours later; upgrade them in place.
  for proof in "$REPO"/checkpoints/*.ots; do
    [[ "$proof" == "${file}.ots" ]] && continue
    "$OTS" upgrade "$proof" >/dev/null 2>&1 || true
  done
else
  log "WARN: $OTS missing — checkpoint published without OpenTimestamps proof"
fi

"$BIN" verify -dir "$REPO/checkpoints" >/dev/null

git add checkpoints
height="$(basename "$file" .json)"
git_bot commit -q -m "checkpoint $((10#$height))"
git push -q
log "pushed"
