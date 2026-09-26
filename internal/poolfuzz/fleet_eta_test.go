package poolfuzz

import "testing"

func TestEstimateFleetETASeconds(t *testing.T) {
	if got := EstimateFleetETASeconds(0, 100); got != 0 {
		t.Fatalf("done: %d", got)
	}
	if got := EstimateFleetETASeconds(100, 0); got != -1 {
		t.Fatalf("warming: %d", got)
	}
	// 3150 shards/h → 3150 remaining ≈ 1h
	got := EstimateFleetETASeconds(3150, 3150)
	if got < 3500 || got > 3700 {
		t.Fatalf("1h ETA got %d", got)
	}
}

func TestAnnotateCampaignFleetETA(t *testing.T) {
	items := []map[string]any{
		{"id": "a", "budget_runs": 100, "runs_done": 25},
		{"id": "b", "budget_runs": 8, "runs_done": 8},
	}
	AnnotateCampaignFleetETA(items, 300) // 300/h → 75 rem → 15 min = 900s
	if items[0]["remaining_runs"] != 75 {
		t.Fatalf("rem=%v", items[0]["remaining_runs"])
	}
	eta, _ := items[0]["eta_sec_fleet"].(int64)
	if eta < 850 || eta > 950 {
		t.Fatalf("eta=%v", items[0]["eta_sec_fleet"])
	}
	if items[1]["eta_sec_fleet"] != int64(0) {
		t.Fatalf("complete eta=%v", items[1]["eta_sec_fleet"])
	}
}
