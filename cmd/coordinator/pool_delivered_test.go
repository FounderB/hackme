package main

import (
	"math"
	"testing"
	"time"
)

func TestDeliveredPoolGHFromShareWork(t *testing.T) {
	m := &workManager{}
	const t0 = int64(1_000_000)
	if _, ok := m.deliveredPoolGHLocked(t0); ok {
		t.Fatal("empty history must not report delivered GH/s")
	}
	// 5 GH/s delivered: 50e9 expected hashes every 10s via shares of share_mod 1e9.
	for i := int64(0); i <= 60; i++ {
		m.noteShareWorkLocked(50, 1_000_000_000, t0+i*10)
	}
	gh, ok := m.deliveredPoolGHLocked(t0 + 600)
	if !ok || math.Abs(gh-5) > 0.01 {
		t.Fatalf("delivered=%v ok=%v want ~5 GH/s", gh, ok)
	}
	if n := len(m.shareWorkSamples); n > int(deliveredGHWindowSec/deliveredSampleEverySec)+2 {
		t.Fatalf("samples not trimmed to window: %d", n)
	}
}

func TestDeliveredPoolGHNeedsMinSpan(t *testing.T) {
	m := &workManager{}
	m.noteShareWorkLocked(10, 1_000_000_000, 100)
	m.noteShareWorkLocked(10, 1_000_000_000, 150)
	if _, ok := m.deliveredPoolGHLocked(150 + deliveredGHMinSpanSec - 51); ok {
		t.Fatal("span below minimum must fall back to device GH/s")
	}
}

func TestDeliveredPoolGHQuietPoolDecays(t *testing.T) {
	m := &workManager{}
	for i := int64(0); i <= 60; i++ {
		m.noteShareWorkLocked(50, 1_000_000_000, i*10)
	}
	// No submits for 20 minutes: the rate over the true span must fall, not freeze at 5 GH/s.
	gh, ok := m.deliveredPoolGHLocked(600 + 1200)
	if !ok || gh >= 2 {
		t.Fatalf("quiet pool delivered=%v ok=%v, want well below 5", gh, ok)
	}
}

func TestEnrichPoolStatsPrefersDeliveredOverDeviceGH(t *testing.T) {
	now := time.Now().Unix()
	wm := &workManager{
		worker: map[string]workerPayoutStat{
			// Device-busy GH/s claims 15 GH/s, like a GPU rig idling on RTT between short leases.
			"rig-gpu": {LastHashrateGHS: 15, LastSeenUnix: now, LastPoHSeenUnix: now},
		},
	}
	for i := int64(-600); i <= 0; i += 10 {
		wm.noteShareWorkLocked(30, 1_000_000_000, now+i) // 3 GH/s actually delivered
	}
	out := map[string]any{}
	enrichPoolStatsForPublic(out, nil, wm)
	gh, _ := out["pool_hashrate_gh_s"].(float64)
	if math.Abs(gh-3) > 0.2 {
		t.Fatalf("public pool_hashrate_gh_s=%v want delivered ~3", gh)
	}
	if dev, _ := out["pool_hashrate_gh_s_device"].(float64); dev < 14 {
		t.Fatalf("device GH/s must stay visible separately, got %v", dev)
	}
	if out["pool_hashrate_kind"] != "delivered_verified_shares_10m" {
		t.Fatalf("kind=%v", out["pool_hashrate_kind"])
	}
}

// Solvency invariant: with clamped M from poolLoadTargetM, full-fleet pay per day (GH·1e9·86400·base/M)
// never exceeds daily emission base·86400/block_sec — independent of fleet size. PR #37 (M from
// GH·modMin) broke this by ~15,000× and only the daily budget kept the pool solvent.
func TestPoolLoadTargetMPayNeverExceedsEmission(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_POOL_BLOCK_SEC", "30")
	t.Setenv("HACKME_COORDINATOR_POOL_LOAD_SOLVENCY", "")
	const base = 0.01
	emission := base * 86400 / 30
	wm := &workManager{}
	for _, gh := range []float64{0.05, 0.5, 3, 15, 120, 2000, 20000} {
		for _, miners := range []int{1, 28, 1000} {
			M := wm.clampTargetMod(poolLoadTargetM(gh, miners, poolTargetModMin, true))
			if M == 0 {
				t.Fatalf("gh=%v miners=%d: zero M", gh, miners)
			}
			pay := gh * 1e9 * 86400 * base / float64(M)
			if pay > emission*1.001 {
				t.Fatalf("gh=%v miners=%d M=%d pays %.2f HMC/day > emission %.2f", gh, miners, M, pay, emission)
			}
		}
	}
}
