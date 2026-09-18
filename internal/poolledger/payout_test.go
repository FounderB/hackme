package poolledger

import (
	"strings"
	"testing"
)

func TestCheckTreasuryInvariantDriftFails(t *testing.T) {
	err := CheckTreasuryInvariant(1.0, 1.000002)
	if err == nil || !strings.Contains(err.Error(), LedgerDriftDetected) {
		t.Fatalf("want %s, got %v", LedgerDriftDetected, err)
	}
}

func TestComputeAttemptPayoutFoundOnly(t *testing.T) {
	p := ComputeAttemptPayout(1_000_000, 1.0, false, 0.25, true)
	if p != 0 {
		t.Fatalf("found-only non-hit want 0, got %v", p)
	}
}

func TestComputeAttemptPayoutLiveChainSolveWipesBonus(t *testing.T) {
	p := ComputeAttemptPayoutLive(1_000_000, 1.0, true, 0.25, false, true, 0)
	if p != 1.0 {
		t.Fatalf("chainSolveOK should wipe found bonus, got %v", p)
	}
}

func TestComputeAttemptPayoutLiveClamp(t *testing.T) {
	p := ComputeAttemptPayoutLive(10_000_000, 1.0, true, 0.25, false, false, 2.0)
	if p != 2.0 {
		t.Fatalf("want clamp 2.0, got %v", p)
	}
}
