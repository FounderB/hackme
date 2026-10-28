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

rc=0
created=""
# Refuses (non-zero) when the existing chain of checkpoints does not verify, when the node key
# differs from the published signer, or when the tip has not advanced (stalled chain → alert).
# A refusal still lets this run publish anything pending below, then fails the unit.
if created="$("$BIN" create -db "$DB" -data-dir "$DATA_DIR" -dir "$REPO/checkpoints" -min-blocks "$MIN_BLOCKS")"; then
  log "created ${created#"$REPO"/}"
else
  rc=1
  log "ERROR: checkpoint create refused (see above)"
fi

# OpenTimestamps never blocks publication: stamp any checkpoint still missing a proof (this run's
# or one left by an earlier failed run) and upgrade pending proofs once Bitcoin confirms them.
if [[ -x "$OTS" ]]; then
  for cp in "$REPO"/checkpoints/*.json; do
    [[ -e "$cp" && ! -e "${cp}.ots" ]] || continue
    if "$OTS" stamp "$cp" >/dev/null 2>&1; then
      log "ots stamped ${cp#"$REPO"/}"
    else
      log "WARN: ots stamp failed for ${cp#"$REPO"/} — retried next run"
    fi
  done
  for proof in "$REPO"/checkpoints/*.ots; do
    [[ -e "$proof" ]] || continue
    "$OTS" upgrade "$proof" >/dev/null 2>&1 || true
  done
else
  log "WARN: $OTS missing — publishing without OpenTimestamps proofs"
fi

"$BIN" verify -dir "$REPO/checkpoints" >/dev/null

git add checkpoints
if git diff --cached --quiet; then
  log "nothing to publish"
else
  if [[ -n "$created" ]]; then
    height="$(basename "$created" .json)"
    msg="checkpoint $((10#$height))"
  else
    msg="checkpoint proofs: OpenTimestamps upgrades"
  fi
  git_bot commit -q -m "$msg"
  git push -q
  log "pushed: $msg"
fi
exit "$rc"
