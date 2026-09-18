package poolsync

import "testing"

func TestFuzzSettlePullEnabled(t *testing.T) {
	t.Setenv("HACKME_FUZZ_SETTLE_PULL", "")
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "")
	t.Setenv("HACKME_POOL_COORDINATOR_ADMIN_TOKEN", "")
	t.Setenv("HACKME_POOL_COORDINATOR_TOKEN", "")
	if FuzzSettlePullEnabled() {
		t.Fatal("want disabled without admin token")
	}
	// Worker pool token must NOT enable settle pull (public pool returns 401).
	t.Setenv("HACKME_POOL_COORDINATOR_TOKEN", "worker-only-token")
	if FuzzSettlePullEnabled() {
		t.Fatal("worker pool token must not enable settle pull")
	}
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "admin-test")
	if !FuzzSettlePullEnabled() {
		t.Fatal("want enabled with admin token")
	}
	t.Setenv("HACKME_FUZZ_SETTLE_PULL", "0")
	if FuzzSettlePullEnabled() {
		t.Fatal("want disabled when HACKME_FUZZ_SETTLE_PULL=0")
	}
}

func TestPreferPoolSyncDirect(t *testing.T) {
	t.Setenv("HACKME_POOL_DIRECT", "")
	t.Setenv("HACKME_DESKTOP_GPU_POOL", "1")
	if !preferPoolSyncDirect() {
		t.Fatal("desktop gpu pool should prefer direct direct")
	}
}
