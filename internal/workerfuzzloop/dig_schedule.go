package workerfuzzloop

import (
	"fmt"
	"os"
	"time"
)

// DigSchedule decides pause / claim-gap scaling from live PoH GH/s vs calibration.
// Protect GPU when GHS collapses; dig Dig/Hunt faster when hashrate is healthy.
type DigSchedule struct {
	Pause      time.Duration
	GapScale   float64 // 1 = normal MinClaimGap; <1 = more aggressive dig
	Boosted    bool
	PausedBack bool
}

// DigBoostFloorPct is the default calib % above which hybrid dig tightens claim gap.
const DigBoostFloorPct = 70

// DigGapBoostScale is MinClaimGap multiplier while boosted.
const DigGapBoostScale = 0.5

// ScheduleDig returns pause/boost from PoH hashrate signals on cfg.
func ScheduleDig(cfg Config, st *Stats) DigSchedule {
	out := DigSchedule{GapScale: 1}
	if cfg.BackpressureFloorPct <= 0 || cfg.PohGHSMilli == nil || cfg.CalibGHSMilli == nil {
		return out
	}
	calib := cfg.CalibGHSMilli.Load()
	cur := cfg.PohGHSMilli.Load()
	if calib < 1000 || cur <= 0 {
		return out
	}
	floorPct := cfg.BackpressureFloorPct
	if floorPct > 100 {
		floorPct = 100
	}
	floor := calib * int64(floorPct) / 100
	if floor < 1 {
		floor = 1
	}
	if cur < floor {
		if st != nil {
			st.PausedBack.Add(1)
		}
		fmt.Fprintf(os.Stderr, "%s: backpressure — PoH %.2f GH/s < %d%% of calib %.2f; pausing fuzz 5s\n",
			cfg.LogPrefix, float64(cur)/1000.0, floorPct, float64(calib)/1000.0)
		out.Pause = 5 * time.Second
		out.PausedBack = true
		return out
	}
	boostPct := cfg.DigBoostFloorPct
	if boostPct <= 0 {
		boostPct = DigBoostFloorPct
	}
	if boostPct <= 100 {
		boostFloor := calib * int64(boostPct) / 100
		if cur >= boostFloor {
			out.GapScale = DigGapBoostScale
			out.Boosted = true
			if st != nil {
				st.DigBoosted.Add(1)
			}
		}
	}
	return out
}
