package chain

import (
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ASERT difficulty for PoH (absolutely scheduled exponentially rising targets, the
// aserti3-2d scheme Bitcoin Cash has used since 2020), replacing the per-block micro step,
// the 5-block window and the wall-clock stall easing from the activation height on.
//
// Why: the old three-mechanism retarget oscillated (M swung 251…1.8e8, ~48% of blocks hit the
// +35% cap, ~10% were cut by node-local stall easing written to meta outside of blocks) and
// settled at ~27.7 s by accident. ASERT holds the absolute schedule — 30 s per block on average,
// so halving dates are real — and the next M is a pure integer function of block headers,
// so any follower or verifier can recompute it (the stall easing could not be).
//
//	M(n) = anchorMod · 2^((T·(h_parent − h_anchor) − (t_parent − t_anchor)) / halflife)
//
// Ahead of schedule (parent earlier than T per block since the anchor) ⇒ exponent > 0 ⇒ harder.
const (
	// PoHASERTTargetSec is the scheduled seconds per block.
	PoHASERTTargetSec int64 = 30
	// PoHASERTHalfLifeSec: being one half-life ahead of (behind) schedule doubles (halves) M.
	PoHASERTHalfLifeSec int64 = 3600
	// PoHASERTAnchorMod is M for the activation block. A fixed constant (median of mainnet block
	// moduli just before activation), not the previous block's M: the legacy retarget sawtooth
	// can leave a single block at M=251, which would mint thousands of instant blocks.
	PoHASERTAnchorMod uint64 = 17_500_000
	// PoHASERTMaxFutureSec bounds how far a block timestamp may run ahead of the validator clock.
	PoHASERTMaxFutureSec int64 = 120

	defaultPoHASERTActivationHeight uint64 = 495_400

	asertRadixBits = 16
	asertRadix     = int64(1) << asertRadixBits
)

var (
	pohASERTHeightOnce sync.Once
	pohASERTHeight     uint64
)

// PoHASERTActivationHeight is the first block index whose M comes from ASERT.
// Lab override: HACKME_POH_ASERT_ACTIVATION_HEIGHT=<uint64> (must be ≥ 1).
func PoHASERTActivationHeight() uint64 {
	pohASERTHeightOnce.Do(func() {
		pohASERTHeight = defaultPoHASERTActivationHeight
		if v := strings.TrimSpace(os.Getenv("HACKME_POH_ASERT_ACTIVATION_HEIGHT")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil && n >= 1 {
				pohASERTHeight = n
			}
		}
	})
	return pohASERTHeight
}

// ResetPoHASERTActivationHeightForTest clears the cached env-resolved height (tests only).
func ResetPoHASERTActivationHeightForTest() {
	pohASERTHeightOnce = sync.Once{}
	pohASERTHeight = 0
}

// ASERTActiveForIndex reports whether block index uses ASERT difficulty.
func ASERTActiveForIndex(index uint64) bool {
	return index >= 1 && index >= PoHASERTActivationHeight()
}

// floorDiv is integer division rounding toward −∞ (Go's / truncates toward zero).
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// asertPow2Frac returns 2^(frac/65536)·65536 for frac in [0, 65536), using the aserti3-2d cubic
// approximation (max relative error ≈ 0.013%), evaluated exactly in integers.
func asertPow2Frac(frac uint64) *big.Int {
	f := new(big.Int).SetUint64(frac)
	f2 := new(big.Int).Mul(f, f)
	f3 := new(big.Int).Mul(f2, f)
	acc := new(big.Int).Mul(big.NewInt(195_766_423_245_049), f)
	acc.Add(acc, new(big.Int).Mul(big.NewInt(971_821_376), f2))
	acc.Add(acc, new(big.Int).Mul(big.NewInt(5_127), f3))
	acc.Add(acc, new(big.Int).Lsh(big.NewInt(1), 47))
	acc.Rsh(acc, 48)
	return acc.Add(acc, big.NewInt(asertRadix))
}

// ASERTTargetMod returns M for block parentHeight+1. Pure integer arithmetic: identical on every
// platform. parentHeight must be ≥ anchorHeight. The result is clamped to the PoH bounds.
func ASERTTargetMod(anchorMod, anchorHeight uint64, anchorTime int64, parentHeight uint64, parentTime int64) uint64 {
	if anchorMod == 0 {
		anchorMod = PoHASERTAnchorMod
	}
	if parentHeight < anchorHeight {
		parentHeight = anchorHeight
	}
	dh := int64(parentHeight - anchorHeight)
	dt := parentTime - anchorTime
	// Fixed-point exponent (16 fractional bits). |num| stays far below 2^47 for any real chain.
	num := PoHASERTTargetSec*dh - dt
	exp := floorDiv(num*asertRadix, PoHASERTHalfLifeSec)
	shifts := exp >> asertRadixBits // arithmetic shift = floor
	frac := uint64(exp & (asertRadix - 1))

	// anchorMod (< 2^64) × factor (< 2^17): beyond ±80 shifts the clamp decides anyway.
	if shifts > 80 {
		return ClampPoHTargetMod(pohRetargetMaxMod)
	}
	if shifts < -80 {
		return ClampPoHTargetMod(pohRetargetMinMod)
	}
	m := new(big.Int).SetUint64(anchorMod)
	m.Mul(m, asertPow2Frac(frac))
	if shifts >= 0 {
		m.Lsh(m, uint(shifts))
	} else {
		m.Rsh(m, uint(-shifts))
	}
	m.Rsh(m, asertRadixBits)
	if !m.IsUint64() || m.Uint64() > pohRetargetMaxMod {
		return ClampPoHTargetMod(pohRetargetMaxMod)
	}
	return ClampPoHTargetMod(m.Uint64())
}
