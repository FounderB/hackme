package main

import (
	"testing"

	"hackme/internal/poolsync"
)

func TestCoordinatorAdminTokenPrefersOrdersPoolAdminEnv(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "")
	t.Setenv("HACKME_POOL_COORDINATOR_ADMIN_TOKEN", "orders-admin-secret")
	t.Setenv("HACKME_POOL_COORDINATOR_TOKEN", "worker-only-token")

	got := coordinatorAdminToken()
	if got != "orders-admin-secret" {
		t.Fatalf("coordinatorAdminToken()=%q want orders-admin-secret (Orders publish token)", got)
	}
	if poolsync.CoordinatorAdminToken() != got {
		t.Fatalf("pool.go helper drifted from poolsync.CoordinatorAdminToken: %q vs %q", got, poolsync.CoordinatorAdminToken())
	}
}

func TestCoordinatorAdminTokenDoesNotFallBackToWorkerToken(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "")
	t.Setenv("HACKME_POOL_COORDINATOR_ADMIN_TOKEN", "")
	t.Setenv("HACKME_POOL_COORDINATOR_TOKEN", "worker-only-token")

	got := coordinatorAdminToken()
	if got != "" {
		t.Fatalf("coordinatorAdminToken()=%q want empty — worker token must not unlock admin progress/findings", got)
	}
}

func TestCoordinatorAdminTokenHubAdminStillWins(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "hub-admin")
	t.Setenv("HACKME_POOL_COORDINATOR_ADMIN_TOKEN", "orders-admin")
	t.Setenv("HACKME_POOL_COORDINATOR_TOKEN", "worker")

	got := coordinatorAdminToken()
	if got != "hub-admin" {
		t.Fatalf("coordinatorAdminToken()=%q want hub-admin", got)
	}
}
