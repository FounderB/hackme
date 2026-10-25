package gpupoh

import "testing"

func TestBlake3ModDivMatchesPercent(t *testing.T) {
	// Include lease-scaled share mods (~batch/32 up to multi-GB) used by SHARE_STRICT PPS.
	mods := []uint64{1, 2, 3, 97, 1_000_003, 20_618_406, 1 << 20, (1 << 20) + 1, 7 << 3,
		1 << 29, (1 << 30) + 16, 2 << 30, (64 << 30) / 32}
	for _, mod := range mods {
		d, ok := prepareBlake3ModDiv(mod)
		if !ok {
			t.Fatalf("mod=%d prepare failed", mod)
		}
		for _, w := range []uint64{0, 1, 2, 3, 7, 97, 1000, 1<<32 + 3, ^uint64(0) - 17, mod, mod * 3, mod*5 + 1} {
			want := w%mod == 0
			got := workDivisibleByMod(w, d)
			if got != want {
				t.Fatalf("mod=%d work=%d want %v got %v (tz=%d inv=%#x lim=%#x)",
					mod, w, want, got, d.Tz, d.Inv, d.Lim)
			}
		}
		// denser scan near multiples
		for i := uint64(0); i < 5000; i++ {
			w := i * (mod/2 + 1)
			if workDivisibleByMod(w, d) != (w%mod == 0) {
				t.Fatalf("scan mod=%d work=%d", mod, w)
			}
		}
	}
}

func TestModInverseOddUint64(t *testing.T) {
	for _, odd := range []uint64{1, 3, 5, 7, 9, 97, 1_000_003, 0xdeadbeefdeadbeef | 1} {
		inv := modInverseOddUint64(odd)
		if odd*inv != 1 {
			t.Fatalf("odd=%#x inv=%#x product=%#x", odd, inv, odd*inv)
		}
	}
}
