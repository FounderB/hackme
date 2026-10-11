package hunt

import "testing"

func TestParseHuntEngine(t *testing.T) {
	cases := map[string]string{
		"":           EngineLibFuzzer,
		"auto":       EngineLibFuzzer,
		"libfuzzer":  EngineLibFuzzer,
		"aflpp":      EngineAFLPP,
		"afl++":      EngineAFLPP,
		"byte":       EngineByte,
		"legacy":     EngineByte,
		"nope":       EngineByte,
	}
	for in, want := range cases {
		if got := ParseHuntEngine(in); got != want {
			t.Fatalf("ParseHuntEngine(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsCoverageEngine(t *testing.T) {
	if !IsCoverageEngine("libfuzzer") || !IsCoverageEngine("aflpp") {
		t.Fatal("coverage")
	}
	if IsCoverageEngine("byte") {
		t.Fatal("byte not coverage")
	}
}

func TestEngineLeaseSecClamp(t *testing.T) {
	if got := clampEngineLeaseSec(1); got != MinEngineLeaseSec {
		t.Fatalf("min %d", got)
	}
	if got := clampEngineLeaseSec(9999); got != MaxEngineLeaseSec {
		t.Fatalf("max %d", got)
	}
}

func TestHuntEngineFromConfigExplicit(t *testing.T) {
	cfg := map[string]any{"hunt_engine": "aflpp"}
	if got := HuntEngineFromConfig(cfg); got != EngineAFLPP {
		t.Fatalf("got %q", got)
	}
	cfg["hunt_engine"] = "byte"
	if got := HuntEngineFromConfig(cfg); got != EngineByte {
		t.Fatalf("byte got %q", got)
	}
}

func TestEngineProfileDeepFromPackage(t *testing.T) {
	if got := EngineProfileFromConfig(map[string]any{"hunt_package": "hunt_heavy"}); got != EngineProfileDeep {
		t.Fatalf("got %q", got)
	}
	if got := EngineProfileFromConfig(map[string]any{"hunt_package": "hunt_lite"}); got != EngineProfileFleet {
		t.Fatalf("lite got %q", got)
	}
}

func TestCoverageKindForEngine(t *testing.T) {
	if CoverageKindForEngine(EngineLibFuzzer) != "hunt_libfuzzer" {
		t.Fatal("lf")
	}
	if CoverageKindForEngine(EngineAFLPP) != "hunt_aflpp" {
		t.Fatal("afl")
	}
}

func TestRunnerAvailableByte(t *testing.T) {
	ok, reason := RunnerAvailable(EngineByte)
	if !ok || reason != "" {
		t.Fatalf("byte always available: %v %s", ok, reason)
	}
}
