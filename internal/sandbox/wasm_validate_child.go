package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
)

const (
	// Parent hard ceiling for RLIMIT_AS (second line of defense after the
	// child's self-limit). Absolute caps below ~1 GiB abort Go before wazero.
	defaultWasmValidateASMB = 2048
	minWasmValidateASMB     = 1024
	maxWasmValidateASMB     = 2048
	// Extra RSS/VAS budget above the child's already-reserved VmSize for a
	// legitimate CompileModule. Multi-GiB LEB bombs still OOM the child.
	defaultWasmValidateChildBudgetMB = 384
	minWasmValidateChildBudgetMB     = 128
	maxWasmValidateChildBudgetMB     = 1024
	defaultWasmValidateWallMS        = 3000
	wasmValidateArg                  = "-hackme-wasm-validate"
	wasmValidateOK                   = "OK\n"
)

// Cap concurrent RLIMIT children — unbounded parallel re-execs exhaust AS /
// pthread slots and false-quarantine good gates under pool load.
var wasmValidateChildSem = make(chan struct{}, 2)

// RunValidateChildMain is the validate-child entry (via package init, or an
// explicit call from main). Reads WASM from stdin, applies a self RLIMIT_AS
// of VmSize+budget, runs hostile filter + CompileModule, prints OK or error.
func RunValidateChildMain() {
	// First action: pin AS relative to this process's already-warm Go runtime.
	// Parent prlimit-after-Start races mallocinit; self-limit does not.
	_ = applySelfWasmValidateASLimit(wasmValidateChildBudgetMB())

	maxBytes := MaxCheckWasmBytes()
	if maxBytes < 1024 {
		maxBytes = 1024
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, int64(maxBytes)+1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: read wasm: %v\n", err)
		os.Exit(2)
	}
	if len(raw) == 0 || len(raw) > maxBytes {
		fmt.Fprintf(os.Stderr, "sandbox: wasm size\n")
		os.Exit(2)
	}
	if err := rejectWasmHostileSections(raw); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	stripped, err := stripWasmCustomSections(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)
	compiled, err := rt.CompileModule(ctx, stripped)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: compile wasm: %v\n", err)
		os.Exit(1)
	}
	_ = compiled.Close(ctx)
	_, _ = os.Stdout.WriteString(wasmValidateOK)
	os.Exit(0)
}

// IsValidateChildArg reports whether argv requests the wasm-validate child mode.
func IsValidateChildArg(args []string) bool {
	for _, a := range args {
		if a == wasmValidateArg {
			return true
		}
	}
	return false
}

func wasmValidateASMB() int {
	v := strings.TrimSpace(os.Getenv("HACKME_WASM_VALIDATE_AS_MB"))
	if v == "" {
		return defaultWasmValidateASMB
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultWasmValidateASMB
	}
	if n < minWasmValidateASMB {
		return minWasmValidateASMB
	}
	if n > maxWasmValidateASMB {
		return maxWasmValidateASMB
	}
	return n
}

func wasmValidateChildBudgetMB() int {
	v := strings.TrimSpace(os.Getenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB"))
	if v == "" {
		return defaultWasmValidateChildBudgetMB
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultWasmValidateChildBudgetMB
	}
	if n < minWasmValidateChildBudgetMB {
		return minWasmValidateChildBudgetMB
	}
	if n > maxWasmValidateChildBudgetMB {
		return maxWasmValidateChildBudgetMB
	}
	return n
}

func wasmValidateWall() time.Duration {
	v := strings.TrimSpace(os.Getenv("HACKME_WASM_VALIDATE_WALL_MS"))
	if v == "" {
		return time.Duration(defaultWasmValidateWallMS) * time.Millisecond
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 200 {
		return time.Duration(defaultWasmValidateWallMS) * time.Millisecond
	}
	if n > 30000 {
		n = 30000
	}
	return time.Duration(n) * time.Millisecond
}

// wasmValidateChildDisabled skips the RLIMIT child (desk/tests that already
// exercise CompileModule in-process). Production keeps the child ON.
// Explicit HACKME_WASM_VALIDATE_CHILD=1 forces the child even under go test
// (positive soak / regression tests).
func wasmValidateChildDisabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_WASM_VALIDATE_CHILD")))
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return true
	}
	if v == "1" || v == "true" || v == "on" || v == "yes" {
		return false
	}
	for _, a := range os.Args {
		if strings.HasPrefix(a, "-test.") {
			return true
		}
	}
	exe, _ := os.Executable()
	if strings.HasSuffix(exe, ".test") || strings.Contains(exe, "__debug_bin") {
		return true
	}
	return false
}

// wasmValidateTransientFailure is a Go-runtime / capacity failure under RLIMIT_AS,
// not proof the module is hostile. Callers must not quarantine on these.
func wasmValidateTransientFailure(msg string) bool {
	low := strings.ToLower(msg)
	for _, needle := range []string{
		"pthread_create failed",
		"resource temporarily unavailable",
		"failed to reserve page summary memory",
		"start wasm-validate child",
	} {
		if strings.Contains(low, needle) {
			return true
		}
	}
	return false
}

// validateWasmInChild runs CompileModule in a self-reexec child with RLIMIT_AS.
// If the child OOMs or times out, the module is rejected and this process lives.
func validateWasmInChild(ctx context.Context, wasm []byte) error {
	if wasmValidateChildDisabled() {
		return validateWasmInProcess(ctx, wasm)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("sandbox: resolve executable: %w", err)
	}
	select {
	case wasmValidateChildSem <- struct{}{}:
		defer func() { <-wasmValidateChildSem }()
	case <-ctx.Done():
		return fmt.Errorf("sandbox: wasm-validate child: %w", ctx.Err())
	}
	wall := wasmValidateWall()
	runCtx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exe, wasmValidateArg)
	cmd.Stdin = bytes.NewReader(wasm)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"GOMAXPROCS=1",
		// Child applies VmSize+budget itself; do not inherit a desk CHILD=0.
		"HACKME_WASM_VALIDATE_CHILD=1",
	)
	setWasmValidateChildAttr(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sandbox: start wasm-validate child: %w", err)
	}
	// Second line: absolute ceiling (generous). Child has already lowered to
	// VmSize+budget when it enters RunValidateChildMain; if we win the race
	// first, we only set the high ceiling so mallocinit can finish.
	_ = setWasmValidateASLimit(cmd.Process.Pid, wasmValidateASMB())
	waitErr := cmd.Wait()
	if runCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("sandbox: wasm-validate child wall exceeded")
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return fmt.Errorf("sandbox: wasm-validate child failed: %s", msg)
	}
	if !strings.HasPrefix(stdout.String(), "OK") {
		return fmt.Errorf("sandbox: wasm-validate child missing OK")
	}
	return nil
}

// validateWasmInProcess compiles without a child (lab / child-disabled path).
func validateWasmInProcess(ctx context.Context, wasm []byte) error {
	stripped, err := stripWasmCustomSections(wasm)
	if err != nil {
		return err
	}
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)
	compiled, err := rt.CompileModule(ctx, stripped)
	if err != nil {
		return fmt.Errorf("sandbox: compile wasm: %w", err)
	}
	return compiled.Close(ctx)
}
