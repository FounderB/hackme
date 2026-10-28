package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/chain"
	"hackme/internal/store"
)

func TestLeaderBlockRateMetricsNotZeroOnLiveChain(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "rate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := chain.New(db)
	addr := "HMC-1111111111111111"
	if _, _, err := svc.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		m, err := svc.PoHTargetMod(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var nonce, eval uint64
		for n := uint64(0); ; n++ {
			if v := chain.PohEval(n); v%m == 0 {
				nonce, eval = n, v
				break
			}
		}
		if _, err := svc.AppendPoHBlock(ctx, addr, nonce, eval, 0.01, m, ""); err != nil {
			t.Fatal(err)
		}
	}
	leaderRateCache.Lock()
	leaderRateCache.hasValue = false
	leaderRateCache.Unlock()
	a := &app{chain: svc}
	var s MetricsSnapshot
	a.fillLeaderBlockRateMetrics(ctx, &s)
	if s.MiningPohBlocksLast1h != 3 {
		t.Fatalf("blocks last 1h = %d, want 3", s.MiningPohBlocksLast1h)
	}
	if s.MiningObservedBlockSec == 0 {
		t.Fatalf("observed block sec must be set (>0 or -1), got 0")
	}
	// A concurrent /api/metrics caller holds the heavy lock: serve the cached value, not 0
	// (this flickered "0 blocks/hour" on the live dashboard).
	metricsHeavyMu.Lock()
	leaderRateCache.Lock()
	leaderRateCache.at = time.Time{} // stale: would recompute if the lock were free
	leaderRateCache.Unlock()
	var busy MetricsSnapshot
	a.fillLeaderBlockRateMetrics(ctx, &busy)
	metricsHeavyMu.Unlock()
	if busy.MiningPohBlocksLast1h != 3 {
		t.Fatalf("lock contended: blocks last 1h = %d, want cached 3", busy.MiningPohBlocksLast1h)
	}
}
