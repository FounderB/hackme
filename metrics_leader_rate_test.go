package main

import (
	"context"
	"path/filepath"
	"testing"

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
	a := &app{chain: svc}
	var s MetricsSnapshot
	a.fillLeaderBlockRateMetrics(ctx, &s)
	if s.MiningPohBlocksLast1h != 3 {
		t.Fatalf("blocks last 1h = %d, want 3", s.MiningPohBlocksLast1h)
	}
	if s.MiningObservedBlockSec == 0 {
		t.Fatalf("observed block sec must be set (>0 or -1), got 0")
	}
}
