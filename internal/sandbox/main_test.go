package sandbox

import (
	"os"
	"testing"
)

// TestMain wires -hackme-wasm-validate so child re-exec from this package's
// tests (when HACKME_WASM_VALIDATE_CHILD=1) can compile under RLIMIT_AS.
func TestMain(m *testing.M) {
	if IsValidateChildArg(os.Args) {
		RunValidateChildMain()
		return
	}
	os.Exit(m.Run())
}
