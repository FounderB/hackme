package poolfuzz

import (
	"sync/atomic"
)

// Rolling sample of Dig CLEAN hygiene submits: whether edges_touched was reported.
// Used to auto-enable CLEAN_EDGES_REQUIRE once the fleet mostly sends the field.
const cleanEdgesRollSize = 64

var (
	cleanEdgesRoll [cleanEdgesRollSize]atomic.Uint32 // 0=empty, 1=omit, 2=reported
	cleanEdgesIdx  atomic.Uint64
)

func noteCleanEdgesReport(reported bool) {
	i := cleanEdgesIdx.Add(1) - 1
	v := uint32(1)
	if reported {
		v = 2
	}
	cleanEdgesRoll[i%cleanEdgesRollSize].Store(v)
}

// cleanEdgesReportRate returns fraction of recent Dig CLEAN samples that included edges_touched.
func cleanEdgesReportRate() (rate float64, n int) {
	var reported, total int
	for i := 0; i < cleanEdgesRollSize; i++ {
		switch cleanEdgesRoll[i].Load() {
		case 1:
			total++
		case 2:
			total++
			reported++
		}
	}
	if total == 0 {
		return 0, 0
	}
	return float64(reported) / float64(total), total
}

func resetCleanEdgesRollForTest() {
	for i := range cleanEdgesRoll {
		cleanEdgesRoll[i].Store(0)
	}
	cleanEdgesIdx.Store(0)
}

// CleanEdgesFloorStats is exposed on pool stats for ops soak decisions.
func CleanEdgesFloorStats() map[string]any {
	rate, n := cleanEdgesReportRate()
	return map[string]any{
		"edges_report_rate":    rate,
		"edges_report_samples": n,
		"edges_require_mode":   cleanEdgesRequireMode(),
		"edges_require_active": cleanEdgesRequireActive(),
		"auto_on_at_rate":      0.90,
		"auto_on_min_samples":  32,
		"clean_floor_enabled":  cleanFloorEnabled(),
		"min_ms_per_exec":      envIntClamped("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", map[bool]int{true: 1, false: 0}[cleanEdgesRequireActive()], 0, 1000),
		"min_edges_default":    envIntClamped("HACKME_POOL_CLEAN_MIN_EDGES", 1, 0, 100000),
		"min_edges_deep":       envIntClamped("HACKME_POOL_CLEAN_MIN_EDGES_DEEP", 2, 0, 100000),
	}
}
