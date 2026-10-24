package main

import (
	"path/filepath"
	"testing"

	"hackme/internal/store"
)

func TestRevokeWorkerDeletesDurableAccrual(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "accrual-revoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wm := &workManager{
		worker:         map[string]workerPayoutStat{},
		abuse:          map[string]workerAbuseState{},
		ipAbuse:        map[string]workerAbuseState{},
		totalPayoutHMC: 5,
		totalPayoutSUP: 0.2,
	}
	wm.attachDedupDB(db)
	wm.mu.Lock()
	wm.worker["evil"] = workerPayoutStat{
		AcceptedAtt:   9_000_000,
		PayoutHMC:     5,
		PayoutSUP:     0.2,
		PayoutAddress: "HMC-evil",
		LastClientIP:  "9.9.9.9",
	}
	wm.mu.Unlock()
	wm.persistWorkerAccrual("evil", true)
	if _, ok := wm.fetchDurableAccrual("evil"); !ok {
		t.Fatal("expected durable row before revoke")
	}

	out := wm.revokeWorker("evil", "9.9.9.9", true)
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("revoke: %+v", out)
	}
	if _, ok := wm.fetchDurableAccrual("evil"); ok {
		t.Fatal("durable accrual must be deleted on revoke")
	}

	// Submit-path hydrate must not revive unpaid after revoke.
	wm.prefetchWorkerAccrual("evil")
	if _, exists := wm.worker["evil"]; exists {
		t.Fatal("prefetch must not restore revoked worker")
	}

	// Restart path: loadWorkerAccruals must also stay empty for this id.
	wm2 := &workManager{worker: map[string]workerPayoutStat{}}
	wm2.attachDedupDB(db)
	if _, exists := wm2.worker["evil"]; exists {
		t.Fatalf("restart restored revoked accrual: %+v", wm2.worker["evil"])
	}
}

func TestRevokeAbsentWorkerStillDeletesDurable(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "accrual-prune.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wm := &workManager{
		worker:  map[string]workerPayoutStat{},
		abuse:   map[string]workerAbuseState{},
		ipAbuse: map[string]workerAbuseState{},
	}
	wm.attachDedupDB(db)
	wm.mu.Lock()
	wm.worker["ghost"] = workerPayoutStat{AcceptedAtt: 1000, PayoutHMC: 1.5}
	wm.mu.Unlock()
	wm.persistWorkerAccrual("ghost", true)
	// Simulate UI prune: memory gone, SQLite still has unpaid.
	wm.mu.Lock()
	delete(wm.worker, "ghost")
	wm.mu.Unlock()

	out := wm.revokeWorker("ghost", "1.1.1.1", true)
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("revoke absent: %+v", out)
	}
	if _, ok := wm.fetchDurableAccrual("ghost"); ok {
		t.Fatal("prune+ban must still DELETE durable accrual")
	}
}

func TestPersistWorkerAccrualMonotonic(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "accrual-mono.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	wm := &workManager{worker: map[string]workerPayoutStat{}}
	wm.attachDedupDB(db)
	wm.mu.Lock()
	wm.worker["w"] = workerPayoutStat{AcceptedAtt: 200, PayoutHMC: 2}
	wm.mu.Unlock()
	wm.persistWorkerAccrual("w", true)

	// Stale write with lower attempts must not clobber.
	wm.mu.Lock()
	wm.worker["w"] = workerPayoutStat{AcceptedAtt: 50, PayoutHMC: 0.5}
	wm.mu.Unlock()
	wm.persistWorkerAccrual("w", true)

	st, ok := wm.fetchDurableAccrual("w")
	if !ok || st.AcceptedAtt != 200 || st.PayoutHMC < 2-1e-9 {
		t.Fatalf("stale overwrite: ok=%v st=%+v", ok, st)
	}
}

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
