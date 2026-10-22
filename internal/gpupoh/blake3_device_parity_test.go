//go:build cuda

package gpupoh

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestBlake3CUDADeviceMatchesHost locks device poh_search_blake3 against host
// Blake3Search / chain.PohWorkBlake3 on a live NVIDIA GPU (e.g. RTX 5060 Ti).
func TestBlake3CUDADeviceMatchesHost(t *testing.T) {
	_ = os.Setenv("HACKME_CUDA_VERBOSE", "1")
	_ = os.Unsetenv("HACKME_CUDA_BLAKE3_HOST")

	accs, err := DiscoverAccelerators()
	if err != nil {
		t.Fatalf("DiscoverAccelerators: %v", err)
	}
	if len(accs) == 0 {
		t.Skip("no CUDA accelerators")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()

	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-5060tiparity0001"
	const mod = uint64(97)
	const scan = uint64(1 << 20) // 1M nonces — enough for several hits at mod 97

	// Find first host hit in range for known-good nonce.
	var hostHit uint64
	var haveHit bool
	for n := uint64(0); n < scan; n++ {
		if pohWorkBlake3Mirror(prev, addr, n)%mod == 0 {
			hostHit = n
			haveHit = true
			break
		}
	}
	if !haveHit {
		t.Fatal("no host hit in scan window")
	}

	a := accs[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Force device path: SearchPoH with formula 3.
	t0 := time.Now()
	ok, nonce, err := a.SearchPoH(ctx, SearchRequest{
		Base:           0,
		Count:          hostHit + 1,
		Mod:            mod,
		PrevHash:       prev,
		MinerAddress:   addr,
		FormulaVersion: 3,
	})
	elapsed := time.Since(t0)
	if err != nil {
		t.Fatalf("SearchPoH blake3: %v", err)
	}
	if !ok {
		t.Fatal("device search found=false (expected hit)")
	}
	if nonce != hostHit {
		t.Fatalf("device nonce=%d hostHit=%d (first-hit mismatch)", nonce, hostHit)
	}
	if pohWorkBlake3Mirror(prev, addr, nonce)%mod != 0 {
		t.Fatal("device nonce fails host Blake3 work check")
	}
	t.Logf("device hit nonce=%d in %s (~%.2f MH/s) label=%s",
		nonce, elapsed.Round(time.Millisecond),
		float64(hostHit+1)/elapsed.Seconds()/1e6, a.Label())

	// Larger batch: every device hit must verify on host.
	const batch = uint64(1 << 22)
	t1 := time.Now()
	ok2, n2, err := a.SearchPoH(ctx, SearchRequest{
		Base:           0,
		Count:          batch,
		Mod:            10_007,
		PrevHash:       prev,
		MinerAddress:   addr,
		FormulaVersion: 3,
	})
	el2 := time.Since(t1)
	if err != nil {
		t.Fatalf("SearchPoH large: %v", err)
	}
	ghs := float64(batch) / el2.Seconds() / 1e9
	t.Logf("blake3 batch=%d found=%v nonce=%d elapsed=%s ~%.3f GH/s", batch, ok2, n2, el2.Round(time.Millisecond), ghs)
	if ok2 {
		if pohWorkBlake3Mirror(prev, addr, n2)%10007 != 0 {
			t.Fatalf("large-batch nonce %d fails host check", n2)
		}
		// Host must also see a hit at or before n2 in the same window.
		hostOK, hostN, err := Blake3Search(context.Background(), 0, n2+1, 10007, prev, addr)
		if err != nil || !hostOK || hostN != n2 {
			t.Fatalf("host re-search mismatch: ok=%v n=%d err=%v want %d", hostOK, hostN, err, n2)
		}
	}
}

func TestBlake3CUDADeviceForcedHostFallbackStillCorrect(t *testing.T) {
	t.Setenv("HACKME_CUDA_BLAKE3_HOST", "1")
	accs, err := DiscoverAccelerators()
	if err != nil || len(accs) == 0 {
		t.Skip("no CUDA")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()
	prev := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	addr := "HMC-hostfallback000"
	mod := uint64(251)
	ok, nonce, err := accs[0].SearchPoH(context.Background(), SearchRequest{
		Base: 0, Count: 1 << 18, Mod: mod, PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok && pohWorkBlake3Mirror(prev, addr, nonce)%mod != 0 {
		t.Fatal("host-fallback hit invalid")
	}
}

func TestBlake3HostBenchGHS(t *testing.T) {
	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-benchhost000001"
	const batch = uint64(1 << 22)
	t0 := time.Now()
	ok, n, err := Blake3Search(context.Background(), 0, batch, 10_007, prev, addr)
	el := time.Since(t0)
	if err != nil {
		t.Fatal(err)
	}
	ghs := float64(batch) / el.Seconds() / 1e9
	t.Logf("host Blake3Search batch=%d found=%v nonce=%d elapsed=%s ~%.3f GH/s", batch, ok, n, el.Round(time.Millisecond), ghs)
}
