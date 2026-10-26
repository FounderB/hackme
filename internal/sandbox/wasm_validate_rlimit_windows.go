//go:build windows

package sandbox

import "os/exec"

func setWasmValidateChildAttr(cmd *exec.Cmd) {}

func setWasmValidateASLimit(pid, mb int) error {
	// No RLIMIT_AS on Windows; child + wall timeout still contain hangs.
	return nil
}
