package gpupoh

// blake3ModDiv packs a Lemire / Montgomery-style exact divisibility check for
// work % mod == 0 without a 64-bit GPU division.
//
// Write mod = odd * 2^tz. Then work is divisible by mod iff:
//
//	(work & ((1<<tz)-1)) == 0 && (work>>tz) * inv <= lim
//
// with inv = odd⁻¹ (mod 2⁶⁴) and lim = floor((2⁶⁴−1)/odd).
type blake3ModDiv struct {
	Inv uint64
	Lim uint64
	Tz  uint32 // trailing zero bits of mod (0..63); mod==0 rejected by caller
}

func prepareBlake3ModDiv(mod uint64) (blake3ModDiv, bool) {
	if mod == 0 {
		return blake3ModDiv{}, false
	}
	tz := uint32(0)
	odd := mod
	for odd&1 == 0 {
		odd >>= 1
		tz++
	}
	inv := modInverseOddUint64(odd)
	lim := ^uint64(0) / odd
	return blake3ModDiv{Inv: inv, Lim: lim, Tz: tz}, true
}

// modInverseOddUint64 returns x such that odd*x ≡ 1 (mod 2⁶⁴). odd must be odd.
func modInverseOddUint64(odd uint64) uint64 {
	// Newton iteration for 2-adics; starts from inverse mod 2⁸.
	x := odd
	// odd*odd ≡ 1 (mod 8) for odd odd; refine.
	x = x * (2 - odd*x) // mod 2^4… grows; classic doubling
	x = x * (2 - odd*x)
	x = x * (2 - odd*x)
	x = x * (2 - odd*x)
	x = x * (2 - odd*x) // 2^64
	return x
}

func workDivisibleByMod(work uint64, d blake3ModDiv) bool {
	if d.Tz > 0 {
		if work&((uint64(1)<<d.Tz)-1) != 0 {
			return false
		}
		work >>= d.Tz
	}
	return work*d.Inv <= d.Lim
}
