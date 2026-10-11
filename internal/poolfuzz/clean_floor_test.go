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

func TestCleanFloorRejectsZeroDuration(t *testing.T) {
	t.Setenv("HACKME_POOL_CLEAN_FLOOR", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_DURATION_MS", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "floor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"dig_package":      "audit",
		"exec_per_unit":    8,
		"check_semantics":  "detector",
		"wasm_check_hex":   mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":      []any{uint64(42)},
	}, "property")
	id := "floor-dur"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-empty", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-empty", WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 0, SegmentExecDone: w.ExecPerUnit,
	})
	if err == nil || !strings.Contains(err.Error(), "clean_floor") {
		t.Fatalf("want clean_floor reject, got %v", err)
	}
}

func TestCleanFloorRejectsZeroEdgesWhenReported(t *testing.T) {
	t.Setenv("HACKME_POOL_CLEAN_FLOOR", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_EDGES", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "floor-e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed":  true,
		"dig_package":       "audit",
		"exec_per_unit":     8,
		"check_semantics":   "detector",
		"coverage_kind":     "wasm_edge_bitmap",
		"guided_scheduling": true,
		"wasm_check_hex":    mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":       []any{uint64(42)},
	}, "property")
	id := "floor-edges"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-zero-edge", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-zero-edge", WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 5, SegmentExecDone: w.ExecPerUnit,
		EdgesTouched: 0, EdgesTouchedOK: true,
	})
	if err == nil || !strings.Contains(err.Error(), "edges_touched") {
		t.Fatalf("want edges floor reject, got %v", err)
	}
}

func TestCleanFloorHonestWorkerPasses(t *testing.T) {
	t.Setenv("HACKME_POOL_CLEAN_FLOOR", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "floor-ok.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed": true,
		"dig_package":      "audit",
		"exec_per_unit":    8,
		"check_semantics":  "detector",
		"wasm_check_hex":   mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":      []any{uint64(42)},
	}, "property")
	id := "floor-ok"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-honest", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	out, err := svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-honest", MinerAddress: "HMC-bbbbbbbbbbbbbbbb",
		WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 12, SegmentExecDone: w.ExecPerUnit,
		EdgesTouched: 3, EdgesTouchedOK: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ReplayStatus != replayReasonHygieneSkip {
		t.Fatalf("status=%q", out.ReplayStatus)
	}
}

func TestCleanFloorRejectsOmitWhenRequireOn(t *testing.T) {
	resetCleanEdgesRollForTest()
	t.Setenv("HACKME_POOL_CLEAN_FLOOR", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_EDGES", "1")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "floor-req.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed":  true,
		"dig_package":       "audit",
		"exec_per_unit":     8,
		"check_semantics":   "detector",
		"coverage_kind":     "wasm_edge_bitmap",
		"guided_scheduling": true,
		"wasm_check_hex":    mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":       []any{uint64(42)},
	}, "property")
	id := "floor-req"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-omit", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-omit", WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 20, SegmentExecDone: w.ExecPerUnit,
		// EdgesTouchedOK false — omit
	})
	if err == nil || !strings.Contains(err.Error(), "edges_touched required") {
		t.Fatalf("want omit reject, got %v", err)
	}
}

func TestCleanFloorDeepNeedsMoreEdges(t *testing.T) {
	t.Setenv("HACKME_POOL_CLEAN_FLOOR", "1")
	t.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	t.Setenv("HACKME_POOL_CLEAN_MIN_EDGES_DEEP", "2")
	t.Setenv("HACKME_POOL_REPLAY_SAMPLE_PCT", "0")
	t.Setenv("HACKME_POOL_CANARY", "0")

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "floor-deep.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
		"pool_distributed":  true,
		"dig_package":       "deep",
		"product_mode":      "deep",
		"exec_per_unit":     8,
		"check_semantics":   "detector",
		"coverage_kind":     "wasm_edge_bitmap",
		"guided_scheduling": true,
		"wasm_check_hex":    mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm"),
		"seed_corpus":       []any{uint64(42)},
	}, "property")
	id := "floor-deep"
	if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	_ = svc.EnsureWorkItems(ctx, id, time.Now().Unix())
	w, ok, err := svc.Claim(ctx, "w-deep", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	_, err = svc.SubmitWithOutcome(ctx, SubmitRequest{
		WorkerID: "w-deep", WorkID: w.WorkID, CampaignID: id, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, InputBytes: w.InputBytes,
		CheckResult: 0, DurationMS: 20, SegmentExecDone: w.ExecPerUnit,
		EdgesTouched: 1, EdgesTouchedOK: true,
	})
	if err == nil || !strings.Contains(err.Error(), "below floor") {
		t.Fatalf("want deep edges floor reject, got %v", err)
	}
}

func TestCleanEdgesAutoRequire(t *testing.T) {
	resetCleanEdgesRollForTest()
	t.Cleanup(resetCleanEdgesRollForTest)
	t.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "auto")
	if cleanEdgesRequireActive() {
		t.Fatal("empty roll should not auto-require")
	}
	for i := 0; i < 40; i++ {
		noteCleanEdgesReport(true)
	}
	if !cleanEdgesRequireActive() {
		t.Fatal("expected auto-require after high report rate")
	}
	st := CleanEdgesFloorStats()
	if st["edges_require_active"] != true {
		t.Fatalf("stats=%v", st)
	}
}
