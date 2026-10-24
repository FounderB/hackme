// Blake3 local backend burn bench (CUDA device vs OpenCL path vs CPU host).
//
//	source scripts/ops/cuda_env.sh
//	go run -tags cuda,opencl ./cmd/blake3bench -backend cuda
//	go run -tags cuda,opencl ./cmd/blake3bench -backend opencl
//	go run -tags cuda,opencl ./cmd/blake3bench -backend cpu
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"hackme/internal/gpupoh"
)

func main() {
	backend := flag.String("backend", "cuda", "cuda|opencl|cpu")
	batch := flag.Uint64("batch", 1<<26, "nonce count per sample")
	samples := flag.Int("samples", 3, "timed samples after 1 warmup")
	device := flag.Int("device", 0, "GPU device index")
	flag.Parse()

	req := gpupoh.SearchRequest{
		Base: 1, Count: *batch, Mod: 1_000_003,
		FormulaVersion: 3,
		PrevHash:       "00",
		MinerAddress:   "HMC-bench",
	}

	var (
		label string
		search func(context.Context, gpupoh.SearchRequest) (bool, uint64, error)
		cleanup func()
	)

	switch *backend {
	case "cpu":
		label = "CPU host Blake3Search"
		search = func(ctx context.Context, r gpupoh.SearchRequest) (bool, uint64, error) {
			return gpupoh.Blake3Search(ctx, r.Base, r.Count, r.Mod, r.PrevHash, r.MinerAddress)
		}
		cleanup = func() {}
	case "cuda", "opencl":
		if *backend == "opencl" {
			_ = os.Setenv("HACKME_FORCE_OPENCL", "1")
			_ = os.Setenv("HACKME_GPU_BACKEND", "opencl")
		} else {
			_ = os.Unsetenv("HACKME_FORCE_OPENCL")
			_ = os.Setenv("HACKME_GPU_BACKEND", "cuda")
		}
		accs, err := gpupoh.DiscoverAccelerators()
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}
		var pick gpupoh.Accelerator
		for _, a := range accs {
			if a == nil {
				continue
			}
			if a.Backend() == *backend || (*backend == "opencl" && a.Backend() == "opencl") {
				if a.DeviceIndex() == *device || pick == nil {
					pick = a
					if a.DeviceIndex() == *device {
						break
					}
				}
			}
		}
		if pick == nil {
			fmt.Fprintf(os.Stderr, "no %s accelerator found\n", *backend)
			os.Exit(2)
		}
		label = fmt.Sprintf("%s %s", pick.Backend(), pick.Label())
		search = func(ctx context.Context, r gpupoh.SearchRequest) (bool, uint64, error) {
			return pick.SearchPoH(ctx, r)
		}
		cleanup = func() { _ = pick.Close() }
	default:
		fmt.Fprintf(os.Stderr, "unknown backend %q\n", *backend)
		os.Exit(2)
	}
	defer cleanup()

	fmt.Printf("backend=%s label=%q batch=%d samples=%d\n", *backend, label, *batch, *samples)
	var sum float64
	var n int
	for i := 0; i < 1+*samples; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		t0 := time.Now()
		_, _, err := search(ctx, req)
		wall := time.Since(t0).Seconds()
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sample %d err: %v\n", i, err)
			continue
		}
		ghs := float64(*batch) / wall / 1e9
		tag := "sample"
		if i == 0 {
			tag = "warmup"
		} else {
			sum += ghs
			n++
		}
		fmt.Printf("%s wall=%.4fs ghs=%.3f\n", tag, wall, ghs)
	}
	if n == 0 {
		os.Exit(1)
	}
	fmt.Printf("BEST_AVG_GHS=%.3f (%s)\n", sum/float64(n), label)
}
