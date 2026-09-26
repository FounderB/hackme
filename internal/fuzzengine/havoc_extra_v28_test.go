package fuzzengine

import (
	"testing"
)

func TestDeepHavocV28Deterministic(t *testing.T) {
	base := []byte(`{"a":1,"nested":{"b":[1,2,3]}}`)
	dict := []byte(`"null""true""false"{}[]`)
	corpus := [][]byte{[]byte(`{}`), []byte(`[1]`), base}
	cfg := map[string]any{"havoc_deep_v28": true, "mutator_dict": dict}
	a := MutateBytesForHunt(base, StageHavocBase+12, 424242, 256, cfg, corpus)
	b := MutateBytesForHunt(base, StageHavocBase+12, 424242, 256, cfg, corpus)
	if string(a) != string(b) {
		t.Fatal("deep v28 must stay deterministic for pool replay")
	}
}

func TestDeepHavocV28OptInDoesNotChangeCore(t *testing.T) {
	base := []byte(`{"hello":"world"}`)
	dict := []byte(`"a""b"`)
	corpus := [][]byte{[]byte(`{}`), base}
	core := MutateBytesForHunt(base, StageHavocBase+5, 99, 128, map[string]any{"mutator_dict": dict}, corpus)
	// Explicit false must match core path
	off := MutateBytesForHunt(base, StageHavocBase+5, 99, 128, map[string]any{"mutator_dict": dict, "havoc_deep_v28": false}, corpus)
	if string(core) != string(off) {
		t.Fatal("havoc_deep_v28=false must match core v2.7 path")
	}
	deep := MutateBytesForHunt(base, StageHavocBase+5, 99, 128, map[string]any{"mutator_dict": dict, "havoc_deep_v28": true}, corpus)
	if string(core) == string(deep) {
		t.Fatal("deep v28 should change output vs core for same salt")
	}
}

func TestCompareDeepV28ABGain(t *testing.T) {
	base := []byte(`{"a":1,"nested":{"b":[1,2,3],"tag":"<x/>"}}`)
	dict := []byte(`"null""true""false""a""nested"{}[]`)
	corpus := [][]byte{
		[]byte(`{}`), []byte(`[1,2]`), []byte(`{"x":null}`),
		[]byte(`<root><child/></root>`), base,
	}
	rep := CompareDeepV28Detailed(base, dict, corpus, 5000, 256)
	t.Logf("deep-v28 detailed: %+v", rep)
	if rep.DiffRate < 0.95 {
		t.Fatalf("deep should rewrite nearly all samples, diff_rate=%.3f", rep.DiffRate)
	}
	if rep.AvgByteDelta < 50 {
		t.Fatalf("expected meaningful byte delta, got %.1f", rep.AvgByteDelta)
	}
	if rep.AvgLenDelta < 5 {
		t.Fatalf("expected length movement, got %.1f", rep.AvgLenDelta)
	}
	if rep.DeepUnique < rep.CoreUnique {
		t.Fatalf("unique should not regress: core=%d deep=%d", rep.CoreUnique, rep.DeepUnique)
	}
	if rep.DeepLens < rep.CoreLens {
		t.Fatalf("lens diversity should not shrink: core=%d deep=%d", rep.CoreLens, rep.DeepLens)
	}
	if rep.SatDeepUnique < rep.SatCoreUnique {
		t.Fatalf("saturated unique must not regress: core=%d deep=%d", rep.SatCoreUnique, rep.SatDeepUnique)
	}
}

func TestWalkingNBitFlipDeterministic(t *testing.T) {
	base := []byte{0x00, 0xff, 0x55, 0xaa, 0x01, 0x02, 0x03, 0x04}
	a := walkingNBitFlip(base, 123)
	b := walkingNBitFlip(base, 123)
	if string(a) != string(b) {
		t.Fatal("walkingNBitFlip must be deterministic")
	}
	if string(a) == string(base) {
		t.Fatal("expected bits flipped")
	}
}
