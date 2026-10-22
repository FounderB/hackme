package gpupoh

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zeebo/blake3"
)

// TestBlake3CUDAKernelSourceMatchesHostEncoding locks the device kernel contract
// against host Blake3Search / chain.PohWorkBlake3 before height 500000.
// Full NVRTC device↔host smoke needs -tags cuda + toolkit; this guards the .cu source.
func TestBlake3CUDAKernelSourceMatchesHostEncoding(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	cuPath := filepath.Join(filepath.Dir(thisFile), "poh_search.cu")
	raw, err := os.ReadFile(cuPath)
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
		"atomicMin(out_nonce",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("poh_search.cu missing %q (Blake3 device contract drift)", want)
		}
	}

	// Host encoding golden: LE nonce + LE work uint64 from digest[0:8].
	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-parity"
	nonce := uint64(0x0102030405060708)
	msg := append(append([]byte{}, []byte(strings.ToLower(prev))...), []byte(addr)...)
	var nb [8]byte
	binary.LittleEndian.PutUint64(nb[:], nonce)
	msg = append(msg, nb[:]...)
	sum := blake3.Sum256(msg)
	work := binary.LittleEndian.Uint64(sum[0:8])
	if work != pohWorkBlake3Mirror(prev, addr, nonce) {
		t.Fatal("host mirror mismatch")
	}
	if work == 0 {
		t.Fatal("unexpected zero work")
	}
}
