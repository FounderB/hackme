package poolfuzz

import (
	"fmt"
	"os"
	"strings"

	"hackme/internal/fuzzengine"
	"hackme/internal/fuzzingcli"
)

// Clean hygiene floor rejects empty/fake Dig CLEAN submits without full segment replay.
// Defaults tighten after fleet soak: edges_touched auto-required when ≥90% of recent
// Dig CLEAN samples report it; deep SKU uses a higher edge floor.
//
// Env knobs:
//
//	HACKME_POOL_CLEAN_FLOOR            default ON (0/false/off to disable)
//	HACKME_POOL_CLEAN_MIN_DURATION_MS  absolute floor (default 1 for multi-exec)
//	HACKME_POOL_CLEAN_MIN_MS_PER_EXEC  per-exec ms floor (default 0; auto 1 when edges require active)
//	HACKME_POOL_CLEAN_MIN_EDGES        min unique edges when reported (default 1 smoke / 2 deep)
//	HACKME_POOL_CLEAN_MIN_EDGES_DEEP   override deep min edges (default 2)
//	HACKME_POOL_CLEAN_EDGES_REQUIRE    1/on = always require; 0/off = never;
//	                                   unset/auto = ON when fleet report rate ≥ 90% (n≥32)

func cleanFloorEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_POOL_CLEAN_FLOOR")))
	if v == "" {
		return true
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func cleanEdgesRequireMode() string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_POOL_CLEAN_EDGES_REQUIRE")))
	if v == "" || v == "auto" {
		return "auto"
	}
	switch v {
	case "0", "false", "no", "off":
		return "off"
	case "1", "true", "yes", "on":
		return "on"
	default:
		return "auto"
	}
}

func cleanEdgesRequireActive() bool {
	switch cleanEdgesRequireMode() {
	case "on":
		return true
	case "off":
		return false
	default:
		rate, n := cleanEdgesReportRate()
		return n >= 32 && rate >= 0.90
	}
}

// checkCleanHygieneFloor validates Dig CLEAN hygiene_skip submits before pay.
// Legacy workers omit EdgesTouchedOK — soft until REQUIRE=on or auto fleet threshold.
func checkCleanHygieneFloor(req SubmitRequest, execPer int, cfg map[string]any) error {
	if !cleanFloorEnabled() || execPer <= 1 {
		return nil
	}
	bitmap := fuzzengine.CoverageUsesWasmEdge(cfg)
	if bitmap {
		noteCleanEdgesReport(req.EdgesTouchedOK)
	}

	minDur := envIntClamped("HACKME_POOL_CLEAN_MIN_DURATION_MS", 1, 0, 600000)
	msDef := 0
	if cleanEdgesRequireActive() {
		msDef = 1 // tighten duration with edges require (fleet upgraded)
	}
	msPer := envIntClamped("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", msDef, 0, 1000)
	need := minDur
	if msPer > 0 {
		per := execPer * msPer
		if per > need {
			need = per
		}
	}
	if req.DurationMS < need {
		return fmt.Errorf("poolfuzz: clean_floor: duration_ms %d below floor %d (exec=%d)", req.DurationMS, need, execPer)
	}
	if !bitmap {
		return nil
	}

	pkg := strings.TrimSpace(strings.ToLower(jsonString(cfg["dig_package"])))
	if pkg == "" {
		pkg = fuzzingcli.DigPackageFromDepthTier(fuzzengine.ParseDepthTier(cfg))
	}
	deep := fuzzingcli.IsDeepDigPackage(pkg)
	minEdges := envIntClamped("HACKME_POOL_CLEAN_MIN_EDGES", 1, 0, 100000)
	if deep {
		minEdges = envIntClamped("HACKME_POOL_CLEAN_MIN_EDGES_DEEP", 2, 0, 100000)
	}
	if minEdges <= 0 {
		return nil
	}
	if !req.EdgesTouchedOK {
		if cleanEdgesRequireActive() {
			return fmt.Errorf("poolfuzz: clean_floor: edges_touched required for wasm_edge_bitmap CLEAN")
		}
		return nil
	}
	if req.EdgesTouched < minEdges {
		return fmt.Errorf("poolfuzz: clean_floor: edges_touched %d below floor %d", req.EdgesTouched, minEdges)
	}
	return nil
}
