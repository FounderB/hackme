package poolfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/store"
)

func TestLoadPoolCorpusSeedsFallsBackWhenCrashOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "crash-only.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	id := "camp-crash-only"
	now := time.Now().Unix()
	if err := svc.RegisterCampaign(ctx, Campaign{
		ID: id, CampaignType: "property", Title: "t", Status: "running",
		BudgetRuns: 10, Config: map[string]any{"guided_scheduling": true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.upsertPoolCorpusSeed(ctx, id, 42, []byte("ab"), 8, 1, 2, true, now); err != nil {
		t.Fatal(err)
	}
	if err := svc.upsertPoolCorpusSeed(ctx, id, 43, []byte("cd"), 9, 3, 4, true, now); err != nil {
		t.Fatal(err)
	}
	seeds, err := svc.loadPoolCorpusSeeds(ctx, id, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected crash-only fallback len=2, got %d", len(seeds))
	}
}
