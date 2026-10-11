package fuzzingcli

import (
	"testing"

	"hackme/internal/fuzzengine"
)

func TestB2BPackageFor(t *testing.T) {
	p, err := B2BPackageFor("audit")
	if err != nil {
		t.Fatal(err)
	}
	if p.DepthTier != fuzzengine.DepthWasmNative || p.BudgetHMC != 5 || !p.PoolDistributed {
		t.Fatalf("audit package: %+v", p)
	}
	if p.BudgetSeconds < 3600 {
		t.Fatalf("audit should have multi-hour SLA budget_seconds, got %d", p.BudgetSeconds)
	}
	if len(p.SignalTypes) < 2 {
		t.Fatalf("audit signals=%v", p.SignalTypes)
	}
	if _, err := B2BPackageFor("nope"); err == nil {
		t.Fatal("expected error for unknown package")
	}
	if p, err := B2BPackageFor("pro"); err != nil || p.Name != "audit" {
		t.Fatalf("pro alias: %+v err=%v", p, err)
	}
}

func TestB2BPackagesHonestDepth(t *testing.T) {
	scan, _ := B2BPackageFor("scan")
	audit, _ := B2BPackageFor("audit")
	deep, _ := B2BPackageFor("deep")
	if scan.DepthTier == audit.DepthTier || audit.DepthTier == deep.DepthTier {
		t.Fatal("packages must use distinct depth tiers")
	}
	if deep.DepthTier != fuzzengine.DepthBytesCorpus {
		t.Fatalf("deep tier=%s", deep.DepthTier)
	}
	if deep.BudgetRuns <= audit.BudgetRuns || deep.BudgetSeconds <= audit.BudgetSeconds {
		t.Fatalf("deep must exceed audit on runs and wall clock: deep=%+v audit=%+v", deep, audit)
	}
	if deep.BudgetHMC <= audit.BudgetHMC || deep.RewardHMC <= audit.RewardHMC {
		t.Fatalf("deep money must exceed audit: deep=%+v audit=%+v", deep, audit)
	}
	if deep.MutationRounds < 8 || !deep.CoverageGuided {
		t.Fatalf("deep mutation/coverage: %+v", deep)
	}
	if scan.CreatePoHOrder || !audit.CreatePoHOrder || !deep.CreatePoHOrder {
		t.Fatalf("PoH flags scan=%v audit=%v deep=%v", scan.CreatePoHOrder, audit.CreatePoHOrder, deep.CreatePoHOrder)
	}
	dq := QuoteDigBilling(deep)
	aq := QuoteDigBilling(audit)
	if dq.EscrowSplit != "40_60" || aq.EscrowSplit != "20_80" {
		t.Fatalf("billing splits deep=%s audit=%s", dq.EscrowSplit, aq.EscrowSplit)
	}
	if dq.EstPerRunHMC <= aq.EstPerRunHMC {
		t.Fatalf("deep per-run should pay miners more: deep=%v audit=%v", dq.EstPerRunHMC, aq.EstPerRunHMC)
	}
	if dq.BillingModel != "deep_depth" || aq.BillingModel != "smoke_runs" {
		t.Fatalf("billing models deep=%s audit=%s", dq.BillingModel, aq.BillingModel)
	}
}

func TestIsLoopbackBase(t *testing.T) {
	if !IsLoopbackBase("http://127.0.0.1:8080") {
		t.Fatal("127.0.0.1 should be loopback")
	}
	if !IsLoopbackBase("http://localhost:8080") {
		t.Fatal("localhost should be loopback")
	}
	if IsLoopbackBase("https://hackme.tech") {
		t.Fatal("hackme.tech should not be loopback")
	}
	// Report #24: userinfo must not spoof loopback (real dial host is after @).
	for _, bad := range []string{
		"http://127.0.0.1:8080@evil.example",
		"http://localhost:99@evil.example",
		"http://127.0.0.1:8080@evil.example/",
		"http://localhost:99@evil.example/",
	} {
		if IsLoopbackBase(bad) {
			t.Fatalf("userinfo spoof must be blocked: %q", bad)
		}
	}
}
