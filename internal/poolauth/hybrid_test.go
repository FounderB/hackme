package poolauth

import (
	"os"
	"testing"
)

func TestHybridSignerEnabledDefaultWithWorkerToken(t *testing.T) {
	t.Setenv("HACKME_POOL_HYBRID_SIGNER_ENABLED", "")
	t.Setenv("HACKME_COORDINATOR_ALLOW_INSECURE", "")
	t.Setenv("HACKME_COORDINATOR_WORKER_TOKEN", "wt")
	if !HybridSignerEnabled() {
		t.Fatal("want hybrid on when worker token set")
	}
	t.Setenv("HACKME_POOL_HYBRID_SIGNER_ENABLED", "0")
	if HybridSignerEnabled() {
		t.Fatal("explicit 0 should disable")
	}
	_ = os.Unsetenv
}

func TestWorkerPayoutMapRawPrefersHackMePrefix(t *testing.T) {
	t.Setenv("HACKME_WORKER_PAYOUT_MAP", "a=HMC-aaaaaaaaaaaaaaaa")
	t.Setenv("WORKER_PAYOUT_MAP", "b=HMC-bbbbbbbbbbbbbbbb")
	if got := WorkerPayoutMapRaw(); got != "a=HMC-aaaaaaaaaaaaaaaa" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("HACKME_WORKER_PAYOUT_MAP", "")
	if got := WorkerPayoutMapRaw(); got != "b=HMC-bbbbbbbbbbbbbbbb" {
		t.Fatalf("fallback got %q", got)
	}
}
