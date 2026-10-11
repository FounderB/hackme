package hunt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hackme/internal/fuzzupstream"
)

// LookAFLClangFast resolves afl-clang-fast / afl-cc for instrumented Hunt builds.
func LookAFLClangFast() (string, error) {
	for _, name := range []string{"afl-clang-fast", "afl-cc", "afl-clang"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	prefixes := []string{
		strings.TrimSpace(os.Getenv("HACKME_PREFIX")),
		"/opt/hackme",
		filepath.Join(os.Getenv("HOME"), ".local/share/hackme/toolchains"),
	}
	for _, pref := range prefixes {
		if pref == "" {
			continue
		}
		for _, rel := range []string{
			"aflpp/bin/afl-clang-fast",
			"aflpp/bin/afl-cc",
			"bin/afl-clang-fast",
			"bin/afl-cc",
		} {
			p := filepath.Join(pref, rel)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("hunt: afl-clang-fast/afl-cc not found (install_hunt_engines.sh)")
}

// BuildAFLInstrumentedHarness compiles the catalog stdin driver with afl-cc + ASAN.
func BuildAFLInstrumentedHarness(ctx context.Context, repoRoot, targetID string) (binPath string, err error) {
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	id, ok := fuzzupstream.SanitizeCatalogID(targetID)
	if !ok {
		return "", fmt.Errorf("hunt: invalid target id %q", targetID)
	}
	manifest, err := fuzzupstream.LoadManifest(repoRoot)
	if err != nil {
		return "", err
	}
	t, err := manifest.TargetByID(id)
	if err != nil {
		return "", err
	}
	binPath, _, err = fuzzupstream.BuildTargetAFL(ctx, repoRoot, t)
	return binPath, err
}

// ResolveCoverageHarness prefers an afl-cc instrumented catalog binary for showmap/AFL++.
// Falls back to cached ASAN stdin harness when afl-cc is unavailable (showmap will yield
// empty edges → no structural edge pay).
func ResolveCoverageHarness(ctx context.Context, repoRoot, targetID string) (path string, instrumented bool, err error) {
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	if bin, berr := BuildAFLInstrumentedHarness(ctx, repoRoot, targetID); berr == nil && bin != "" {
		return bin, true, nil
	} else if berr != nil {
		err = berr
	}
	// ASAN stdin cache (weak / no compile-time edges).
	pattern := filepath.Join(repoRoot, ".cache", "oss-cve-bin", catalogTargetSegment(targetID)+"-*.bin")
	matches, _ := filepath.Glob(pattern)
	var best string
	var bestMod time.Time
	for _, m := range matches {
		if strings.Contains(m, "msan") {
			continue
		}
		st, e := os.Stat(m)
		if e != nil || st.IsDir() {
			continue
		}
		if best == "" || st.ModTime().After(bestMod) {
			best = m
			bestMod = st.ModTime()
		}
	}
	if best != "" {
		return best, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("hunt: no coverage harness for %s", targetID)
}
