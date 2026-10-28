package chain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/store"
)

func TestRewardWindowBreakdownCountsRecentBlocks(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "rw.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	addr := "HMC-rewardwindow0001"
	if _, _, err := svc.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		appendNext(t, ctx, svc, addr)
	}
	rw, err := svc.RewardWindowBreakdownSince(ctx, time.Now().Unix()-3600)
	if err != nil {
		t.Fatal(err)
	}
	if rw.Blocks != 3 || rw.BaseBlocks != 3 {
		t.Fatalf("window blocks=%d base=%d, want 3/3", rw.Blocks, rw.BaseBlocks)
	}
	future, err := svc.RewardWindowBreakdownSince(ctx, time.Now().Unix()+3600)
	if err != nil {
		t.Fatal(err)
	}
	if future.Blocks != 0 {
		t.Fatalf("future window counted %d blocks", future.Blocks)
	}
}

// Opt-in: on a mainnet DB copy the 1 h window must finish well inside the /api/metrics budget.
func TestRewardWindowFastOnProdSnapshot(t *testing.T) {
	snap := os.Getenv("HACKME_ASERT_PROD_SNAPSHOT")
	if snap == "" {
		t.Skip("set HACKME_ASERT_PROD_SNAPSHOT")
	}
	ctx := context.Background()
	db, err := store.Open(snap)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	_, tipHash, _ := svc.Tip(ctx)
	_ = tipHash
	tipTime, err := blockTimestampQ(ctx, db, func() uint64 { h, _, _ := svc.Tip(ctx); return h }())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	rw, err := svc.RewardWindowBreakdownSince(ctx, tipTime-3600)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("1h window on snapshot: %d blocks in %v", rw.Blocks, elapsed)
	if rw.Blocks < 60 || elapsed > 500*time.Millisecond {
		t.Fatalf("window blocks=%d elapsed=%v", rw.Blocks, elapsed)
	}
}
