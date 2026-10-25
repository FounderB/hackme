package gpupoh

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBlake3OpenCLKernelSourceMatchesHostEncoding locks the OpenCL .cl device
// contract against host Blake3Search / CUDA poh_search_blake3 semantics.
func TestBlake3OpenCLKernelSourceMatchesHostEncoding(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	clPath := filepath.Join(filepath.Dir(thisFile), "poh_search.cl")
	raw, err := os.ReadFile(clPath)
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, want := range []string{
		"poh_search_blake3",
		"nonce as 8-byte little-endian",
		"prev_hash_hex_ascii_lower",
		"miner_address_utf8",
		"digest[0]",
		"record_min_nonce",
		"atomic_inc(out_share_count",
		"out_shares[si] = n",
		"share_mod",
		"BLAKE3_NONCES_PER_THREAD",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("poh_search.cl missing %q (Blake3 device contract drift)", want)
		}
	}
	// Algebraic kernel must remain for fv<3.
	if !strings.Contains(src, "__kernel void poh_search(") {
		t.Fatal("poh_search.cl missing algebraic poh_search kernel")
	}
}
