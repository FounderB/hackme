#!/usr/bin/env bash
# Install Hunt coverage engines for hybrid workers (libFuzzer via clang + bundled AFL++).
# Does NOT require miners to git clone AFL++ from GitHub — downloads a pinned release tarball.
#
# Usage:
#   bash scripts/ops/install_hunt_engines.sh --desktop
#   sudo bash scripts/ops/install_hunt_engines.sh --system --prefix /opt/hackme
#   bash scripts/ops/install_hunt_engines.sh --check-only
#
# Env:
#   AFLPP_VERSION   default 4.21c (release tag without leading v)
#   SKIP_AFLPP=1    only ensure clang
#   SKIP_CLANG=1    only install AFL++
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AFLPP_VERSION="${AFLPP_VERSION:-4.21c}"
MODE_DESKTOP=0
MODE_SYSTEM=0
CHECK_ONLY=0
HACKME_PREFIX=""
SKIP_AFLPP="${SKIP_AFLPP:-0}"
SKIP_CLANG="${SKIP_CLANG:-0}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --desktop) MODE_DESKTOP=1; shift ;;
    --system) MODE_SYSTEM=1; shift ;;
    --check-only) CHECK_ONLY=1; shift ;;
    --prefix) HACKME_PREFIX="${2:-}"; shift 2 ;;
    *) echo "[hunt-engines] unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$HACKME_PREFIX" ]]; then
  if [[ "$MODE_DESKTOP" == "1" ]]; then
    HACKME_PREFIX="${HACKME_PREFIX:-$ROOT_DIR/logs/desktop/toolchains}"
  elif [[ "$(id -u)" -eq 0 ]] || [[ "$MODE_SYSTEM" == "1" ]]; then
    HACKME_PREFIX="${HACKME_PREFIX:-/opt/hackme}"
  else
    HACKME_PREFIX="${HACKME_PREFIX:-$HOME/.local/share/hackme/toolchains}"
  fi
fi

log() { echo "[hunt-engines] $*"; }
warn() { echo "[hunt-engines] WARN: $*" >&2; }

have() { command -v "$1" >/dev/null 2>&1; }

afl_bin() { echo "$HACKME_PREFIX/aflpp/bin/afl-fuzz"; }

check_status() {
  local ok=1
  if have clang; then
    log "clang: $(command -v clang) ($(clang --version 2>/dev/null | head -1))"
  else
    warn "clang missing — libFuzzer Hunt engine unavailable"
    ok=0
  fi
  if [[ -x "$(afl_bin)" ]] || have afl-fuzz; then
    local p
    p="$(afl_bin)"
    [[ -x "$p" ]] || p="$(command -v afl-fuzz)"
    log "afl-fuzz: $p"
  else
    warn "afl-fuzz missing — AFL++ Deep Hunt engine unavailable"
    ok=0
  fi
  return $((1 - ok))
}

if [[ "$CHECK_ONLY" == "1" ]]; then
  check_status && exit 0 || exit 1
fi

mkdir -p "$HACKME_PREFIX/bin" "$HACKME_PREFIX/aflpp"

ensure_clang() {
  if [[ "$SKIP_CLANG" == "1" ]]; then
    return 0
  fi
  if have clang; then
    log "clang already present"
    return 0
  fi
  if [[ "$(id -u)" -eq 0 ]] && have apt-get; then
    log "apt-get install clang (libFuzzer)"
    DEBIAN_FRONTEND=noninteractive apt-get install -y clang || warn "apt clang failed"
  else
    warn "install clang via your package manager for libFuzzer Hunt (fleet default)"
  fi
}

install_aflpp_bundle() {
  if [[ "$SKIP_AFLPP" == "1" ]]; then
    return 0
  fi
  local dest="$HACKME_PREFIX/aflpp"
  if [[ -x "$dest/bin/afl-fuzz" ]]; then
    log "AFL++ already at $dest/bin/afl-fuzz"
    ln -sfn "$dest/bin/afl-fuzz" "$HACKME_PREFIX/bin/afl-fuzz"
    return 0
  fi
  if have afl-fuzz; then
    log "system afl-fuzz present: $(command -v afl-fuzz)"
    return 0
  fi

  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="x86_64" ;;
    aarch64|arm64) arch="aarch64" ;;
    *) warn "unsupported arch $arch for prebuilt AFL++ — skip"; return 0 ;;
  esac

  # Prefer a pre-vendored tarball in the release/hybrid bundle (no GitHub clone).
  local local_tb=""
  for cand in \
    "$ROOT_DIR/third_party/aflpp/aflpp-${AFLPP_VERSION}-linux-${arch}.tar.gz" \
    "$ROOT_DIR/dist/toolchains/aflpp-${AFLPP_VERSION}-linux-${arch}.tar.gz" \
    "${HACKME_AFLPP_TARBALL:-}"; do
    [[ -n "$cand" && -f "$cand" ]] && { local_tb="$cand"; break; }
  done

  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  if [[ -n "$local_tb" ]]; then
    log "extracting bundled AFL++ from $local_tb"
    tar -xzf "$local_tb" -C "$tmp"
  else
    # Fallback: download official release source tarball (not git clone) and build.
    local url="https://github.com/AFLplusplus/AFLplusplus/archive/refs/tags/v${AFLPP_VERSION}.tar.gz"
    log "downloading AFL++ v${AFLPP_VERSION} release tarball (no git clone)"
    if have curl; then
      curl -fsSL "$url" -o "$tmp/aflpp.tgz" || { warn "download failed"; return 0; }
    elif have wget; then
      wget -qO "$tmp/aflpp.tgz" "$url" || { warn "download failed"; return 0; }
    else
      warn "curl/wget missing — cannot fetch AFL++"
      return 0
    fi
    tar -xzf "$tmp/aflpp.tgz" -C "$tmp"
    local src
    src="$(find "$tmp" -maxdepth 1 -type d -name 'AFLplusplus-*' | head -1)"
    if [[ -z "$src" ]]; then
      warn "AFL++ extract failed"
      return 0
    fi
    if ! have clang; then
      warn "clang required to build AFL++ from source"
      return 0
    fi
    # Prefer llvm-config-N when bare llvm-config is missing (Ubuntu packages).
    local llvm_config=""
    if have llvm-config; then
      llvm_config="$(command -v llvm-config)"
    else
      for cand in llvm-config-18 llvm-config-17 llvm-config-16 llvm-config-15; do
        if have "$cand"; then llvm_config="$(command -v "$cand")"; break; fi
      done
    fi
    log "building AFL++ PREFIX=$dest LLVM_CONFIG=${llvm_config:-none}"
    # Optional gcc_plugin/LTO may fail — still harvest afl-fuzz if the main tools built.
    set +e
    if [[ -n "$llvm_config" ]]; then
      make -C "$src" all PREFIX="$dest" -j"$(nproc 2>/dev/null || echo 2)" LLVM_CONFIG="$llvm_config"
    else
      make -C "$src" all PREFIX="$dest" -j"$(nproc 2>/dev/null || echo 2)"
    fi
    local make_rc=$?
    if [[ -x "$src/afl-fuzz" ]]; then
      mkdir -p "$dest/bin"
      # install target can fail on optional bits; copy core tools explicitly
      make -C "$src" install PREFIX="$dest" LLVM_CONFIG="${llvm_config:-llvm-config}" >/dev/null 2>&1
      for b in afl-fuzz afl-showmap afl-clang-fast afl-cc afl-c++ afl-clang-fast++; do
        [[ -x "$src/$b" ]] && cp -a "$src/$b" "$dest/bin/" || true
      done
      # supporting libs next to prefix when present
      for f in afl-compiler-rt.o libAFLDriver.a; do
        [[ -f "$src/$f" ]] && cp -a "$src/$f" "$dest/" || true
      done
      make_rc=0
    fi
    set -e
    if [[ "$make_rc" -ne 0 && ! -x "$dest/bin/afl-fuzz" && ! -x "$src/afl-fuzz" ]]; then
      warn "AFL++ build failed"
      return 0
    fi
  fi

  # If tarball laid out bin/ at top level or build left afl-fuzz in src tree:
  if [[ ! -x "$dest/bin/afl-fuzz" ]]; then
    local found
    found="$(find "$tmp" -type f -name afl-fuzz -perm -111 2>/dev/null | head -1 || true)"
    if [[ -n "$found" ]]; then
      mkdir -p "$dest/bin"
      cp -a "$(dirname "$found")/." "$dest/bin/" || true
    fi
  fi
  # Last-resort: copy a working system/hub afl-fuzz into the prefix (no GitHub clone).
  if [[ ! -x "$dest/bin/afl-fuzz" ]] && have afl-fuzz; then
    mkdir -p "$dest/bin"
    cp -a "$(command -v afl-fuzz)" "$dest/bin/afl-fuzz" || true
    log "copied system afl-fuzz → $dest/bin/afl-fuzz"
  fi
  if [[ -x "$dest/bin/afl-fuzz" ]]; then
    ln -sfn "$dest/bin/afl-fuzz" "$HACKME_PREFIX/bin/afl-fuzz"
    log "AFL++ installed → $dest/bin/afl-fuzz"
  else
    warn "afl-fuzz still missing after install attempt"
  fi
}

write_env() {
  local envf="$HACKME_PREFIX/.env.hunt-engines"
  cat >"$envf" <<EOF
# Generated by install_hunt_engines.sh
PATH=${HACKME_PREFIX}/bin:${HACKME_PREFIX}/aflpp/bin:\${PATH}
HACKME_AFL_FUZZ=${HACKME_PREFIX}/aflpp/bin/afl-fuzz
HACKME_PREFIX=${HACKME_PREFIX}
EOF
  chmod 644 "$envf" 2>/dev/null || true
  log "wrote $envf"
}

ensure_clang
install_aflpp_bundle
write_env
check_status || warn "partial install — Hunt soft-skips missing engines; Dig unchanged"
log "done (promise: distributed coverage-guided Hunt — not OSS-Fuzz replacement)"
