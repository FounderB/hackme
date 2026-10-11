package hunt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LookAFLShowmap resolves afl-showmap next to afl-fuzz / PATH.
func LookAFLShowmap() (string, error) {
	if p, err := exec.LookPath("afl-showmap"); err == nil {
		return p, nil
	}
	if afl, err := LookAFLFuzz(); err == nil {
		cand := filepath.Join(filepath.Dir(afl), "afl-showmap")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf("hunt: afl-showmap not found")
}

// ShowmapCoverageBitmap runs afl-showmap -b on an instrumented harness with stdin input.
// Returns a non-empty AFL-style edge bitmap when instrumentation fired; nil/empty otherwise.
// This is the coordinator-side source of truth for Hunt pay-for-edges (not structural fingerprints).
func ShowmapCoverageBitmap(ctx context.Context, harness string, input []byte) ([]byte, error) {
	harness = strings.TrimSpace(harness)
	if harness == "" {
		return nil, fmt.Errorf("hunt showmap: empty harness")
	}
	if len(input) == 0 {
		return nil, nil
	}
	showmap, err := LookAFLShowmap()
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "hunt-showmap-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	inFile := filepath.Join(tmpDir, "in.bin")
	outFile := filepath.Join(tmpDir, "map.bin")
	if err := os.WriteFile(inFile, input, 0o600); err != nil {
		return nil, err
	}

	wall := 8 * time.Second
	runCtx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()

	// AFL++: -b writes a binary map; stdin harness reads from stdin.
	args := []string{"-q", "-b", "-o", outFile, "--", harness}
	env := append(os.Environ(),
		"AFL_QUIET=1",
		"AFL_MAP_SIZE=65536",
		"ASAN_OPTIONS=detect_leaks=0:halt_on_error=0:allocator_may_return_null=1:abort_on_error=0",
		"UBSAN_OPTIONS=halt_on_error=0",
	)
	stdin, err := os.Open(inFile)
	if err != nil {
		return nil, err
	}
	defer stdin.Close()

	harnessDir := filepath.Dir(harness)
	err = RunIsolated(runCtx, IsolateOpts{
		Bin:         showmap,
		Args:        args,
		Env:         env,
		Dir:         tmpDir,
		Wall:        wall,
		// ASAN instrumented targets need large VA; RLIMIT_AS kills showmap before map write.
		RSSLimitMB:  0,
		Stdin:       stdin,
		AllowBare:   true, // showmap prove must not soft-fail on missing bwrap
		ExtraRoBind: []string{harnessDir},
		ExtraBind:   []string{tmpDir},
	})
	// showmap may exit non-zero on crashy inputs; still read map if present.
	b, rerr := os.ReadFile(outFile)
	if rerr != nil || len(b) == 0 {
		if err != nil {
			return nil, fmt.Errorf("hunt showmap: %w", err)
		}
		if rerr != nil {
			return nil, rerr
		}
		return nil, nil
	}
	// Normalize: keep raw map (typically 65536); truncate huge.
	if len(b) > 1<<20 {
		b = b[:1<<20]
	}
	return b, nil
}
