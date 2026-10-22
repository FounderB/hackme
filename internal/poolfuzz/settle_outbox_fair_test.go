package poolfuzz

import (
	"context"
	"path/filepath"
	"testing"

	"hackme/internal/store"
)

func TestFairSettleOutboxWindowDoesNotMonopolize(t *testing.T) {
	var items []SettleOutboxItem
	for i := 0; i < 200; i++ {
		items = append(items, SettleOutboxItem{ID: int64(i + 1), CampaignID: "orphan-head", Kind: "run"})
	}
	for i := 0; i < 40; i++ {
		items = append(items, SettleOutboxItem{ID: int64(1000 + i), CampaignID: "local-a", Kind: "run"})
	}
	for i := 0; i < 40; i++ {
		items = append(items, SettleOutboxItem{ID: int64(2000 + i), CampaignID: "local-b", Kind: "run"})
	}
	out := fairSettleOutboxWindow(items, 64)
	if len(out) != 64 {
		t.Fatalf("len=%d want 64", len(out))
	}
	counts := map[string]int{}
	for _, it := range out {
		counts[it.CampaignID]++
	}
	if counts["local-a"] == 0 || counts["local-b"] == 0 {
		t.Fatalf("fair window must include local campaigns, got %v", counts)
	}
	if counts["orphan-head"] >= 64 {
		t.Fatalf("orphan must not fill entire window: %v", counts)
	}
}

func TestListPendingSettleOutboxFairAcrossCampaigns(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fair-outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if _, err := svc.EnqueueSettleOutbox(ctx, "run", "orphan", "HMC-orphan", "", int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		if _, err := svc.EnqueueSettleOutbox(ctx, "run", "bootstrap-a", "HMC-a", "", int64(1000+i)); err != nil {
			t.Fatal(err)
		}
	}
	items, err := svc.ListPendingSettleOutbox(ctx, 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 32 {
		t.Fatalf("len=%d want 32", len(items))
	}
	seen := map[string]int{}
	for _, it := range items {
		seen[it.CampaignID]++
	}
	if seen["bootstrap-a"] == 0 {
		t.Fatalf("expected bootstrap-a in fair window, got %v", seen)
	}
}

func TestCountPendingSettleOutboxExactNotFairWindow(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "count-outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		if _, err := svc.EnqueueSettleOutbox(ctx, "run", "orphan-head", "HMC-orphan", "", int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.EnqueueSettleOutbox(ctx, "run", "close-me", "HMC-local", "", int64(5000+i)); err != nil {
			t.Fatal(err)
		}
	}
	// Fair window may omit close-me if monopolized; COUNT must still see it.
	n, err := svc.CountPendingSettleOutbox(ctx, "close-me")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("count=%d want 3", n)
	}
	zero, err := svc.CountPendingSettleOutbox(ctx, "never-enqueued")
	if err != nil {
		t.Fatal(err)
	}
	if zero != 0 {
		t.Fatalf("empty campaign count=%d", zero)
	}
	items, err := svc.ListPendingSettleOutboxForCampaign(ctx, "close-me", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("list len=%d want 3", len(items))
	}
}
