// Blake3 local backend burn bench (CUDA device vs OpenCL path vs CPU host).
//
//	source scripts/ops/cuda_env.sh
//	go run -tags cuda,opencl ./cmd/blake3bench -backend cuda
//	go run -tags cuda,opencl ./cmd/blake3bench -backend opencl
//	go run -tags cuda,opencl ./cmd/blake3bench -backend cpu
//	go run -tags cuda,opencl ./cmd/blake3bench -prefix-sweep   # 0..160, one process each
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zeebo/blake3"

	"hackme/internal/gpupoh"
)

func main() {
	backend := flag.String("backend", "cuda", "cuda|opencl|cpu")
	batch := flag.Uint64("batch", 1<<26, "nonce count per sample")
	samples := flag.Int("samples", 3, "timed samples after 1 warmup")
	device := flag.Int("device", 0, "GPU device index")
	prefixSweep := flag.Bool("prefix-sweep", false, "parity: each prefix len 0..160 in a fresh process")
	prefixLen := flag.Int("prefix-len", -1, "child: check one prefix length (used by -prefix-sweep)")
	flag.Parse()

	if *prefixSweep {
		os.Exit(runPrefixSweep())
	}
	if *prefixLen >= 0 {
		os.Exit(runPrefixLenChild(*backend, *device, *prefixLen))
	}

	req := gpupoh.SearchRequest{
		Base: 1, Count: *batch, Mod: 1_000_003,
		FormulaVersion: 3,
		PrevHash:       "00",
		MinerAddress:   "HMC-bench",
	}

	var (
		label   string
		search  func(context.Context, gpupoh.SearchRequest) (bool, uint64, error)
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

func runPrefixSweep() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "executable: %v\n", err)
		return 1
	}
	// Prefer the go-run path when invoked via `go run`.
	self := exe
	fail := 0
	for plen := 0; plen <= 160; plen++ {
		cmd := exec.Command(self, "-prefix-len", fmt.Sprintf("%d", plen), "-backend", "cuda")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL plen=%d: %v\n", plen, err)
			fail++
			continue
		}
	}
	if fail > 0 {
		fmt.Fprintf(os.Stderr, "prefix-sweep: %d/%d failed\n", fail, 161)
		return 1
	}
	fmt.Printf("prefix-sweep: 0..160 OK\n")
	return 0
}

func runPrefixLenChild(backend string, device, plen int) int {
	if plen < 0 || plen > 160 {
		fmt.Fprintf(os.Stderr, "prefix-len out of range: %d\n", plen)
		return 2
	}
	_ = os.Unsetenv("HACKME_CUDA_BLAKE3_HOST")
	_ = os.Unsetenv("HACKME_FORCE_OPENCL")
	_ = os.Setenv("HACKME_GPU_BACKEND", "cuda")

	prev, addr := prefixPartsForLen(plen)
	const mod = uint64(97)
	const scan = uint64(1 << 18)

	var hostHit uint64
	var have bool
	for n := uint64(0); n < scan; n++ {
		if pohWork(prev, addr, n)%mod == 0 {
			hostHit = n
			have = true
			break
		}
	}
	if !have {
		fmt.Fprintf(os.Stderr, "plen=%d: no host hit in scan\n", plen)
		return 1
	}

	var search func(context.Context, gpupoh.SearchRequest) (bool, uint64, error)
	var cleanup func()
	switch backend {
	case "cpu":
		search = func(ctx context.Context, r gpupoh.SearchRequest) (bool, uint64, error) {
			return gpupoh.Blake3Search(ctx, r.Base, r.Count, r.Mod, r.PrevHash, r.MinerAddress)
		}
		cleanup = func() {}
	default:
		accs, err := gpupoh.DiscoverAccelerators()
		if err != nil || len(accs) == 0 {
			fmt.Fprintf(os.Stderr, "plen=%d: no accelerator: %v\n", plen, err)
			return 1
		}
		var pick gpupoh.Accelerator
		for _, a := range accs {
			if a != nil && a.Backend() == "cuda" && (a.DeviceIndex() == device || pick == nil) {
				pick = a
				if a.DeviceIndex() == device {
					break
				}
			}
		}
		if pick == nil {
			pick = accs[0]
		}
		search = func(ctx context.Context, r gpupoh.SearchRequest) (bool, uint64, error) {
			return pick.SearchPoH(ctx, r)
		}
		cleanup = func() {
			for _, a := range accs {
				_ = a.Close()
			}
		}
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ok, nonce, err := search(ctx, gpupoh.SearchRequest{
		Base: 0, Count: hostHit + 1, Mod: mod,
		PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "plen=%d SearchPoH: %v\n", plen, err)
		return 1
	}
	if !ok || nonce != hostHit {
		fmt.Fprintf(os.Stderr, "plen=%d mismatch ok=%v nonce=%d hostHit=%d\n", plen, ok, nonce, hostHit)
		return 1
	}
	fmt.Printf("OK plen=%d nonce=%d\n", plen, nonce)
	return 0
}

func prefixPartsForLen(plen int) (prev, addr string) {
	if plen <= 0 {
		return "", ""
	}
	if plen <= 64 {
		return strings.Repeat("a", plen), ""
	}
	return strings.Repeat("b", 64), strings.Repeat("X", plen-64)
}

func pohWork(prev, addr string, nonce uint64) uint64 {
	msg := make([]byte, 0, len(prev)+len(addr)+8)
	msg = append(msg, prev...)
	msg = append(msg, addr...)
	var nb [8]byte
	binary.LittleEndian.PutUint64(nb[:], nonce)
	msg = append(msg, nb[:]...)
	sum := blake3.Sum256(msg)
	return binary.LittleEndian.Uint64(sum[0:8])
}
