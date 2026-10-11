package hunt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const inventoryLibFuzzerBuildTag = "inv-lf-inprocess-v1"

// InventorySourceHasLibFuzzerEntry reports whether source_rel exports LLVMFuzzerTestOneInput.
func InventorySourceHasLibFuzzerEntry(pinPath, sourceRel string) bool {
	srcPath, err := resolveSourceFile("", pinPath, sourceRel)
	if err != nil {
		return false
	}
	b, err := SafeReadFileUnder(pinPath, srcPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), inventoryMarker)
}

func inventoryLibFuzzerHash(pinSHA, sourceRel string, content []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.TrimSpace(pinSHA)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.TrimSpace(sourceRel)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(inventoryLibFuzzerBuildTag))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(content)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// BuildInventoryInProcessLibFuzzer compiles pin source (+ companions) as an in-process
// libFuzzer binary. Use when source_rel already exports LLVMFuzzerTestOneInput — avoids
// the stdin wrapper + subprocess LF driver (~0 exec/s).
func BuildInventoryInProcessLibFuzzer(ctx context.Context, repoRoot string, pin *RepoPinResult, sourceRel string) (string, error) {
	if pin == nil || strings.TrimSpace(pin.Path) == "" {
		return "", fmt.Errorf("hunt: inventory libfuzzer: pin required")
	}
	sourceRel = strings.TrimSpace(sourceRel)
	if sourceRel == "" {
		return "", fmt.Errorf("hunt: inventory libfuzzer: source_rel required")
	}
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	if !InventorySourceHasLibFuzzerEntry(pin.Path, sourceRel) {
		return "", fmt.Errorf("hunt: inventory libfuzzer: no %s in %s", inventoryMarker, sourceRel)
	}
	srcPath, err := resolveSourceFile(repoRoot, pin.Path, sourceRel)
	if err != nil {
		return "", err
	}
	content, err := SafeReadFileUnder(pin.Path, srcPath)
	if err != nil {
		return "", err
	}
	hash := inventoryLibFuzzerHash(pin.CommitSHA, sourceRel, content)
	cachePath, err := SafeCacheFile(repoRoot, "hunt-lf-inventory", hash, "bin")
	if err != nil {
		return "", err
	}
	if st, err := SafeStatUnder(repoRoot, cachePath); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
		return cachePath, nil
	}
	if _, err := exec.LookPath("clang"); err != nil {
		return "", fmt.Errorf("hunt: inventory libfuzzer: clang required")
	}
	plan, err := planInventoryCompile(pin.Path, sourceRel)
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "hunt-inv-lf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	buildCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	outBin := filepath.Join(tmpDir, "fuzzer.bin")
	if err := compileLinkInventoryLibFuzzer(buildCtx, plan, outBin); err != nil {
		return "", err
	}
	in, err := os.ReadFile(outBin)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}
	tmp, err := SafeCacheFile(repoRoot, "hunt-lf-inventory", hash, "bin.tmp")
	if err != nil {
		return "", err
	}
	if err := SafeWriteFileUnder(repoRoot, tmp, in, 0o755); err != nil {
		return "", err
	}
	if err := SafeRenameUnder(repoRoot, tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return cachePath, nil
}

func compileLinkInventoryLibFuzzer(ctx context.Context, plan *inventoryCompilePlan, outBin string) error {
	units := append([]string{plan.MainAbs}, plan.CompanionAbs...)
	objs := make([]string, 0, len(units))
	sanitize := "-fsanitize=fuzzer,address,undefined"
	for i, src := range units {
		obj := filepath.Join(filepath.Dir(outBin), fmt.Sprintf("lf-unit%d.o", i))
		compiler, std, langX := compileUnitFlags(plan.Compiler, src)
		args := []string{
			sanitize,
			"-fno-omit-frame-pointer",
			"-g", "-O1",
			"-std=" + std,
			"-c",
			"-o", obj,
		}
		for _, inc := range plan.IncludeDirs {
			args = append(args, "-I", inc)
		}
		if langX != "" {
			args = append(args, "-x", langX)
		}
		args = append(args, src)
		cmd := exec.CommandContext(ctx, compiler, args...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s %s: %w (%s)", compiler, filepath.Base(src), err, strings.TrimSpace(stderr.String()))
		}
		objs = append(objs, obj)
	}
	args := []string{sanitize, "-g", "-O1", "-o", outBin}
	args = append(args, objs...)
	cmd := exec.CommandContext(ctx, plan.Compiler, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s link: %w (%s)", plan.Compiler, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
