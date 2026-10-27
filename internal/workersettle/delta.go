package workersettle

// AccrualDelta computes unpaid settle amount in units after coordinator resets.
//
// Rules (same as legacy settle_worker_payouts.sh):
//   - Never lower SettledUnits (lifetime watermark only moves up).
//   - When SettledUnits > AccruedUnits (coordinator restart), keep watermark and
//     pay only session growth above SessionAnchorUnits (re-anchored if needed).
//   - When accrual has caught up, clear the session anchor.
type AccrualDelta struct {
	BaselineUnits  uint64
	DeltaUnits     uint64
	NeedAnchor     bool
	NewAnchorUnits uint64
	ClearAnchor    bool
	Rebase         bool
}

// ComputeAccrualDelta returns unpaid delta and any session-anchor mutation.
func ComputeAccrualDelta(accrued, settled, sessionAnchor uint64) AccrualDelta {
	var out AccrualDelta
	if settled > accrued {
		out.Rebase = true
		if sessionAnchor == 0 || accrued < sessionAnchor {
			out.NeedAnchor = true
			out.NewAnchorUnits = accrued
			sessionAnchor = accrued
		}
		out.BaselineUnits = sessionAnchor
	} else {
		if sessionAnchor > 0 {
			out.ClearAnchor = true
		}
		out.BaselineUnits = settled
	}
	if accrued > out.BaselineUnits {
		out.DeltaUnits = accrued - out.BaselineUnits
	}
	return out
}

// ShouldSettle reports whether delta meets the configured floor.
func ShouldSettle(delta, minUnits uint64, force bool, forceMinUnits uint64) bool {
	if delta >= minUnits {
		return true
	}
	return force && delta >= forceMinUnits
}

// CapChunk clamps a payout to treasury headroom and max chunk size.
func CapChunk(delta, balance, reserve, maxChunk uint64) uint64 {
	if balance <= reserve {
		return 0
	}
	avail := balance - reserve
	capU := avail
	if maxChunk > 0 && maxChunk < capU {
		capU = maxChunk
	}
	if delta < capU {
		return delta
	}
	return capU
}
