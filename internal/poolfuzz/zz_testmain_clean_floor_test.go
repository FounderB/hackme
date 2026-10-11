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
	resetCleanEdgesRollForTest()
	os.Exit(m.Run())
}
