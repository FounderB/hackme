package chain

import (
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
)

// DefaultPoHTargetMod is the initial PoH modulus M after genesis (eval(nonce)%M==0).
const DefaultPoHTargetMod uint64 = 1_000_000

// PoHRetargetTargetSec is the desired average wall-clock seconds between PoH blocks.
const PoHRetargetTargetSec int64 = 30

// PoHRetargetWindowBlocks — retarget runs when the new tip height is a positive multiple
// of this value (e.g. 5, 10, 15…), comparing wall time across the last window of blocks.
const PoHRetargetWindowBlocks int64 = 5

const (
	// PoHTargetMinMod is the lower bound for PoH modulus.
	PoHTargetMinMod uint64 = 251
	// PoHTargetMaxMod is the upper bound for PoH modulus.
	// Raised to support larger aggregate hashrate before saturation.
	PoHTargetMaxMod uint64 = 10_000_000_000_000

	// Per-window retarget change limits to reduce oscillations.
	poHRetargetMaxStepUp   = 4.0
	poHRetargetMaxStepDown = 1.0 / 4.0

	// Per-block micro-adjust limits for faster adaptation between full windows.
	// Helps absorb sudden hashrate spikes/drops without waiting full retarget window.
	poHRetargetMicroMaxStepUp   = 1.35
	poHRetargetMicroMaxStepDown = 1.0 / 1.35

	pohRetargetMinMod = PoHTargetMinMod
	pohRetargetMaxMod = PoHTargetMaxMod
)

// PoHFormulaV2ActivationHeight is a legacy stub gate for synthetic_poh_v2 (mix64).
// Mainnet activates Blake3 v3 earlier via PoHFormulaV3ActivationHeight; v2 remains
// reachable only when V3 is overridden above this height (lab).
const PoHFormulaV2ActivationHeight uint64 = 2_102_400

// defaultPoHFormulaV3ActivationHeight is Blake3 PoH v3 mainnet gate (rc17.4).
// Moved earlier than 500k so free algebraic mining ends sooner; fleet must
// ship Blake3-capable workerpoh before tip reaches this height.
// Lab override: HACKME_POH_V3_ACTIVATION_HEIGHT=<uint64>.
const defaultPoHFormulaV3ActivationHeight uint64 = 485_000

var (
	pohV3HeightOnce sync.Once
	pohV3Height     uint64
)

// PoHFormulaV3ActivationHeight returns the height at which Blake3 PoH v3 activates
// for the next block index (index >= height ⇒ v3).
func PoHFormulaV3ActivationHeight() uint64 {
	pohV3HeightOnce.Do(func() {
		pohV3Height = defaultPoHFormulaV3ActivationHeight
		if v := strings.TrimSpace(os.Getenv("HACKME_POH_V3_ACTIVATION_HEIGHT")); v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				pohV3Height = n
			}
		}
	})
	return pohV3Height
}

// ResetPoHFormulaV3ActivationHeightForTest clears the cached env-resolved height (tests only).
func ResetPoHFormulaV3ActivationHeightForTest() {
	pohV3HeightOnce = sync.Once{}
	pohV3Height = 0
}

// ActivePoHTaskKindForIndex selects the baseline PoH profile for a target block height.
func ActivePoHTaskKindForIndex(index uint64) TaskKind {
	if index >= PoHFormulaV3ActivationHeight() {
		return TaskKindSyntheticPoHV3
	}
	if index >= PoHFormulaV2ActivationHeight {
		return TaskKindSyntheticPoHV2
	}
	return TaskKindSyntheticPoH
}

// PoHFormulaVersionForIndex returns 1, 2, or 3 for metrics/claim payloads.
func PoHFormulaVersionForIndex(index uint64) int {
	switch ActivePoHTaskKindForIndex(index) {
	case TaskKindSyntheticPoHV3:
		return 3
	case TaskKindSyntheticPoHV2:
		return 2
	default:
		return 1
	}
}

// PoHFormulaLabelForKind returns a human-readable formula label for block payloads/UI.
func PoHFormulaLabelForKind(kind TaskKind) string {
	switch kind {
	case TaskKindSyntheticPoHV3:
		return "blake3_v3(prev||miner||nonce_le)%M==0"
	case TaskKindSyntheticPoHV2:
		return "eval_v2(n)=mix64(11n+17)"
	default:
		return "eval_v1(n)=n*7+13"
	}
}

// PoHFormulaLabelForIndex returns the active formula label for a target block height.
func PoHFormulaLabelForIndex(index uint64) string {
	return PoHFormulaLabelForKind(ActivePoHTaskKindForIndex(index))
}

// PohEval is the v1 baseline formula for compatibility with older call sites/tests.
// It matches embedded WASM eval(i64)->i64: n*7+13.
func PohEval(n uint64) uint64 {
	return n*7 + 13
}

// PohEvalV2 is the legacy mix64 baseline profile (stub; superseded by Blake3 v3 on mainnet).
func PohEvalV2(n uint64) uint64 {
	x := n*11 + 17
	x ^= x >> 23
	x *= 0x9e3779b185ebca87
	x ^= x >> 29
	return x + 0x7f4a7c15
}

// PohEvalByKind evaluates nonce with a specific baseline profile.
// TaskKindSyntheticPoHV3 requires prev_hash + miner_address — use PohWorkForIndex instead.
func PohEvalByKind(kind TaskKind, n uint64) uint64 {
	switch kind {
	case TaskKindSyntheticPoHV2:
		return PohEvalV2(n)
	case TaskKindSyntheticPoHV3:
		// Cannot evaluate Blake3 without tip binding; callers must use PohWorkForIndex.
		return 0
	default:
		return PohEval(n)
	}
}

// PohEvalForIndex evaluates nonce using the baseline profile active at block index.
// For v3 this returns 0 — use PohWorkForIndex(prev, miner, ...).
func PohEvalForIndex(index, n uint64) uint64 {
	return PohEvalByKind(ActivePoHTaskKindForIndex(index), n)
}

// PohWorkForIndex returns the PoH work value that must satisfy work%M==0 at index.
func PohWorkForIndex(index, nonce uint64, prevHashHex, minerAddress string) uint64 {
	switch ActivePoHTaskKindForIndex(index) {
	case TaskKindSyntheticPoHV3:
		return PohWorkBlake3(prevHashHex, minerAddress, nonce)
	default:
		return PohEvalForIndex(index, nonce)
	}
}

// RetargetWindow returns the next modulus after a completed window of blocks.
// M_next = M_prev * T_ideal / T_actual (then clamped by caller):
//   - blocks faster than target (T_actual < T_ideal) → larger M (harder);
//   - slower → smaller M (easier).
//
// T_ideal = windowBlocks * targetSecPerBlock (e.g. 5 * 30s = 150s).
func RetargetWindow(prevMod uint64, actualSec, idealSec int64) uint64 {
	if prevMod < pohRetargetMinMod {
		prevMod = pohRetargetMinMod
	}
	if prevMod > pohRetargetMaxMod {
		prevMod = pohRetargetMaxMod
	}
	if idealSec < 1 {
		idealSec = 1
	}
	act := float64(actualSec)
	if actualSec < 1 {
		act = 1
	}
	ratio := float64(idealSec) / act
	// Limit per-window changes for smoother network adaptation.
	const maxRatio = poHRetargetMaxStepUp
	const minRatio = poHRetargetMaxStepDown
	if ratio > maxRatio {
		ratio = maxRatio
	}
	if ratio < minRatio {
		ratio = minRatio
	}
	nextF := float64(prevMod) * ratio
	if math.IsNaN(nextF) || math.IsInf(nextF, 0) {
		return pohRetargetMaxMod
	}
	if nextF >= float64(pohRetargetMaxMod) {
		return pohRetargetMaxMod
	}
	if nextF <= float64(pohRetargetMinMod) {
		return pohRetargetMinMod
	}
	return uint64(nextF + 0.5)
}

// RetargetMicroStep applies a bounded per-block adjustment using the latest block interval.
// This complements RetargetWindow and improves reaction speed to sudden hashrate shifts.
func RetargetMicroStep(prevMod uint64, actualSec, targetSec int64) uint64 {
	if prevMod < pohRetargetMinMod {
		prevMod = pohRetargetMinMod
	}
	if prevMod > pohRetargetMaxMod {
		prevMod = pohRetargetMaxMod
	}
	if targetSec < 1 {
		targetSec = 1
	}
	act := float64(actualSec)
	if actualSec < 1 {
		act = 1
	}
	ratio := float64(targetSec) / act
	if ratio > poHRetargetMicroMaxStepUp {
		ratio = poHRetargetMicroMaxStepUp
	}
	if ratio < poHRetargetMicroMaxStepDown {
		ratio = poHRetargetMicroMaxStepDown
	}
	nextF := float64(prevMod) * ratio
	if math.IsNaN(nextF) || math.IsInf(nextF, 0) {
		return pohRetargetMaxMod
	}
	if nextF >= float64(pohRetargetMaxMod) {
		return pohRetargetMaxMod
	}
	if nextF <= float64(pohRetargetMinMod) {
		return pohRetargetMinMod
	}
	return uint64(nextF + 0.5)
}

// ClampPoHTargetMod keeps a modulus within the allowed PoH retarget bounds.
// Pre-v3 (7n+13) also nudges M off multiples of 7 so algebraic PoH stays solvable.
// Blake3 v3 accepts any M in bounds; the nudge is harmless post-activation and
// keeps mixed-era nodes from stalling if retarget lands on 7|M before height gate.
func ClampPoHTargetMod(p uint64) uint64 {
	p = ClampPoHTargetModBounds(p)
	return sanitizePoHTargetMod7n13(p)
}

// ClampPoHTargetModBounds clamps M to [PoHTargetMinMod, PoHTargetMaxMod] only.
// Pool coordinators use this after Blake3 v3 (7n+13 solvability is obsolete there).
func ClampPoHTargetModBounds(p uint64) uint64 {
	if p < pohRetargetMinMod {
		return pohRetargetMinMod
	}
	if p > pohRetargetMaxMod {
		return pohRetargetMaxMod
	}
	return p
}

// IsPoHTargetModAtCap reports whether M is at the configured upper bound.
func IsPoHTargetModAtCap(m uint64) bool {
	return ClampPoHTargetModBounds(m) >= pohRetargetMaxMod
}

// sanitizePoHTargetMod7n13 nudges M off multiples of 7 while staying in bounds.
// Kept for chain meta / pre-v3 algebraic PoH solvability (report #29).
func sanitizePoHTargetMod7n13(m uint64) uint64 {
	if m%7 != 0 {
		return m
	}
	if m < pohRetargetMaxMod {
		return m + 1 // m≡0 (mod 7) ⇒ m+1 ≢ 0 (mod 7)
	}
	if m > pohRetargetMinMod {
		return m - 1
	}
	return pohRetargetMinMod
}
