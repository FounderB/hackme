package main

import (
	"testing"

	"hackme/internal/chain"
)

func TestValidateShareNoncesV1(t *testing.T) {
	mod := uint64(1_000_003)
	div := uint64(1024)
	sm := shareModFor(mod, div)
	if sm == 0 {
		t.Fatal("share mod")
	}
	base := uint64(0)
	batch := uint64(50_000)
	var hit uint64
	for n := base; n < base+batch; n++ {
		if chain.PohEval(n)%sm == 0 {
			hit = n
			break
		}
	}
	if hit == 0 {
		t.Fatal("no share hit")
	}
	ok, rej := validateShareNonces([]uint64{hit, hit, base + batch + 1, hit + 1}, base, batch, sm, 1, "", "")
	if len(ok) != 1 || ok[0] != hit {
		t.Fatalf("accepted=%v rej=%d", ok, rej)
	}
	if rej < 1 {
		t.Fatalf("expected rejects, got %d", rej)
	}
}

func TestSharePayoutPath(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         10_000,
		targetMod:            997,
		leaseSec:             60,
		rewardPerM:           1.0,
		baseRewardHMC:        0.01,
		foundBonus:           0.0,
		shareDivisor:         1024,
		shareStrict:          false,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, mod, _, ok, reason := wm.claim("w-share", 10_000)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	sm := shareModFor(mod, wm.shareDivisor)
	var shares []uint64
	for n := base; n < base+size && len(shares) < 3; n++ {
		if chain.PohEval(n)%sm == 0 {
			shares = append(shares, n)
		}
	}
	if len(shares) == 0 {
		t.Skip("no shares in lease")
	}
	_, reasonSub, payout, _, _ := wm.submit(submitWorkRequest{
		WorkerID:    "w-share",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-share", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 1.0,
		ShareNonces: shares,
	})
	// First bool is Found (false here), not HTTP ok — empty reason means accepted.
	if reasonSub != "" {
		t.Fatalf("submit failed: %s", reasonSub)
	}
	want := float64(len(shares)) * rewardPerShare(0.01, 1024)
	if payout < want*0.99 || payout > want*1.01 {
		t.Fatalf("payout=%v want~%v", payout, want)
	}
	if wm.sharePaidSubmits < 1 || wm.shareAccepts < uint64(len(shares)) {
		t.Fatalf("share counters accepts=%d paid=%d", wm.shareAccepts, wm.sharePaidSubmits)
	}
}

func TestRewardPerShare(t *testing.T) {
	if rewardPerShare(0.01, 1024) <= 0 {
		t.Fatal("zero")
	}
	if rewardPerShare(0.01, 1) != 0 {
		t.Fatal("divisor<2")
	}
}
