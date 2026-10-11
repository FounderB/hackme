package fuzzingcli

import "testing"

func TestQuoteDigBillingSmokeVsDeep(t *testing.T) {
	audit, err := B2BPackageFor("audit")
	if err != nil {
		t.Fatal(err)
	}
	deep, err := B2BPackageFor("deep")
	if err != nil {
		t.Fatal(err)
	}
	aq := QuoteDigBilling(audit)
	dq := QuoteDigBilling(deep)
	if aq.ProductMode != "smoke" || dq.ProductMode != "deep" {
		t.Fatalf("modes audit=%s deep=%s", aq.ProductMode, dq.ProductMode)
	}
	if !dq.SeedsMerged || aq.SeedsMerged {
		t.Fatalf("seeds_merged audit=%v deep=%v", aq.SeedsMerged, dq.SeedsMerged)
	}
	if dq.RunsPoolShare != 0.40 || aq.RunsPoolShare != 0.20 {
		t.Fatalf("shares audit=%v deep=%v", aq.RunsPoolShare, dq.RunsPoolShare)
	}
}
