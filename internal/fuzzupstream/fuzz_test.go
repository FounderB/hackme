package fuzzupstream

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildAllTargets(t *testing.T) {
	root := repoRoot(t)
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, tgt := range m.Targets {
		tgt := tgt
		t.Run(tgt.ID, func(t *testing.T) {
			t.Parallel()
			driverSrc := DriverSourcePath(root, tgt)
			if _, err := os.Stat(driverSrc); err != nil {
				t.Skipf("driver not in repo: %s", tgt.Driver)
			}
			if TargetLanguage(tgt) == "rust" {
				if !RustNightlyASANAvailable() {
					t.Skip("rustc +nightly unavailable")
				}
			} else if _, err := exec.LookPath("clang"); err != nil {
				t.Skip("clang not installed")
			}
			bin, clone, err := BuildTarget(ctx, root, tgt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(bin); err != nil {
				t.Fatalf("binary: %v", err)
			}
			if _, err := os.Stat(clone); err != nil {
				t.Fatalf("clone: %v", err)
			}
		})
	}
}

func TestHuntSerdeJSONSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	if !RustNightlyASANAvailable() {
		t.Skip("rustc +nightly unavailable")
	}
	root := repoRoot(t)
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	tgt, err := m.TargetByID("serde_json")
	if err != nil {
		t.Skip("serde_json not in catalog")
	}
	if TargetLanguage(tgt) != "rust" {
		t.Fatalf("expected language=rust, got %q", tgt.Language)
	}
	ctx := context.Background()
	bin, _, err := BuildTarget(ctx, root, tgt)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Hunt(ctx, root, tgt, bin, seedsFromManifest(m), 500, 4096, 45)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Language != "rust" {
		t.Fatalf("report language=%q", rep.Language)
	}
	if rep.Iterations < 50 {
		t.Fatalf("expected iterations, got %d", rep.Iterations)
	}
	t.Logf("serde_json smoke: iterations=%d crashes=%d verdict=%s lang=%s", rep.Iterations, len(rep.Crashes), rep.Verdict, rep.Language)
}

func TestHuntMemchrSmoke(t *testing.T) {
	smokeRustTarget(t, "memchr", 300, 30)
}

func TestHuntQuickXMLSmoke(t *testing.T) {
	smokeRustTarget(t, "quick_xml", 300, 30)
}

func smokeRustTarget(t *testing.T, id string, budget, wallSec int) {
	t.Helper()
	if testing.Short() {
		t.Skip("short")
	}
	if !RustNightlyASANAvailable() {
		t.Skip("rustc +nightly unavailable")
	}
	root := repoRoot(t)
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	tgt, err := m.TargetByID(id)
	if err != nil {
		t.Skipf("%s not in catalog", id)
	}
	if TargetLanguage(tgt) != "rust" {
		t.Fatalf("%s language=%q want rust", id, tgt.Language)
	}
	ctx := context.Background()
	bin, _, err := BuildTarget(ctx, root, tgt)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Hunt(ctx, root, tgt, bin, seedsFromManifest(m), budget, 4096, wallSec)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Language != "rust" {
		t.Fatalf("report language=%q", rep.Language)
	}
	if rep.Iterations < 20 {
		t.Fatalf("expected iterations, got %d", rep.Iterations)
	}
	t.Logf("%s smoke: iterations=%d crashes=%d verdict=%s", id, rep.Iterations, len(rep.Crashes), rep.Verdict)
}

func TestTargetLanguage(t *testing.T) {
	if TargetLanguage(Target{}) != "c" {
		t.Fatal("default c")
	}
	if TargetLanguage(Target{Language: "Rust"}) != "rust" {
		t.Fatal("rust")
	}
}

func TestHuntJsmnSmoke(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	if testing.Short() {
		t.Skip("short")
	}
	root := repoRoot(t)
	m, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	tgt, err := m.TargetByID("jsmn")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bin, _, err := BuildTarget(ctx, root, tgt)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Hunt(ctx, root, tgt, bin, seedsFromManifest(m), 2000, 4096, 30)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Iterations < 100 {
		t.Fatalf("expected iterations, got %d", rep.Iterations)
	}
	t.Logf("jsmn smoke: iterations=%d crashes=%d verdict=%s", rep.Iterations, len(rep.Crashes), rep.Verdict)
}

func writeBinScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunInputExitZeroEchoIsNotCrash(t *testing.T) {
	bin := writeBinScript(t, "#!/bin/sh\ncat\nexit 0\n")
	crash, info, _, err := RunInputDetailed(context.Background(), bin, []byte("unknown field 'heap-buffer-overflow' ignored\n"), DefaultRunInputOpts())
	if err != nil {
		t.Fatal(err)
	}
	if crash || info.Security {
		t.Fatalf("exit 0 echo must not be an ASAN crash: crash=%v info=%+v", crash, info)
	}
}

func TestRunInputNonZeroWithoutBannerIsNotBounty(t *testing.T) {
	bin := writeBinScript(t, "#!/bin/sh\necho heap-buffer-overflow\nexit 1\n")
	crash, info, _, err := RunInputDetailed(context.Background(), bin, nil, DefaultRunInputOpts())
	if err != nil {
		t.Fatal(err)
	}
	if crash || info.Security || info.Class == "asan" {
		t.Fatalf("exit 1 plus a bare substring must stay clean: crash=%v info=%+v", crash, info)
	}
}

func TestRunInputSignalWithoutBannerNeedsTriage(t *testing.T) {
	bin := writeBinScript(t, "#!/bin/sh\nkill -ABRT $$\n")
	crash, info, _, err := RunInputDetailed(context.Background(), bin, nil, DefaultRunInputOpts())
	if err != nil {
		t.Fatal(err)
	}
	if !crash || info.Security || info.Subtype != "needs_triage" {
		t.Fatalf("signal without banner: crash=%v info=%+v", crash, info)
	}
}

func TestRunInputASANBannerIsSecurityCrash(t *testing.T) {
	bin := writeBinScript(t, "#!/bin/sh\necho '==1==ERROR: AddressSanitizer: heap-buffer-overflow'\nexit 1\n")
	crash, info, _, err := RunInputDetailed(context.Background(), bin, nil, DefaultRunInputOpts())
	if err != nil {
		t.Fatal(err)
	}
	if !crash || !info.Security || info.Class != "asan" {
		t.Fatalf("canonical ASAN banner: crash=%v info=%+v", crash, info)
	}
}

func TestRunInputDetailedMissingBinaryDoesNotFailOpen(t *testing.T) {
	crash, _, _, err := RunInputDetailed(context.Background(), filepath.Join(t.TempDir(), "no-such-bin"), []byte("{}"), DefaultRunInputOpts())
	if crash {
		t.Fatal("missing binary must not report crash")
	}
	if err == nil {
		t.Fatal("missing binary must return error (not CLEAN fail-open)")
	}
}

func TestRunInputDetailedLibFuzzerUsesFileOnce(t *testing.T) {
	// Fake cargo-fuzz/libFuzzer driver: marker in the script body triggers detection;
	// hang forever on empty argv (stdin mode), exit 0 when given a file + -runs=1.
	bin := writeBinScript(t, `#!/bin/sh
# SUMMARY: libFuzzer: timeout
if [ -n "$1" ] && [ "$2" = "-runs=1" ]; then
  cat "$1" >/dev/null
  exit 0
fi
sleep 30
exit 1
`)
	if !binaryLooksLikeLibFuzzer(bin) {
		t.Fatal("expected libFuzzer marker detection")
	}
	start := time.Now()
	crash, _, _, err := RunInputDetailed(context.Background(), bin, []byte("AAAA"), DefaultRunInputOpts())
	if err != nil {
		t.Fatal(err)
	}
	if crash {
		t.Fatal("fake libFuzzer one-shot must be CLEAN")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("libFuzzer one-shot took too long: %v", time.Since(start))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found")
	return ""
}
