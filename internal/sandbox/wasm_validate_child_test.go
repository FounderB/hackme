package sandbox

import (
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestValidateCheckWasmChildAcceptsMinimalGate is the regression that catches
// "default AS too low for Go runtime" — real child re-exec must return nil.
func TestValidateCheckWasmChildAcceptsMinimalGate(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB", "384")
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, raw); err != nil {
		t.Fatalf("minimal gate must pass real child validate: %v", err)
	}
}

func TestValidateCheckWasmChildRejectsNameBomb(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB", "384")
	// Section 0 custom with huge declared name vector — hostile pre-parser or child.
	bomb := append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
		// custom section id=0, size large claim via LEB — keep payload small
		0x00, 0x0b, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x01, 0x80, 0x80, 0x80, 0x40,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, bomb); err == nil {
		t.Fatal("name/custom bomb must be rejected")
	}
}

func TestStripWasmCustomOnlyErrors(t *testing.T) {
	// magic + single empty-ish custom section only
	mod := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x04, 0x01, 0x61, 0x00, 0x00, // custom name "a", empty payload
	}
	if _, err := stripWasmCustomSections(mod); err == nil {
		t.Fatal("custom-only module must error, not return original")
	}
}

func TestWasmValidateChildEnvOmitsSecrets(t *testing.T) {
	t.Setenv("HACKME_ADMIN_TOKEN", "secret-should-not-leak")
	t.Setenv("ADMIN_TOKEN", "also-secret")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "0") // must not pass through as 0
	env := wasmValidateChildEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "secret-should-not-leak") || strings.Contains(joined, "also-secret") {
		t.Fatalf("child env leaked secrets: %q", joined)
	}
	foundChild := false
	for _, e := range env {
		if strings.HasPrefix(e, "HACKME_WASM_VALIDATE_CHILD=") {
			foundChild = true
			if e != "HACKME_WASM_VALIDATE_CHILD=1" {
				t.Fatalf("child must force VALIDATE_CHILD=1, got %q", e)
			}
		}
		if strings.HasPrefix(e, "HACKME_ADMIN_TOKEN=") || strings.HasPrefix(e, "ADMIN_TOKEN=") {
			t.Fatalf("unexpected secret key in child env: %q", e)
		}
	}
	if !foundChild {
		t.Fatal("missing HACKME_WASM_VALIDATE_CHILD=1")
	}
	_ = os.Getenv("PATH") // keep import used on windows-less paths
}

func TestCompileInstantiateValidateCheckMinimalGate(t *testing.T) {
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	stripped, err := stripWasmCustomSections(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := compileInstantiateValidateCheck(ctx, stripped); err != nil {
		t.Fatalf("minimal gate must pass interpreter instantiate+exports: %v", err)
	}
}

func TestWasmValidateTransientFailureInfraMarker(t *testing.T) {
	if !wasmValidateTransientFailure(`sandbox: wasm-validate child infra: no sandbox marker (Error in initialization)`) {
		t.Fatal("missing-marker infra must be transient (no quarantine)")
	}
	if !wasmValidateTransientFailure(`cuda: Error in initialization OutOfMemory`) {
		t.Fatal("CUDA init OOM must be transient")
	}
	if wasmValidateTransientFailure(`sandbox: wasm-validate child failed: hostile leb`) {
		t.Fatal("hostile reject must NOT be transient")
	}
}

func TestValidateWasmInChildNoMarkerIsInfra(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	// /bin/true exits 0 with empty stdout — no sandbox marker.
	t.Setenv("HACKME_WASM_VALIDATE_BIN", "/bin/true")
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = validateWasmInChild(ctx, raw)
	if err == nil {
		t.Fatal("expected infra error when child prints no marker")
	}
	if !wasmValidateTransientFailure(err.Error()) {
		t.Fatalf("no-marker must be transient, got: %v", err)
	}
	if !strings.Contains(err.Error(), "no sandbox marker") {
		t.Fatalf("want no sandbox marker, got: %v", err)
	}
}

func TestValidateCheckWasmChildViaHackmeWasmCheck(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB", "384")
	dir := t.TempDir()
	bin := filepath.Join(dir, "hackme-wasm-check")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/hackme-wasm-check")
	cmd.Dir = filepath.Join("..", "..") // module root from internal/sandbox
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback: resolve via go list
		root, listErr := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
		if listErr != nil {
			t.Fatalf("build hackme-wasm-check: %v\n%s", err, out)
		}
		cmd = exec.Command("go", "build", "-o", bin, "./cmd/hackme-wasm-check")
		cmd.Dir = strings.TrimSpace(string(root))
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("build hackme-wasm-check: %v\n%s", err2, out2)
		}
	}
	t.Setenv("HACKME_WASM_VALIDATE_BIN", bin)
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, raw); err != nil {
		t.Fatalf("minimal gate via hackme-wasm-check: %v", err)
	}
}
