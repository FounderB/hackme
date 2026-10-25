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

func TestSoftShareKeepsAttemptPay(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         10_000,
		targetMod:            10_000_000, // ≥ lease so M economic cap does not bind attempt-pay
		leaseSec:             60,
		rewardPerM:           1.0,
		baseRewardHMC:        0.01,
		foundBonus:           0.0,
		shareDivisor:         1024,
		shareStrict:          false,
		shareRateMin:         0.30,
		shareRateMinExpected: 8.0,
		shareRateReject:      false,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, mod, _, ok, reason := wm.claim("w-share-soft", 10_000)
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
		WorkerID:    "w-share-soft",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-share-soft", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 1.0,
		ShareNonces: shares,
	})
	if reasonSub != "" {
		t.Fatalf("submit failed: %s", reasonSub)
	}
	// Soft: attempt-pay, not share-pay (audit: share-pay underpays honest CPU harvest).
	wantAttempt := (float64(size) / 1_000_000.0) * 1.0
	sharePay := float64(len(shares)) * rewardPerShare(0.01, 1024)
	if payout < wantAttempt*0.99 || payout > wantAttempt*1.01 {
		t.Fatalf("payout=%v want attempt~%v (share would be %v)", payout, wantAttempt, sharePay)
	}
	if wm.sharePaidSubmits != 0 {
		t.Fatalf("soft must not share-pay, paid_submits=%d", wm.sharePaidSubmits)
	}
	if wm.shareAccepts < uint64(len(shares)) {
		t.Fatalf("share accepts=%d want>=%d", wm.shareAccepts, len(shares))
	}
	st := wm.worker["w-share-soft"]
	if st.ShareAccepted < uint64(len(shares)) || st.ShareExpected <= 0 {
		t.Fatalf("share stats accepted=%d expected=%v", st.ShareAccepted, st.ShareExpected)
	}
}

func TestStrictSharePayoutPath(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         10_000,
		targetMod:            997,
		leaseSec:             60,
		rewardPerM:           1.0,
		baseRewardHMC:        0.01,
		foundBonus:           0.0,
		shareDivisor:         1024,
		shareStrict:          true,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, mod, _, ok, reason := wm.claim("w-share-strict", 10_000)
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
		WorkerID:    "w-share-strict",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-share-strict", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 1.0,
		ShareNonces: shares,
	})
	if reasonSub != "" {
		t.Fatalf("submit failed: %s", reasonSub)
	}
	want := float64(len(shares)) * rewardPerShare(0.01, 1024)
	if payout < want*0.99 || payout > want*1.01 {
		t.Fatalf("payout=%v want~%v", payout, want)
	}
	if wm.sharePaidSubmits < 1 {
		t.Fatalf("strict should share-pay, paid=%d", wm.sharePaidSubmits)
	}
}

func TestShareRateReject(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         10_000,
		targetMod:            997,
		leaseSec:             60,
		rewardPerM:           1.0,
		baseRewardHMC:        0.01,
		shareDivisor:         1024,
		shareStrict:          false,
		shareRateMin:         0.30,
		shareRateMinExpected: 2.0,
		shareRateReject:      true,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	// Seed low share rate without rejecting yet.
	wm.worker["w-rate"] = workerPayoutStat{ShareExpected: 1.5, ShareAccepted: 0}
	base, size, _, _, _, ok, reason := wm.claim("w-rate", 10_000)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	// expectedShares(10000, ~997, 1024) ≈ 10k*1024/997 ≫ 2; no shares → reject
	_, reasonSub, _, _, _ := wm.submit(submitWorkRequest{
		WorkerID:    "w-rate",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-rate", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 1.0,
		ShareNonces: nil,
	})
	if reasonSub != "share_rate_low" {
		t.Fatalf("want share_rate_low, got %q", reasonSub)
	}
	if wm.shareRateLowRejects < 1 {
		t.Fatalf("rejects=%d", wm.shareRateLowRejects)
	}
}

func TestExpectedShares(t *testing.T) {
	// attempts * 1024 / M
	got := expectedShares(4_194_304, 972_000_000, 1024)
	if got < 4.4 || got > 4.5 {
		t.Fatalf("expected~4.42 got %v", got)
	}
	if expectedShares(0, 997, 1024) != 0 || expectedShares(1000, 0, 1024) != 0 {
		t.Fatal("zero cases")
	}
}

func TestSoftShareScanAttempts(t *testing.T) {
	if softShareScanAttempts(4_194_304, false) != shareHarvestScanCap {
		t.Fatalf("soft must cap to %d", shareHarvestScanCap)
	}
	if softShareScanAttempts(4_194_304, true) != 4_194_304 {
		t.Fatal("strict uses full lease")
	}
	if softShareScanAttempts(1000, false) != 1000 {
		t.Fatal("under cap unchanged")
	}
}

func TestSoftShareExpectedUsesScanCap(t *testing.T) {
	wm := withTestTip(&workManager{
		defaultBatch:         4_194_304,
		maxClaimBatch:        4_194_304,
		targetMod:            972_000_000,
		leaseSec:             60,
		rewardPerM:           1.0,
		baseRewardHMC:        0.01,
		shareDivisor:         1024,
		shareStrict:          false,
		shareRateMin:         0.30,
		shareRateMinExpected: 100, // do not flag in this unit test
		shareRateReject:      false,
		maxWorkers:           1000,
		maxActiveLeases:      1000,
		maxDedupEntries:      1000,
		active:               make(map[workKey]leaseRecord),
		worker:               make(map[string]workerPayoutStat),
		acceptedResultHashes: make(map[string]struct{}),
		acceptedFoundNonces:  make(map[uint64]struct{}),
	})
	base, size, _, mod, _, ok, reason := wm.claim("w-cap", 4_194_304)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	_, reasonSub, _, _, _ := wm.submit(submitWorkRequest{
		WorkerID:    "w-cap",
		BaseNonce:   base,
		BatchSize:   size,
		WorkID:      buildWorkID("w-cap", base, size),
		Attempts:    size,
		Found:       false,
		HashrateGHS: 50.0,
		ShareNonces: nil,
	})
	if reasonSub != "" {
		t.Fatalf("submit: %s", reasonSub)
	}
	st := wm.worker["w-cap"]
	want := expectedShares(shareHarvestScanCap, mod, 1024)
	full := expectedShares(size, mod, 1024)
	if st.ShareExpected < want*0.99 || st.ShareExpected > want*1.01 {
		t.Fatalf("expected=%v want capped~%v (full would be %v)", st.ShareExpected, want, full)
	}
	if st.ShareExpected > full*0.2 {
		t.Fatalf("soft expected must be far below full-lease mean")
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
