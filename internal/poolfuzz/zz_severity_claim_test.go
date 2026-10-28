package poolfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/store"
)

// Guard: finding severity (and so the bounty slice) must come from the coordinator's replay
// trap (SubmitWithOutcome overwrites req.Trap), never from the miner's claim — relabelling a
// real finding "out of bounds" must not escalate it to critical.
func TestFindingSeverityIgnoresMinerTrapClaim(t *testing.T) {
	wasmHex := mustReadWasmHex(t, "../../tasks/artifacts/security/rust_script_push_bounds_guard.wasm")
	run := func(t *testing.T, id string, overrideTrap *string) string {
		t.Helper()
		db, err := store.Open(filepath.Join(t.TempDir(), id+".db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		svc := &Service{DB: db}
		ctx := context.Background()
		cfg := fuzzengine.NormalizeCampaignConfig(map[string]any{
			"pool_distributed": true,
			"check_semantics":  "detector",
			"wasm_check_hex":   wasmHex,
			"seed_corpus":      []any{uint64(0x4c | (521 << 8))}, // real bounds violation
			"mutation_rounds":  0,
		}, "property")
		if err := svc.RegisterCampaign(ctx, Campaign{ID: id, CampaignType: "property", Title: "t", Status: "running", BudgetRuns: 4, BudgetSeconds: 60, Config: cfg}); err != nil {
			t.Fatal(err)
		}
		if err := svc.EnsureWorkItems(ctx, id, time.Now().Unix()); err != nil {
			t.Fatal(err)
		}
		inN, actual := actualInputForWorkItem(t, ctx, svc, id, 1, cfg)
		cr, _, trap, err := ExecuteLocally(ctx, wasmHex, actual, 800)
		if err != nil || cr != 1 {
			t.Fatalf("expected a real violation: cr=%d err=%v", cr, err)
		}
		if overrideTrap != nil {
			trap = *overrideTrap
		}
		leaseWorkItemForTest(t, ctx, db, id, 1, "worker-sev")
		if err := svc.Submit(ctx, SubmitRequest{
			WorkerID: "worker-sev", WorkID: id + ":1", CampaignID: id, MinerAddress: "HMC-cccccccccccccccc",
			ItemID: 1, InputN: inN, ActualInput: actual, CheckResult: cr, DurationMS: 1, Trap: trap,
		}); err != nil {
			t.Fatal(err)
		}
		var sev string
		if err := db.QueryRowContext(ctx, `SELECT severity FROM fuzz_findings WHERE campaign_id=? LIMIT 1`, id).Scan(&sev); err != nil {
			t.Fatalf("finding for %s: %v", id, err)
		}
		return sev
	}
	honest := run(t, "sev-honest", nil)
	fake := "wasm trap: out of bounds memory access"
	claimed := run(t, "sev-claimed", &fake)
	if claimed != honest {
		t.Fatalf("miner Trap claim changed severity: honest=%q claimed=%q", honest, claimed)
	}
}
