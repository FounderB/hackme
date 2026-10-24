package main

import (
	"path/filepath"
	"testing"

	"hackme/internal/store"
)

func TestWorkerAccrualPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "accrual.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wm := &workManager{worker: map[string]workerPayoutStat{}}
	wm.attachDedupDB(db)

	wm.mu.Lock()
	wm.worker["worker-kapa-pc"] = workerPayoutStat{
		AcceptedRanges:  3,
		AcceptedHits:    1,
		AcceptedAtt:     12_582_912,
		PayoutHMC:       1.25,
		PayoutSUP:       0.04,
		PayoutAddress:   "HMC-91fe007e4036c602",
		SignedSubmits:   3,
		LastHashrateGHS: 0.18,
		PeakHashrateGHS: 0.22,
		LastSeenUnix:    100,
		LastPoHSeenUnix: 100,
	}
	wm.totalPayoutHMC = 1.25
	wm.totalPayoutSUP = 0.04
	wm.mu.Unlock()
	wm.persistWorkerAccrual("worker-kapa-pc", true)

	wm2 := &workManager{worker: map[string]workerPayoutStat{}}
	wm2.attachDedupDB(db)
	st := wm2.worker["worker-kapa-pc"]
	if st.PayoutHMC < 1.25-1e-9 || st.AcceptedAtt != 12_582_912 {
		t.Fatalf("restored=%+v", st)
	}
	if wm2.totalPayoutHMC < 1.25-1e-9 {
		t.Fatalf("total=%v", wm2.totalPayoutHMC)
	}

	// Simulate prune then prefetch hydrate.
	wm2.mu.Lock()
	wm2.totalPayoutHMC -= st.PayoutHMC
	wm2.totalPayoutSUP -= st.PayoutSUP
	delete(wm2.worker, "worker-kapa-pc")
	wm2.mu.Unlock()
	wm2.prefetchWorkerAccrual("worker-kapa-pc")
	st2 := wm2.worker["worker-kapa-pc"]
	if st2.PayoutHMC < 1.25-1e-9 {
		t.Fatalf("hydrate=%+v", st2)
	}
	if wm2.totalPayoutHMC < 1.25-1e-9 {
		t.Fatalf("total after hydrate=%v", wm2.totalPayoutHMC)
	}
}
