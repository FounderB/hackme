package poolfuzz

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/store"
)

func TestHuntCanaryPayloadFromConfig(t *testing.T) {
	cfg := map[string]any{"hunt_canary_input_hex": "414243"} // ABC
	u, b, ok := huntCanaryPayload(cfg)
	if !ok || string(b) != "ABC" || u == 0 {
		t.Fatalf("payload ok=%v u=%d b=%q", ok, u, b)
	}
	if _, _, ok := huntCanaryPayload(map[string]any{}); ok {
		t.Fatal("empty cfg should not yield payload")
	}
}

func TestShouldInjectHuntCanaryNeedsPayload(t *testing.T) {
	t.Setenv("HACKME_POOL_CANARY", "1")
	t.Setenv("HACKME_HUNT_CANARY_PCT", "100")
	cfg := map[string]any{"work_kind": "hunt_shard", "pool_distributed": true}
	if shouldInjectHuntCanary("camp", 1, cfg) {
		t.Fatal("no payload → no inject")
	}
	cfg["canary_input_hex"] = "00"
	if !poolDistributed(cfg) {
		t.Skip("poolDistributed false in this env")
	}
	if !shouldInjectHuntCanary("camp", 1, cfg) {
		t.Fatal("pct=100 + payload should inject when distributed")
	}
}

func TestHuntCanaryMissRejectsFakeClean(t *testing.T) {
	t.Setenv("HACKME_POOL_CANARY", "1")
	t.Setenv("HACKME_HUNT_CANARY_PCT", "100")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "hunt-canary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()

	cid := "hunt-canary"
	if _, err := db.Exec(`INSERT INTO fuzz_campaigns(id, campaign_type, title, status, config_json, budget_runs, created_at)
		VALUES(?,?,?,?,?,1,1)`, cid, "hunt", "t", "running", `{"work_kind":"hunt_shard"}`); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO fuzz_work_items(campaign_id, input_n, status, created_at, updated_at)
		VALUES(?,?,?,?,?)`, cid, 1, "leased", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := res.LastInsertId()
	if err != nil || itemID <= 0 {
		t.Fatalf("item id: %v %d", err, itemID)
	}
	if err := svc.markWorkCanary(ctx, cid, itemID); err != nil {
		t.Fatal(err)
	}
	ok, err := svc.workItemIsCanary(ctx, cid, itemID)
	if err != nil || !ok {
		t.Fatalf("expected canary flag, ok=%v err=%v", ok, err)
	}

	cfg := map[string]any{
		"work_kind":        "hunt_shard",
		"canary_input_hex": "deadbeef",
		"check_semantics":  "native_crash",
	}
	req := SubmitRequest{
		CampaignID: cid, ItemID: itemID,
		CheckResult: 0, Trap: "",
	}
	err = svc.rejectCanaryMissIfNeeded(ctx, cfg, req, 0, []byte{0xde, 0xad, 0xbe, 0xef})
	if err == nil || !strings.Contains(err.Error(), "canary_miss") {
		t.Fatalf("want canary_miss on Hunt CLEAN, got %v", err)
	}
	req.CheckResult = 1
	req.Trap = "hunt_crash:heap-buffer-overflow"
	if err := svc.rejectCanaryMissIfNeeded(ctx, cfg, req, 0, []byte{0xde}); err != nil {
		t.Fatalf("crash claim should pass canary gate: %v", err)
	}
}
