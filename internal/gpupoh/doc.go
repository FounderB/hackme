// Package gpupoh implements optional GPU batch search for PoH.
// Pre-activation: eval(n)=n*7+13, eval%mod==0.
// Post Blake3 v3 activation: Blake3(prev||miner||nonce_le) work%mod==0 (LE uint64).
//
// Build tags:
//
//	go build -tags cuda -o hackme .          // NVIDIA CUDA (per-GPU NVRTC arch, e.g. compute_120)
//	go build -tags opencl -o hackme .        // OpenCL (AMD / Intel / others; CGO + OpenCL headers)
//	go build -tags "cuda,opencl" -o hackme . // NVIDIA: CUDA; AMD/Intel: OpenCL fallback
//	bash scripts/ops/build_gpu_workers.sh    // Build workerpoh-cuda + workerpoh-opencl
//	bash scripts/ops/detect_gpu_backend.sh   // Print cuda | opencl | cpu for this host
//
// Production worker build: bash scripts/ops/build_cuda_worker.sh → bin/workerpoh-cuda
// See docs/CUDA_PRODUCTION.md.
//
// Default builds (!cuda && !opencl) expose stubs only (CPU PoH).
//
// Environment:
//
//	HACKME_USE_CUDA=0       — force CPU PoH even when GPU build tags are on (same as before).
//	HACKME_FORCE_OPENCL=1 — skip CUDA and use OpenCL only (when built with opencl).
//	HACKME_OPENCL_BLAKE3_HOST=1 — force host Blake3Search (skip device poh_search_blake3).
//	HACKME_CUDA_BLAKE3_HOST=1 — force host Blake3Search on CUDA (skip device kernel).
//
// CUDA requires CGO, a C compiler, NVIDIA driver, and CUDA toolkit (nvrtc).
// OpenCL requires CGO, OpenCL ICD/headers (Khronos, vendor SDK, or CUDA toolkit include path).
package gpupoh
