//go:build opencl

package gpupoh

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestBlake3OpenCLDeviceMatchesHost locks device poh_search_blake3 against host
// Blake3Search on a live OpenCL GPU (NVIDIA ICD, AMD, or Intel).
func TestBlake3OpenCLDeviceMatchesHost(t *testing.T) {
	_ = os.Setenv("HACKME_FORCE_OPENCL", "1")
	_ = os.Setenv("HACKME_GPU_BACKEND", "opencl")
	_ = os.Setenv("HACKME_OPENCL_VERBOSE", "1")
	_ = os.Unsetenv("HACKME_OPENCL_BLAKE3_HOST")

	accs, err := DiscoverAccelerators()
	if err != nil {
		t.Fatalf("DiscoverAccelerators: %v", err)
	}
	var pick Accelerator
	for _, a := range accs {
		if a != nil && a.Backend() == "opencl" {
			pick = a
			break
		}
	}
	if pick == nil {
		t.Skip("no OpenCL accelerators")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()

	oa, ok := pick.(*oclAccel)
	if !ok || !oa.haveB3 {
		t.Fatal("OpenCL accelerator missing Blake3 device kernel")
	}

	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-oclparity0000001"
	const mod = uint64(97)
	const scan = uint64(1 << 20)

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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t0 := time.Now()
	res, err := pick.(ShareSearcher).SearchPoHCollect(ctx, SearchRequest{
		Base:           0,
		Count:          hostHit + 1,
		Mod:            mod,
		PrevHash:       prev,
		MinerAddress:   addr,
		FormulaVersion: 3,
	})
	elapsed := time.Since(t0)
	if err != nil {
		t.Fatalf("SearchPoHCollect blake3: %v", err)
	}
	if !res.Found {
		t.Fatal("device search found=false (expected hit)")
	}
	if res.Nonce != hostHit {
		t.Fatalf("device nonce=%d hostHit=%d (first-hit mismatch)", res.Nonce, hostHit)
	}
	if res.Scanned == 0 || res.Scanned > hostHit+1 {
		t.Fatalf("Scanned=%d want in (0, %d]", res.Scanned, hostHit+1)
	}
	if pohWorkBlake3Mirror(prev, addr, res.Nonce)%mod != 0 {
		t.Fatal("device nonce fails host Blake3 work check")
	}
	t.Logf("opencl device hit nonce=%d scanned=%d in %s (~%.2f MH/s) label=%s",
		res.Nonce, res.Scanned, elapsed.Round(time.Millisecond),
		float64(res.Scanned)/elapsed.Seconds()/1e6, pick.Label())

	// Share collect: share_mod == mod so the block hit is also a share.
	res2, err := pick.(ShareSearcher).SearchPoHCollect(ctx, SearchRequest{
		Base: 0, Count: hostHit + 1, Mod: mod, ShareMod: mod, MaxShares: 8,
		PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	})
	if err != nil {
		t.Fatalf("SearchPoHCollect shares: %v", err)
	}
	if !res2.Found || res2.Nonce != hostHit {
		t.Fatalf("share-pass found=%v nonce=%d want %d", res2.Found, res2.Nonce, hostHit)
	}
	if len(res2.Shares) == 0 {
		t.Fatal("expected in-kernel shares")
	}
	foundShare := false
	for _, s := range res2.Shares {
		if s == hostHit {
			foundShare = true
		}
		if pohWorkBlake3Mirror(prev, addr, s)%mod != 0 {
			t.Fatalf("share nonce %d fails host check", s)
		}
	}
	if !foundShare {
		t.Fatalf("block hit %d missing from shares %v", hostHit, res2.Shares)
	}
}

func TestBlake3OpenCLDeviceForcedHostFallbackStillCorrect(t *testing.T) {
	t.Setenv("HACKME_FORCE_OPENCL", "1")
	t.Setenv("HACKME_GPU_BACKEND", "opencl")
	t.Setenv("HACKME_OPENCL_BLAKE3_HOST", "1")
	accs, err := DiscoverAccelerators()
	if err != nil || len(accs) == 0 {
		t.Skip("no OpenCL")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()
	var pick Accelerator
	for _, a := range accs {
		if a != nil && a.Backend() == "opencl" {
			pick = a
			break
		}
	}
	if pick == nil {
		t.Skip("no OpenCL accelerator")
	}
	prev := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	addr := "HMC-oclhostfallback0"
	mod := uint64(251)
	ok, nonce, err := pick.SearchPoH(context.Background(), SearchRequest{
		Base: 0, Count: 1 << 18, Mod: mod, PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok && pohWorkBlake3Mirror(prev, addr, nonce)%mod != 0 {
		t.Fatal("host-fallback hit invalid")
	}
}

func TestBlake3OpenCLScannedChunksFullLease(t *testing.T) {
	t.Setenv("HACKME_FORCE_OPENCL", "1")
	t.Setenv("HACKME_GPU_BACKEND", "opencl")
	t.Setenv("HACKME_OPENCL_BLAKE3_HOST", "")
	_ = os.Unsetenv("HACKME_OPENCL_BLAKE3_HOST")

	accs, err := DiscoverAccelerators()
	if err != nil || len(accs) == 0 {
		t.Skip("no OpenCL")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()
	var pick *oclAccel
	for _, a := range accs {
		if oa, ok := a.(*oclAccel); ok && oa.haveB3 {
			pick = oa
			break
		}
	}
	if pick == nil {
		t.Skip("no OpenCL Blake3 device")
	}

	// Count just above maxBatch would be huge; instead verify Scanned == Count on a
	// mid-size range with no early exit (mod huge → no hit).
	const count = uint64(1 << 18)
	res, err := pick.SearchPoHCollect(context.Background(), SearchRequest{
		Base: 0, Count: count, Mod: 1<<62 | 1, // odd huge mod → almost never hits
		PrevHash: "aa", MinerAddress: "HMC-scan", FormulaVersion: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != count {
		t.Fatalf("Scanned=%d want %d (no silent truncate)", res.Scanned, count)
	}
}
