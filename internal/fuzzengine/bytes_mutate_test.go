package fuzzengine

import "testing"

func TestMutateBytesDeterministic(t *testing.T) {
	base := U64LayoutToBytes(0x4c | (521 << 8))
	a := MutateBytes(base, 0, 99, 4096)
	b := MutateBytes(base, 0, 99, 4096)
	if string(a) != string(b) {
		t.Fatal("MutateBytes not deterministic")
	}
}

func TestDeterministicStagesCoverBands(t *testing.T) {
	base := []byte(`{"a":1,"b":[2,3,4]}`)
	seen := map[string]struct{}{}
	for stage := 0; stage < StageDeterministicMax; stage++ {
		out := MutateBytes(base, MutationStage(stage), 0xC0FFEE, 4096)
		if string(out) == string(base) {
			t.Fatalf("stage %d produced identical output", stage)
		}
		seen[string(out)] = struct{}{}
		// Replay stability across calls.
		again := MutateBytes(base, MutationStage(stage), 0xC0FFEE, 4096)
		if string(out) != string(again) {
			t.Fatalf("stage %d not replay-stable", stage)
		}
	}
	if len(seen) < 32 {
		t.Fatalf("expected diverse deterministic outputs, got %d unique", len(seen))
	}
}

func TestGuidedBytesUsesCorpus(t *testing.T) {
	cfg := map[string]any{"input_mode": "bytes"}
	violation := U64LayoutToBytes(PackWasmCheckInput(0x4c, 521, 0))
	seeds := []PoolCorpusSeed{{Input: PackInputBytesToU64(violation), Energy: 3}}
	linear, _ := GuidedInputForWork(11, cfg, nil)
	guided, b := GuidedInputForWork(11, cfg, seeds)
	if len(b) == 0 {
		t.Fatal("expected byte payload")
	}
	if PackInputBytesToU64(b) == linear && guided == linear {
		t.Fatal("guided bytes should differ from linear fallback")
	}
}
