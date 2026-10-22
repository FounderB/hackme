package main

import (
	"testing"

	"hackme/internal/chain"
)

func TestLeaseHasUnreportedFoundV1(t *testing.T) {
	mod := uint64(997)
	base := uint64(0)
	var hit uint64
	for n := base; n < base+10000; n++ {
		if chain.PohEval(n)%mod == 0 {
			hit = n
			break
		}
	}
	if hit == 0 {
		t.Fatal("no v1 hit")
	}
	if !leaseHasUnreportedFound(base, hit+1, mod, 1, "", "", 10000) {
		t.Fatal("expected unreported found in range covering hit")
	}
	if leaseHasUnreportedFound(hit+1, 100, mod, 1, "", "", 100) {
		t.Fatal("range after hit should be clean for small budget if next hit is far")
	}
}

func TestAttemptRecheckRejectsMissedFound(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         1000,
		targetMod:            997,
		leaseSec:             30,
		rewardPerM:           1.0,
		foundBonus:           0.01,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		payoutFoundOnly:      false,
		attemptRecheckPct:    100, // always recheck
		attemptRecheckBudget: 10000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, _, _, ok, reason := wm.claim("w-miss", 1000)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	// Ensure the lease contains a v1 hit so recheck must reject found=false.
	hasHit := false
	for n := base; n < base+size; n++ {
		if chain.PohEval(n)%wm.targetMod == 0 {
			hasHit = true
			break
		}
	}
	if !hasHit {
		t.Skip("no hit in this lease; retarget lucky miss")
	}
	okSub, reasonSub, payout, _, _ := wm.submit(submitWorkRequest{
		WorkerID:    "w-miss",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-miss", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 1.0,
	})
	if okSub || reasonSub != "missed_found_unreported" {
		t.Fatalf("want missed_found_unreported, got ok=%v reason=%q payout=%v", okSub, reasonSub, payout)
	}
	if wm.attemptRecheckReject < 1 {
		t.Fatalf("recheck reject counter=%d", wm.attemptRecheckReject)
	}
}

func TestAttemptRecheckSkippedWhenFoundOnly(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         1000,
		targetMod:            997,
		leaseSec:             30,
		rewardPerM:           1.0,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		payoutFoundOnly:      true,
		attemptRecheckPct:    100,
		attemptRecheckBudget: 10000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, _, _, ok, _ := wm.claim("w-fo", 1000)
	if !ok {
		t.Fatal("claim")
	}
	// submit()'s first bool is "found accepted", not overall success — HTTP uses reason=="".
	_, reason, payout, _, _ := wm.submit(submitWorkRequest{
		WorkerID: "w-fo", BaseNonce: base, BatchSize: size,
		WorkID: buildWorkID("w-fo", base, size), Attempts: size, Found: false, HashrateGHS: 2,
	})
	if reason != "" {
		t.Fatalf("found-only attempt submit should succeed with empty reason, got %q", reason)
	}
	if payout != 0 {
		t.Fatalf("payout=%v want 0", payout)
	}
	if wm.attemptRecheckReject != 0 || wm.attemptRecheckRolls != 0 {
		t.Fatalf("found-only must skip recheck, rolls=%d reject=%d", wm.attemptRecheckRolls, wm.attemptRecheckReject)
	}
}
