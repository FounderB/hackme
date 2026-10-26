package sandbox

import (
	"os"
	"testing"
)

// TestMain is intentionally thin: package init() already handles
// -hackme-wasm-validate for child re-exec under HACKME_WASM_VALIDATE_CHILD=1.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
