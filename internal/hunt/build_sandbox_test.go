package hunt

import (
	"strings"
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

func TestScrubBuildEnvDropsHubTokens(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HACKME_ADMIN_TOKEN=admin",
		"HACKME_SETTLEMENT_ADMIN_TOKEN=settle",
		"HACKME_COORDINATOR_ADMIN_TOKEN=coord",
		"AFL_PATH=/opt/afl",
	}
	out := ScrubBuildEnv(in)
	joined := strings.Join(out, "\n")
	for _, bad := range []string{"HACKME_ADMIN_TOKEN=", "HACKME_SETTLEMENT_ADMIN_TOKEN=", "HACKME_COORDINATOR_ADMIN_TOKEN="} {
		if strings.Contains(joined, bad) {
			t.Fatalf("token leaked: %s\n%s", bad, joined)
		}
	}
	if !strings.Contains(joined, "PATH=") || !strings.Contains(joined, "AFL_PATH=") {
		t.Fatalf("expected compiler knobs kept: %v", out)
	}
}

func TestUniqueBindPathsDedupes(t *testing.T) {
	got := uniqueBindPaths("/tmp/a", "/tmp/a", "", "/tmp/b")
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}
