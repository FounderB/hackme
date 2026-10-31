package hunt

import (
	"testing"
)

func TestBuildSandboxEnabledDefaultOn(t *testing.T) {
	t.Setenv("HACKME_HUNT_BUILD_SANDBOX", "")
	if !BuildSandboxEnabled() {
		t.Fatal("empty env: build sandbox should default ON")
	}
	t.Setenv("HACKME_HUNT_BUILD_SANDBOX", "0")
	if BuildSandboxEnabled() {
		t.Fatal("HACKME_HUNT_BUILD_SANDBOX=0 should disable cage")
	}
}

func TestUniqueBindPathsDedupes(t *testing.T) {
	got := uniqueBindPaths("/tmp/a", "/tmp/a", "", "/tmp/b")
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}
