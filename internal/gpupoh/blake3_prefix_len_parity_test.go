//go:build cuda

package gpupoh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Child entry: HACKME_BLAKE3_PREFIX_LEN=<n> runs one length in a fresh process so a
// prior CUDA IllegalAddress cannot poison later lengths (dead context → silent CPU).
func TestBlake3CUDAPrefixLenChild(t *testing.T) {
	raw := os.Getenv("HACKME_BLAKE3_PREFIX_LEN")
	if raw == "" {
		t.Skip("child only; set HACKME_BLAKE3_PREFIX_LEN")
	}
	plen, err := strconv.Atoi(raw)
	if err != nil || plen < 0 || plen > 160 {
		t.Fatalf("bad HACKME_BLAKE3_PREFIX_LEN=%q", raw)
	}
	runBlake3PrefixLenParity(t, plen)
}

// TestBlake3CUDAPrefixLenParitySweep spawns one process per prefix length.
// Default: production (84) + regression band around the stail_words OOB (120–160).
// Full 0–160: HACKME_BLAKE3_PREFIX_SWEEP=1 (or blake3bench -prefix-sweep).
func TestBlake3CUDAPrefixLenParitySweep(t *testing.T) {
	if os.Getenv("HACKME_BLAKE3_PREFIX_LEN") != "" {
		t.Skip("running as child")
	}
	accs, err := DiscoverAccelerators()
	if err != nil || len(accs) == 0 {
		t.Skip("no CUDA")
	}
	for _, a := range accs {
		_ = a.Close()
	}

	lengths := []int{0, 64, 84, 120, 128, 132, 133, 134, 140, 152, 153, 160}
	if os.Getenv("HACKME_BLAKE3_PREFIX_SWEEP") == "1" {
		lengths = make([]int, 0, 161)
		for i := 0; i <= 160; i++ {
			lengths = append(lengths, i)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, plen := range lengths {
		plen := plen
		t.Run(fmt.Sprintf("len_%03d", plen), func(t *testing.T) {
			cmd := exec.Command(exe, "-test.run=^TestBlake3CUDAPrefixLenChild$", "-test.v", "-test.count=1")
			cmd.Env = append(os.Environ(),
				"HACKME_BLAKE3_PREFIX_LEN="+strconv.Itoa(plen),
				"HACKME_CUDA_VERBOSE=1",
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("plen=%d: %v\n%s", plen, err, out)
			}
			t.Logf("plen=%d ok\n%s", plen, out)
		})
	}
}

func runBlake3PrefixLenParity(t *testing.T, plen int) {
	t.Helper()
	_ = os.Unsetenv("HACKME_CUDA_BLAKE3_HOST")

	accs, err := DiscoverAccelerators()
	if err != nil {
		t.Fatalf("DiscoverAccelerators: %v", err)
	}
	if len(accs) == 0 {
		t.Skip("no CUDA")
	}
	defer func() {
		for _, a := range accs {
			_ = a.Close()
		}
	}()

	prev, addr := prefixPartsForLen(plen)
	if got := len(prev) + len(addr); got != plen {
		t.Fatalf("prefixPartsForLen(%d)=%d", plen, got)
	}

	const mod = uint64(97)
	const scan = uint64(1 << 18)

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
		t.Fatalf("plen=%d: no host hit in %d nonces", plen, scan)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ok, nonce, err := accs[0].SearchPoH(ctx, SearchRequest{
		Base: 0, Count: hostHit + 1, Mod: mod,
		PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	})
	if err != nil {
		t.Fatalf("plen=%d SearchPoH: %v", plen, err)
	}
	if !ok {
		t.Fatalf("plen=%d: device found=false want nonce=%d", plen, hostHit)
	}
	if nonce != hostHit {
		t.Fatalf("plen=%d: device nonce=%d hostHit=%d", plen, nonce, hostHit)
	}
	if pohWorkBlake3Mirror(prev, addr, nonce)%mod != 0 {
		t.Fatalf("plen=%d: nonce fails host work check", plen)
	}
	t.Logf("plen=%d hit nonce=%d label=%s", plen, nonce, accs[0].Label())
}

// prefixPartsForLen builds PrevHash||MinerAddress of exact byte length plen.
// PrevHash is lower-hex-ish ASCII so ToLower is a no-op; addr is printable UTF-8.
func prefixPartsForLen(plen int) (prev, addr string) {
	if plen <= 0 {
		return "", ""
	}
	// Prefer tip-shaped prev (64) when long enough — matches production layout.
	if plen <= 64 {
		return strings.Repeat("a", plen), ""
	}
	prev = strings.Repeat("b", 64)
	addr = strings.Repeat("X", plen-64)
	return prev, addr
}
