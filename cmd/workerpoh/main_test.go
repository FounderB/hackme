package main

import (
	"math"
	"testing"
)

func TestEnvUint64(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want uint64
	}{
		{name: "unset returns fallback", set: "", want: 4194304},
		{name: "plain value", set: "GPU_CHUNK_TEST", val: "8388608", want: 8388608},
		{name: "whitespace trimmed", set: "GPU_CHUNK_TEST", val: " 16777216 ", want: 16777216},
		{name: "zero rejected", set: "GPU_CHUNK_TEST", val: "0", want: 4194304},
		{name: "negative rejected", set: "GPU_CHUNK_TEST", val: "-1", want: 4194304},
		{name: "garbage rejected", set: "GPU_CHUNK_TEST", val: "4M", want: 4194304},
		{name: "overflow rejected", set: "GPU_CHUNK_TEST", val: "99999999999999999999999", want: 4194304},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envUint64("GPU_CHUNK_TEST", 4194304); got != tc.want {
				t.Fatalf("envUint64 = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEnvIntMs(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want int
	}{
		{name: "unset returns fallback", set: "", want: 2500},
		{name: "plain value", set: "SEARCH_TIMEOUT_TEST", val: "12000", want: 12000},
		{name: "whitespace trimmed", set: "SEARCH_TIMEOUT_TEST", val: " 6000 ", want: 6000},
		{name: "zero allowed (cooldown semantics: no sleep)", set: "SEARCH_TIMEOUT_TEST", val: "0", want: 0},
		{name: "negative rejected", set: "SEARCH_TIMEOUT_TEST", val: "-5", want: 2500},
		{name: "garbage rejected", set: "SEARCH_TIMEOUT_TEST", val: "2.5s", want: 2500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envIntMs("SEARCH_TIMEOUT_TEST", 2500); got != tc.want {
				t.Fatalf("envIntMs = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEnvIntPositive(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want int
	}{
		{name: "unset returns fallback", set: "", want: 2500},
		{name: "plain value", set: "SEARCH_TIMEOUT_TEST", val: "12000", want: 12000},
		{name: "whitespace trimmed", set: "SEARCH_TIMEOUT_TEST", val: " 6000 ", want: 6000},
		// Regression: a zero GPU search timeout is an already-expired context, so
		// every GPU Search would fail and silently fall back to CPU.
		{name: "zero rejected (expired context guard)", set: "SEARCH_TIMEOUT_TEST", val: "0", want: 2500},
		{name: "negative rejected", set: "SEARCH_TIMEOUT_TEST", val: "-5", want: 2500},
		{name: "garbage rejected", set: "SEARCH_TIMEOUT_TEST", val: "2.5s", want: 2500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envIntPositive("SEARCH_TIMEOUT_TEST", 2500); got != tc.want {
				t.Fatalf("envIntPositive = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEffectiveClaimCooldownBlake3FastKernel(t *testing.T) {
	t.Setenv("HACKME_WORKER_CLAIM_COOLDOWN_MS", "100")
	t.Setenv("HACKME_WORKER_SUBGH_COOLDOWN_MS", "25000")
	// ghs<1 would previously force 25s; with no kernel timing, mild floor applies.
	got := effectiveClaimCooldownMS("gpu", 0.3)
	if got < 100 || got > 25000 {
		t.Fatalf("slow/missing kernel path: got %d", got)
	}
	// Explicit 0 disables sub-GH floor.
	t.Setenv("HACKME_WORKER_SUBGH_COOLDOWN_MS", "0")
	if got := effectiveClaimCooldownMS("gpu", 0.3); got != 100 {
		t.Fatalf("subgh disabled: got %d want 100", got)
	}
	// High reported GH/s never hits sub-GH floor.
	t.Setenv("HACKME_WORKER_SUBGH_COOLDOWN_MS", "25000")
	if got := effectiveClaimCooldownMS("gpu", 6.0); got != 100 {
		t.Fatalf("multi-GH/s: got %d want 100", got)
	}
}

func TestSubmitHashrateBlake3OmitsStickyCalibEMA(t *testing.T) {
	// Simulate pre-Blake3 FORCE/calib still sitting in EMA; fv≥3 must report wall only.
	prevEMA := submitHashrateEMA
	prevCal := gpuCalibratedGHS
	t.Cleanup(func() {
		submitHashrateEMA = prevEMA
		gpuCalibratedGHS = prevCal
	})
	submitHashrateEMA = 80
	gpuCalibratedGHS = 80
	t.Setenv("HASHRATE_GHS", "")
	t.Setenv("HACKME_WORKER_HASHRATE_GHS", "")
	t.Setenv("HACKME_WORKER_DECLARED_HASHRATE_GHS", "")
	t.Setenv("HACKME_GPU_HASHRATE_FLOOR_GHS", "")
	t.Setenv("HACKME_CUDA_CALIBRATE_GHS", "")

	const batch uint64 = 4_000_000
	got := submitHashrateGHSForFormula(batch, 2.0, "cpu", 3)
	// wall = 4e6/2/1e9 = 0.002 GH/s — must not resurrect 80 GH/s FORCE/calib EMA.
	if got > 0.01 || math.IsNaN(got) || got <= 0 {
		t.Fatalf("blake3 must omit FORCE/calib EMA and report wall, got %v", got)
	}
}

func TestCollectShareNoncesStrideCoversLargeLease(t *testing.T) {
	const shareMod = uint64(1_000_003)
	base := uint64(0)
	batch := uint64(16 << 20) // 16M
	// Prefix-only scan of 256K would miss shares past the window; stride must find some.
	out := collectShareNonces(base, batch, shareMod, 1, "", "", 8, 0, false, 256*1024)
	if len(out) == 0 {
		t.Fatal("stride harvest found no shares in 16M lease")
	}
	for _, n := range out {
		if n < base || n >= base+batch {
			t.Fatalf("nonce out of lease: %d", n)
		}
	}
}
