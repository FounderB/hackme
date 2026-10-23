package poolfuzz

import (
	"os"
	"testing"
)

// Package defaults keep Dig CLEAN floors from poisoning unrelated poolfuzz tests.
// Individual tests opt into REQUIRE / MS_PER_EXEC via t.Setenv.
func TestMain(m *testing.M) {
	_ = os.Setenv("HACKME_POOL_CLEAN_EDGES_REQUIRE", "0")
	_ = os.Setenv("HACKME_POOL_CLEAN_MIN_MS_PER_EXEC", "0")
	// Package tests historically trusted miner CLEAN on hygiene_skip (MinimalGate+
	// detector). Production default for HACKME_POOL_HYGIENE_PROVE is ON; opt out
	// here and let hygiene_prove_test.go opt back in.
	_ = os.Setenv("HACKME_POOL_HYGIENE_PROVE", "0")
	if os.Getenv("HACKME_POOL_CANARY") == "" {
		_ = os.Setenv("HACKME_POOL_CANARY", "0")
	}
	// Legacy Hunt tests use byte L1 + iterations_per_shard. Coverage-engine tests
	// opt in via t.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "1").
	if os.Getenv("HACKME_HUNT_COVERAGE_ENGINE") == "" {
		_ = os.Setenv("HACKME_HUNT_COVERAGE_ENGINE", "0")
	}
	// Hunt isolate STRICT defaults ON (no bare after cage fail). CI/dev hosts often
	// lack userns for bwrap uid maps — allow bare fallback like hunt/*_test.go.
	if os.Getenv("HACKME_HUNT_ISOLATE_STRICT") == "" {
		_ = os.Setenv("HACKME_HUNT_ISOLATE_STRICT", "0")
	}
	resetCleanEdgesRollForTest()
	os.Exit(m.Run())
}
