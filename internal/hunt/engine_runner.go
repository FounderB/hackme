package hunt

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EngineShardResult is the outcome of one coverage-guided Hunt lease window.
type EngineShardResult struct {
	Engine        string
	EngineVersion string
	WallSec       int
	CorpusDir     string
	CorpusBefore  int
	CorpusAfter   int
	NewCorpus     int
	CrashFiles    []string
	SkippedReason string
	Err           string
	Ran           bool
}

// EngineShardOpts configures a coverage-guided Hunt shard (lease = wall seconds).
type EngineShardOpts struct {
	RepoRoot  string
	TargetID  string
	Engine    string
	WallSec   int
	// SeedFiles optional seeds copied into the engine corpus before the run.
	SeedFiles [][]byte
	// Published harness (inventory / custom) — used as LF stdin driver / AFL target.
	HarnessHash          string
	HarnessFetchURL      string
	HarnessContentSHA256 string
	ArtifactDB           *sql.DB // coordinator prove path; workers fetch via URL
}

// LookPathFn is overridable in tests (defaults to exec.LookPath).
var LookPathFn = exec.LookPath

// RunnerAvailable reports whether the local machine can run the engine.
func RunnerAvailable(engine string) (ok bool, reason string) {
	engine = ParseHuntEngine(engine)
	switch engine {
	case EngineByte:
		return true, ""
	case EngineLibFuzzer:
		if _, err := LookPathFn("clang"); err != nil {
			return false, "clang_missing"
		}
		return true, ""
	case EngineAFLPP:
		if _, err := LookAFLFuzz(); err != nil {
			return false, "afl_missing"
		}
		return true, ""
	default:
		return false, "unknown_engine"
	}
}

// RunEngineShard runs libFuzzer or AFL++ for WallSec and returns corpus growth + crash paths.
// Soft-skips (Ran=false) when the runner is missing — Dig/byte paths are untouched.
func RunEngineShard(ctx context.Context, opts EngineShardOpts) EngineShardResult {
	engine := ParseHuntEngine(opts.Engine)
	wall := opts.WallSec
	if wall <= 0 {
		wall = EngineLeaseSecFromConfig(nil, engine)
	}
	wall = clampEngineLeaseSec(wall)
	out := EngineShardResult{Engine: engine, WallSec: wall}
	if !IsCoverageEngine(engine) {
		out.SkippedReason = "byte_engine"
		return out
	}
	if ok, reason := RunnerAvailable(engine); !ok {
		out.SkippedReason = reason
		return out
	}
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	targetID := strings.TrimSpace(opts.TargetID)
	if targetID == "" {
		out.SkippedReason = "no_target"
		return out
	}

	switch engine {
	case EngineLibFuzzer:
		return runLibFuzzerEngineShard(ctx, repoRoot, targetID, wall, opts)
	case EngineAFLPP:
		return runAFLPPEngineShard(ctx, repoRoot, targetID, wall, opts)
	default:
		out.SkippedReason = "unknown_engine"
		return out
	}
}

func runLibFuzzerEngineShard(ctx context.Context, repoRoot, targetID string, wall int, opts EngineShardOpts) EngineShardResult {
	out := EngineShardResult{Engine: EngineLibFuzzer, EngineVersion: LibFuzzerVersionHint(), WallSec: wall}
	corpusDir := PersistentLibFuzzerCorpusDir(repoRoot, targetID)
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		out.Err = err.Error()
		out.SkippedReason = "corpus_dir"
		return out
	}
	out.CorpusDir = corpusDir
	before, _ := countCorpusFiles(corpusDir)
	out.CorpusBefore = before
	for i, b := range opts.SeedFiles {
		if len(b) == 0 || len(b) > 65536 {
			continue
		}
		_ = os.WriteFile(filepath.Join(corpusDir, fmt.Sprintf("claim-seed-%d.bin", i)), b, 0o600)
	}
	sess := LibFuzzerSessionOpts{
		RepoRoot: repoRoot,
		TargetID: targetID,
		WallSec:  wall,
		Persist:  true,
	}
	// Custom inventory: published ASAN stdin harness + generic LF subprocess driver.
	if hash := strings.TrimSpace(opts.HarnessHash); hash != "" {
		stdinPath, merr := MaterializeHarness(ctx, repoRoot, hash, opts.HarnessFetchURL, opts.HarnessContentSHA256, opts.ArtifactDB)
		if merr != nil {
			out.Err = merr.Error()
			out.SkippedReason = "harness"
			return out
		}
		sess.StdinBin = stdinPath
	}
	n, err := RunLibFuzzerSession(ctx, sess)
	_ = n
	after, _ := countCorpusFiles(corpusDir)
	out.CorpusAfter = after
	if after > before {
		out.NewCorpus = after - before
	}
	out.CrashFiles = collectCrashArtifacts(filepath.Dir(corpusDir), 16)
	if err != nil {
		out.Err = err.Error()
	}
	// Soft-skip only on preflight (no clang/build/target/harness) — never after a wall was spent.
	// Empty corpus after LF ran → CLEAN floor submit + re-seed boot starters (avoid lease churn).
	if after == 0 {
		_ = seedPersistentCorpus(repoRoot, targetID, corpusDir)
		if after2, _ := countCorpusFiles(corpusDir); after2 > after {
			out.CorpusAfter = after2
			after = after2
		}
	}
	if err != nil && after == 0 && isLibFuzzerPreflightErr(err) {
		out.SkippedReason = "lf_session"
		out.Ran = false
		return out
	}
	out.Ran = true
	return out
}

func isLibFuzzerPreflightErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Import/empty-corpus after a session is NOT preflight (wall was spent).
	if strings.Contains(msg, "no corpus files") || strings.Contains(msg, "corpus still empty") {
		return false
	}
	for _, needle := range []string{
		"clang required",
		"invalid target",
		"unknown target",
		"libfuzzer import",
		"missing subprocess harness",
		"clang subprocess libfuzzer",
		"build target",
		"manifest",
		"bwrap required",
		"harness artifact",
		"hunt artifact",
		"no coverage harness",
		"inventory harness",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func runAFLPPEngineShard(ctx context.Context, repoRoot, targetID string, wall int, opts EngineShardOpts) EngineShardResult {
	out := EngineShardResult{Engine: EngineAFLPP, WallSec: wall}
	afl, err := LookAFLFuzz()
	if err != nil {
		out.SkippedReason = "afl_missing"
		out.Err = err.Error()
		return out
	}
	out.EngineVersion = AFLFuzzVersion(afl)

	// Prefer stdin ASAN harness from catalog AFL build, else published inventory harness.
	harness, err := resolveAFLStdinHarness(ctx, repoRoot, targetID, opts)
	if err != nil {
		out.SkippedReason = "harness"
		out.Err = err.Error()
		return out
	}
	seeds := opts.SeedFiles

	work := filepath.Join(repoRoot, ".cache", "hunt-afl", catalogTargetSegment(targetID))
	inDir := filepath.Join(work, "in")
	outDir := filepath.Join(work, "out")
	_ = os.MkdirAll(inDir, 0o755)
	_ = os.RemoveAll(outDir)
	_ = os.MkdirAll(outDir, 0o755)

	// Seed from persistent LF corpus + claim seeds.
	persist := PersistentLibFuzzerCorpusDir(repoRoot, targetID)
	copyCorpusFiles(persist, inDir, 32)
	for i, b := range seeds {
		if len(b) == 0 || len(b) > 65536 {
			continue
		}
		_ = os.WriteFile(filepath.Join(inDir, fmt.Sprintf("claim-%d.bin", i)), b, 0o600)
	}
	if n, _ := countCorpusFiles(inDir); n == 0 {
		_ = os.WriteFile(filepath.Join(inDir, "seed1.json"), []byte("{}"), 0o600)
		_ = os.WriteFile(filepath.Join(inDir, "seed2.bin"), []byte{0x00}, 0o600)
	}
	beforePersist, _ := countCorpusFiles(persist)
	out.CorpusBefore = beforePersist
	out.CorpusDir = persist

	env := append(os.Environ(),
		"AFL_SKIP_CPUFREQ=1",
		"AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1",
		"ASAN_OPTIONS=detect_leaks=0:abort_on_error=1:allocator_may_return_null=1",
		"UBSAN_OPTIONS=halt_on_error=1",
	)
	args := []string{"-i", inDir, "-o", outDir, "-V", fmt.Sprint(wall), "--", harness}
	runErr := RunIsolated(ctx, IsolateOpts{
		Bin:        afl,
		Args:       args,
		Env:        env,
		Dir:        work,
		Wall:       time.Duration(wall+30) * time.Second,
		RSSLimitMB: 2048,
	})
	if runErr != nil {
		out.Err = runErr.Error()
	}

	// Import queue into persistent corpus only — crash artifacts stay separate
	// (uploaded as crash bytes, never mixed into seed corpus).
	imported := 0
	for _, d := range []string{
		filepath.Join(outDir, "default", "queue"),
		filepath.Join(outDir, "queue"),
	} {
		imported += copyCorpusFiles(d, persist, 64)
	}
	for _, d := range []string{
		filepath.Join(outDir, "default", "crashes"),
		filepath.Join(outDir, "crashes"),
	} {
		out.CrashFiles = append(out.CrashFiles, listFiles(d, 16)...)
	}
	afterPersist, _ := countCorpusFiles(persist)
	out.CorpusAfter = afterPersist
	if afterPersist > beforePersist {
		out.NewCorpus = afterPersist - beforePersist
	} else {
		out.NewCorpus = imported
	}
	out.Ran = true
	return out
}

func resolveAFLStdinHarness(ctx context.Context, repoRoot, targetID string, opts EngineShardOpts) (string, error) {
	covOpts := CoverageHarnessOpts{
		HarnessHash:          opts.HarnessHash,
		HarnessFetchURL:      opts.HarnessFetchURL,
		HarnessContentSHA256: opts.HarnessContentSHA256,
		ArtifactDB:           opts.ArtifactDB,
	}
	// Prefer afl-cc + ASAN instrumented harness (real AFL edges). Fall back to ASAN-only / published.
	if path, instrumented, err := ResolveCoverageHarness(ctx, repoRoot, targetID, covOpts); err == nil && path != "" {
		if instrumented {
			return path, nil
		}
		// Keep ASAN / published path as last resort so Deep shards still run (weak feedback).
		bestASAN := path
		if aflPath, aerr := BuildAFLInstrumentedHarness(ctx, repoRoot, targetID); aerr == nil && aflPath != "" {
			return aflPath, nil
		}
		return bestASAN, nil
	}
	_, stdinBin, err := BuildLibFuzzerImport(ctx, repoRoot, targetID)
	if err != nil {
		return "", err
	}
	if stdinBin != "" {
		return stdinBin, nil
	}
	return "", fmt.Errorf("hunt: no AFL stdin harness for %s", targetID)
}

func copyCorpusFiles(src, dst string, max int) int {
	if max < 1 {
		max = 32
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return 0
	}
	_ = os.MkdirAll(dst, 0o755)
	n := 0
	for _, e := range ents {
		if n >= max {
			break
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil || len(b) == 0 || len(b) > 1<<20 {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(filepath.Base(src), "crash") || strings.Contains(src, "crashes") {
			name = "afl-crash-" + name
		} else if !strings.HasPrefix(name, "afl-") {
			name = "afl-" + name
		}
		dest := filepath.Join(dst, name)
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if err := os.WriteFile(dest, b, 0o600); err == nil {
			n++
		}
	}
	return n
}

func listFiles(dir string, max int) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if len(out) >= max {
			break
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "README.txt" {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func collectCrashArtifacts(dir string, max int) []string {
	var out []string
	for _, sub := range []string{"crashes", "crash", "artifacts"} {
		out = append(out, listFiles(filepath.Join(dir, sub), max)...)
		if len(out) >= max {
			return out[:max]
		}
	}
	return out
}
