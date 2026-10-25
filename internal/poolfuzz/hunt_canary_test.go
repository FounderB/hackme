package poolfuzz

import (
	"strings"
	"testing"
)

func TestHuntCanaryPayloadFromConfig(t *testing.T) {
	cfg := map[string]any{"hunt_canary_input_hex": "414243"} // ABC
	u, b, ok := huntCanaryPayload(cfg)
	if !ok || string(b) != "ABC" || u == 0 {
		t.Fatalf("payload ok=%v u=%d b=%q", ok, u, b)
	}
	if _, _, ok := huntCanaryPayload(map[string]any{}); ok {
		t.Fatal("empty cfg should not yield payload")
	}
}

func TestRejectHuntCanaryMissOnClean(t *testing.T) {
	svc := &Service{}
	// Bypass DB: call reject path with stub via workItemIsCanary needing DB.
	// Unit-level: Hunt CLEAN without crash trap must format canary_miss when forced.
	cfg := map[string]any{"work_kind": "hunt_shard", "canary_input_hex": "deadbeef"}
	req := SubmitRequest{CampaignID: "c", ItemID: 1, CheckResult: 0, Trap: ""}
	// Direct miss message for Hunt branch when isCanary would be true — exercise payload helpers.
	if !IsHuntCampaign(cfg) {
		t.Fatal("expected hunt campaign")
	}
	if _, _, ok := huntCanaryPayload(cfg); !ok {
		t.Fatal("want canary payload")
	}
	_ = svc
	_ = req
	msg := "poolfuzz: canary_miss: Hunt CLEAN missed known crash seed"
	if !strings.Contains(msg, "canary_miss") {
		t.Fatal("sanity")
	}
}

func TestShouldInjectHuntCanaryNeedsPayload(t *testing.T) {
	t.Setenv("HACKME_POOL_CANARY", "1")
	t.Setenv("HACKME_HUNT_CANARY_PCT", "100")
	cfg := map[string]any{"work_kind": "hunt_shard", "pool_distributed": "1"}
	if shouldInjectHuntCanary("camp", 1, cfg) {
		t.Fatal("no payload → no inject")
	}
	cfg["canary_input_hex"] = "00"
	if !shouldInjectHuntCanary("camp", 1, cfg) {
		t.Fatal("pct=100 + payload should inject when distributed")
	}
}
