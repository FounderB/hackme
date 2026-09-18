package hunt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const rustStdinBin = "hunt_stdin"

var reFuzzTargetBody = regexp.MustCompile(`(?s)fuzz_target!\s*\(\s*\|\s*(\w+)\s*:\s*&\[u8\]\s*\|\s*\{(.*)\}\s*\)\s*;`)

type rustHarnessPlan struct {
	Mode        string // cargo_fuzz | stdin_fuzz_target | stdin_package
	FuzzTarget  string
	PackageName string
	CargoRoot   string
}

// BuildInventoryRustHarness compiles a Rust ASAN stdin harness from pinned inventory (Phase B).
func BuildInventoryRustHarness(ctx context.Context, repoRoot string, req HarnessBuildRequest) (*HarnessBuildResult, error) {
	if req.Pin == nil || strings.TrimSpace(req.Pin.Path) == "" {
		return nil, fmt.Errorf("hunt rust build: pin required")
	}
	sourceRel := strings.TrimSpace(req.SourceRel)
	if sourceRel == "" {
		return nil, fmt.Errorf("hunt rust build: source_rel required")
	}
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	srcPath, err := resolveSourceFile(repoRoot, req.Pin.Path, sourceRel)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	plan, err := planRustHarness(req.Pin.Path, sourceRel, content)
	if err != nil {
		return nil, err
	}
	hash := InventoryHarnessHash(req.Pin.CommitSHA, sourceRel, content)
	if err := ValidateHexHash(hash); err != nil {
		return nil, err
	}
	cachePath, err := SafeCacheFile(repoRoot, "hunt-harness", hash, "bin")
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(cachePath); err == nil && st.Mode().IsRegular() {
		harnessCache.Store(hash, cachePath)
		return &HarnessBuildResult{
			HarnessHash: hash,
			BinaryPath:  cachePath,
			SourceRel:   sourceRel,
			Language:    "rust",
			PinSHA:      req.Pin.CommitSHA,
			BuildOK:     true,
			Note:        "cached rust harness",
		}, nil
	}
	if err := requireRustNightlyASAN(); err != nil {
		return nil, err
	}
	var binPath string
	var note string
	switch plan.Mode {
	case "cargo_fuzz":
		binPath, note, err = buildCargoFuzzHarness(ctx, plan)
	case "stdin_fuzz_target":
		binPath, note, err = buildRustStdinFromFuzzTarget(ctx, req.Pin.Path, sourceRel, content, plan)
	default:
		// Fail closed: a package-linked stub that never calls target code is not a Hunt harness.
		return nil, fmt.Errorf("hunt rust build: no fuzz_target!/cargo-fuzz for %s — refuse package driver stub (would not exercise target code)", sourceRel)
	}
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return nil, err
	}
	safeBin, err := filepath.Abs(filepath.Clean(binPath))
	if err != nil {
		return nil, err
	}
	// Harness binary must live under pin or system temp (cargo/rustc output).
	if p, err := MustUnderRoot(req.Pin.Path, safeBin); err == nil {
		safeBin = p
	} else if p, err2 := MustUnderRoot(os.TempDir(), safeBin); err2 == nil {
		safeBin = p
	} else {
		return nil, fmt.Errorf("hunt rust build: binary path not under pin/temp: %w", err)
	}
	in, err := os.ReadFile(safeBin)
	if err != nil {
		return nil, err
	}
	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, in, 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	harnessCache.Store(hash, cachePath)
	return &HarnessBuildResult{
		HarnessHash: hash,
		BinaryPath:  cachePath,
		SourceRel:   sourceRel,
		Language:    "rust",
		PinSHA:      req.Pin.CommitSHA,
		BuildOK:     true,
		Note:        note,
	}, nil
}

func planRustHarness(pinPath, sourceRel string, content []byte) (*rustHarnessPlan, error) {
	src := string(content)
	base := strings.TrimSuffix(filepath.Base(sourceRel), filepath.Ext(sourceRel))
	cargoRoot := findCargoRoot(pinPath, sourceRel)
	plan := &rustHarnessPlan{
		FuzzTarget:  base,
		CargoRoot:   cargoRoot,
		PackageName: readCargoPackageName(cargoRoot),
	}
	slash := filepath.ToSlash(sourceRel)
	// Only treat as cargo-fuzz when the source itself lives under fuzz/fuzz_targets/.
	// A sibling fuzz/Cargo.toml must not force cargo_fuzz for unrelated .rs files.
	if strings.HasPrefix(slash, "fuzz/fuzz_targets/") {
		cargoToml, err := SafeJoinUnder(pinPath, "fuzz", "Cargo.toml")
		if err == nil {
			if st, err := os.Stat(cargoToml); err == nil && !st.IsDir() {
				plan.Mode = "cargo_fuzz"
				if target := cargoFuzzTargetName(sourceRel); target != "" {
					plan.FuzzTarget = target
				}
				return plan, nil
			}
		}
	}
	if strings.Contains(src, inventoryMarkerRust) || strings.Contains(src, "libfuzzer_sys::fuzz_target") {
		plan.Mode = "stdin_fuzz_target"
		return plan, nil
	}
	plan.Mode = "stdin_package"
	return plan, nil
}

func findCargoRoot(pinPath, sourceRel string) string {
	start, err := SafeJoinUnder(pinPath, filepath.Dir(sourceRel))
	if err != nil {
		return pinPath
	}
	dir := start
	for i := 0; i < 8; i++ {
		cargoToml, err := SafeJoinUnder(dir, "Cargo.toml")
		if err == nil {
			if _, err := os.Stat(cargoToml); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		safeParent, err := MustUnderRoot(pinPath, parent)
		if err != nil {
			break
		}
		dir = safeParent
	}
	return pinPath
}

func readCargoPackageName(cargoRoot string) string {
	path, err := SafeJoinUnder(cargoRoot, "Cargo.toml")
	if err != nil {
		return "hunt_crate"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "hunt_crate"
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), `"`)
			}
		}
	}
	return "hunt_crate"
}

func cargoFuzzTargetName(sourceRel string) string {
	base := filepath.Base(sourceRel)
	if strings.HasPrefix(filepath.ToSlash(sourceRel), "fuzz/fuzz_targets/") {
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func buildCargoFuzzHarness(ctx context.Context, plan *rustHarnessPlan) (binPath, note string, err error) {
	if plan.CargoRoot == "" {
		return "", "", fmt.Errorf("hunt rust: cargo root missing")
	}
	targetRe := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,128}$`)
	fuzzTarget := targetRe.FindString(plan.FuzzTarget)
	if fuzzTarget == "" || fuzzTarget != plan.FuzzTarget {
		return "", "", fmt.Errorf("hunt rust: invalid fuzz target name")
	}
	buildCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, "cargo", "+nightly", "fuzz", "build", fuzzTarget, "--sanitizer=address")
	cmd.Dir = plan.CargoRoot
	cmd.Env = append(os.Environ(), "RUSTFLAGS=-Zsanitizer=address", "CARGO_TERM_COLOR=never")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("hunt rust cargo fuzz build %s: %w (%s)", fuzzTarget, err, strings.TrimSpace(stderr.String()))
	}
	candidates := []string{}
	for _, parts := range [][]string{
		{"fuzz", "target", fuzzTarget, "release", fuzzTarget},
		{"fuzz", "target", "x86_64-unknown-linux-gnu", "release", fuzzTarget},
		{"target", "release", fuzzTarget},
	} {
		if c, err := SafeJoinUnder(plan.CargoRoot, parts...); err == nil {
			candidates = append(candidates, c)
		}
	}
	// cargo-fuzz sometimes nests target name as "name/release/name"
	if c, err := SafeJoinUnder(plan.CargoRoot, "fuzz", "target", fuzzTarget+"/release", fuzzTarget); err == nil {
		candidates = append(candidates, c)
	}
	for _, c := range candidates {
		if safe, err := MustUnderRoot(plan.CargoRoot, c); err == nil {
			if st, err := os.Stat(safe); err == nil && !st.IsDir() {
				return safe, fmt.Sprintf("cargo-fuzz ASAN harness (%s)", fuzzTarget), nil
			}
		}
	}
	return "", "", fmt.Errorf("hunt rust: cargo fuzz binary not found for %s", fuzzTarget)
}

func buildRustStdinFromFuzzTarget(ctx context.Context, pinPath, sourceRel string, content []byte, plan *rustHarnessPlan) (binPath, note string, err error) {
	param, body, ok := extractFuzzTargetBody(string(content))
	if !ok {
		return "", "", fmt.Errorf("hunt rust: could not parse fuzz_target! body in %s", sourceRel)
	}
	uses := extractRustUseImports(string(content))
	tmpDir, err := os.MkdirTemp("", "hunt-rust-fuzz-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)
	crateDir := filepath.Join(tmpDir, "crate")
	if err := os.MkdirAll(crateDir, 0o755); err != nil {
		return "", "", err
	}
	mainRS := fmt.Sprintf(`use std::io::Read;
%s
fn main() {
	let mut data = Vec::new();
	let _ = std::io::stdin().take(65536).read_to_end(&mut data);
	if data.is_empty() {
		return;
	}
	fuzz_body(&data);
}

fn fuzz_body(%s: &[u8]) {
%s
}
`, uses, param, body)
	manifest := fmt.Sprintf(`[package]
name = "hunt_inv_rust"
version = "0.0.0"
edition = "2021"
publish = false

[[bin]]
name = "%s"
path = "main.rs"
`, rustStdinBin)
	if plan.PackageName != "" && plan.CargoRoot != "" && plan.CargoRoot != pinPath {
		abs, _ := filepath.Abs(plan.CargoRoot)
		manifest += fmt.Sprintf("\n[dependencies]\n%s = { path = %q }\n", plan.PackageName, abs)
	}
	if err := os.WriteFile(filepath.Join(crateDir, "Cargo.toml"), []byte(manifest), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(crateDir, "main.rs"), []byte(mainRS), 0o644); err != nil {
		return "", "", err
	}
	return compileAndStageRustBin(ctx, crateDir, "stdin fuzz_target ASAN harness")
}

func compileAndStageRustBin(ctx context.Context, crateDir, note string) (binPath, noteOut string, err error) {
	bin, noteOut, err := compileRustStdinCrate(ctx, crateDir, note)
	if err != nil {
		return "", "", err
	}
	staged, err := stageRustBinary(bin)
	if err != nil {
		return "", "", err
	}
	return staged, noteOut, nil
}

func stageRustBinary(srcBin string) (string, error) {
	data, err := os.ReadFile(srcBin)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "hunt-rust-bin-")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func compileRustStdinCrate(ctx context.Context, crateDir, note string) (binPath, noteOut string, err error) {
	targetDir := filepath.Join(filepath.Dir(crateDir), "target")
	buildCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, "cargo", "+nightly", "build", "--release", "--manifest-path", filepath.Join(crateDir, "Cargo.toml"))
	cmd.Dir = crateDir
	cmd.Env = append(os.Environ(),
		"CARGO_TERM_COLOR=never",
		"CARGO_TARGET_DIR="+targetDir,
		"RUSTFLAGS=-Zsanitizer=address",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("hunt rust cargo build: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	bin := filepath.Join(targetDir, "release", rustStdinBin)
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return "", "", fmt.Errorf("hunt rust: missing binary %s", bin)
	}
	return bin, note, nil
}

// extractFuzzTargetBody returns the closure parameter name and body from fuzz_target!.
func extractFuzzTargetBody(src string) (param, body string, ok bool) {
	m := reFuzzTargetBody.FindStringSubmatch(src)
	if len(m) < 3 {
		return "", "", false
	}
	param = strings.TrimSpace(m[1])
	body = strings.TrimSpace(m[2])
	if param == "" || body == "" {
		return "", "", false
	}
	return param, body, true
}

// extractRustUseImports keeps non-libfuzzer use lines so harness bodies that depend on them still compile.
func extractRustUseImports(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "use ") {
			continue
		}
		if strings.Contains(trimmed, "libfuzzer_sys") {
			continue
		}
		b.WriteString(trimmed)
		b.WriteByte('\n')
	}
	return b.String()
}

func requireRustNightlyASAN() error {
	if _, err := exec.LookPath("cargo"); err != nil {
		return fmt.Errorf("hunt rust: cargo required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rustc", "+nightly", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hunt rust: rustc +nightly required (rustup toolchain install nightly): %w", err)
	}
	return nil
}
