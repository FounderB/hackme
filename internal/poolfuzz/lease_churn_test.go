package poolfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/store"
)

func TestReclaimExpiredLeasesIncrementsAttempts(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "co.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	now := time.Now().Unix()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"check_semantics":  "detector",
		"wasm_check_hex":   "0061736d0100000001060160017e017f0302010007090105636865636b00000a0601040041010b",
	}, "property")
	id := "camp-reclaim-attempts"
	if err := svc.RegisterCampaign(ctx, Campaign{
		ID: id, CampaignType: "property", Title: "reclaim", Status: "running",
		BudgetRuns: 1, Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx,
		`UPDATE fuzz_work_items SET status='leased', lease_owner='w', lease_until=?, updated_at=?, attempts=2 WHERE campaign_id=?`,
		now-10, now, id)
	n, err := svc.ReclaimExpiredLeases(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("reclaim n=%d err=%v", n, err)
	}
	var st, errMsg string
	var attempts int
	_ = db.QueryRowContext(ctx,
		`SELECT status, attempts, last_error FROM fuzz_work_items WHERE campaign_id=?`, id).
		Scan(&st, &attempts, &errMsg)
	if st != "pending" || attempts != 3 || errMsg != "reclaimed_expired_lease" {
		t.Fatalf("status=%s attempts=%d err=%q", st, attempts, errMsg)
	}
}

func TestForceCompleteLeaseChurnItems(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "co.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	now := time.Now().Unix()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"check_semantics":  "detector",
		"wasm_check_hex":   "0061736d0100000001060160017e017f0302010007090105636865636b00000a0601040041010b",
	}, "property")
	id := "camp-churn"
	if err := svc.RegisterCampaign(ctx, Campaign{
		ID: id, CampaignType: "property", Title: "churn", Status: "running",
		BudgetRuns: 2, Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	// One done, one pending with high attempts (lease churn).
	_, _ = db.ExecContext(ctx,
		`UPDATE fuzz_work_items SET status='done', result_ok=1, attempts=1, updated_at=?
		 WHERE campaign_id=? AND id=(SELECT MIN(id) FROM fuzz_work_items WHERE campaign_id=?)`,
		now, id, id)
	_, _ = db.ExecContext(ctx,
		`UPDATE fuzz_work_items SET status='pending', attempts=8, hunt_proven_crashes=3, updated_at=?
		 WHERE campaign_id=? AND status!='done'`,
		now, id)

	n, err := svc.ForceCompleteLeaseChurnItems(ctx, now, 20)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("force_done=%d want 1", n)
	}
	var st, lastErr string
	var ok, doneN int
	_ = db.QueryRowContext(ctx,
		`SELECT status, result_ok, last_error FROM fuzz_work_items WHERE campaign_id=? AND last_error='lease_churn_exhausted'`,
		id).Scan(&st, &ok, &lastErr)
	if st != "done" || ok != 1 {
		t.Fatalf("item status=%s ok=%d err=%s", st, ok, lastErr)
	}
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fuzz_work_items WHERE campaign_id=? AND status='done'`, id).Scan(&doneN)
	if doneN != 2 {
		t.Fatalf("done=%d want 2", doneN)
	}
	var campSt string
	_ = db.QueryRowContext(ctx, `SELECT status FROM fuzz_campaigns WHERE id=?`, id).Scan(&campSt)
	if campSt != "completed" {
		t.Fatalf("campaign status=%s want completed", campSt)
	}
}

func TestForceCompleteStalledNearDoneCampaigns(t *testing.T) {
	t.Setenv("HACKME_POOL_STALLED_NEAR_DONE_SEC", "600")
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "co.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	now := time.Now().Unix()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"check_semantics":  "detector",
		"wasm_check_hex":   "0061736d0100000001060160017e017f0302010007090105636865636b00000a0601040041010b",
	}, "property")
	id := "camp-stalled-31of32"
	if err := svc.RegisterCampaign(ctx, Campaign{
		ID: id, CampaignType: "property", Title: "stalled", Status: "running",
		BudgetRuns: 3, Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	// Ensure 3 work items exist.
	if err := svc.EnsureWorkItems(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx,
		`UPDATE fuzz_work_items SET status='done', result_ok=1, updated_at=? WHERE campaign_id=? AND id IN (
		   SELECT id FROM fuzz_work_items WHERE campaign_id=? ORDER BY id LIMIT 2)`,
		now-3600, id, id)
	_, _ = db.ExecContext(ctx,
		`UPDATE fuzz_work_items SET status='leased', lease_owner='zombie', lease_until=?, attempts=0, hunt_proven_crashes=12, updated_at=?
		 WHERE campaign_id=? AND status!='done'`,
		now+60, now, id)

	n, err := svc.ForceCompleteStalledNearDoneCampaigns(ctx, now, 20)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("force_done=%d want 1", n)
	}
	var campSt, lastErr string
	_ = db.QueryRowContext(ctx, `SELECT status FROM fuzz_campaigns WHERE id=?`, id).Scan(&campSt)
	if campSt != "completed" {
		t.Fatalf("campaign=%s want completed", campSt)
	}
	_ = db.QueryRowContext(ctx,
		`SELECT last_error FROM fuzz_work_items WHERE campaign_id=? AND last_error='stalled_near_done_force_complete'`,
		id).Scan(&lastErr)
	if lastErr != "stalled_near_done_force_complete" {
		t.Fatalf("last_error=%q", lastErr)
	}
}

func TestLeaseSecondsForConfigCoverageEngineHas2xWall(t *testing.T) {
	t.Setenv("HACKME_HUNT_ENGINE", "")
	t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1")
	cfg := map[string]any{
		"campaign_type":    "hunt",
		"work_kind":        "hunt_shard",
		"hunt_engine":      "libfuzzer",
		"engine_lease_sec": 90,
	}
	sec := leaseSecondsForConfig(cfg)
	// eng=90 → max(180, 210)=210
	if sec != 210 {
		t.Fatalf("lease_sec=%d want 210", sec)
	}
}
