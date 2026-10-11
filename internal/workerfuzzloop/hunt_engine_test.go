package workerfuzzloop

import (
	"strings"
	"testing"

	"hackme/internal/hunt"
)

func TestDetectHuntEnginesIncludesByte(t *testing.T) {
	got := DetectHuntEngines()
	found := false
	for _, e := range got {
		if e == hunt.EngineByte {
			found = true
		}
	}
	if !found {
		t.Fatalf("byte missing: %v", got)
	}
	csv := HuntEnginesClaimCSV()
	if !strings.Contains(csv, hunt.EngineByte) {
		t.Fatalf("csv %q", csv)
	}
}

func TestClaimRespEngineFieldsJSON(t *testing.T) {
	cr := ClaimResp{HuntEngine: hunt.EngineLibFuzzer, EngineLeaseSec: 60, EngineProfile: "fleet"}
	if !hunt.IsCoverageEngine(cr.HuntEngine) {
		t.Fatal("coverage")
	}
	if cr.EngineLeaseSec != 60 {
		t.Fatal("lease")
	}
}

func TestCorpusSeedBytes(t *testing.T) {
	b, ok := corpusSeedBytes(map[string]any{"input_bytes_hex": "7b7d"})
	if !ok || string(b) != "{}" {
		t.Fatalf("got %q ok=%v", b, ok)
	}
	if _, ok := corpusSeedBytes(nil); ok {
		t.Fatal("nil")
	}
}
