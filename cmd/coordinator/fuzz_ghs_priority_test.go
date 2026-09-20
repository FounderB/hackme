package main

import (
	"testing"
	"time"
)

func TestAllowFuzzClaimByGHSDefersDigOnlyWhenHybridOnline(t *testing.T) {
	t.Setenv("HACKME_FUZZ_CLAIM_GHS_PRIORITY", "1")
	t.Setenv("HACKME_FUZZ_CLAIM_GHS_DIG_ONLY_PCT", "25")
	now := time.Now().Unix()
	wm := &workManager{worker: map[string]workerPayoutStat{
		"gpu-1": {LastHashrateGHS: 40, PeakHashrateGHS: 40, LastSeenUnix: now},
		"dig-1": {LastHashrateGHS: 0, LastSeenUnix: now},
	}}
	if ok, _ := wm.allowFuzzClaimByGHS("gpu-1", now); !ok {
		t.Fatal("hybrid must always claim")
	}
	deferHits := 0
	admitHits := 0
	for w := int64(0); w < 40; w++ {
		ok, reason := wm.allowFuzzClaimByGHS("dig-1", now+w*10)
		if ok {
			admitHits++
		} else {
			if reason != "ghs_priority_defer" {
				t.Fatalf("reason=%q", reason)
			}
			deferHits++
		}
	}
	if deferHits == 0 {
		t.Fatal("expected some dig-only defers when hybrid online")
	}
	if admitHits == 0 {
		t.Fatal("dig-only must still get minority admit slots")
	}
}

func TestAllowFuzzClaimByGHSAllowsDigOnlyWhenNoHybrid(t *testing.T) {
	t.Setenv("HACKME_FUZZ_CLAIM_GHS_PRIORITY", "1")
	now := time.Now().Unix()
	wm := &workManager{worker: map[string]workerPayoutStat{
		"dig-a": {LastHashrateGHS: 0, LastSeenUnix: now},
		"dig-b": {LastHashrateGHS: 0.1, LastSeenUnix: now},
	}}
	if ok, reason := wm.allowFuzzClaimByGHS("dig-a", now); !ok {
		t.Fatalf("dig-only fleet must work without hybrid: %s", reason)
	}
}

func TestAllowFuzzClaimByGHSDisabled(t *testing.T) {
	t.Setenv("HACKME_FUZZ_CLAIM_GHS_PRIORITY", "0")
	now := time.Now().Unix()
	wm := &workManager{worker: map[string]workerPayoutStat{
		"gpu-1": {LastHashrateGHS: 40, LastSeenUnix: now},
		"dig-1": {LastHashrateGHS: 0, LastSeenUnix: now},
	}}
	for w := int64(0); w < 20; w++ {
		if ok, _ := wm.allowFuzzClaimByGHS("dig-1", now+w*10); !ok {
			t.Fatal("priority off => dig-only always allowed")
		}
	}
}

func TestFuzzFleetCapacityAndETA(t *testing.T) {
	now := time.Now().Unix()
	wm := &workManager{worker: map[string]workerPayoutStat{
		"gpu-1": {LastHashrateGHS: 30, PeakHashrateGHS: 30, LastSeenUnix: now},
		"gpu-2": {LastHashrateGHS: 20, PeakHashrateGHS: 20, LastSeenUnix: now},
		"dig-1": {LastHashrateGHS: 0, LastSeenUnix: now},
		"old":   {LastHashrateGHS: 50, LastSeenUnix: now - 10_000},
	}}
	cap := wm.fuzzFleetCapacity(now)
	if cap.HybridWorkersOnline != 2 || cap.DigOnlyWorkersOnline != 1 {
		t.Fatalf("cap=%+v", cap)
	}
	if cap.FleetHashrateGHS < 49 || cap.FleetHashrateGHS > 51 {
		t.Fatalf("fleet ghs=%v", cap.FleetHashrateGHS)
	}
	if cap.EstShardsPerHour < 1 {
		t.Fatalf("est shards=%v", cap.EstShardsPerHour)
	}
	eta := estimateETASeconds(360, cap.EstShardsPerHour)
	if eta <= 0 {
		t.Fatalf("eta=%d", eta)
	}
	if estimateETASeconds(0, cap.EstShardsPerHour) != 0 {
		t.Fatal("complete eta")
	}
	if estimateETASeconds(10, 0) != -1 {
		t.Fatal("unknown eta")
	}
}
