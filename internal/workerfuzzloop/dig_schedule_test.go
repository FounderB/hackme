package workerfuzzloop

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleDigBackpressureAndBoost(t *testing.T) {
	var poh, calib atomic.Int64
	calib.Store(100_000) // 100 GH/s
	cfg := Config{
		LogPrefix:            "test",
		PohGHSMilli:          &poh,
		CalibGHSMilli:        &calib,
		BackpressureFloorPct: 35,
		DigBoostFloorPct:     70,
	}
	var st Stats

	poh.Store(20_000) // 20% — below floor
	s := ScheduleDig(cfg, &st)
	if !s.PausedBack || s.Pause != 5*time.Second {
		t.Fatalf("expected pause: %+v", s)
	}
	if st.PausedBack.Load() != 1 {
		t.Fatalf("paused counter=%d", st.PausedBack.Load())
	}

	poh.Store(50_000) // 50% — mid band
	s = ScheduleDig(cfg, &st)
	if s.Pause != 0 || s.Boosted || s.GapScale != 1 {
		t.Fatalf("mid band: %+v", s)
	}

	poh.Store(80_000) // 80% — boost
	s = ScheduleDig(cfg, &st)
	if !s.Boosted || s.GapScale != DigGapBoostScale || s.Pause != 0 {
		t.Fatalf("boost: %+v", s)
	}
	if st.DigBoosted.Load() != 1 {
		t.Fatalf("boost counter=%d", st.DigBoosted.Load())
	}
}

func TestScheduleDigDisabledWithoutSignals(t *testing.T) {
	cfg := Config{BackpressureFloorPct: 35}
	s := ScheduleDig(cfg, nil)
	if s.Pause != 0 || s.Boosted {
		t.Fatalf("%+v", s)
	}
}

func TestScheduleDigBoostDisabledHighPct(t *testing.T) {
	var poh, calib atomic.Int64
	calib.Store(100_000)
	poh.Store(95_000)
	cfg := Config{
		PohGHSMilli:          &poh,
		CalibGHSMilli:        &calib,
		BackpressureFloorPct: 10,
		DigBoostFloorPct:     101, // >100 disables boost
	}
	s := ScheduleDig(cfg, &Stats{})
	if s.Boosted {
		t.Fatalf("boost should be off: %+v", s)
	}
}
