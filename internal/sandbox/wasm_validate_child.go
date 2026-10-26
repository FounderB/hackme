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
	// Go's runtime reserves large virtual address space; RLIMIT_AS below ~1 GiB
	// aborts the child before wazero runs ("failed to reserve page summary memory").
	// Concurrent children also need headroom for pthread stacks — default 2048 MiB
	// still stops multi-GiB LEB prealloc bombs while surviving pool submit bursts.
	defaultWasmValidateASMB   = 2048
	minWasmValidateASMB       = 1024
	maxWasmValidateASMB       = 2048
	defaultWasmValidateWallMS = 3000
	wasmValidateArg           = "-hackme-wasm-validate"
	wasmValidateOK            = "OK\n"
)

// Cap concurrent RLIMIT children — many parallel re-execs under AS exhaust
// pthread_create ("Resource temporarily unavailable") and false-quarantine good gates.
var wasmValidateChildSem = make(chan struct{}, 1)

// RunValidateChildMain is the early-process entry for node/coordinator when
// argv contains -hackme-wasm-validate. Reads WASM from stdin, runs hostile
// section filter + CompileModule, prints OK or an error, exits 0/1.
// Must not re-enter child mode (no nested spawn).
func RunValidateChildMain() {
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
func wasmValidateChildDisabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_WASM_VALIDATE_CHILD")))
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return true
	}
	if v == "1" || v == "true" || v == "on" || v == "yes" {
		return false
	}
	// go test / delve binaries have no -hackme-wasm-validate main unless TestMain
	// wires it — default to in-process there; production binaries keep child ON.
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
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1")
	setWasmValidateChildAttr(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sandbox: start wasm-validate child: %w", err)
	}
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
		// OOM / signal kills look like exit errors — treat as reject.
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
