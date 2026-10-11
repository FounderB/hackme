package poolfuzz

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/store"
)

// Empty CLEAN must not PayRun without coordinator WASM prove on the locked input.
func TestHygieneProveRequiredForEscrowPayRun(t *testing.T) {
	t.Setenv("HACKME_POOL_FULL_REPLAY", "")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT_DEEP", "0")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_SEED", "hygiene-prove")
	t.Setenv("HACKME_POOL_HYGIENE_PROVE", "1")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "hygiene-prove.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	spy := &spySettler{}
	svc := &Service{DB: db, Settler: spy}
	ctx := context.Background()

	safeInput := uint64(42)
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"dig_package":      "audit",
		"budget_hmc":       5.0,
		"escrow_enabled":   true,
		"exec_per_unit":    8,
		"check_semantics":  "detector",
		"wasm_check_hex":   mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":      []any{safeInput},
		"mutation_rounds":  0,
	}, "property")
	id := "hygiene-prove-pay"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 2, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-prove", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	out, err := svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-prove", MinerAddress: "HMC-aaaaaaaaaaaaaaaa",
		WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 8, SegmentExecDone: w.ExecPerUnit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ReplayStatus != replayReasonHygieneSkip {
		t.Fatalf("status=%q want hygiene_skip", out.ReplayStatus)
	}
	spy.mu.Lock()
	runs := spy.runs
	spy.mu.Unlock()
	if runs != 1 {
		t.Fatalf("honest CLEAN with WASM prove must PayRun once, got %d", runs)
	}

	// Second item: miner lies CLEAN but we disable prove → still default ON; forge without matching prove.
	w2, ok, err := svc.Claim(ctx, "w-prove", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim2: ok=%v err=%v", ok, err)
	}
	// Claim a canary-like hit input by forging check_result=0 while locking a known
	// detector hit is covered elsewhere; here disable prove and ensure legacy path
	// is opt-in only.
	t.Setenv("HACKME_POOL_HYGIENE_PROVE", "0")
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-prove", MinerAddress: "HMC-aaaaaaaaaaaaaaaa",
		WorkID: w2.WorkID, CampaignID: id, ItemID: w2.ItemID,
		InputN: w2.InputN, ActualInput: w2.ActualInput, InputBytes: w2.InputBytes,
		CheckResult: 0, DurationMS: 8, SegmentExecDone: w2.ExecPerUnit,
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.mu.Lock()
	runs2 := spy.runs
	spy.mu.Unlock()
	if runs2 != 2 {
		t.Fatalf("legacy prove-off still pays (rollback), got runs=%d", runs2)
	}
	_ = strings.TrimSpace
}

func TestHygieneProveRejectsWithoutWasmEscrow(t *testing.T) {
	t.Setenv("HACKME_POOL_FULL_REPLAY", "")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_HYGIENE_PROVE", "1")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "hygiene-nowasm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	spy := &spySettler{}
	svc := &Service{DB: db, Settler: spy}
	ctx := context.Background()

	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"dig_package":      "audit",
		"budget_hmc":       5.0,
		"escrow_enabled":   true,
		"exec_per_unit":    8,
		"check_semantics":  "detector",
		"seed_corpus":      []any{uint64(1)},
		"mutation_rounds":  0,
	}, "property")
	// Strip wasm if normalize injected one.
	delete(cfg, "wasm_check_hex")
	id := "hygiene-nowasm"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-nowasm", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-nowasm", MinerAddress: "HMC-aaaaaaaaaaaaaaaa",
		WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 8, SegmentExecDone: w.ExecPerUnit,
	})
	if err == nil || !strings.Contains(err.Error(), "wasm_check_hex") {
		t.Fatalf("expected wasm prove error, got %v", err)
	}
	spy.mu.Lock()
	runs := spy.runs
	spy.mu.Unlock()
	if runs != 0 {
		t.Fatalf("must not PayRun without wasm prove, runs=%d", runs)
	}
}
