package poolfuzz

import (
	"context"
	"testing"

	"hackme/internal/fuzzengine"
	"hackme/internal/hunt"
)

func TestHuntEngineByteFallbackDefaultOff(t *testing.T) {
	t.Setenv("HACKME_HUNT_ENGINE_BYTE_FALLBACK", "")
	if HuntEngineAllowByteFallback() {
		t.Fatal("default OFF for SKU honesty")
	}
	t.Setenv("HACKME_HUNT_ENGINE_BYTE_FALLBACK", "1")
	if !HuntEngineAllowByteFallback() {
		t.Fatal("explicit ON")
	}
	t.Setenv("HACKME_HUNT_ENGINE_BYTE_FALLBACK_DEEP", "")
	if HuntEngineAllowByteFallbackForConfig(map[string]any{"hunt_package": "hunt_heavy"}) {
		t.Fatal("deep must not fallback without DEEP=1")
	}
}

func TestDecideHuntEnginePayIgnoresMinerEdges(t *testing.T) {
	t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1")
	// Fake miner edges must not unlock full pay without coordinator proof.
	kind, full := DecideHuntEnginePay(0, 0, 0, 9999, true)
	if full || kind != HuntEnginePayFloor {
		t.Fatalf("empty CLEAN want floor, got kind=%s full=%v", kind, full)
	}
	kind, full = DecideHuntEnginePay(3, 2, 0, 0, false)
	if !full || kind != HuntEnginePayEdges {
		t.Fatalf("proven edges want full, got kind=%s full=%v", kind, full)
	}
	kind, full = DecideHuntEnginePay(0, 0, 1, 0, false)
	if !full || kind != HuntEnginePayCrash {
		t.Fatalf("crash want full, got kind=%s full=%v", kind, full)
	}
	kind, full = DecideHuntEnginePay(0, 5, 0, 100, true)
	if full || kind != HuntEnginePayFloor {
		t.Fatalf("seeds without proven edges → floor, got kind=%s full=%v", kind, full)
	}
}

func TestWorkerSupportsHuntEngine(t *testing.T) {
	if WorkerSupportsHuntEngine(nil, hunt.EngineLibFuzzer) {
		t.Fatal("legacy must not imply libfuzzer")
	}
	if !WorkerSupportsHuntEngine(nil, hunt.EngineByte) {
		t.Fatal("legacy implies byte")
	}
	ads := []string{hunt.EngineLibFuzzer, hunt.EngineByte}
	if !WorkerSupportsHuntEngine(ads, hunt.EngineLibFuzzer) {
		t.Fatal("libfuzzer advertised")
	}
	if WorkerSupportsHuntEngine(ads, hunt.EngineAFLPP) {
		t.Fatal("afl not advertised")
	}
}

func TestWorkerHuntEnginesParse(t *testing.T) {
	got := WorkerHuntEngines("libfuzzer,aflpp,byte,libfuzzer")
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestLeaseProvenTrack(t *testing.T) {
	clearLeaseProven("c1", 7)
	noteLeaseProven("c1", 7, 2, 1, 0, hunt.EngineLibFuzzer, "v1")
	noteLeaseProven("c1", 7, 1, 0, 1, "", "")
	tr := peekLeaseProven("c1", 7)
	if tr.SeedsAccepted != 3 || tr.ProvenEdges != 1 || tr.CrashesOK != 1 {
		t.Fatalf("peek %#v", tr)
	}
	taken := takeLeaseProven("c1", 7)
	if taken.ProvenEdges != 1 {
		t.Fatalf("take %#v", taken)
	}
	if peekLeaseProven("c1", 7).SeedsAccepted != 0 {
		t.Fatal("should be cleared")
	}
}

func TestApplyHuntEngineByteFallback(t *testing.T) {
	w := &ClaimedWork{
		HuntEngine:         hunt.EngineLibFuzzer,
		EngineLeaseSec:     60,
		ExecPerUnit:        1,
		IterationsPerShard: 48,
		CoverageKind:       "hunt_libfuzzer",
	}
	ApplyHuntEngineByteFallback(w, map[string]any{"iterations_per_shard": 48})
	if w.HuntEngine != hunt.EngineByte || w.EngineLeaseSec != 0 || w.ExecPerUnit != 48 {
		t.Fatalf("fallback %#v", w)
	}
}

func TestHuntSubmitExecExpected(t *testing.T) {
	t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1")
	cfg := map[string]any{"iterations_per_shard": 48, "work_kind": "hunt_shard", "hunt_engine": "libfuzzer"}
	if got := huntSubmitExecExpected(cfg, SubmitRequest{HuntEngine: hunt.EngineLibFuzzer}); got != 1 {
		t.Fatalf("engine want 1 got %d", got)
	}
	if got := huntSubmitExecExpected(cfg, SubmitRequest{HuntEngine: hunt.EngineByte}); got != 48 {
		t.Fatalf("byte want 48 got %d", got)
	}
	// Legacy omit → L1 iterations (must not infer libfuzzer from campaign cfg).
	if got := huntSubmitExecExpected(cfg, SubmitRequest{}); got != 48 {
		t.Fatalf("legacy omit want 48 got %d", got)
	}
	if huntEngineSubmitMode(SubmitRequest{}) {
		t.Fatal("empty hunt_engine must not be engine mode")
	}
}

func TestBuildHuntClaimCoverageEngine(t *testing.T) {
	t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1")
	t.Setenv("HACKME_HUNT_ENGINE", "libfuzzer")
	// Smoke: parsing + exec expected only (full claim needs harness DB).
	cfg := map[string]any{
		"work_kind": "hunt_shard", "campaign_type": "hunt",
		"hunt_engine": "libfuzzer", "iterations_per_shard": 48,
	}
	if eng := hunt.HuntEngineFromConfig(cfg); eng != hunt.EngineLibFuzzer {
		t.Fatalf("engine %q", eng)
	}
	if !HuntEngineEnabled() {
		t.Fatal("enabled")
	}
}

func TestProveHuntCorpusSeedEdgesRequiresHarnessExec(t *testing.T) {
	t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1")
	t.Setenv("HACKME_POOL_HUNT_REPLAY", "1")
	orig := huntEngineSeedExec
	t.Cleanup(func() { huntEngineSeedExec = orig })

	// Without harness exec, structural novelty must not count.
	huntEngineSeedExec = func(context.Context, *Service, string, map[string]any, []byte) (bool, bool, string, error) {
		return false, false, "", nil
	}
	svc := &Service{}
	cfg := map[string]any{
		"work_kind": "hunt_shard", "campaign_type": "hunt",
		"upstream_target_id": "jsmn", "harness_hash": "x",
	}
	seeds := []fuzzengine.PoolCorpusSeed{{InputBytes: []byte(`{"novel":true}`), Energy: 2}}
	n, err := svc.ProveHuntCorpusSeedEdges(context.Background(), "c-prove", cfg, seeds, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want 0 proven without harness exec, got %d", n)
	}

	// Harness CLEAN + new bucket → counts (recordCoverage needs DB; use stub Service with nil DB
	// would fail — only assert crash path skips edge count).
	huntEngineSeedExec = func(context.Context, *Service, string, map[string]any, []byte) (bool, bool, string, error) {
		return true, true, "hunt_crash:test", nil
	}
	n, err = svc.ProveHuntCorpusSeedEdges(context.Background(), "c-prove", cfg, seeds, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("crash seeds must not count as edge prove, got %d", n)
	}
}
