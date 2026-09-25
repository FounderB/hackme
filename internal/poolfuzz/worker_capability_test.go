package poolfuzz

import "testing"

func TestHuntHarnessCapable(t *testing.T) {
	if !HuntHarnessCapable(HuntHarnessLibFuzzerOneshot) {
		t.Fatal("oneshot")
	}
	if !HuntHarnessCapable("LibFuzzer_Oneshot") {
		t.Fatal("case")
	}
	if HuntHarnessCapable("") || HuntHarnessCapable("stdin") {
		t.Fatal("old/empty must fail")
	}
}

func TestWorkerVersionAllowed(t *testing.T) {
	if !WorkerVersionAllowed("", "") {
		t.Fatal("no min allows empty")
	}
	if WorkerVersionAllowed("", "0.1.0-rc17.2") {
		t.Fatal("empty got fails when min set")
	}
	if !WorkerVersionAllowed("0.1.0-rc17.2", "0.1.0-rc17.2") {
		t.Fatal("newer rc")
	}
	if WorkerVersionAllowed("0.1.0-rc17.0", "0.1.0-rc17.2") {
		t.Fatal("older rc")
	}
	if !WorkerVersionAllowed("0.1.0-rc17.2", "0.1.0-rc17.2") {
		t.Fatal("equal")
	}
	if versionCmpLoose("0.2.0", "0.1.9") <= 0 {
		t.Fatal("0.2 > 0.1.9")
	}
	if versionCmpLoose("1.0.0", "1.0.0-rc1") <= 0 {
		t.Fatal("release > rc")
	}
}

func TestCampaignClaimTierCustomerMarkers(t *testing.T) {
	if got := campaignClaimTier("hunt-customer-acme-20260101", "x", ""); got != "customer" {
		t.Fatalf("hunt-customer- id: %q", got)
	}
	if got := campaignClaimTier("hunt-x", "Customer Hunt · acme", ""); got != "customer" {
		t.Fatalf("title marker: %q", got)
	}
	if got := campaignClaimTier("hunt-x", "x", "customer:acme"); got != "customer" {
		t.Fatalf("owner customer:: %q", got)
	}
	if got := campaignClaimTier("hunt-x", "x", "founder:me"); got != "other" {
		t.Fatalf("founder: %q", got)
	}
}
