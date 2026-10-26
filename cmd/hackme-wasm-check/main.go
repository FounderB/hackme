// Command hackme-wasm-check is a tiny no-cgo WASM validate child.
//
// GPU workers (especially workerpoh-cuda) must not re-exec themselves under
// RLIMIT_AS: gorgonia.org/cu's package init calls cuInit before sandbox.init
// and needs multi-GiB virtual address space. This binary links only sandbox +
// wazero so Dig/hybrid fuzz validation works on NVIDIA fleet the same as CPU.
//
// Invoked as: hackme-wasm-check -hackme-wasm-validate <wasm-on-stdin
// (sandbox package init handles the flag before main runs.)
package main

import (
	"fmt"
	"os"

	_ "hackme/internal/sandbox"
)

func main() {
	// If someone runs the binary without the validate flag, print usage.
	fmt.Fprintf(os.Stderr, "hackme-wasm-check: use -hackme-wasm-validate (stdin wasm)\n")
	os.Exit(2)
}
