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
	resetCleanEdgesRollForTest()
	os.Exit(m.Run())
}
