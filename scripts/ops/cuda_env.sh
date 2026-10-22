#!/usr/bin/env bash
# Source from build scripts: export CUDA_HOME, CGO_*, LD_LIBRARY_PATH for native CUDA builds.
#   source scripts/ops/cuda_env.sh
set -euo pipefail

cuda_find_home() {
  local d
  # Prefer full toolkit trees (nvcc + NVRTC) over thin .deps/cuda-home stubs
  # that only symlink headers — RTX 50 needs CUDA ≥12.8 NVRTC for compute_120.
  for d in \
    "${CUDA_PATH:-}" \
    /usr/local/cuda-12.8 \
    /usr/local/cuda-13.0 \
    /usr/local/cuda-13 \
    /usr/local/cuda \
    /usr/local/cuda-12.6 \
    /usr/local/cuda-12.4 \
    /usr/local/cuda-12.2 \
    /usr/local/cuda-12.0 \
    /opt/cuda \
    /usr/lib/cuda \
    "${CUDA_HOME:-}"
  do
    [[ -n "$d" && -f "$d/include/nvrtc.h" ]] || continue
    # Skip stub prefixes that lack libnvrtc / nvcc when a real toolkit exists.
    if [[ ! -x "$d/bin/nvcc" && ! -e "$d/lib64/libnvrtc.so" && ! -e "$d/lib/libnvrtc.so" && ! -e "$d/lib/x86_64-linux-gnu/libnvrtc.so" ]]; then
      if [[ -x /usr/local/cuda-12.8/bin/nvcc || -f /usr/local/cuda-12.8/include/nvrtc.h ]]; then
        continue
      fi
    fi
    printf '%s' "$d"
    return 0
  done
  # Debian/Ubuntu split packages: libnvrtc-dev → /usr/include/nvrtc.h (no full cuda tree)
  if [[ -f /usr/include/nvrtc.h && -f /usr/include/cuda.h ]]; then
    printf '%s' "/usr"
    return 0
  fi
  if [[ -f /usr/include/nvrtc.h ]]; then
    printf '%s' "/usr"
    return 0
  fi
  # Unprivileged prefix (e.g. dpkg-deb -x into /tmp/hm-prefix)
  if [[ -n "${HACKME_CUDA_PREFIX:-}" && -f "${HACKME_CUDA_PREFIX}/include/nvrtc.h" ]]; then
    printf '%s' "${HACKME_CUDA_PREFIX}"
    return 0
  fi
  if [[ -f /tmp/hm-prefix/usr/include/nvrtc.h ]]; then
    printf '%s' "/tmp/hm-prefix/usr"
    return 0
  fi
  return 1
}

if ! CUDA_HOME="$(cuda_find_home)"; then
  echo "[cuda-env] CUDA toolkit not found (need nvrtc.h). Install from repo root:" >&2
  echo "  cd ~/Desktop/HackMe && sudo bash scripts/ops/install_cuda_dev_ubuntu.sh" >&2
  return 1 2>/dev/null || exit 1
fi

export CUDA_HOME
if [[ "$CUDA_HOME" == "/usr" ]]; then
  export PATH="${PATH:-}"
  export CGO_ENABLED=1
  export CGO_CFLAGS="${CGO_CFLAGS:-} -I/usr/include"
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L/usr/lib/x86_64-linux-gnu -lcuda -lnvrtc -lcudart"
  export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu:${LD_LIBRARY_PATH:-}"
elif [[ "$CUDA_HOME" == */hm-prefix/usr || -d "${CUDA_HOME}/lib/x86_64-linux-gnu" ]]; then
  export PATH="${PATH:-}"
  export CGO_ENABLED=1
  export CGO_CFLAGS="${CGO_CFLAGS:-} -I${CUDA_HOME}/include"
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L${CUDA_HOME}/lib/x86_64-linux-gnu -lcuda -lnvrtc"
  export LD_LIBRARY_PATH="${CUDA_HOME}/lib/x86_64-linux-gnu:${LD_LIBRARY_PATH:-}"
else
  export PATH="$CUDA_HOME/bin:${PATH:-}"
  export CGO_ENABLED=1
  export CGO_CFLAGS="${CGO_CFLAGS:-} -I${CUDA_HOME}/include"
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L${CUDA_HOME}/lib64 -L${CUDA_HOME}/lib -lcuda -lnvrtc -lcudart"
  export LD_LIBRARY_PATH="${CUDA_HOME}/lib64:${CUDA_HOME}/lib:${LD_LIBRARY_PATH:-}"
fi

if command -v nvidia-smi >/dev/null 2>&1; then
  echo "[cuda-env] CUDA_HOME=$CUDA_HOME driver=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null | head -1 || echo '?')"
else
  echo "[cuda-env] CUDA_HOME=$CUDA_HOME (nvidia-smi not in PATH)"
fi
