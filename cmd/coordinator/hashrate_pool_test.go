package main

import (
	"math"
	"testing"
)

func TestClampWorkerHashrateGHSAllowsRealGPUClass(t *testing.T) {
	// Real RTX-class OpenCL/CUDA calibrations land ~140–150 GH/s; the old 128 cap
	// silently clipped billing/retarget. Ceiling is 512 (500/512 family).
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{-1, 0},
		{math.NaN(), 0},
		{math.Inf(1), 0},
		{1.5, 1.5},
		{128, 128},
		{149.5, 149.5},
		{500, 500},
		{512, 512},
		{513, 512},
		{1000, 512},
	}
	for _, tc := range cases {
		got := clampWorkerHashrateGHS(tc.in)
		if math.IsNaN(tc.want) {
			if !math.IsNaN(got) {
				t.Fatalf("in=%v got %v want NaN", tc.in, got)
			}
			continue
		}
		if got != tc.want {
			t.Fatalf("in=%v got %v want %v", tc.in, got, tc.want)
		}
	}
	if maxWorkerHashrateGHS != 512 {
		t.Fatalf("maxWorkerHashrateGHS=%v want 512", maxWorkerHashrateGHS)
	}
}

func TestEffectiveWorkerHashrateGHSUsesPeakWhenEMACollapsed(t *testing.T) {
	st := workerPayoutStat{LastHashrateGHS: 0.002, PeakHashrateGHS: 36.4}
	got := effectiveWorkerHashrateGHS(st)
	want := 36.4 * 0.72
	if got < want*0.99 || got > want*1.01 {
		t.Fatalf("effective=%v want~%v", got, want)
	}
}

func TestEffectiveWorkerHashrateGHSKeepsLiveSample(t *testing.T) {
	st := workerPayoutStat{LastHashrateGHS: 28.5, PeakHashrateGHS: 36.0}
	if got := effectiveWorkerHashrateGHS(st); got != 28.5 {
		t.Fatalf("got %v want 28.5", got)
	}
}

func TestWorkerRateLimitPerMinGPUMinFloor(t *testing.T) {
	wm := &workManager{
		claimPerMin: 6000,
		worker: map[string]workerPayoutStat{
			"gpu-rig": {LastHashrateGHS: 0.002, PeakHashrateGHS: 0.004},
		},
	}
	lim := wm.workerRateLimitPerMin("gpu-rig", wm.claimPerMin)
	if lim < 120 {
		t.Fatalf("GPU pool rig should not be throttled to %d/min", lim)
	}
}

func TestSmoothWorkerHashrateGHSRecoversFromStuckLow(t *testing.T) {
	got := smoothWorkerHashrateGHS(0.002, 35.0)
	if got < 20 {
		t.Fatalf("expected fast recovery toward sample, got %v", got)
	}
}

func TestSubmitPrefersReportedHashrateOverWallGH(t *testing.T) {
	// Parent shells / desktop env must not force hybrid-strict on unit tests.
	t.Setenv("HACKME_POOL_HYBRID_SIGNER_ENABLED", "")
	t.Setenv("HACKME_POOL_HYBRID_SIGNER_STRICT", "")
	wm := newWorkManagerFromEnv()
	now := int64(1_800_000_000)
	base, batch, _, _, _, ok, _ := wm.claim("w-gh", 0)
	if !ok {
		t.Fatal("claim failed")
	}
	req := submitWorkRequest{
		WorkerID:    "w-gh",
		BaseNonce:   base,
		BatchSize:   batch,
		Attempts:    batch,
		HashrateGHS: 32.5,
	}
	_, reason, _, _, _ := wm.submit(req)
	if reason != "" {
		t.Fatalf("submit failed: %s", reason)
	}
	st := wm.worker["w-gh"]
	if st.LastHashrateGHS < 10 {
		t.Fatalf("coordinator should record reported GH/s, got %v", st.LastHashrateGHS)
	}
	_ = now
}
