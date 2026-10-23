package hunt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAllowBareTriesBwrapFirst(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "bwrap-used")
	fake := filepath.Join(dir, "bwrap")
	script := "#!/bin/sh\necho used >\"" + marker + "\"\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HACKME_HUNT_ISOLATE_REQUIRE", "1")
	t.Setenv("HACKME_HUNT_ISOLATE_STRICT", "1")

	bin := filepath.Join(dir, "harness")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho should-not-run\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := RunIsolated(context.Background(), IsolateOpts{
		Bin:       bin,
		Args:      nil,
		Wall:      2 * time.Second,
		AllowBare: true,
	})
	if err != nil {
		t.Fatalf("AllowBare must still use bwrap when present: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("AllowBare must not skip bwrap: marker missing (regression: early bare bypass)")
	}
}

func TestScrubIsolateEnvDropsSecrets(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"COORD_TOKEN=supersecret",
		"HACKME_ADMIN_TOKEN=admin",
		"HACKME_POOL_COORDINATOR_TOKEN=pooltok",
		"HACKME_POOL_COORDINATOR_URL=http://127.0.0.1:18083",
		"HACKME_MINER_ED25519_SEED_HEX=deadbeef",
		"AWS_SECRET_ACCESS_KEY=aws",
		"http_proxy=http://evil",
		"AFL_SKIP_CPUFREQ=1",
	}
	out := scrubIsolateEnv(in)
	joined := strings.Join(out, "\n")
	for _, bad := range []string{
		"COORD_TOKEN=", "HACKME_ADMIN_TOKEN=", "HACKME_POOL_COORDINATOR_TOKEN=",
		"HACKME_MINER_ED25519_SEED_HEX=", "AWS_SECRET_ACCESS_KEY=", "http_proxy=",
	} {
		if strings.Contains(joined, bad) {
			t.Fatalf("secret leaked in isolate env: %s\n%s", bad, joined)
		}
	}
	keep := map[string]bool{}
	for _, e := range out {
		k, _, _ := strings.Cut(e, "=")
		keep[k] = true
	}
	if !keep["PATH"] || !keep["AFL_SKIP_CPUFREQ"] || !keep["HACKME_POOL_COORDINATOR_URL"] {
		t.Fatalf("expected non-secret knobs kept: %#v", out)
	}
}
