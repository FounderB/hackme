package fuzzengine

import "testing"

func TestCorpusSnapshotRoundTrip(t *testing.T) {
	seeds := []PoolCorpusSeed{
		{Input: 42, InputBytes: []byte("GITHUB_PAT=ghp_test"), Energy: 3, Edge: 7, Path: 11},
		{Input: 99, InputBytes: []byte{0, 1, 2}, Energy: 1, Edge: 2, Path: 3},
	}
	b, sha, err := EncodeCorpusSnapshot(seeds)
	if err != nil {
		t.Fatal(err)
	}
	if sha == "" || len(b) == 0 {
		t.Fatal("expected snapshot bytes")
	}
	got, err := DecodeCorpusSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(seeds) {
		t.Fatalf("len=%d want %d", len(got), len(seeds))
	}
	if string(got[0].InputBytes) != string(seeds[0].InputBytes) {
		t.Fatalf("bytes mismatch")
	}
	maps := CorpusSeedsClaimMaps(seeds)
	back, err := CorpusSeedsFromClaimMaps(maps)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(seeds) || back[0].Input != seeds[0].Input {
		t.Fatalf("claim maps roundtrip failed")
	}
}

// Workers receive corpus seeds through the claim maps while the coordinator's
// verification replay loads the frozen snapshot; both channels must yield the
// same seed tuples or the two sides schedule different inputs. Crash seeds and
// empty-InputBytes seeds are the lossy edge cases — they must degrade
// identically on both channels.
func TestClaimMapsMatchSnapshotChannel(t *testing.T) {
	seeds := []PoolCorpusSeed{
		{Input: 42, InputBytes: []byte(`{"a":1}`), Energy: 3, Edge: 7, Path: 11},
		{Input: 99, Energy: 1, Edge: 2, Path: 3, Crash: true}, // empty InputBytes + Crash flag
		{InputBytes: []byte(`{"c":"x"}`), Energy: 5, Edge: 9}, // zero Input u64
	}
	viaClaim, err := CorpusSeedsFromClaimMaps(CorpusSeedsClaimMaps(seeds))
	if err != nil {
		t.Fatal(err)
	}
	snapJSON, _, err := EncodeCorpusSnapshot(seeds)
	if err != nil {
		t.Fatal(err)
	}
	viaSnapshot, err := DecodeCorpusSnapshot(snapJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(viaClaim) != len(viaSnapshot) {
		t.Fatalf("channel sizes differ: claim %d vs snapshot %d", len(viaClaim), len(viaSnapshot))
	}
	for i := range viaSnapshot {
		a, b := viaClaim[i], viaSnapshot[i]
		if a.Input != b.Input || a.Energy != b.Energy || a.Edge != b.Edge || a.Path != b.Path ||
			string(a.InputBytes) != string(b.InputBytes) || a.Crash != b.Crash {
			t.Fatalf("seed %d differs across channels: claim %+v vs snapshot %+v", i, a, b)
		}
	}
}
