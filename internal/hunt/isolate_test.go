package hunt

import (
	"strings"
	"testing"
)

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
