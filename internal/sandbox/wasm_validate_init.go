package sandbox

import "os"

// init intercepts -hackme-wasm-validate before any main/flag.Parse in every
// binary that links this package (node, coordinator, workerfuzz, workerpoh,
// tools/*, tests). Without this, re-exec hits "flag provided but not defined".
func init() {
	if IsValidateChildArg(os.Args) {
		RunValidateChildMain()
	}
}
