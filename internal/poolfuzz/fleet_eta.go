package poolfuzz

// EstimateFleetETASeconds maps remaining Dig/Hunt shards to wall seconds via live
// fleet capacity (hybrid + dig-only heuristic shards/h). Returns -1 when capacity
// is warming / unknown; 0 when remaining is done.
func EstimateFleetETASeconds(remaining int, shardsPerHour float64) int64 {
	if remaining <= 0 {
		return 0
	}
	if shardsPerHour < 1 {
		return -1
	}
	sec := float64(remaining) / shardsPerHour * 3600.0
	if sec < 1 {
		return 1
	}
	return int64(sec + 0.5)
}

// AnnotateCampaignFleetETA sets remaining_runs + eta_sec_fleet on each marketplace row.
// Does not invent GPU-ASAN claims — ETA is pool Dig throughput only.
func AnnotateCampaignFleetETA(items []map[string]any, shardsPerHour float64) {
	for _, item := range items {
		if item == nil {
			continue
		}
		bud := intFromJSON(item["budget_runs"])
		done := intFromJSON(item["runs_done"])
		rem := bud - done
		if rem < 0 {
			rem = 0
		}
		item["remaining_runs"] = rem
		item["eta_sec_fleet"] = EstimateFleetETASeconds(rem, shardsPerHour)
	}
}
